package channels

import (
	"context"
	"time"

	"chatclaw/internal/sqlite"

	"github.com/uptrace/bun"
)

// Platform identifiers
const (
	PlatformDingTalk = "dingtalk"
	PlatformFeishu   = "feishu"
	PlatformWeCom    = "wecom"
	PlatformQQ       = "qq"
	PlatformTwitter  = "twitter"
)

// Connection status
const (
	StatusOnline  = "online"
	StatusOffline = "offline"
	StatusError   = "error"
)

// Connection type
const (
	ConnTypeGateway = "gateway"
	ConnTypeWebhook = "webhook"
)

// Channel DTO exposed to frontend
type Channel struct {
	ID int64 `json:"id"`

	Platform       string `json:"platform"`
	Name           string `json:"name"`
	Avatar         string `json:"avatar"`
	Enabled        bool   `json:"enabled"`
	ConnectionType string `json:"connection_type"`
	ExtraConfig    string `json:"extra_config"`
	AgentID        int64  `json:"agent_id"`

	Status          string     `json:"status"`
	LastConnectedAt *time.Time `json:"last_connected_at"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ChannelStats aggregated stats for the channel list
type ChannelStats struct {
	Total        int `json:"total"`
	Connected    int `json:"connected"`
	Disconnected int `json:"disconnected"`
}

// CreateChannelInput input for creating a new channel
type CreateChannelInput struct {
	Platform       string `json:"platform"`
	Name           string `json:"name"`
	Avatar         string `json:"avatar"`
	ConnectionType string `json:"connection_type"`
	ExtraConfig    string `json:"extra_config"`
}

// UpdateChannelInput input for updating a channel
type UpdateChannelInput struct {
	Name        *string `json:"name"`
	Avatar      *string `json:"avatar"`
	Enabled     *bool   `json:"enabled"`
	ExtraConfig *string `json:"extra_config"`
}

// PlatformMeta describes a supported platform (for the frontend grid)
type PlatformMeta struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	AuthType string `json:"auth_type"` // token, qrcode
}

// channelModel database model
type channelModel struct {
	bun.BaseModel `bun:"table:channels,alias:ch"`

	ID             int64      `bun:"id,pk,autoincrement"`
	Platform       string     `bun:"platform,notnull"`
	Name           string     `bun:"name,notnull"`
	Avatar         string     `bun:"avatar,notnull"`
	Enabled        bool       `bun:"enabled,notnull"`
	ConnectionType string     `bun:"connection_type,notnull"`
	ExtraConfig    string     `bun:"extra_config,notnull"`
	AgentID        int64      `bun:"agent_id,notnull"`
	Status         string     `bun:"status,notnull"`
	LastConnectedAt *time.Time `bun:"last_connected_at"`
	CreatedAt      time.Time  `bun:"created_at,notnull"`
	UpdatedAt      time.Time  `bun:"updated_at,notnull"`
}

var _ bun.BeforeInsertHook = (*channelModel)(nil)

func (*channelModel) BeforeInsert(ctx context.Context, query *bun.InsertQuery) error {
	now := sqlite.NowUTC()
	query.Value("created_at", "?", now)
	query.Value("updated_at", "?", now)
	return nil
}

var _ bun.BeforeUpdateHook = (*channelModel)(nil)

func (*channelModel) BeforeUpdate(ctx context.Context, query *bun.UpdateQuery) error {
	query.Set("updated_at = ?", sqlite.NowUTC())
	return nil
}

func (m *channelModel) toDTO() Channel {
	return Channel{
		ID:              m.ID,
		Platform:        m.Platform,
		Name:            m.Name,
		Avatar:          m.Avatar,
		Enabled:         m.Enabled,
		ConnectionType:  m.ConnectionType,
		ExtraConfig:     m.ExtraConfig,
		AgentID:         m.AgentID,
		Status:          m.Status,
		LastConnectedAt: m.LastConnectedAt,
		CreatedAt:       m.CreatedAt,
		UpdatedAt:       m.UpdatedAt,
	}
}
