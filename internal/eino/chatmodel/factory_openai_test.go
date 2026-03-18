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

func TestNewOpenAIChatModel_EnableThinkingField(t *testing.T) {
	tests := []struct {
		name                 string
		cfg                  *ProviderConfig
		wantEnableThinking   bool
		wantEnableThinkingOK bool
	}{
		{
			name: "disable thinking sets enable_thinking false",
			cfg: &ProviderConfig{
				ProviderID:      "openai",
				ProviderType:    "openai",
				APIKey:          "test-token",
				ModelID:         "gpt-test",
				DisableThinking: true,
			},
			wantEnableThinking:   false,
			wantEnableThinkingOK: true,
		},
		{
			name: "chatwiki does not implicitly set enable_thinking",
			cfg: &ProviderConfig{
				ProviderID:   "chatwiki",
				ProviderType: "openai",
				APIKey:       "test-token",
				ModelID:      "deepseek-r1",
			},
			wantEnableThinkingOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requestSeen := false

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/chat/completions" {
					t.Fatalf("unexpected request path: %s", r.URL.Path)
				}
				requestSeen = true

				defer r.Body.Close()
				var payload map[string]any
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Fatalf("decode request body: %v", err)
				}

				got, exists := payload["enable_thinking"]
				if exists != tt.wantEnableThinkingOK {
					t.Fatalf("enable_thinking existence mismatch: got %v payload=%#v", exists, payload)
				}
				if tt.wantEnableThinkingOK && got != tt.wantEnableThinking {
					t.Fatalf("unexpected enable_thinking: got %#v want %#v", got, tt.wantEnableThinking)
				}

				_ = json.NewEncoder(w).Encode(map[string]any{
					"id":      "chatcmpl-test",
					"object":  "chat.completion",
					"created": time.Now().Unix(),
					"model":   tt.cfg.ModelID,
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

			cfg := *tt.cfg
			cfg.APIEndpoint = server.URL

			chatModel, err := NewChatModel(context.Background(), &cfg)
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
			if !requestSeen {
				t.Fatal("expected chat completions request")
			}
			if msg == nil || msg.Content != "ok" {
				t.Fatalf("unexpected response message: %#v", msg)
			}
		})
	}
}
