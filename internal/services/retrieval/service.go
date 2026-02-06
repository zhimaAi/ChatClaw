package retrieval

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"

	"willchat/internal/fts/tokenizer"

	"github.com/cloudwego/eino/components/embedding"
	"github.com/uptrace/bun"
)

// RRF constant for Reciprocal Rank Fusion scoring.
// k=60 is a commonly used value that balances the influence of top-ranked results
// while still giving weight to lower-ranked items. Higher k values give more uniform
// weighting across ranks, while lower k values emphasize top results more heavily.
const rrfK = 60

// SearchInput defines input parameters for retrieval
type SearchInput struct {
	LibraryIDs []int64 // Library IDs to search in
	Query      string  // Search query text
	Level      *int    // Optional level filter (0/1/2)
	TopK       int     // Maximum results to return
	MinScore   float64 // Minimum score threshold for filtering results
}

// SearchResult represents a single retrieval result
type SearchResult struct {
	NodeID       int64   `json:"node_id"`
	DocumentID   int64   `json:"document_id"`
	DocumentName string  `json:"document_name"`
	Content      string  `json:"content"`
	Level        int     `json:"level"`
	Score        float64 `json:"score"` // RRF normalized score
}

// rankedResult is used internally for RRF calculation
type rankedResult struct {
	nodeID int64
	rank   int
	score  float64
}

// Service provides document retrieval capabilities
type Service struct {
	db       *bun.DB
	embedder embedding.Embedder
}

// NewService creates a new retrieval service
func NewService(db *bun.DB, embedder embedding.Embedder) *Service {
	return &Service{
		db:       db,
		embedder: embedder,
	}
}

// Search performs hybrid search combining vector and full-text retrieval with RRF fusion
func (s *Service) Search(ctx context.Context, input SearchInput) ([]SearchResult, error) {
	if len(input.LibraryIDs) == 0 {
		return nil, nil
	}
	if input.Query == "" {
		return nil, nil
	}
	if input.TopK <= 0 {
		input.TopK = 10
	}

	// Fetch more results than needed for better RRF fusion
	fetchK := max(input.TopK*3, 30)

	var wg sync.WaitGroup
	var vecResults []rankedResult
	var ftsResults []rankedResult
	var vecErr, ftsErr error

	// Parallel: vector search
	wg.Add(1)
	go func() {
		defer wg.Done()
		vecResults, vecErr = s.vectorSearch(ctx, input.LibraryIDs, input.Query, input.Level, fetchK)
	}()

	// Parallel: full-text search
	wg.Add(1)
	go func() {
		defer wg.Done()
		ftsResults, ftsErr = s.fullTextSearch(ctx, input.LibraryIDs, input.Query, input.Level, fetchK)
	}()

	wg.Wait()

	if vecErr != nil {
		log.Printf("[retrieval] vector search error: %v", vecErr)
	}
	if ftsErr != nil {
		log.Printf("[retrieval] full-text search error: %v", ftsErr)
	}

	// RRF fusion
	merged := s.rrfMerge(vecResults, ftsResults)

	// Limit to topK
	if len(merged) > input.TopK {
		merged = merged[:input.TopK]
	}

	if len(merged) == 0 {
		return nil, nil
	}

	// Fetch full node details
	return s.fetchNodeDetails(ctx, merged)
}

// vectorSearch performs KNN search using sqlite-vec
func (s *Service) vectorSearch(ctx context.Context, libraryIDs []int64, query string, level *int, topK int) ([]rankedResult, error) {
	if s.embedder == nil {
		return nil, nil
	}

	// Embed the query
	vectors, err := s.embedder.EmbedStrings(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	if len(vectors) == 0 || len(vectors[0]) == 0 {
		return nil, fmt.Errorf("empty embedding result")
	}

	queryVec := formatVector(vectors[0])

	// Build the KNN query
	// We need to join with document_nodes to filter by library_id and level
	// sqlite-vec KNN query: SELECT id, distance FROM doc_vec WHERE content MATCH ? AND k = ?
	sql := `
		WITH knn AS (
			SELECT v.id, v.distance
			FROM doc_vec v
			WHERE v.content MATCH ?
			  AND k = ?
		)
		SELECT knn.id, knn.distance
		FROM knn
		INNER JOIN document_nodes n ON n.id = knn.id
		WHERE n.library_id IN (?)
	`
	args := []interface{}{queryVec, topK * 2, bun.In(libraryIDs)}

	if level != nil {
		sql += " AND n.level = ?"
		args = append(args, *level)
	}

	sql += " ORDER BY knn.distance ASC LIMIT ?"
	args = append(args, topK)

	type vecRow struct {
		ID       int64   `bun:"id"`
		Distance float64 `bun:"distance"`
	}

	var rows []vecRow
	if err := s.db.NewRaw(sql, args...).Scan(ctx, &rows); err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}

	results := make([]rankedResult, len(rows))
	for i, row := range rows {
		results[i] = rankedResult{
			nodeID: row.ID,
			rank:   i + 1,
		}
	}

	return results, nil
}

// fullTextSearch performs FTS5 search on doc_fts
func (s *Service) fullTextSearch(ctx context.Context, libraryIDs []int64, query string, level *int, topK int) ([]rankedResult, error) {
	// Build FTS5 match query
	matchQuery := tokenizer.BuildMatchQuery(query)
	if matchQuery == "" {
		return nil, nil
	}

	// Build library_id filter for FTS5
	// Format: library_id:1 OR library_id:2 OR ...
	// Validate library IDs to ensure they are positive
	var libParts []string
	for _, id := range libraryIDs {
		if id > 0 {
			libParts = append(libParts, fmt.Sprintf("library_id:%d", id))
		}
	}
	if len(libParts) == 0 {
		return nil, nil
	}
	libFilter := strings.Join(libParts, " OR ")

	// Combine with match query
	ftsQuery := fmt.Sprintf("(%s) AND (%s)", matchQuery, libFilter)

	// Add level filter if specified
	if level != nil {
		ftsQuery = fmt.Sprintf("(%s) AND level:%d", ftsQuery, *level)
	}

	sql := `
		SELECT rowid, bm25(doc_fts) AS score
		FROM doc_fts
		WHERE doc_fts MATCH ?
		ORDER BY score ASC
		LIMIT ?
	`

	type ftsRow struct {
		RowID int64   `bun:"rowid"`
		Score float64 `bun:"score"`
	}

	var rows []ftsRow
	if err := s.db.NewRaw(sql, ftsQuery, topK).Scan(ctx, &rows); err != nil {
		return nil, fmt.Errorf("full-text search: %w", err)
	}

	results := make([]rankedResult, len(rows))
	for i, row := range rows {
		results[i] = rankedResult{
			nodeID: row.RowID,
			rank:   i + 1,
		}
	}

	return results, nil
}

// rrfMerge combines results using Reciprocal Rank Fusion
func (s *Service) rrfMerge(vecResults, ftsResults []rankedResult) []rankedResult {
	scores := make(map[int64]float64)

	// Add vector search scores
	for _, r := range vecResults {
		scores[r.nodeID] += rrfScore(r.rank)
	}

	// Add full-text search scores
	for _, r := range ftsResults {
		scores[r.nodeID] += rrfScore(r.rank)
	}

	// Convert to slice and sort by score descending
	var merged []rankedResult
	for nodeID, score := range scores {
		merged = append(merged, rankedResult{
			nodeID: nodeID,
			score:  score,
		})
	}

	sort.Slice(merged, func(i, j int) bool {
		return merged[i].score > merged[j].score
	})

	return merged
}

// rrfScore calculates RRF score for a given rank
func rrfScore(rank int) float64 {
	return 1.0 / float64(rrfK+rank)
}

// fetchNodeDetails retrieves full node information for the merged results
func (s *Service) fetchNodeDetails(ctx context.Context, merged []rankedResult) ([]SearchResult, error) {
	if len(merged) == 0 {
		return nil, nil
	}

	// Collect node IDs
	nodeIDs := make([]int64, len(merged))
	scoreMap := make(map[int64]float64)
	for i, r := range merged {
		nodeIDs[i] = r.nodeID
		scoreMap[r.nodeID] = r.score
	}

	// Fetch node details with document name
	sql := `
		SELECT n.id, n.document_id, n.content, n.level, d.original_name
		FROM document_nodes n
		INNER JOIN documents d ON d.id = n.document_id
		WHERE n.id IN (?)
	`

	type nodeRow struct {
		ID           int64  `bun:"id"`
		DocumentID   int64  `bun:"document_id"`
		Content      string `bun:"content"`
		Level        int    `bun:"level"`
		OriginalName string `bun:"original_name"`
	}

	var rows []nodeRow
	if err := s.db.NewRaw(sql, bun.In(nodeIDs)).Scan(ctx, &rows); err != nil {
		return nil, fmt.Errorf("fetch node details: %w", err)
	}

	// Build result map for ordering
	nodeMap := make(map[int64]nodeRow)
	for _, row := range rows {
		nodeMap[row.ID] = row
	}

	// Build results in merged order (by score)
	results := make([]SearchResult, 0, len(merged))
	for _, m := range merged {
		row, ok := nodeMap[m.nodeID]
		if !ok {
			continue
		}
		results = append(results, SearchResult{
			NodeID:       row.ID,
			DocumentID:   row.DocumentID,
			DocumentName: row.OriginalName,
			Content:      row.Content,
			Level:        row.Level,
			Score:        scoreMap[row.ID],
		})
	}

	return results, nil
}

// formatVector converts a float64 slice to sqlite-vec format string
func formatVector(vec []float64) string {
	if len(vec) == 0 {
		return "[]"
	}
	var b strings.Builder
	b.Grow(len(vec) * 12) // Pre-allocate approximate space
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
