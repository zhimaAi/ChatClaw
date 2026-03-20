package chat

import (
	"context"
	"time"

	"chatclaw/internal/sqlite"

	"github.com/uptrace/bun"
)

// Message status constants
const (
	StatusPending     = "pending"
	StatusStreaming   = "streaming"
	StatusSuccess     = "success"
	StatusError       = "error"
	StatusCancelled   = "cancelled"
	StatusInterrupted = "interrupted"
)

// Message role constants
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleSystem    = "system"
	RoleTool      = "tool"
)

// ImagePayload describes a single image or file attached to a message.
// Kind = "image" for images; Kind = "file" for document attachments.
type ImagePayload struct {
	ID           string `json:"id,omitempty"`
	Kind         string `json:"kind"`                   // "image" or "file"
	Source       string `json:"source"`                 // "inline_base64" or "local_file"
	MimeType     string `json:"mime_type"`
	Base64       string `json:"base64"`                 // without "data:" prefix
	DataURL      string `json:"data_url,omitempty"`     // optional convenience field for frontend
	Width        int    `json:"width,omitempty"`
	Height       int    `json:"height,omitempty"`
	FileName     string `json:"file_name,omitempty"`
	FilePath     string `json:"file_path,omitempty"`     // local file path when saved to work dir
	Size         int64  `json:"size,omitempty"`
	OriginalName string `json:"original_name,omitempty"` // user's original filename (preserved for display)
}

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
	Segments        string    `json:"segments,omitempty"`    // JSON array for interleaved content/tool-call order
	ImagesJSON      string    `json:"images_json,omitempty"` // raw JSON string of []ImagePayload
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// SendMessageInput input for sending a message
type SendMessageInput struct {
	ConversationID int64          `json:"conversation_id"`
	Content        string         `json:"content"`
	TabID          string         `json:"tab_id"`
	Images         []ImagePayload `json:"images,omitempty"` // from frontend (base64)
}

// EditAndResendInput input for editing and resending a message
type EditAndResendInput struct {
	ConversationID int64          `json:"conversation_id"`
	MessageID      int64          `json:"message_id"`
	NewContent     string         `json:"new_content"`
	TabID          string         `json:"tab_id"`
	Images         []ImagePayload `json:"images,omitempty"` // images to attach (for resending with new images)
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
	ImagesJSON      string    `bun:"images_json,notnull"`
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
		ImagesJSON:      m.ImagesJSON,
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
	Delta            string   `json:"delta"`
	RunPath          []string `json:"run_path,omitempty"`
	ParentToolCallID string   `json:"parent_tool_call_id,omitempty"`
}

// ChatThinkingEvent event sent for thinking content
type ChatThinkingEvent struct {
	ChatEvent
	Delta            string   `json:"delta"`
	RunPath          []string `json:"run_path,omitempty"`
	ParentToolCallID string   `json:"parent_tool_call_id,omitempty"`
}

// ChatToolEvent event sent for tool calls and results
type ChatToolEvent struct {
	ChatEvent
	Type             string   `json:"type"` // "call" or "result"
	ToolCallID       string   `json:"tool_call_id"`
	ToolName         string   `json:"tool_name"`
	ArgsJSON         string   `json:"args_json,omitempty"`
	ResultJSON       string   `json:"result_json,omitempty"`
	RunPath          []string `json:"run_path,omitempty"`
	ParentToolCallID string   `json:"parent_tool_call_id,omitempty"`
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

// RetrievalItem represents a single retrieval result from knowledge base or memory.
type RetrievalItem struct {
	Source  string  `json:"source"` // "knowledge" or "memory"
	Content string  `json:"content"`
	Score   float64 `json:"score"`
}

// ChatRetrievalEvent event sent when retrieval results are available (chat mode).
type ChatRetrievalEvent struct {
	ChatEvent
	Items []RetrievalItem `json:"items"`
}

// ChatUserMessageEvent event sent when a user message is inserted (for external callers like MCP).
type ChatUserMessageEvent struct {
	ChatEvent
	Content    string `json:"content"`
	ImagesJSON string `json:"images_json,omitempty"`
}

// Event names
const (
	EventChatStart       = "chat:start"
	EventChatChunk       = "chat:chunk"
	EventChatThinking    = "chat:thinking"
	EventChatTool        = "chat:tool"
	EventChatRetrieval   = "chat:retrieval"
	EventChatComplete    = "chat:complete"
	EventChatStopped     = "chat:stopped"
	EventChatError       = "chat:error"
	EventChatUserMessage = "chat:user-message"
)
