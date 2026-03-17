package embedding

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"chatclaw/internal/services/chatwiki"
)

func TestChatWikiEmbedder_UsesStandardOpenAIEmbeddingRequestBody(t *testing.T) {
	chatwikiTestResetCatalogCache()

	var requests []map[string]any
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/manage/chatclaw/showModelConfigList":
			_, _ = w.Write([]byte(`{
				"data": {
					"embedding_models": [
						{"id": 34, "model_name": "text-embedding", "type": "embedding", "enabled": 1}
					]
				}
			}`))
		case "/chatclaw/v1/embeddings":
			defer r.Body.Close()
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode request body: %v", err)
			}
			if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
				t.Fatalf("unexpected authorization header: %q", got)
			}
			if got := r.Header.Get("Token"); got != "" {
				t.Fatalf("expected empty token header, got %q", got)
			}
			mu.Lock()
			requests = append(requests, payload)
			mu.Unlock()
			_, _ = w.Write([]byte(`{
				"data": [
					{"index": 0, "embedding": [0.1, 0.2]},
					{"index": 1, "embedding": [0.3, 0.4]}
				]
			}`))
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	embedder := newChatWikiEmbedder(&ProviderConfig{
		ProviderID:  "chatwiki",
		APIKey:      "test-token",
		APIEndpoint: server.URL + "/chatclaw/v1",
		ModelID:     "text-embedding",
		Dimension:   1024,
	})
	vectors, err := embedder.EmbedStrings(context.Background(), []string{"hello", "world"})
	if err != nil {
		t.Fatalf("EmbedStrings returned error: %v", err)
	}
	if len(vectors) != 2 {
		t.Fatalf("expected 2 vectors, got %#v", vectors)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 1 {
		t.Fatalf("expected 1 embeddings request, got %d", len(requests))
	}
	if got := requests[0]["model"]; got != "text-embedding" {
		t.Fatalf("expected model=text-embedding, got %#v", requests[0])
	}
	if _, exists := requests[0]["use_model"]; exists {
		t.Fatalf("did not expect use_model in request: %#v", requests[0])
	}
	if _, exists := requests[0]["self_owned_model_config_id"]; exists {
		t.Fatalf("did not expect self_owned_model_config_id in request: %#v", requests[0])
	}
}

func chatwikiTestResetCatalogCache() {
	chatwiki.ResetOpenAIModelCatalogCacheForTest()
}
