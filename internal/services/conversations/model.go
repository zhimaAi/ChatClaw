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

// AgentType constants
const (
	AgentTypeEino     = "eino"
	AgentTypeOpenClaw = "openclaw"
)

// ChatMode constants
const (
	ChatModeChat = "chat" // Direct LLM conversation with knowledge/memory retrieval
	ChatModeTask = "task" // ReAct agent with tool calling
)

// TeamType constants
const (
	TeamTypePerson = "person"
	TeamTypeTeam   = "team"
)

// ConversationSource constants
const (
	// ConversationSourceManual is the default source for regular user-created chats.
	ConversationSourceManual = ""
	// ConversationSourceOpenClawCron marks conversations created for OpenClaw cron runs.
	ConversationSourceOpenClawCron = "openclaw_cron"
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

// NormalizeTeamType validates and canonicalizes team type values.
// Empty values default to person.
func NormalizeTeamType(raw string) (string, bool) {
	switch strings.TrimSpace(raw) {
	case "":
		return TeamTypePerson, true
	case TeamTypePerson:
		return TeamTypePerson, true
	case TeamTypeTeam:
		return TeamTypeTeam, true
	default:
		return "", false
	}
}

// NormalizeConversationSource canonicalizes conversation source values.
// Unknown values fall back to the default manual source.
func NormalizeConversationSource(raw string) string {
	trimmed := strings.TrimSpace(raw)
	switch {
	case trimmed == ConversationSourceOpenClawCron:
		return ConversationSourceOpenClawCron
	case strings.HasPrefix(trimmed, ConversationSourceOpenClawCron+":"):
		return trimmed
	default:
		return ConversationSourceManual
	}
}

// Conversation 会话 DTO（暴露给前端）
type Conversation struct {
	ID int64 `json:"id"`

	AgentID            int64   `json:"agent_id"`
	AgentType          string  `json:"agent_type"`
	ConversationSource string  `json:"conversation_source"`
	Name               string  `json:"name"`
	ExternalID         string  `json:"external_id"` // External unique identifier (e.g., channel conversation key)
	LastMessage        string  `json:"last_message"`
	IsPinned           bool    `json:"is_pinned"`
	LLMProviderID      string  `json:"llm_provider_id"`
	LLMModelID         string  `json:"llm_model_id"`
	LibraryIDs         []int64 `json:"library_ids"`
	EnableThinking     bool    `json:"enable_thinking"`
	OpenClawSessionKey string  `json:"openclaw_session_key"`
	ChatMode           string  `json:"chat_mode"`
	TeamType           string  `json:"team_type"`
	DialogueID         int64   `json:"dialogue_id"`     // team mode only
	TeamLibraryID      string  `json:"team_library_id"` // optional: ChatWiki team library id for recall

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CreateConversationInput 创建会话的输入参数
type CreateConversationInput struct {
	AgentID            int64   `json:"agent_id"`
	AgentType          string  `json:"agent_type"`
	ConversationSource string  `json:"conversation_source"`
	Name               string  `json:"name"`
	ExternalID         string  `json:"external_id"` // Optional external unique identifier
	LastMessage        string  `json:"last_message"`
	LLMProviderID      string  `json:"llm_provider_id"`
	LLMModelID         string  `json:"llm_model_id"`
	LibraryIDs         []int64 `json:"library_ids"`
	EnableThinking     bool    `json:"enable_thinking"`
	OpenClawSessionKey string  `json:"openclaw_session_key"`
	ChatMode           string  `json:"chat_mode"`
	TeamType           string  `json:"team_type"`
	DialogueID         int64   `json:"dialogue_id"`     // team mode only, default 0
	TeamLibraryID      string  `json:"team_library_id"` // optional: ChatWiki team library id for recall
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
	TeamType       *string  `json:"team_type"`
	DialogueID     *int64   `json:"dialogue_id"`     // team mode only
	TeamLibraryID  *string  `json:"team_library_id"` // optional
}

// conversationModel 数据库模型
type conversationModel struct {
	bun.BaseModel `bun:"table:conversations,alias:c"`

	ID        int64     `bun:"id,pk,autoincrement"`
	CreatedAt time.Time `bun:"created_at,notnull"`
	UpdatedAt time.Time `bun:"updated_at,notnull"`

	AgentID            int64  `bun:"agent_id,notnull"`
	AgentType          string `bun:"agent_type,notnull"`
	ConversationSource string `bun:"conversation_source,notnull"`
	Name               string `bun:"name,notnull"`
	ExternalID         string `bun:"external_id,notnull"`
	LastMessage        string `bun:"last_message,notnull"`
	IsPinned           bool   `bun:"is_pinned,notnull"`
	LLMProviderID      string `bun:"llm_provider_id,notnull"`
	LLMModelID         string `bun:"llm_model_id,notnull"`
	LibraryIDs         string `bun:"library_ids,notnull"` // JSON array stored as string
	EnableThinking     bool   `bun:"enable_thinking,notnull"`
	OpenClawSessionKey string `bun:"openclaw_session_key,notnull"`
	ChatMode           string `bun:"chat_mode,notnull"`
	TeamType           string `bun:"team_type,notnull"`
	DialogueID         int64  `bun:"dialogue_id,notnull"`     // team mode only, default 0
	TeamLibraryID      string `bun:"team_library_id,notnull"` // optional, default ''
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

	teamType, ok := NormalizeTeamType(m.TeamType)
	if !ok {
		teamType = TeamTypePerson
	}

	agentType := m.AgentType
	if agentType == "" {
		agentType = AgentTypeEino
	}

	return Conversation{
		ID: m.ID,

		AgentID:            m.AgentID,
		AgentType:          agentType,
		ConversationSource: NormalizeConversationSource(m.ConversationSource),
		Name:               m.Name,
		ExternalID:         m.ExternalID,
		LastMessage:        m.LastMessage,
		IsPinned:           m.IsPinned,
		LLMProviderID:      m.LLMProviderID,
		LLMModelID:         m.LLMModelID,
		LibraryIDs:         libraryIDs,
		EnableThinking:     m.EnableThinking,
		OpenClawSessionKey: m.OpenClawSessionKey,
		ChatMode:           chatMode,
		TeamType:           teamType,
		DialogueID:         m.DialogueID,
		TeamLibraryID:      m.TeamLibraryID,

		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	}
}
