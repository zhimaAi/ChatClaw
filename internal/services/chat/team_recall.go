package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/uptrace/bun"
)

const (
	teamRecallSize       = 5
	teamRecallSimilarity = "0.7"
	teamRecallSearchType = "1"
)

// teamRecallResponse represents a flexible API response (code/data or data-only).
type teamRecallResponse struct {
	Code int             `json:"code"`
	Data json.RawMessage `json:"data"`
}

// teamRecallItem represents one recalled chunk (content + score/similarity).
type teamRecallItem struct {
	Content    string  `json:"content"`
	Score      float64 `json:"score"`
	Similarity float64 `json:"similarity"`
}

// getChatWikiBindingBaseAndToken returns the latest binding server_url (same base as team library list API) and token.
// Recall API path is appended to this base, e.g. /manage/chatclaw/libraryRecall.
func getChatWikiBindingBaseAndToken(ctx context.Context, db *bun.DB) (baseURL string, token string, err error) {
	var row struct {
		ServerURL string `bun:"server_url"`
		Token     string `bun:"token"`
	}
	err = db.NewSelect().
		Table("chatwiki_bindings").
		Column("server_url", "token").
		OrderExpr("id DESC").
		Limit(1).
		Scan(ctx, &row)
	if err != nil {
		return "", "", err
	}
	baseURL = strings.TrimSuffix(strings.TrimSpace(row.ServerURL), "/")
	token = strings.TrimSpace(row.Token)
	return baseURL, token, nil
}

// retrieveFromTeamLibrary calls the external library recall API and returns results in the same shape as local retrieval.
func (s *ChatService) retrieveFromTeamLibrary(ctx context.Context, db *bun.DB, teamLibraryID string, question string, size int) []retrievalResult {
	if teamLibraryID == "" || strings.TrimSpace(question) == "" {
		return nil
	}
	if size <= 0 {
		size = teamRecallSize
	}

	baseURL, token, err := getChatWikiBindingBaseAndToken(ctx, db)
	if err != nil || token == "" || baseURL == "" {
		s.app.Logger.Warn("[chat] team recall: no ChatWiki binding (server_url/token)", "error", err)
		return nil
	}

	reqURL := baseURL + "/manage/chatclaw/libraryRecall"

	form := url.Values{}
	// Single request: id is one library id or multiple ids in one string, e.g. "100,102,103"
	form.Set("id", strings.TrimSpace(teamLibraryID))
	form.Set("question", strings.TrimSpace(question))
	form.Set("size", fmt.Sprintf("%d", size))
	form.Set("similarity", teamRecallSimilarity)
	form.Set("search_type", teamRecallSearchType)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, strings.NewReader(form.Encode()))
	if err != nil {
		s.app.Logger.Warn("[chat] team recall: request build failed", "error", err)
		return nil
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("token", token)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		s.app.Logger.Warn("[chat] team recall: request failed", "error", err)
		return nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		s.app.Logger.Warn("[chat] team recall: read body failed", "error", err)
		return nil
	}
	if resp.StatusCode != http.StatusOK {
		s.app.Logger.Warn("[chat] team recall: non-200", "status", resp.StatusCode, "body", string(body))
		return nil
	}

	var wrap teamRecallResponse
	if err := json.Unmarshal(body, &wrap); err != nil {
		// Try parsing as direct array
		var list []teamRecallItem
		if err2 := json.Unmarshal(body, &list); err2 != nil {
			var list2 []map[string]interface{}
			if err3 := json.Unmarshal(body, &list2); err3 == nil {
				return parseTeamRecallMaps(list2)
			}
			s.app.Logger.Warn("[chat] team recall: parse failed", "error", err, "body", string(body))
			return nil
		}
		return teamRecallItemsToResults(list)
	}

	// Data may be array or wrapped
	var list []teamRecallItem
	if err := json.Unmarshal(wrap.Data, &list); err != nil {
		var list2 []map[string]interface{}
		if err2 := json.Unmarshal(wrap.Data, &list2); err2 == nil {
			return parseTeamRecallMaps(list2)
		}
		s.app.Logger.Warn("[chat] team recall: parse data failed", "error", err)
		return nil
	}
	out := teamRecallItemsToResults(list)
	s.app.Logger.Info("[chat] team recall ok", "library_id", teamLibraryID, "results", len(out))
	return out
}

func teamRecallItemsToResults(list []teamRecallItem) []retrievalResult {
	out := make([]retrievalResult, 0, len(list))
	for _, it := range list {
		score := it.Score
		if score == 0 {
			score = it.Similarity
		}
		out = append(out, retrievalResult{Content: strings.TrimSpace(it.Content), Score: score})
	}
	return out
}

func parseTeamRecallMaps(list []map[string]interface{}) []retrievalResult {
	out := make([]retrievalResult, 0, len(list))
	for _, m := range list {
		c, _ := m["content"].(string)
		s := 0.0
		if v, ok := m["score"].(float64); ok {
			s = v
		} else if v, ok := m["similarity"].(float64); ok {
			s = v
		}
		out = append(out, retrievalResult{Content: strings.TrimSpace(c), Score: s})
	}
	return out
}
