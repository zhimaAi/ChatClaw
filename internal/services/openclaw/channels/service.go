package openclawchannels

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"chatclaw/internal/errs"
	"chatclaw/internal/services/channels"
	"chatclaw/internal/services/openclawagents"
	"chatclaw/internal/services/openclawruntime"
	"chatclaw/internal/sqlite"

	"github.com/uptrace/bun"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// OpenClawChannelService provides Feishu-focused channel management for OpenClaw.
// It delegates to the shared channels infrastructure while filtering by OpenClaw agents.
type OpenClawChannelService struct {
	app             *application.App
	gateway         *channels.Gateway
	agentsSvc       *openclawagents.OpenClawAgentsService
	channelSvc      *channels.ChannelService
	openclawManager *openclawruntime.Manager
}

var errOpenClawChannelRPCUnavailable = errors.New("openclaw channel rpc unavailable")

func NewOpenClawChannelService(
	app *application.App,
	gw *channels.Gateway,
	agentsSvc *openclawagents.OpenClawAgentsService,
	channelSvc *channels.ChannelService,
	openclawManager *openclawruntime.Manager,
) *OpenClawChannelService {
	return &OpenClawChannelService{
		app:             app,
		gateway:         gw,
		agentsSvc:       agentsSvc,
		channelSvc:      channelSvc,
		openclawManager: openclawManager,
	}
}

func (s *OpenClawChannelService) db() (*bun.DB, error) {
	db := sqlite.DB()
	if db == nil {
		return nil, errs.New("error.sqlite_not_initialized")
	}
	return db, nil
}

// openClawChannelVisibilitySQL matches migration 202603251200: channels.agent_id is a bare int and can
// collide between ChatClaw (agents) and OpenClaw (openclaw_agents); only treat a row as OpenClaw-bound
// when agent_id exists in openclaw_agents and not in agents.
const openClawChannelVisibilitySQL = `(ch.openclaw_scope = 1 OR (ch.agent_id > 0 AND EXISTS (SELECT 1 FROM openclaw_agents AS oa WHERE oa.id = ch.agent_id) AND NOT EXISTS (SELECT 1 FROM agents AS a WHERE a.id = ch.agent_id)))`

// ListChannels returns channels in OpenClaw scope or bound to OpenClaw-only agents (all platforms).
func (s *OpenClawChannelService) ListChannels() ([]channels.Channel, error) {
	db, err := s.db()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var models []channelModel
	q := db.NewSelect().
		Model(&models).
		Where(openClawChannelVisibilitySQL).
		OrderExpr("ch.id DESC")
	if err := q.Scan(ctx); err != nil {
		return nil, errs.Wrap("error.channel_list_failed", err)
	}

	out := make([]channels.Channel, 0, len(models))
	for i := range models {
		ch := models[i].toDTO()
		if s.gateway.IsConnected(ch.ID) {
			ch.Status = channels.StatusOnline
		}
		out = append(out, ch)
	}
	return out, nil
}

// ListAllFeishuChannels returns all Feishu channels (including unbound ones)
// for the "add existing bot" workflow.
func (s *OpenClawChannelService) ListAllFeishuChannels() ([]channels.Channel, error) {
	db, err := s.db()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var models []channelModel
	q := db.NewSelect().
		Model(&models).
		Where("ch.platform = ?", channels.PlatformFeishu).
		Where(openClawChannelVisibilitySQL).
		OrderExpr("ch.id DESC")
	if err := q.Scan(ctx); err != nil {
		return nil, errs.Wrap("error.channel_list_failed", err)
	}

	out := make([]channels.Channel, 0, len(models))
	for i := range models {
		ch := models[i].toDTO()
		if s.gateway.IsConnected(ch.ID) {
			ch.Status = channels.StatusOnline
		}
		out = append(out, ch)
	}
	return out, nil
}

// GetChannelStats returns stats for OpenClaw-scoped channels.
func (s *OpenClawChannelService) GetChannelStats() (*channels.ChannelStats, error) {
	chList, err := s.ListChannels()
	if err != nil {
		return nil, err
	}

	stats := &channels.ChannelStats{Total: len(chList)}
	for _, ch := range chList {
		if ch.Status == channels.StatusOnline {
			stats.Connected++
		} else {
			stats.Disconnected++
		}
	}
	return stats, nil
}

// GetSupportedPlatforms returns the same platform list as ChatClaw for UI parity (tabs + add dialog).
// Only Feishu is actually createable; the frontend shows others as disabled with "coming soon".
func (s *OpenClawChannelService) GetSupportedPlatforms() []channels.PlatformMeta {
	return []channels.PlatformMeta{
		{ID: channels.PlatformDingTalk, Name: "DingTalk", AuthType: "token"},
		{ID: channels.PlatformFeishu, Name: "Feishu", AuthType: "token"},
		{ID: channels.PlatformWeCom, Name: "WeCom", AuthType: "token"},
		{ID: channels.PlatformQQ, Name: "QQ", AuthType: "token"},
		{ID: channels.PlatformTwitter, Name: "X (Twitter)", AuthType: "token"},
	}
}

// CreateChannel creates a new Feishu channel. When agent_id > 0, binds that OpenClaw agent;
// when agent_id is 0, creates an unbound channel (UI binds via BindAgent or auto-generate later).
func (s *OpenClawChannelService) CreateChannel(input CreateChannelInput) (*channels.Channel, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, errs.New("error.channel_name_required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.ensureOpenClawReady(); err != nil {
		return nil, err
	}
	openClawChannelID, err := s.createOpenClawFeishuChannel(ctx, name, input.ExtraConfig)
	if err != nil {
		return nil, errs.Wrap("error.channel_create_failed", err)
	}
	extraConfigWithID, err := withOpenClawChannelID(input.ExtraConfig, openClawChannelID)
	if err != nil {
		return nil, errs.Wrap("error.channel_create_failed", err)
	}

	ch, err := s.channelSvc.CreateChannel(channels.CreateChannelInput{
		Platform:       channels.PlatformFeishu,
		Name:           name,
		Avatar:         input.Avatar,
		ConnectionType: channels.ConnTypeGateway,
		ExtraConfig:    extraConfigWithID,
		OpenClawScope:  true,
	})
	if err != nil {
		_ = s.deleteOpenClawChannel(context.Background(), openClawChannelID)
		return nil, err
	}

	if input.AgentID > 0 {
		if err := s.channelSvc.BindAgent(ch.ID, input.AgentID); err != nil {
			return nil, err
		}
		ch.AgentID = input.AgentID
	}
	if strings.TrimSpace(openClawChannelID) == "" {
		if err := s.syncOpenClawFeishuChannelsConfig(ctx); err != nil {
			_ = s.channelSvc.DeleteChannel(ch.ID)
			return nil, errs.Wrap("error.channel_create_failed", err)
		}
	}

	return ch, nil
}

// UpdateChannel delegates to the shared ChannelService.
func (s *OpenClawChannelService) UpdateChannel(id int64, input channels.UpdateChannelInput) (*channels.Channel, error) {
	if id <= 0 {
		return nil, errs.New("error.channel_id_required")
	}
	if err := s.ensureOpenClawReady(); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	m, err := s.getChannelModel(ctx, id)
	if err != nil {
		return nil, err
	}

	name := m.Name
	if input.Name != nil {
		name = strings.TrimSpace(*input.Name)
	}
	extraConfig := m.ExtraConfig
	if input.ExtraConfig != nil {
		extraConfig = strings.TrimSpace(*input.ExtraConfig)
	}

	openClawChannelID := extractOpenClawChannelID(extraConfig)
	needConfigFallbackSync := false
	if openClawChannelID == "" {
		openClawChannelID, err = s.createOpenClawFeishuChannel(ctx, name, extraConfig)
		if err != nil {
			return nil, errs.Wrap("error.channel_update_failed", err)
		}
		if strings.TrimSpace(openClawChannelID) == "" {
			needConfigFallbackSync = true
		}
	}
	if err := s.updateOpenClawFeishuChannel(ctx, openClawChannelID, name, extraConfig); err != nil {
		if errors.Is(err, errOpenClawChannelRPCUnavailable) {
			needConfigFallbackSync = true
		} else {
			return nil, errs.Wrap("error.channel_update_failed", err)
		}
	}
	if input.Enabled != nil {
		if err := s.setOpenClawChannelEnabled(ctx, openClawChannelID, *input.Enabled); err != nil {
			if errors.Is(err, errOpenClawChannelRPCUnavailable) {
				needConfigFallbackSync = true
			} else {
				return nil, errs.Wrap("error.channel_update_failed", err)
			}
		}
	}

	extraConfigWithID, err := withOpenClawChannelID(extraConfig, openClawChannelID)
	if err != nil {
		return nil, errs.Wrap("error.channel_update_failed", err)
	}
	localInput := input
	localInput.ExtraConfig = &extraConfigWithID
	updated, err := s.channelSvc.UpdateChannel(id, localInput)
	if err != nil {
		return nil, err
	}
	if needConfigFallbackSync {
		if err := s.syncOpenClawFeishuChannelsConfig(ctx); err != nil {
			return nil, errs.Wrap("error.channel_update_failed", err)
		}
	}
	return updated, nil
}

// DeleteChannel delegates to the shared ChannelService.
func (s *OpenClawChannelService) DeleteChannel(id int64) error {
	if id <= 0 {
		return errs.New("error.channel_id_required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	m, err := s.getChannelModel(ctx, id)
	if err != nil {
		return err
	}
	openClawChannelID := extractOpenClawChannelID(m.ExtraConfig)
	needConfigFallbackSync := openClawChannelID == ""
	if openClawChannelID != "" {
		if err := s.ensureOpenClawReady(); err != nil {
			return err
		}
		if err := s.deleteOpenClawChannel(ctx, openClawChannelID); err != nil {
			if errors.Is(err, errOpenClawChannelRPCUnavailable) {
				needConfigFallbackSync = true
			} else {
				return errs.Wrap("error.channel_delete_failed", err)
			}
		}
	}
	if err := s.channelSvc.DeleteChannel(id); err != nil {
		return err
	}
	if needConfigFallbackSync {
		if err := s.syncOpenClawFeishuChannelsConfig(ctx); err != nil {
			return errs.Wrap("error.channel_delete_failed", err)
		}
	}
	return nil
}

// BindAgent delegates to the shared ChannelService.
func (s *OpenClawChannelService) BindAgent(id int64, agentID int64) error {
	return s.channelSvc.BindAgent(id, agentID)
}

// UnbindAgent delegates to the shared ChannelService.
func (s *OpenClawChannelService) UnbindAgent(id int64) error {
	return s.channelSvc.UnbindAgent(id)
}

// ConnectChannel delegates to the shared ChannelService.
func (s *OpenClawChannelService) ConnectChannel(id int64) error {
	if id <= 0 {
		return errs.New("error.channel_id_required")
	}
	if err := s.ensureOpenClawReady(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	m, err := s.getChannelModel(ctx, id)
	if err != nil {
		return err
	}
	openClawChannelID := extractOpenClawChannelID(m.ExtraConfig)
	needConfigFallbackSync := false
	if openClawChannelID == "" {
		openClawChannelID, err = s.createOpenClawFeishuChannel(ctx, m.Name, m.ExtraConfig)
		if err != nil {
			return errs.Wrap("error.channel_connect_failed", err)
		}
		if strings.TrimSpace(openClawChannelID) == "" {
			needConfigFallbackSync = true
		}
		extraConfigWithID, encodeErr := withOpenClawChannelID(m.ExtraConfig, openClawChannelID)
		if encodeErr == nil {
			if _, updateErr := s.channelSvc.UpdateChannel(id, channels.UpdateChannelInput{ExtraConfig: &extraConfigWithID}); updateErr != nil {
				return updateErr
			}
		}
	}
	if !needConfigFallbackSync {
		if err := s.setOpenClawChannelEnabled(ctx, openClawChannelID, true); err != nil {
			if errors.Is(err, errOpenClawChannelRPCUnavailable) {
				needConfigFallbackSync = true
			} else {
				return errs.Wrap("error.channel_connect_failed", err)
			}
		}
	}
	if err := s.channelSvc.ConnectChannel(id); err != nil {
		return err
	}
	if needConfigFallbackSync {
		if err := s.syncOpenClawFeishuChannelsConfig(ctx); err != nil {
			return errs.Wrap("error.channel_connect_failed", err)
		}
	}
	return nil
}

// DisconnectChannel delegates to the shared ChannelService.
func (s *OpenClawChannelService) DisconnectChannel(id int64) error {
	if id <= 0 {
		return errs.New("error.channel_id_required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	m, err := s.getChannelModel(ctx, id)
	if err != nil {
		return err
	}
	openClawChannelID := extractOpenClawChannelID(m.ExtraConfig)
	needConfigFallbackSync := openClawChannelID == ""
	if openClawChannelID != "" {
		if err := s.ensureOpenClawReady(); err != nil {
			return err
		}
		if err := s.setOpenClawChannelEnabled(ctx, openClawChannelID, false); err != nil {
			if errors.Is(err, errOpenClawChannelRPCUnavailable) {
				needConfigFallbackSync = true
			} else {
				return errs.Wrap("error.channel_disconnect_failed", err)
			}
		}
	}
	if err := s.channelSvc.DisconnectChannel(id); err != nil {
		return err
	}
	if needConfigFallbackSync {
		if err := s.syncOpenClawFeishuChannelsConfig(ctx); err != nil {
			return errs.Wrap("error.channel_disconnect_failed", err)
		}
	}
	return nil
}

// VerifyChannelConfig verifies Feishu credentials.
func (s *OpenClawChannelService) VerifyChannelConfig(extraConfig string) error {
	return s.channelSvc.VerifyChannelConfig(channels.PlatformFeishu, extraConfig)
}

// EnsureAgentForChannel auto-creates an OpenClaw agent and binds it to the channel.
func (s *OpenClawChannelService) EnsureAgentForChannel(channelID int64) (int64, error) {
	if channelID <= 0 {
		return 0, errs.New("error.channel_id_required")
	}

	db, err := s.db()
	if err != nil {
		return 0, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var m channelModel
	if err := db.NewSelect().Model(&m).Where("id = ?", channelID).Limit(1).Scan(ctx); err != nil {
		return 0, errs.Wrap("error.channel_read_failed", err)
	}
	if m.AgentID != 0 {
		return m.AgentID, nil
	}

	agent, err := s.agentsSvc.CreateAgent(openclawagents.CreateOpenClawAgentInput{
		Name: fmt.Sprintf("%s Agent", m.Name),
	})
	if err != nil {
		return 0, errs.Wrap("error.channel_agent_create_failed", err)
	}

	if err := s.channelSvc.BindAgent(channelID, agent.ID); err != nil {
		return 0, err
	}

	return agent.ID, nil
}

// ListAgents returns all OpenClaw agents for the bind dialog.
func (s *OpenClawChannelService) ListAgents() ([]openclawagents.OpenClawAgent, error) {
	return s.agentsSvc.ListAgents()
}

// CreateChannelInput for OpenClaw channel creation.
type CreateChannelInput struct {
	Name        string `json:"name"`
	Avatar      string `json:"avatar"`
	ExtraConfig string `json:"extra_config"`
	// AgentID is optional: when 0, the channel is created unbound; bind later via BindAgent.
	AgentID int64 `json:"agent_id"`
}

// appCredentialsJSON is used to parse/build extra_config.
type appCredentialsJSON struct {
	AppID             string `json:"app_id"`
	AppSecret         string `json:"app_secret"`
	OpenClawChannelID string `json:"openclaw_channel_id,omitempty"`
	StreamOutput      *bool  `json:"stream_output_enabled,omitempty"`
}

func parseAppCredentials(extraConfig string) (appID string) {
	extraConfig = strings.TrimSpace(extraConfig)
	if extraConfig == "" {
		return ""
	}
	var cfg appCredentialsJSON
	if err := json.Unmarshal([]byte(extraConfig), &cfg); err != nil {
		return ""
	}
	return strings.TrimSpace(cfg.AppID)
}

func parseAppCredentialsPair(extraConfig string) (appID, appSecret string) {
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

func withOpenClawChannelID(extraConfig, openClawChannelID string) (string, error) {
	extraConfig = strings.TrimSpace(extraConfig)
	if extraConfig == "" {
		return "", nil
	}
	var cfg appCredentialsJSON
	if err := json.Unmarshal([]byte(extraConfig), &cfg); err != nil {
		return "", err
	}
	cfg.OpenClawChannelID = strings.TrimSpace(openClawChannelID)
	raw, err := json.Marshal(cfg)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func extractOpenClawChannelID(extraConfig string) string {
	extraConfig = strings.TrimSpace(extraConfig)
	if extraConfig == "" {
		return ""
	}
	var cfg appCredentialsJSON
	if err := json.Unmarshal([]byte(extraConfig), &cfg); err != nil {
		return ""
	}
	return strings.TrimSpace(cfg.OpenClawChannelID)
}

func (s *OpenClawChannelService) ensureOpenClawReady() error {
	if s.openclawManager == nil || !s.openclawManager.IsReady() {
		return errs.New("error.openclaw_gateway_not_ready_channel")
	}
	return nil
}

func (s *OpenClawChannelService) getChannelModel(ctx context.Context, id int64) (*channelModel, error) {
	db, err := s.db()
	if err != nil {
		return nil, err
	}
	var m channelModel
	if err := db.NewSelect().Model(&m).Where("id = ?", id).Limit(1).Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.Newf("error.channel_not_found", map[string]any{"ID": id})
		}
		return nil, errs.Wrap("error.channel_read_failed", err)
	}
	return &m, nil
}

func (s *OpenClawChannelService) createOpenClawFeishuChannel(ctx context.Context, name, extraConfig string) (string, error) {
	params := s.buildOpenClawChannelParams(name, extraConfig)
	var resp map[string]any
	if err := s.requestOpenClaw(ctx, []string{"channels.create", "channel.create"}, params, &resp); err != nil {
		if errors.Is(err, errOpenClawChannelRPCUnavailable) {
			s.app.Logger.Warn("openclaw channel rpc unavailable, skipping remote create and using local-only channel", "error", err)
			return "", nil
		}
		return "", err
	}
	channelID := firstNonEmpty(
		nestedString(resp, "channel", "id"),
		nestedString(resp, "channel", "channelId"),
		stringVal(resp["id"]),
		stringVal(resp["channelId"]),
	)
	if channelID == "" {
		return "", fmt.Errorf("openclaw channels.create returned empty channel id")
	}
	return channelID, nil
}

func (s *OpenClawChannelService) updateOpenClawFeishuChannel(ctx context.Context, openClawChannelID, name, extraConfig string) error {
	params := s.buildOpenClawChannelParams(name, extraConfig)
	params["channelId"] = openClawChannelID
	params["id"] = openClawChannelID
	if err := s.requestOpenClaw(ctx, []string{"channels.update", "channel.update"}, params, nil); err != nil {
		if errors.Is(err, errOpenClawChannelRPCUnavailable) {
			s.app.Logger.Warn("openclaw channel rpc unavailable, skipping remote update and keeping local-only channel", "channel_id", openClawChannelID, "error", err)
			return errOpenClawChannelRPCUnavailable
		}
		return err
	}
	return nil
}

func (s *OpenClawChannelService) deleteOpenClawChannel(ctx context.Context, openClawChannelID string) error {
	params := map[string]any{
		"channelId": openClawChannelID,
		"id":        openClawChannelID,
	}
	if err := s.requestOpenClaw(ctx, []string{"channels.delete", "channel.delete"}, params, nil); err != nil {
		if errors.Is(err, errOpenClawChannelRPCUnavailable) {
			s.app.Logger.Warn("openclaw channel rpc unavailable, skipping remote delete and deleting local-only channel", "channel_id", openClawChannelID, "error", err)
			return errOpenClawChannelRPCUnavailable
		}
		return err
	}
	return nil
}

func (s *OpenClawChannelService) setOpenClawChannelEnabled(ctx context.Context, openClawChannelID string, enabled bool) error {
	if enabled {
		if err := s.requestOpenClaw(ctx, []string{"channels.enable", "channel.enable"}, map[string]any{
			"channelId": openClawChannelID,
			"id":        openClawChannelID,
		}, nil); err == nil {
			return nil
		}
	}
	if !enabled {
		if err := s.requestOpenClaw(ctx, []string{"channels.disable", "channel.disable"}, map[string]any{
			"channelId": openClawChannelID,
			"id":        openClawChannelID,
		}, nil); err == nil {
			return nil
		}
	}
	err := s.requestOpenClaw(ctx, []string{"channels.update", "channel.update"}, map[string]any{
		"channelId": openClawChannelID,
		"id":        openClawChannelID,
		"enabled":   enabled,
	}, nil)
	if err != nil && errors.Is(err, errOpenClawChannelRPCUnavailable) {
		s.app.Logger.Warn("openclaw channel rpc unavailable, skipping remote enable/disable and using local gateway state only", "channel_id", openClawChannelID, "enabled", enabled, "error", err)
		return errOpenClawChannelRPCUnavailable
	}
	return err
}

func (s *OpenClawChannelService) syncOpenClawFeishuChannelsConfig(ctx context.Context) error {
	channelList, err := s.ListAllFeishuChannels()
	if err != nil {
		return err
	}

	accounts := map[string]any{}
	defaultAccount := ""
	defaultAppID := ""
	defaultSecret := ""
	enabled := false

	for _, ch := range channelList {
		appID, appSecret := parseAppCredentialsPair(ch.ExtraConfig)
		if appID == "" && appSecret == "" {
			continue
		}
		accountKey := fmt.Sprintf("channel_%d", ch.ID)
		accounts[accountKey] = map[string]any{
			"appId": appID,
			"appSecret": map[string]any{
				"value": appSecret,
			},
		}
		if defaultAccount == "" || ch.Enabled {
			defaultAccount = accountKey
			defaultAppID = appID
			defaultSecret = appSecret
		}
		if ch.Enabled {
			enabled = true
		}
	}

	raw, err := json.Marshal(map[string]any{
		"channels": map[string]any{
			"feishu": map[string]any{
				"accounts":       accounts,
				"defaultAccount": defaultAccount,
				"enabled":        enabled && defaultAccount != "",
				"appId":          defaultAppID,
				"appSecret": map[string]any{
					"value": defaultSecret,
				},
			},
		},
	})
	if err != nil {
		return err
	}

	var getResult struct {
		Hash string `json:"hash"`
	}
	if err := s.openclawManager.Request(ctx, "config.get", map[string]any{}, &getResult); err != nil {
		return err
	}
	if err := s.openclawManager.Request(ctx, "config.patch", map[string]any{
		"raw":      string(raw),
		"baseHash": getResult.Hash,
	}, nil); err != nil {
		return err
	}
	return nil
}

func (s *OpenClawChannelService) requestOpenClaw(ctx context.Context, methods []string, params any, out any) error {
	var lastErr error
	allMethodNotFound := true
	for _, method := range methods {
		err := s.openclawManager.Request(ctx, method, params, out)
		if err == nil {
			return nil
		}
		lastErr = err
		if !isMethodNotFound(err) {
			allMethodNotFound = false
			return err
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no openclaw method provided")
	}
	if allMethodNotFound {
		return fmt.Errorf("%w: %v", errOpenClawChannelRPCUnavailable, lastErr)
	}
	return lastErr
}

func isMethodNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "method") &&
		(strings.Contains(msg, "not found") || strings.Contains(msg, "unknown"))
}

func (s *OpenClawChannelService) buildOpenClawChannelParams(name, extraConfig string) map[string]any {
	name = strings.TrimSpace(name)
	extraConfig = strings.TrimSpace(extraConfig)
	cfg := map[string]any{}
	if extraConfig != "" {
		var raw map[string]any
		if json.Unmarshal([]byte(extraConfig), &raw) == nil {
			cfg = raw
		}
	}
	appID := strings.TrimSpace(stringVal(cfg["app_id"]))
	appSecret := strings.TrimSpace(stringVal(cfg["app_secret"]))
	streamEnabled, _ := cfg["stream_output_enabled"].(bool)

	return map[string]any{
		"platform": channels.PlatformFeishu,
		// Keep plugin/type as feishu for backward compatibility with older gateways.
		"type": channels.PlatformFeishu,
		// Newer OpenClaw gateways classify IM channels by channelType (session|...).
		// Using "session" prevents Feishu session runs from being tagged as unknown.
		"channelType": "session",
		"name":        name,
		"enabled":     false,
		"config": map[string]any{
			"app_id":                appID,
			"app_secret":            appSecret,
			"appId":                 appID,
			"appSecret":             appSecret,
			"stream_output_enabled": streamEnabled,
			"streamOutputEnabled":   streamEnabled,
		},
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func nestedString(m map[string]any, key, child string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	cm, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	return stringVal(cm[child])
}

func stringVal(v any) string {
	switch vv := v.(type) {
	case string:
		return vv
	case fmt.Stringer:
		return vv.String()
	default:
		return ""
	}
}
