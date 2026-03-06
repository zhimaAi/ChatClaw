package channels

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"chatclaw/internal/errs"
	"chatclaw/internal/sqlite"

	"github.com/uptrace/bun"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// EnsureAgentFunc creates or retrieves an agent for a channel.
// Returns the agent ID. Called when a channel is connected but has no linked agent.
type EnsureAgentFunc func(channelName string) (agentID int64, err error)

// ChannelService exposes channel CRUD + gateway control to the frontend.
type ChannelService struct {
	app         *application.App
	gateway     *Gateway
	ensureAgent EnsureAgentFunc
}

func NewChannelService(app *application.App, gw *Gateway, ensureAgent EnsureAgentFunc) *ChannelService {
	return &ChannelService{app: app, gateway: gw, ensureAgent: ensureAgent}
}

func (s *ChannelService) db() (*bun.DB, error) {
	db := sqlite.DB()
	if db == nil {
		return nil, errs.New("error.sqlite_not_initialized")
	}
	return db, nil
}

// ListChannels returns all channels ordered by creation time.
func (s *ChannelService) ListChannels() ([]Channel, error) {
	db, err := s.db()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var models []channelModel
	if err := db.NewSelect().
		Model(&models).
		OrderExpr("id DESC").
		Scan(ctx); err != nil {
		return nil, errs.Wrap("error.channel_list_failed", err)
	}

	out := make([]Channel, 0, len(models))
	for i := range models {
		ch := models[i].toDTO()
		if s.gateway.IsConnected(ch.ID) {
			ch.Status = StatusOnline
		}
		out = append(out, ch)
	}
	return out, nil
}

// GetChannelStats returns aggregated statistics.
func (s *ChannelService) GetChannelStats() (*ChannelStats, error) {
	channels, err := s.ListChannels()
	if err != nil {
		return nil, err
	}

	stats := &ChannelStats{Total: len(channels)}
	for _, ch := range channels {
		if ch.Status == StatusOnline {
			stats.Connected++
		} else {
			stats.Disconnected++
		}
	}
	return stats, nil
}

// GetSupportedPlatforms returns all platforms with registered adapters.
func (s *ChannelService) GetSupportedPlatforms() []PlatformMeta {
	allPlatforms := []PlatformMeta{
		{ID: PlatformDingTalk, Name: "钉钉", AuthType: "token"},
		{ID: PlatformFeishu, Name: "飞书", AuthType: "token"},
		{ID: PlatformWeCom, Name: "企微", AuthType: "token"},
		{ID: PlatformQQ, Name: "QQ", AuthType: "token"},
		{ID: PlatformTwitter, Name: "X(Twitter)", AuthType: "token"},
	}
	return allPlatforms
}

// CreateChannel creates a new channel.
func (s *ChannelService) CreateChannel(input CreateChannelInput) (*Channel, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, errs.New("error.channel_name_required")
	}
	platform := strings.TrimSpace(input.Platform)
	if platform == "" {
		return nil, errs.New("error.channel_platform_required")
	}

	connType := strings.TrimSpace(input.ConnectionType)
	if connType == "" {
		connType = ConnTypeGateway
	}

	db, err := s.db()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	m := &channelModel{
		Platform:       platform,
		Name:           name,
		Avatar:         "",
		Enabled:        false,
		ConnectionType: connType,
		ExtraConfig:    input.ExtraConfig,
		Status:         StatusOffline,
	}

	if _, err := db.NewInsert().Model(m).Exec(ctx); err != nil {
		return nil, errs.Wrap("error.channel_create_failed", fmt.Errorf("insert: %w", err))
	}

	// Auto-connect the channel asynchronously
	go func() {
		if err := s.ConnectChannel(m.ID); err != nil {
			s.app.Logger.Warn("auto-connect channel failed", "channel_id", m.ID, "error", err)
		}
	}()

	dto := m.toDTO()
	return &dto, nil
}

// UpdateChannel updates a channel's mutable fields.
func (s *ChannelService) UpdateChannel(id int64, input UpdateChannelInput) (*Channel, error) {
	if id <= 0 {
		return nil, errs.New("error.channel_id_required")
	}

	db, err := s.db()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	q := db.NewUpdate().
		Model((*channelModel)(nil)).
		Where("id = ?", id)

	if input.Name != nil {
		n := strings.TrimSpace(*input.Name)
		if n == "" {
			return nil, errs.New("error.channel_name_required")
		}
		q = q.Set("name = ?", n)
	}
	if input.Enabled != nil {
		q = q.Set("enabled = ?", *input.Enabled)
	}
	if input.ExtraConfig != nil {
		q = q.Set("extra_config = ?", *input.ExtraConfig)
	}

	res, err := q.Exec(ctx)
	if err != nil {
		return nil, errs.Wrap("error.channel_update_failed", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return nil, errs.Newf("error.channel_not_found", map[string]any{"ID": id})
	}

	var m channelModel
	if err := db.NewSelect().Model(&m).Where("id = ?", id).Limit(1).Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.Newf("error.channel_not_found", map[string]any{"ID": id})
		}
		return nil, errs.Wrap("error.channel_read_failed", err)
	}
	dto := m.toDTO()
	return &dto, nil
}

// UnbindAgent removes the AI agent association from a channel.
func (s *ChannelService) UnbindAgent(id int64) error {
	if id <= 0 {
		return errs.New("error.channel_id_required")
	}

	db, err := s.db()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if _, err := db.NewUpdate().
		Model((*channelModel)(nil)).
		Where("id = ?", id).
		Set("agent_id = ?", 0).
		Exec(ctx); err != nil {
		return errs.Wrap("error.channel_unbind_failed", err)
	}

	return nil
}

// DeleteChannel removes a channel and disconnects it if active.
func (s *ChannelService) DeleteChannel(id int64) error {
	if id <= 0 {
		return errs.New("error.channel_id_required")
	}

	_ = s.gateway.DisconnectChannel(context.Background(), id)

	db, err := s.db()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	res, err := db.NewDelete().Model((*channelModel)(nil)).Where("id = ?", id).Exec(ctx)
	if err != nil {
		return errs.Wrap("error.channel_delete_failed", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return errs.Newf("error.channel_not_found", map[string]any{"ID": id})
	}
	return nil
}

// ConnectChannel starts a gateway connection for the given channel.
// If the channel has no linked AI agent, one is created automatically.
func (s *ChannelService) ConnectChannel(id int64) error {
	if id <= 0 {
		return errs.New("error.channel_id_required")
	}

	db, err := s.db()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var m channelModel
	if err := db.NewSelect().Model(&m).Where("id = ?", id).Limit(1).Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errs.Newf("error.channel_not_found", map[string]any{"ID": id})
		}
		return errs.Wrap("error.channel_read_failed", err)
	}

	// Mark as enabled
	if _, err := db.NewUpdate().
		Model((*channelModel)(nil)).
		Where("id = ?", id).
		Set("enabled = ?", true).
		Exec(ctx); err != nil {
		return errs.Wrap("error.channel_update_failed", err)
	}

	ch := m.toDTO()
	if err := s.gateway.ConnectChannel(context.Background(), ch); err != nil {
		return errs.Wrapf("error.channel_connect_failed", err, map[string]any{"Name": ch.Name})
	}

	// Auto-create an AI agent if the channel doesn't have one yet
	if m.AgentID == 0 && s.ensureAgent != nil {
		agentID, err := s.ensureAgent(m.Name)
		if err != nil {
			return errs.Wrap("error.channel_agent_create_failed", err)
		}
		m.AgentID = agentID
		if _, err := db.NewUpdate().
			Model((*channelModel)(nil)).
			Where("id = ?", id).
			Set("agent_id = ?", agentID).
			Exec(ctx); err != nil {
			return errs.Wrap("error.channel_update_failed", err)
		}
		s.app.Logger.Info("auto-created agent for channel", "channel_id", id, "agent_id", agentID)
	}

	return nil
}

// DisconnectChannel stops the gateway connection for the given channel.
func (s *ChannelService) DisconnectChannel(id int64) error {
	if id <= 0 {
		return errs.New("error.channel_id_required")
	}

	db, err := s.db()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Mark as disabled
	if _, err := db.NewUpdate().
		Model((*channelModel)(nil)).
		Where("id = ?", id).
		Set("enabled = ?", false).
		Exec(ctx); err != nil {
		return errs.Wrap("error.channel_update_failed", err)
	}

	return s.gateway.DisconnectChannel(context.Background(), id)
}

// RefreshChannels refreshes gateway connection statuses.
func (s *ChannelService) RefreshChannels() error {
	s.gateway.RefreshStatuses(context.Background())
	return nil
}
