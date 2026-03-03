package conversations

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"chatclaw/internal/sqlite"

	"github.com/uptrace/bun"
)

// ChatMode constants
const (
	ChatModeChat = "chat" // Direct LLM conversation with knowledge/memory retrieval
	ChatModeTask = "task" // ReAct agent with tool calling
)

// NormalizeChatMode validates and canonicalizes chat mode values.
// Empty values default to task mode.
func NormalizeChatMode(raw string) (string, bool) {
	switch strings.TrimSpace(raw) {
	case "":
		return ChatModeTask, true
	case ChatModeChat:
		return ChatModeChat, true
	case ChatModeTask:
		return ChatModeTask, true
	default:
		return "", false
	}
}

// Conversation 会话 DTO（暴露给前端）
type Conversation struct {
	ID int64 `json:"id"`

	AgentID        int64   `json:"agent_id"`
	Name           string  `json:"name"`
	LastMessage    string  `json:"last_message"`
	IsPinned       bool    `json:"is_pinned"`
	LLMProviderID  string  `json:"llm_provider_id"`
	LLMModelID     string  `json:"llm_model_id"`
	LibraryIDs     []int64 `json:"library_ids"`
	EnableThinking bool    `json:"enable_thinking"`
	ChatMode       string  `json:"chat_mode"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CreateConversationInput 创建会话的输入参数
type CreateConversationInput struct {
	AgentID        int64   `json:"agent_id"`
	Name           string  `json:"name"`
	LastMessage    string  `json:"last_message"`
	LLMProviderID  string  `json:"llm_provider_id"`
	LLMModelID     string  `json:"llm_model_id"`
	LibraryIDs     []int64 `json:"library_ids"`
	EnableThinking bool    `json:"enable_thinking"`
	ChatMode       string  `json:"chat_mode"`
}

// UpdateConversationInput 更新会话的输入参数
type UpdateConversationInput struct {
	Name           *string  `json:"name"`
	LastMessage    *string  `json:"last_message"`
	IsPinned       *bool    `json:"is_pinned"`
	LLMProviderID  *string  `json:"llm_provider_id"`
	LLMModelID     *string  `json:"llm_model_id"`
	LibraryIDs     *[]int64 `json:"library_ids"`
	EnableThinking *bool    `json:"enable_thinking"`
	ChatMode       *string  `json:"chat_mode"`
}

// conversationModel 数据库模型
type conversationModel struct {
	bun.BaseModel `bun:"table:conversations,alias:c"`

	ID        int64     `bun:"id,pk,autoincrement"`
	CreatedAt time.Time `bun:"created_at,notnull"`
	UpdatedAt time.Time `bun:"updated_at,notnull"`

	AgentID        int64  `bun:"agent_id,notnull"`
	Name           string `bun:"name,notnull"`
	LastMessage    string `bun:"last_message,notnull"`
	IsPinned       bool   `bun:"is_pinned,notnull"`
	LLMProviderID  string `bun:"llm_provider_id,notnull"`
	LLMModelID     string `bun:"llm_model_id,notnull"`
	LibraryIDs     string `bun:"library_ids,notnull"` // JSON array stored as string
	EnableThinking bool   `bun:"enable_thinking,notnull"`
	ChatMode       string `bun:"chat_mode,notnull"`
}

// BeforeInsert 在 INSERT 时自动设置 created_at 和 updated_at
var _ bun.BeforeInsertHook = (*conversationModel)(nil)

func (*conversationModel) BeforeInsert(ctx context.Context, query *bun.InsertQuery) error {
	now := sqlite.NowUTC()
	query.Value("created_at", "?", now)
	query.Value("updated_at", "?", now)
	return nil
}

// BeforeUpdate 在 UPDATE 时自动设置 updated_at
var _ bun.BeforeUpdateHook = (*conversationModel)(nil)

func (*conversationModel) BeforeUpdate(ctx context.Context, query *bun.UpdateQuery) error {
	query.Set("updated_at = ?", sqlite.NowUTC())
	return nil
}

func (m *conversationModel) toDTO() Conversation {
	// Parse library_ids from JSON string
	var libraryIDs []int64
	if m.LibraryIDs != "" && m.LibraryIDs != "[]" {
		if err := json.Unmarshal([]byte(m.LibraryIDs), &libraryIDs); err != nil {
			slog.Warn("[conversations] failed to parse library_ids", "conversation_id", m.ID, "error", err)
			libraryIDs = []int64{}
		}
	}
	if libraryIDs == nil {
		libraryIDs = []int64{}
	}

	chatMode, ok := NormalizeChatMode(m.ChatMode)
	if !ok {
		chatMode = ChatModeTask
	}

	return Conversation{
		ID: m.ID,

		AgentID:        m.AgentID,
		Name:           m.Name,
		LastMessage:    m.LastMessage,
		IsPinned:       m.IsPinned,
		LLMProviderID:  m.LLMProviderID,
		LLMModelID:     m.LLMModelID,
		LibraryIDs:     libraryIDs,
		EnableThinking: m.EnableThinking,
		ChatMode:       chatMode,

		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	}
}
