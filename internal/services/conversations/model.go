package conversations

import (
	"context"
	"time"

	"willchat/internal/sqlite"

	"github.com/uptrace/bun"
)

// Conversation 会话 DTO（暴露给前端）
type Conversation struct {
	ID int64 `json:"id"`

	AgentID     int64  `json:"agent_id"`
	Name        string `json:"name"`
	LastMessage string `json:"last_message"`
	IsPinned    bool   `json:"is_pinned"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CreateConversationInput 创建会话的输入参数
type CreateConversationInput struct {
	AgentID     int64  `json:"agent_id"`
	Name        string `json:"name"`
	LastMessage string `json:"last_message"`
}

// UpdateConversationInput 更新会话的输入参数
type UpdateConversationInput struct {
	Name        *string `json:"name"`
	LastMessage *string `json:"last_message"`
	IsPinned    *bool   `json:"is_pinned"`
}

// conversationModel 数据库模型
type conversationModel struct {
	bun.BaseModel `bun:"table:conversations,alias:c"`

	ID        int64     `bun:"id,pk,autoincrement"`
	CreatedAt time.Time `bun:"created_at,notnull"`
	UpdatedAt time.Time `bun:"updated_at,notnull"`

	AgentID     int64  `bun:"agent_id,notnull"`
	Name        string `bun:"name,notnull"`
	LastMessage string `bun:"last_message,notnull"`
	IsPinned    bool   `bun:"is_pinned,notnull"`
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
	return Conversation{
		ID: m.ID,

		AgentID:     m.AgentID,
		Name:        m.Name,
		LastMessage: m.LastMessage,
		IsPinned:    m.IsPinned,

		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	}
}
