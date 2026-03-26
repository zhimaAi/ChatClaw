package chat

import (
	"context"
	"encoding/json"
	"testing"
)

type mockOpenClawGateway struct {
	ready           bool
	lastMethod      string
	lastParams      any
	requestErr      error
	queryRequestErr error
}

func (m *mockOpenClawGateway) GatewayURL() string   { return "" }
func (m *mockOpenClawGateway) GatewayToken() string { return "" }
func (m *mockOpenClawGateway) IsReady() bool        { return m.ready }
func (m *mockOpenClawGateway) Request(ctx context.Context, method string, params any, out any) error {
	m.lastMethod = method
	m.lastParams = params
	return m.requestErr
}
func (m *mockOpenClawGateway) QueryRequest(ctx context.Context, method string, params any, out any) error {
	m.lastMethod = "query:" + method
	m.lastParams = params
	return m.queryRequestErr
}
func (m *mockOpenClawGateway) AddEventListener(key string, fn func(event string, payload json.RawMessage)) {
}
func (m *mockOpenClawGateway) RemoveEventListener(key string) {}

func TestBuildOpenClawMessagesFromTranscript(t *testing.T) {
	transcriptMessages := []openClawTranscriptMsg{
		{
			Role: "user",
			Content: []any{
				map[string]any{"type": "text", "text": "[Thu 2026-03-26 11:27 GMT+8] 你好\n\n<chatclaw_context hidden=\"true\">ignore</chatclaw_context>"},
			},
		},
		{
			Role: "assistant",
			Content: []any{
				map[string]any{"type": "thinking", "thinking": "先查一下"},
				map[string]any{"type": "toolCall", "id": "call_1", "name": "read", "arguments": map[string]any{"path": "a.txt"}},
			},
		},
		{
			Role:       "toolResult",
			ToolCallID: "call_1",
			ToolName:   "read",
			Content: []any{
				map[string]any{"type": "text", "text": "tool output"},
			},
		},
		{
			Role: "assistant",
			Content: []any{
				map[string]any{"type": "text", "text": "最终答案"},
			},
		},
	}

	messages := buildOpenClawMessagesFromTranscript(42, transcriptMessages)
	if len(messages) != 3 {
		t.Fatalf("expected 3 chat messages after grouping, got %d", len(messages))
	}
	if messages[0].Role != "user" || messages[0].Content != "你好" {
		t.Fatalf("unexpected first message: role=%q content=%q", messages[0].Role, messages[0].Content)
	}
	if messages[1].Role != "assistant" {
		t.Fatalf("expected grouped assistant message, got %q", messages[1].Role)
	}
	if messages[1].Content != "最终答案" {
		t.Fatalf("unexpected assistant content: %q", messages[1].Content)
	}
	if messages[1].ThinkingContent != "先查一下" {
		t.Fatalf("unexpected thinking content: %q", messages[1].ThinkingContent)
	}
	if messages[2].Role != "tool" || messages[2].ToolCallID != "call_1" {
		t.Fatalf("unexpected tool message: role=%q toolCallID=%q", messages[2].Role, messages[2].ToolCallID)
	}
}
