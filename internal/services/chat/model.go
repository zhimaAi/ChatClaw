package chat

import (
	"context"
	"time"

	"willchat/internal/sqlite"

	"github.com/uptrace/bun"
)

// Message status constants
const (
	StatusPending   = "pending"
	StatusStreaming = "streaming"
	StatusSuccess   = "success"
	StatusError     = "error"
	StatusCancelled = "cancelled"
)

// Message role constants
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleSystem    = "system"
	RoleTool      = "tool"
)

// Message DTO (exposed to frontend)
type Message struct {
	ID              int64     `json:"id"`
	ConversationID  int64     `json:"conversation_id"`
	Role            string    `json:"role"`
	Content         string    `json:"content"`
	ProviderID      string    `json:"provider_id,omitempty"`
	ModelID         string    `json:"model_id,omitempty"`
	Status          string    `json:"status"`
	Error           string    `json:"error,omitempty"`
	InputTokens     int       `json:"input_tokens"`
	OutputTokens    int       `json:"output_tokens"`
	FinishReason    string    `json:"finish_reason,omitempty"`
	ToolCalls       string    `json:"tool_calls,omitempty"`
	ToolCallID      string    `json:"tool_call_id,omitempty"`
	ToolCallName    string    `json:"tool_call_name,omitempty"`
	ThinkingContent string    `json:"thinking_content,omitempty"`
	Segments        string    `json:"segments,omitempty"` // JSON array for interleaved content/tool-call order
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// SendMessageInput input for sending a message
type SendMessageInput struct {
	ConversationID int64  `json:"conversation_id"`
	Content        string `json:"content"`
	TabID          string `json:"tab_id"`
}

// EditAndResendInput input for editing and resending a message
type EditAndResendInput struct {
	ConversationID int64  `json:"conversation_id"`
	MessageID      int64  `json:"message_id"`
	NewContent     string `json:"new_content"`
	TabID          string `json:"tab_id"`
}

// SendMessageResult result of sending a message
type SendMessageResult struct {
	RequestID string `json:"request_id"`
	MessageID int64  `json:"message_id"`
}

// messageModel database model for messages
type messageModel struct {
	bun.BaseModel `bun:"table:messages,alias:m"`

	ID              int64     `bun:"id,pk,autoincrement"`
	CreatedAt       time.Time `bun:"created_at,notnull"`
	UpdatedAt       time.Time `bun:"updated_at,notnull"`
	ConversationID  int64     `bun:"conversation_id,notnull"`
	Role            string    `bun:"role,notnull"`
	Content         string    `bun:"content,notnull"`
	ProviderID      string    `bun:"provider_id,notnull"`
	ModelID         string    `bun:"model_id,notnull"`
	Status          string    `bun:"status,notnull"`
	Error           string    `bun:"error,notnull"`
	InputTokens     int       `bun:"input_tokens,notnull"`
	OutputTokens    int       `bun:"output_tokens,notnull"`
	FinishReason    string    `bun:"finish_reason,notnull"`
	ToolCalls       string    `bun:"tool_calls,notnull"`
	ToolCallID      string    `bun:"tool_call_id,notnull"`
	ToolCallName    string    `bun:"tool_call_name,notnull"`
	ThinkingContent string    `bun:"thinking_content,notnull"`
	Segments        string    `bun:"segments,notnull"`
}

var _ bun.BeforeInsertHook = (*messageModel)(nil)

func (*messageModel) BeforeInsert(ctx context.Context, query *bun.InsertQuery) error {
	now := sqlite.NowUTC()
	query.Value("created_at", "?", now)
	query.Value("updated_at", "?", now)
	return nil
}

var _ bun.BeforeUpdateHook = (*messageModel)(nil)

func (*messageModel) BeforeUpdate(ctx context.Context, query *bun.UpdateQuery) error {
	query.Set("updated_at = ?", sqlite.NowUTC())
	return nil
}

func (m *messageModel) toDTO() Message {
	return Message{
		ID:              m.ID,
		ConversationID:  m.ConversationID,
		Role:            m.Role,
		Content:         m.Content,
		ProviderID:      m.ProviderID,
		ModelID:         m.ModelID,
		Status:          m.Status,
		Error:           m.Error,
		InputTokens:     m.InputTokens,
		OutputTokens:    m.OutputTokens,
		FinishReason:    m.FinishReason,
		ToolCalls:       m.ToolCalls,
		ToolCallID:      m.ToolCallID,
		ToolCallName:    m.ToolCallName,
		ThinkingContent: m.ThinkingContent,
		Segments:        m.Segments,
		CreatedAt:       m.CreatedAt,
		UpdatedAt:       m.UpdatedAt,
	}
}

// ChatEvent represents an event sent to the frontend
type ChatEvent struct {
	ConversationID int64  `json:"conversation_id"`
	TabID          string `json:"tab_id"`
	RequestID      string `json:"request_id"`
	Seq            int    `json:"seq"`
	MessageID      int64  `json:"message_id,omitempty"`
	Ts             int64  `json:"ts"`
}

// ChatStartEvent event sent when generation starts
type ChatStartEvent struct {
	ChatEvent
	Status string `json:"status"`
}

// ChatChunkEvent event sent for content chunks
type ChatChunkEvent struct {
	ChatEvent
	Delta string `json:"delta"`
}

// ChatThinkingEvent event sent for thinking content
type ChatThinkingEvent struct {
	ChatEvent
	Delta string `json:"delta"`
}

// ChatToolEvent event sent for tool calls and results
type ChatToolEvent struct {
	ChatEvent
	Type       string `json:"type"` // "call" or "result"
	ToolCallID string `json:"tool_call_id"`
	ToolName   string `json:"tool_name"`
	ArgsJSON   string `json:"args_json,omitempty"`
	ResultJSON string `json:"result_json,omitempty"`
}

// ChatCompleteEvent event sent when generation completes
type ChatCompleteEvent struct {
	ChatEvent
	Status       string `json:"status"`
	FinishReason string `json:"finish_reason"`
}

// ChatStoppedEvent event sent when generation is stopped
type ChatStoppedEvent struct {
	ChatEvent
	Status string `json:"status"`
}

// ChatErrorEvent event sent when an error occurs
type ChatErrorEvent struct {
	ChatEvent
	Status    string `json:"status"`
	ErrorKey  string `json:"error_key"`
	ErrorData any    `json:"error_data,omitempty"`
}

// Event names
const (
	EventChatStart    = "chat:start"
	EventChatChunk    = "chat:chunk"
	EventChatThinking = "chat:thinking"
	EventChatTool     = "chat:tool"
	EventChatComplete = "chat:complete"
	EventChatStopped  = "chat:stopped"
	EventChatError    = "chat:error"
)
