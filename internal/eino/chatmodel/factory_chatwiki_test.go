package chatmodel

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"
)

func TestNewChatModel_ChatWikiUsesStandardOpenAIRequestBody(t *testing.T) {
	chatRequestSeen := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/manage/chatclaw/showModelConfigList":
			if got := r.Header.Get("Token"); got != "test-token" {
				t.Fatalf("unexpected token header for catalog request: %q", got)
			}
			_, _ = w.Write([]byte(`{"data":{"language_models":[{"id":12,"model_name":"deepseek-r1","type":"llm","enabled":1}]}}`))
		case "/chatclaw/v1/chat/completions":
			chatRequestSeen = true
			if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
				t.Fatalf("unexpected authorization header: %q", got)
			}
			if got := r.Header.Get("Token"); got != "" {
				t.Fatalf("expected empty token header, got %q", got)
			}

			defer r.Body.Close()
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode request body: %v", err)
			}
			if got := payload["model"]; got != "deepseek-r1" {
				t.Fatalf("expected model=deepseek-r1, got %#v", payload)
			}
			if _, exists := payload["use_model"]; exists {
				t.Fatalf("did not expect use_model in payload: %#v", payload)
			}
			if _, exists := payload["self_owned_model_config_id"]; exists {
				t.Fatalf("did not expect self_owned_model_config_id in payload: %#v", payload)
			}

			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "chatcmpl-test",
				"object":  "chat.completion",
				"created": time.Now().Unix(),
				"model":   "deepseek-r1",
				"choices": []map[string]any{
					{
						"index": 0,
						"message": map[string]any{
							"role":    "assistant",
							"content": "ok",
						},
						"finish_reason": "stop",
					},
				},
				"usage": map[string]any{
					"prompt_tokens":     1,
					"completion_tokens": 1,
					"total_tokens":      2,
				},
			})
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	chatModel, err := NewChatModel(context.Background(), &ProviderConfig{
		ProviderID:   "chatwiki",
		ProviderType: "openai",
		APIKey:       "test-token",
		APIEndpoint:  server.URL + "/chatclaw/v1",
		ModelID:      "deepseek-r1",
	})
	if err != nil {
		t.Fatalf("NewChatModel returned error: %v", err)
	}

	msg, err := chatModel.Generate(context.Background(), []*schema.Message{{
		Role:    schema.User,
		Content: "hello",
	}})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if !chatRequestSeen {
		t.Fatal("expected chat completions request")
	}
	if msg == nil || msg.Content != "ok" {
		t.Fatalf("unexpected response message: %#v", msg)
	}
}
