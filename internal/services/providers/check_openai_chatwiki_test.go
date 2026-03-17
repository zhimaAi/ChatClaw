package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCheckOpenAI_ChatWikiUsesStandardOpenAIRequestBody(t *testing.T) {
	requestSeen := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
		requestSeen = true
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("unexpected authorization header: %q", got)
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
	}))
	defer server.Close()

	svc := &ProvidersService{}
	result, err := svc.checkOpenAI(context.Background(), "chatwiki", CheckAPIKeyInput{
		APIKey:      "test-token",
		APIEndpoint: server.URL,
	}, "deepseek-r1")
	if err != nil {
		t.Fatalf("checkOpenAI returned error: %v", err)
	}
	if !requestSeen {
		t.Fatal("expected chat completions request")
	}
	if result == nil || !result.Success {
		t.Fatalf("expected successful result, got %#v", result)
	}
}
