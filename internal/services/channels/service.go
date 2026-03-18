package channels

import (
	"context"
	"database/sql"
	"encoding/json"
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
	// Name is a locale-neutral fallback; UI should use platform ID with i18n (channels.platforms.*).
	allPlatforms := []PlatformMeta{
		{ID: PlatformDingTalk, Name: "DingTalk", AuthType: "token"},
		{ID: PlatformFeishu, Name: "Feishu", AuthType: "token"},
		{ID: PlatformWeCom, Name: "WeCom", AuthType: "token"},
		{ID: PlatformQQ, Name: "QQ", AuthType: "token"},
		{ID: PlatformTwitter, Name: "X (Twitter)", AuthType: "token"},
	}
	return allPlatforms
}

// appCredentialsJSON is the common shape for Feishu/WeCom extra_config.
type appCredentialsJSON struct {
	AppID     string `json:"app_id"`
	AppSecret string `json:"app_secret"`
}

func parseAppCredentials(extraConfig string) (appID, appSecret string) {
	extraConfig = strings.TrimSpace(extraConfig)
	if extraConfig == "" {
		return "", ""
	}
	var cfg appCredentialsJSON
	if err := json.Unmarshal([]byte(extraConfig), &cfg); err != nil {
		return "", ""
	}
	return strings.TrimSpace(cfg.AppID), strings.TrimSpace(cfg.AppSecret)
}

// ensureNoDuplicateCredentials rejects create/update when another channel of the same
// platform already uses the same app_id or app_secret (Feishu / WeCom).
// excludeID > 0 skips that channel (for update).
func (s *ChannelService) ensureNoDuplicateCredentials(ctx context.Context, db *bun.DB, platform, extraConfig string, excludeID int64) error {
	platform = strings.TrimSpace(platform)
	if platform != PlatformFeishu && platform != PlatformWeCom && platform != PlatformDingTalk {
		return nil
	}
	appID, appSecret := parseAppCredentials(extraConfig)
	if appID == "" && appSecret == "" {
		return nil
	}

	var models []channelModel
	if err := db.NewSelect().Model(&models).Where("platform = ?", platform).Scan(ctx); err != nil {
		return errs.Wrap("error.channel_list_failed", err)
	}
	for i := range models {
		if excludeID > 0 && models[i].ID == excludeID {
			continue
		}
		existID, existSecret := parseAppCredentials(models[i].ExtraConfig)
		if appID != "" && existID != "" && appID == existID {
			return errs.New("error.channel_config_duplicate_app_id")
		}
		if appSecret != "" && existSecret != "" && appSecret == existSecret {
			return errs.New("error.channel_config_duplicate_app_secret")
		}
	}
	return nil
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

	if err := s.ensureNoDuplicateCredentials(ctx, db, platform, input.ExtraConfig, 0); err != nil {
		return nil, err
	}

	m := &channelModel{
		Platform:       platform,
		Name:           name,
		Avatar:         input.Avatar,
		Enabled:        false,
		ConnectionType: connType,
		ExtraConfig:    input.ExtraConfig,
		Status:         StatusOffline,
	}

	if _, err := db.NewInsert().Model(m).Exec(ctx); err != nil {
		return nil, errs.Wrap("error.channel_create_failed", fmt.Errorf("insert: %w", err))
	}

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
	if input.Avatar != nil {
		q = q.Set("avatar = ?", *input.Avatar)
	}
	if input.Enabled != nil {
		q = q.Set("enabled = ?", *input.Enabled)
	}
	if input.ExtraConfig != nil {
		// Load platform for duplicate credential check when updating config
		var existing channelModel
		if err := db.NewSelect().Model(&existing).Where("id = ?", id).Limit(1).Scan(ctx); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, errs.Newf("error.channel_not_found", map[string]any{"ID": id})
			}
			return nil, errs.Wrap("error.channel_read_failed", err)
		}
		if err := s.ensureNoDuplicateCredentials(ctx, db, existing.Platform, *input.ExtraConfig, id); err != nil {
			return nil, err
		}
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

// BindAgent updates the AI agent association for a channel.
func (s *ChannelService) BindAgent(id int64, agentID int64) error {
	if id <= 0 {
		return errs.New("error.channel_id_required")
	}
	if agentID <= 0 {
		return errs.New("error.agent_id_required")
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
		Set("agent_id = ?", agentID).
		Exec(ctx); err != nil {
		return errs.Wrap("error.channel_bind_failed", err)
	}

	return nil
}

// EnsureAgentForChannel creates an AI agent for the channel (if not already bound) and binds it.
// Returns the agent ID. Used when the user chooses "auto-generate assistant" in the bind dialog.
func (s *ChannelService) EnsureAgentForChannel(id int64) (int64, error) {
	if id <= 0 {
		return 0, errs.New("error.channel_id_required")
	}

	db, err := s.db()
	if err != nil {
		return 0, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var m channelModel
	if err := db.NewSelect().Model(&m).Where("id = ?", id).Limit(1).Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, errs.Newf("error.channel_not_found", map[string]any{"ID": id})
		}
		return 0, errs.Wrap("error.channel_read_failed", err)
	}

	if m.AgentID != 0 {
		return m.AgentID, nil
	}

	if s.ensureAgent == nil {
		return 0, errs.New("error.channel_agent_create_not_available")
	}

	agentID, err := s.ensureAgent(m.Name)
	if err != nil {
		return 0, errs.Wrap("error.channel_agent_create_failed", err)
	}

	if _, err := db.NewUpdate().
		Model((*channelModel)(nil)).
		Where("id = ?", id).
		Set("agent_id = ?", agentID).
		Exec(ctx); err != nil {
		return 0, errs.Wrap("error.channel_bind_failed", err)
	}

	s.app.Logger.Info("auto-created and bound agent for channel", "channel_id", id, "agent_id", agentID)
	return agentID, nil
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

	if m.AgentID == 0 {
		return errs.New("error.channel_connect_requires_agent")
	}

	// Mark as enabled and touch updated_at (Bun BeforeUpdateHook is not run for this raw Set update)
	if _, err := db.NewUpdate().
		Model((*channelModel)(nil)).
		Where("id = ?", id).
		Set("enabled = ?", true).
		Set("updated_at = ?", sqlite.NowUTC()).
		Exec(ctx); err != nil {
		return errs.Wrap("error.channel_update_failed", err)
	}
	m.Enabled = true

	ch := m.toDTO()
	if err := s.gateway.ConnectChannel(context.Background(), ch); err != nil {
		return errs.Wrapf("error.channel_connect_failed", err, map[string]any{"Name": ch.Name})
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

	// Mark as disabled and touch updated_at
	if _, err := db.NewUpdate().
		Model((*channelModel)(nil)).
		Where("id = ?", id).
		Set("enabled = ?", false).
		Set("updated_at = ?", sqlite.NowUTC()).
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

// VerifyChannelConfig verifies that the given platform credentials (extraConfig) can connect.
// It creates a temporary adapter, attempts Connect, then Disconnect. Used for "验证配置" before saving.
func (s *ChannelService) VerifyChannelConfig(platform string, extraConfig string) error {
	platform = strings.TrimSpace(platform)
	if platform == "" {
		return errs.New("error.channel_platform_required")
	}
	extraConfig = strings.TrimSpace(extraConfig)
	if extraConfig == "" {
		return errs.New("error.channel_extra_config_required")
	}

	adapter := NewAdapter(platform)
	if adapter == nil {
		return errs.Newf("error.channel_platform_unsupported", map[string]any{"Platform": platform})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	noopHandler := func(msg IncomingMessage) {}
	if err := adapter.Connect(ctx, 0, extraConfig, noopHandler); err != nil {
		errStr := err.Error()
		if platform == PlatformDingTalk && (strings.Contains(errStr, "no such host") || strings.Contains(errStr, "lookup ")) {
			return errs.Wrapf("error.dingtalk_network_dns_failed", err, map[string]any{"Platform": platform})
		}
		return errs.Wrapf("error.channel_verify_failed", err, map[string]any{"Platform": platform})
	}
	// Disconnect to avoid leaving a live connection; ignore disconnect error for verify flow
	_ = adapter.Disconnect(context.Background())
	return nil
}
