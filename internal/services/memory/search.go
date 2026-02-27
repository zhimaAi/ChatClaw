package memory

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"chatclaw/internal/eino/embedding"
	"chatclaw/internal/errs"
	"chatclaw/internal/fts/tokenizer"
	"chatclaw/internal/sqlite"

	"github.com/uptrace/bun"
)

const rrfK = 60

// Fixed distance threshold for sqlite-vec recall.
// Smaller distance means higher semantic similarity.
const vecRecallDistanceThreshold = 0.45

type SearchResult struct {
	Type    string
	Content string
	Score   float64
}

type rankedItem struct {
	key  string // "thematic:<id>" or "event:<id>"
	rank int
}

type scoredItem struct {
	key   string
	score float64
}

// SearchMemories performs hybrid FTS5 + vector search on thematic_facts and event_streams,
// then fuses the results using Reciprocal Rank Fusion (RRF).
func SearchMemories(ctx context.Context, agentID int64, queries []string, topK int, matchThreshold float64) ([]SearchResult, error) {
	if db == nil {
		return nil, errs.New("error.memory_db_not_initialized")
	}
	_ = matchThreshold
	if topK <= 0 {
		topK = 10
	}

	mainDB := sqlite.DB()
	fetchK := max(topK*3, 30)

	var wg sync.WaitGroup
	var ftsResults, vecResults []rankedItem
	var ftsErr, vecErr error

	wg.Add(1)
	go func() {
		defer wg.Done()
		ftsResults, ftsErr = ftsSearch(ctx, agentID, queries, fetchK)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		vecResults, vecErr = vecSearch(ctx, mainDB, agentID, queries, fetchK)
	}()

	wg.Wait()

	if ftsErr != nil && vecErr != nil {
		return nil, fmt.Errorf("both search methods failed: fts=%w, vec=%v", ftsErr, vecErr)
	}

	merged := rrfMerge(ftsResults, vecResults)
	if len(merged) > topK {
		merged = merged[:topK]
	}
	if len(merged) == 0 {
		return nil, nil
	}

	return fetchContent(ctx, agentID, merged)
}

// ftsSearch performs FTS5 full-text search on both thematic_facts_fts and event_streams_fts.
// Uses bun.DB.QueryContext (raw *sql.Rows) because bun's struct scanner
// cannot map the FTS5 virtual-table `rowid` pseudo-column.
func ftsSearch(ctx context.Context, agentID int64, queries []string, limit int) ([]rankedItem, error) {
	var allParts []string
	for _, q := range queries {
		if mq := tokenizer.BuildMatchQuery(q); mq != "" {
			allParts = append(allParts, mq)
		}
	}
	if len(allParts) == 0 {
		return nil, nil
	}
	matchQuery := strings.Join(allParts, " OR ")
	ftsQuery := fmt.Sprintf("(%s) AND agent_id:%d", matchQuery, agentID)

	var results []rankedItem
	rank := 0

	for _, table := range []struct {
		fts, prefix string
	}{
		{"thematic_facts_fts", "thematic"},
		{"event_streams_fts", "event"},
	} {
		q := fmt.Sprintf(
			`SELECT rowid, bm25(%s) AS score FROM %s WHERE %s MATCH ? ORDER BY score ASC LIMIT ?`,
			table.fts, table.fts, table.fts,
		)
		rows, err := db.QueryContext(ctx, q, ftsQuery, limit)
		if err != nil {
			continue
		}
		for rows.Next() {
			var rowID int64
			var score float64
			if err := rows.Scan(&rowID, &score); err != nil {
				continue
			}
			rank++
			results = append(results, rankedItem{
				key:  fmt.Sprintf("%s:%d", table.prefix, rowID),
				rank: rank,
			})
		}
		rows.Close()
	}

	return results, nil
}

// vecSearch performs vector KNN search on thematic_facts_vec and event_streams_vec.
func vecSearch(ctx context.Context, mainDB *bun.DB, agentID int64, queries []string, limit int) ([]rankedItem, error) {
	if mainDB == nil {
		return nil, nil
	}

	type settingRow struct {
		Key   string
		Value sql.NullString
	}
	var settings []settingRow
	if err := mainDB.NewSelect().Table("settings").Column("key", "value").
		Where("key IN (?)", bun.In([]string{
			"memory_embedding_provider_id",
			"memory_embedding_model_id",
		})).Scan(ctx, &settings); err != nil {
		return nil, nil
	}

	configMap := make(map[string]string)
	for _, s := range settings {
		if s.Value.Valid {
			configMap[s.Key] = s.Value.String
		}
	}

	providerID := configMap["memory_embedding_provider_id"]
	modelID := configMap["memory_embedding_model_id"]
	if providerID == "" || modelID == "" {
		return nil, nil
	}

	type providerRow struct {
		Type        string `bun:"type"`
		APIKey      string `bun:"api_key"`
		APIEndpoint string `bun:"api_endpoint"`
		ExtraConfig string `bun:"extra_config"`
	}
	var prov providerRow
	if err := mainDB.NewSelect().Table("providers").
		Column("type", "api_key", "api_endpoint", "extra_config").
		Where("provider_id = ?", providerID).
		Scan(ctx, &prov); err != nil {
		return nil, nil
	}

	embedder, err := embedding.NewEmbedder(ctx, &embedding.ProviderConfig{
		ProviderType: prov.Type,
		APIKey:       prov.APIKey,
		APIEndpoint:  prov.APIEndpoint,
		ModelID:      modelID,
		ExtraConfig:  prov.ExtraConfig,
	})
	if err != nil || embedder == nil {
		return nil, nil
	}

	allVecs, err := embedder.EmbedStrings(ctx, queries)
	if err != nil || len(allVecs) == 0 {
		return nil, nil
	}
	queryVec := averageVectors(allVecs)
	vecStr := formatVector(queryVec)

	var results []rankedItem
	rank := 0

	for _, table := range []struct {
		vecTable, srcTable, prefix string
	}{
		{"thematic_facts_vec", "thematic_facts", "thematic"},
		{"event_streams_vec", "event_streams", "event"},
	} {
		if !tableExists(ctx, table.vecTable) {
			continue
		}
		type vecRow struct {
			ID       int64   `bun:"id"`
			Distance float64 `bun:"distance"`
		}
		var rows []vecRow
		if err := db.NewRaw(`
			SELECT v.id, v.distance
			FROM `+table.vecTable+` v
			WHERE v.embedding MATCH ? AND k = ?
		`, vecStr, limit).Scan(ctx, &rows); err == nil {
			for _, r := range rows {
				if r.Distance > vecRecallDistanceThreshold {
					continue
				}
				var aid int64
				if err := db.NewRaw(`SELECT agent_id FROM `+table.srcTable+` WHERE id = ?`, r.ID).Scan(ctx, &aid); err != nil || aid != agentID {
					continue
				}
				rank++
				results = append(results, rankedItem{
					key:  fmt.Sprintf("%s:%d", table.prefix, r.ID),
					rank: rank,
				})
			}
		}
	}

	return results, nil
}

// rrfMerge combines FTS and vector results using Reciprocal Rank Fusion.
// Thematic facts receive a 1.3x boost per the design spec.
func rrfMerge(ftsResults, vecResults []rankedItem) []scoredItem {
	scores := make(map[string]float64)

	for _, r := range ftsResults {
		scores[r.key] += rrfScore(r.rank)
	}
	for _, r := range vecResults {
		scores[r.key] += rrfScore(r.rank)
	}

	for k, s := range scores {
		if strings.HasPrefix(k, "thematic:") {
			scores[k] = s * 1.3
		}
	}

	merged := make([]scoredItem, 0, len(scores))
	for k, s := range scores {
		merged = append(merged, scoredItem{k, s})
	}
	sort.Slice(merged, func(i, j int) bool {
		return merged[i].score > merged[j].score
	})
	return merged
}

func rrfScore(rank int) float64 {
	return 1.0 / float64(rrfK+rank)
}

// fetchContent retrieves the actual content for merged results.
func fetchContent(ctx context.Context, agentID int64, merged []scoredItem) ([]SearchResult, error) {
	results := make([]SearchResult, 0, len(merged))

	for _, m := range merged {
		parts := strings.SplitN(m.key, ":", 2)
		if len(parts) != 2 {
			continue
		}
		typ := parts[0]
		var id int64
		if _, err := fmt.Sscanf(parts[1], "%d", &id); err != nil {
			continue
		}

		switch typ {
		case "thematic":
			var row struct {
				Topic   string `bun:"topic"`
				Content string `bun:"content"`
			}
			if err := db.NewSelect().Table("thematic_facts").
				Column("topic", "content").
				Where("id = ? AND agent_id = ?", id, agentID).
				Scan(ctx, &row); err == nil {
				results = append(results, SearchResult{
					Type:    "thematic",
					Content: row.Topic + ": " + row.Content,
					Score:   m.score,
				})
			}
		case "event":
			var row struct {
				Date    string `bun:"date"`
				Content string `bun:"content"`
			}
			if err := db.NewSelect().Table("event_streams").
				Column("date", "content").
				Where("id = ? AND agent_id = ?", id, agentID).
				Scan(ctx, &row); err == nil {
				score := m.score
				if isRecentDate(row.Date) {
					score *= 1.5
				}
				results = append(results, SearchResult{
					Type:    "event",
					Content: fmt.Sprintf("[%s] %s", row.Date, row.Content),
					Score:   score,
				})
			}
		}
	}

	return results, nil
}

func isRecentDate(dateStr string) bool {
	now := time.Now().UTC()
	today := now.Format("2006-01-02")
	yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")
	return dateStr == today || dateStr == yesterday
}

func tableExists(ctx context.Context, name string) bool {
	var count int
	err := db.NewRaw(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", name,
	).Scan(ctx, &count)
	return err == nil && count > 0
}

func averageVectors(vecs [][]float64) []float64 {
	if len(vecs) == 0 {
		return nil
	}
	if len(vecs) == 1 {
		return vecs[0]
	}
	dim := len(vecs[0])
	avg := make([]float64, dim)
	for _, v := range vecs {
		for i := range v {
			if i < dim {
				avg[i] += v[i]
			}
		}
	}
	n := float64(len(vecs))
	for i := range avg {
		avg[i] /= n
	}
	return avg
}

func formatVector(vec []float64) string {
	if len(vec) == 0 {
		return "[]"
	}
	var b strings.Builder
	b.Grow(len(vec) * 12)
	b.WriteByte('[')
	for i, v := range vec {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(fmt.Sprintf("%f", v))
	}
	b.WriteByte(']')
	return b.String()
}
