package openclawchannels

import (
	"context"
	"time"

	"chatclaw/internal/services/channels"
	"chatclaw/internal/sqlite"

	"github.com/uptrace/bun"
)

// channelModel mirrors the shared channels table for read-only queries.
// Write operations delegate to channels.ChannelService.
type channelModel struct {
	bun.BaseModel `bun:"table:channels,alias:ch"`

	ID              int64      `bun:"id,pk,autoincrement"`
	Platform        string     `bun:"platform,notnull"`
	Name            string     `bun:"name,notnull"`
	Avatar          string     `bun:"avatar,notnull"`
	Enabled         bool       `bun:"enabled,notnull"`
	ConnectionType  string     `bun:"connection_type,notnull"`
	ExtraConfig     string     `bun:"extra_config,notnull"`
	AgentID         int64      `bun:"agent_id,notnull"`
	Status          string     `bun:"status,notnull"`
	LastConnectedAt *time.Time `bun:"last_connected_at"`
	CreatedAt       time.Time  `bun:"created_at,notnull"`
	UpdatedAt       time.Time  `bun:"updated_at,notnull"`
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

func (m *channelModel) toDTO() channels.Channel {
	return channels.Channel{
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
