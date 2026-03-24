package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	einoembedding "github.com/cloudwego/eino/components/embedding"
)

type chatWikiEmbedder struct {
	apiKey      string
	apiEndpoint string
	modelID     string
	dimension   *int
	client      *http.Client
}

func newChatWikiEmbedder(cfg *ProviderConfig) *chatWikiEmbedder {
	client := &http.Client{Timeout: cfg.Timeout}
	if cfg.Timeout == 0 {
		client = http.DefaultClient
	}
	var dimension *int
	if cfg.Dimension > 0 {
		dimension = &cfg.Dimension
	}
	return &chatWikiEmbedder{
		apiKey:      cfg.APIKey,
		apiEndpoint: strings.TrimRight(cfg.APIEndpoint, "/"),
		modelID:     cfg.ModelID,
		dimension:   dimension,
		client:      client,
	}
}

func (e *chatWikiEmbedder) EmbedStrings(ctx context.Context, texts []string, _ ...einoembedding.Option) ([][]float64, error) {
	if len(texts) == 0 {
		return [][]float64{}, nil
	}

	payload := map[string]any{
		"model": e.modelID,
		"input": texts,
	}
	if e.dimension != nil && *e.dimension > 0 {
		payload["dimensions"] = *e.dimension
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode chatwiki embeddings request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.apiEndpoint+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create chatwiki embeddings request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(e.apiKey))

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request chatwiki embeddings: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read chatwiki embeddings response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("chatwiki embeddings status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var parsed struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("decode chatwiki embeddings response: %w", err)
	}
	sort.Slice(parsed.Data, func(i, j int) bool { return parsed.Data[i].Index < parsed.Data[j].Index })

	result := make([][]float64, 0, len(parsed.Data))
	for _, item := range parsed.Data {
		result = append(result, item.Embedding)
	}
	return result, nil
}
