package openclawchannels

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"chatclaw/internal/define"
	"chatclaw/internal/errs"
	openclawagents "chatclaw/internal/openclaw/agents"
	openclawruntime "chatclaw/internal/openclaw/runtime"
	"chatclaw/internal/services/channels"
	"chatclaw/internal/services/conversations"
	"chatclaw/internal/sqlite"

	"github.com/uptrace/bun"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// OpenClawChannelService provides OpenClaw channel management for OpenClaw agents.
// It delegates to the shared channels infrastructure while filtering by OpenClaw agents.
type OpenClawChannelService struct {
	app             *application.App
	gateway         *channels.Gateway
	agentsSvc       *openclawagents.OpenClawAgentsService
	channelSvc      *channels.ChannelService
	convSvc         *conversations.ConversationsService
	openclawManager *openclawruntime.Manager
}

const wecomPluginPackage = "@wecom/wecom-openclaw-plugin"
const wecomPluginID = "wecom-openclaw-plugin"

// Timeouts for OpenClaw CLI (config set), plugin install/list, and gateway restart.
const (
	openClawChannelSyncTimeout = 120 * time.Second
	openClawCLIRetryTimeout  = 90 * time.Second
)

func NewOpenClawChannelService(
	app *application.App,
	gw *channels.Gateway,
	agentsSvc *openclawagents.OpenClawAgentsService,
	channelSvc *channels.ChannelService,
	convSvc *conversations.ConversationsService,
	openclawManager *openclawruntime.Manager,
) *OpenClawChannelService {
	return &OpenClawChannelService{
		app:             app,
		gateway:         gw,
		agentsSvc:       agentsSvc,
		channelSvc:      channelSvc,
		convSvc:         convSvc,
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
		if s.gateway.IsConnected(ch.ID) || s.isPluginManagedChannelOnline(ch) {
			ch.Status = channels.StatusOnline
		}
		out = append(out, ch)
	}
	return out, nil
}

// ListAllFeishuChannels returns all Feishu channels (including unbound ones)
// for the "add existing bot" workflow.
func (s *OpenClawChannelService) ListAllFeishuChannels() ([]channels.Channel, error) {
	return s.listAllChannelsByPlatform(channels.PlatformFeishu)
}

func (s *OpenClawChannelService) listAllChannelsByPlatform(platform string) ([]channels.Channel, error) {
	db, err := s.db()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var models []channelModel
	q := db.NewSelect().
		Model(&models).
		Where("ch.platform = ?", platform).
		Where(openClawChannelVisibilitySQL).
		OrderExpr("ch.id DESC")
	if err := q.Scan(ctx); err != nil {
		return nil, errs.Wrap("error.channel_list_failed", err)
	}

	out := make([]channels.Channel, 0, len(models))
	for i := range models {
		ch := models[i].toDTO()
		if s.gateway.IsConnected(ch.ID) || s.isPluginManagedChannelOnline(ch) {
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
// OpenClaw currently supports creating Feishu and WeCom channels directly.
func (s *OpenClawChannelService) GetSupportedPlatforms() []channels.PlatformMeta {
	return []channels.PlatformMeta{
		{ID: channels.PlatformDingTalk, Name: "DingTalk", AuthType: "token"},
		{ID: channels.PlatformFeishu, Name: "Feishu", AuthType: "token"},
		{ID: channels.PlatformWeCom, Name: "WeCom", AuthType: "token"},
		{ID: channels.PlatformQQ, Name: "QQ", AuthType: "token"},
		{ID: channels.PlatformTwitter, Name: "X (Twitter)", AuthType: "token"},
	}
}

// CreateChannel creates a new OpenClaw channel. When agent_id > 0, binds that OpenClaw agent;
// when agent_id is 0, creates an unbound channel (UI binds via BindAgent or auto-generate later).
func (s *OpenClawChannelService) CreateChannel(input CreateChannelInput) (*channels.Channel, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, errs.New("error.channel_name_required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), openClawChannelSyncTimeout)
	defer cancel()
	if err := s.ensureOpenClawReady(); err != nil {
		return nil, err
	}
	platform := openClawPlatformFromInput(input.Platform, input.ExtraConfig)

	// Create local DB row first to obtain a stable channel ID for the account key.
	ch, err := s.channelSvc.CreateChannel(channels.CreateChannelInput{
		Platform:       platform,
		Name:           name,
		Avatar:         input.Avatar,
		ConnectionType: channels.ConnTypeGateway,
		ExtraConfig:    input.ExtraConfig,
		OpenClawScope:  true,
	})
	if err != nil {
		return nil, err
	}

	accountKey := openClawChannelAccountKey(ch.ID, input.ExtraConfig)
	extraConfigWithID, err := withOpenClawChannelID(input.ExtraConfig, accountKey)
	if err != nil {
		_ = s.channelSvc.DeleteChannel(ch.ID)
		return nil, errs.Wrap("error.channel_create_failed", err)
	}
	if _, err := s.channelSvc.UpdateChannel(ch.ID, channels.UpdateChannelInput{ExtraConfig: &extraConfigWithID}); err != nil {
		_ = s.channelSvc.DeleteChannel(ch.ID)
		return nil, errs.Wrap("error.channel_create_failed", err)
	}
	ch.ExtraConfig = extraConfigWithID

	if err := s.syncOpenClawChannelConfig(ctx, platform, accountKey, name, input.ExtraConfig, false); err != nil {
		_ = s.channelSvc.DeleteChannel(ch.ID)
		return nil, wrapOpenClawSyncErr(err, "error.channel_create_failed", nil)
	}

	if input.AgentID > 0 {
		if err := s.channelSvc.BindAgent(ch.ID, input.AgentID); err != nil {
			return nil, err
		}
		ch.AgentID = input.AgentID
		if err := s.syncChannelRoutingBinding(ch.ID, input.AgentID); err != nil {
			return nil, wrapOpenClawSyncErr(err, "error.channel_create_failed", nil)
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

	ctx, cancel := context.WithTimeout(context.Background(), openClawChannelSyncTimeout)
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

	accountKey := openClawChannelAccountKey(id, extraConfig)
	platform := strings.TrimSpace(m.Platform)
	if platform == "" {
		platform = channels.PlatformFeishu
	}

	enabled := m.Enabled
	if input.Enabled != nil {
		enabled = *input.Enabled
	}

	if err := s.syncOpenClawChannelConfig(ctx, platform, accountKey, name, extraConfig, enabled); err != nil {
		return nil, wrapOpenClawSyncErr(err, "error.channel_update_failed", nil)
	}

	extraConfigWithID, err := withOpenClawChannelID(extraConfig, accountKey)
	if err != nil {
		return nil, errs.Wrap("error.channel_update_failed", err)
	}
	localInput := input
	localInput.ExtraConfig = &extraConfigWithID
	updated, err := s.channelSvc.UpdateChannel(id, localInput)
	if err != nil {
		return nil, err
	}
	return updated, nil
}

// DeleteChannel delegates to the shared ChannelService.
func (s *OpenClawChannelService) DeleteChannel(id int64) error {
	if id <= 0 {
		return errs.New("error.channel_id_required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), openClawChannelSyncTimeout)
	defer cancel()
	m, err := s.getChannelModel(ctx, id)
	if err != nil {
		return err
	}

	accountKey := openClawChannelAccountKey(id, m.ExtraConfig)
	platform := strings.TrimSpace(m.Platform)
	if platform == "" {
		platform = channels.PlatformFeishu
	}
	if err := s.ensureOpenClawReady(); err != nil {
		return err
	}
	if err := s.removeOpenClawChannelConfig(ctx, platform, accountKey); err != nil {
		s.app.Logger.Warn("openclaw config unset channel failed, proceeding with local delete", "platform", platform, "account", accountKey, "error", err)
	}

	if err := s.channelSvc.DeleteChannel(id); err != nil {
		return err
	}

	if platform == channels.PlatformWeCom {
		if err := s.removeManagedRouteBinding(platform, id); err != nil {
			s.app.Logger.Warn("openclaw wecom route binding cleanup failed after delete", "channel_id", id, "error", err)
		}
	}
	if err := s.syncOpenClawPlatformConfigAfterDelete(ctx, platform); err != nil {
		s.app.Logger.Warn("openclaw channel default config sync failed after delete", "platform", platform, "error", err)
	}
	return nil
}

// BindAgent delegates to the shared ChannelService.
func (s *OpenClawChannelService) BindAgent(id int64, agentID int64) error {
	if err := s.channelSvc.BindAgent(id, agentID); err != nil {
		return err
	}
	return s.syncChannelRoutingBinding(id, agentID)
}

// UnbindAgent delegates to the shared ChannelService.
func (s *OpenClawChannelService) UnbindAgent(id int64) error {
	if id <= 0 {
		return errs.New("error.channel_id_required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	m, err := s.getChannelModel(ctx, id)
	if err != nil {
		return err
	}
	if err := s.channelSvc.UnbindAgent(id); err != nil {
		return err
	}
	if strings.TrimSpace(m.Platform) == channels.PlatformWeCom && m.OpenClawScope {
		if err := s.removeManagedRouteBinding(channels.PlatformWeCom, id); err != nil {
			return errs.Wrap("error.channel_bind_failed", err)
		}
		if err := s.restartOpenClawGateway(); err != nil {
			return errs.Wrap("error.channel_bind_failed", err)
		}
	}
	return nil
}

// ConnectChannel enables the OpenClaw plugin-managed channel.
func (s *OpenClawChannelService) ConnectChannel(id int64) error {
	if id <= 0 {
		return errs.New("error.channel_id_required")
	}
	if err := s.ensureOpenClawReady(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), openClawChannelSyncTimeout)
	defer cancel()
	m, err := s.getChannelModel(ctx, id)
	if err != nil {
		return err
	}

	accountKey := openClawChannelAccountKey(id, m.ExtraConfig)
	platform := strings.TrimSpace(m.Platform)
	if platform == "" {
		platform = channels.PlatformFeishu
	}
	if err := s.syncOpenClawChannelConfig(ctx, platform, accountKey, m.Name, m.ExtraConfig, true); err != nil {
		return wrapOpenClawSyncErr(err, "error.channel_connect_failed", map[string]any{"Name": m.Name})
	}

	extraConfigWithID, encodeErr := withOpenClawChannelID(m.ExtraConfig, accountKey)
	if encodeErr == nil {
		if _, updateErr := s.channelSvc.UpdateChannel(id, channels.UpdateChannelInput{ExtraConfig: &extraConfigWithID}); updateErr != nil {
			return updateErr
		}
	}

	if platform == channels.PlatformWeCom {
		if m.AgentID == 0 {
			return errs.New("error.channel_connect_requires_agent")
		}
		if err := s.syncChannelRoutingBinding(id, m.AgentID); err != nil {
			return wrapOpenClawSyncErr(err, "error.channel_connect_failed", map[string]any{"Name": m.Name})
		}
		_ = s.gateway.DisconnectChannel(context.Background(), id)
		enabled := true
		_, err = s.channelSvc.UpdateChannel(id, channels.UpdateChannelInput{Enabled: &enabled})
		return err
	}

	return s.channelSvc.ConnectChannel(id)
}

// DisconnectChannel disables the OpenClaw plugin-managed channel.
func (s *OpenClawChannelService) DisconnectChannel(id int64) error {
	if id <= 0 {
		return errs.New("error.channel_id_required")
	}
	if err := s.ensureOpenClawReady(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), openClawChannelSyncTimeout)
	defer cancel()
	m, err := s.getChannelModel(ctx, id)
	if err != nil {
		return err
	}

	accountKey := openClawChannelAccountKey(id, m.ExtraConfig)
	platform := strings.TrimSpace(m.Platform)
	if platform == "" {
		platform = channels.PlatformFeishu
	}
	if err := s.syncOpenClawChannelConfig(ctx, platform, accountKey, m.Name, m.ExtraConfig, false); err != nil {
		return wrapOpenClawSyncErr(err, "error.channel_disconnect_failed", nil)
	}

	if platform == channels.PlatformWeCom {
		_ = s.gateway.DisconnectChannel(context.Background(), id)
		enabled := false
		_, err = s.channelSvc.UpdateChannel(id, channels.UpdateChannelInput{Enabled: &enabled})
		return err
	}

	return s.channelSvc.DisconnectChannel(id)
}

func (s *OpenClawChannelService) isPluginManagedChannelOnline(ch channels.Channel) bool {
	if strings.TrimSpace(ch.Platform) != channels.PlatformWeCom {
		return false
	}
	if !ch.OpenClawScope || !ch.Enabled {
		return false
	}
	return s.openclawManager != nil && s.openclawManager.IsReady()
}

func (s *OpenClawChannelService) syncChannelRoutingBinding(channelID int64, agentID int64) error {
	if channelID <= 0 || agentID <= 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	m, err := s.getChannelModel(ctx, channelID)
	if err != nil {
		return err
	}
	if !m.OpenClawScope || strings.TrimSpace(m.Platform) != channels.PlatformWeCom {
		return nil
	}
	agent, err := s.agentsSvc.GetAgent(agentID)
	if err != nil {
		return err
	}
	if err := s.upsertManagedRouteBinding(channels.PlatformWeCom, channelID, strings.TrimSpace(agent.OpenClawAgentID)); err != nil {
		return err
	}
	return s.restartOpenClawGateway()
}

func (s *OpenClawChannelService) upsertManagedRouteBinding(platform string, channelID int64, openclawAgentID string) error {
	cfg, configPath, err := loadOpenClawJSONConfig()
	if err != nil {
		return err
	}
	bindings := configBindings(cfg)
	bindings = removeManagedBindings(bindings, platform, channelID)
	bindings = append([]any{map[string]any{
		"type":    "route",
		"agentId": strings.TrimSpace(openclawAgentID),
		"match": map[string]any{
			"channel":   strings.TrimSpace(platform),
			"accountId": "default",
		},
	}}, bindings...)
	cfg["bindings"] = bindings
	ensurePerChannelPeerDMScope(cfg)
	return saveOpenClawJSONConfig(configPath, cfg)
}

func (s *OpenClawChannelService) removeManagedRouteBinding(platform string, channelID int64) error {
	cfg, configPath, err := loadOpenClawJSONConfig()
	if err != nil {
		return err
	}
	cfg["bindings"] = removeManagedBindings(configBindings(cfg), platform, channelID)
	return saveOpenClawJSONConfig(configPath, cfg)
}

func loadOpenClawJSONConfig() (map[string]any, string, error) {
	root, err := define.OpenClawDataRootDir()
	if err != nil {
		return nil, "", err
	}
	configPath := filepath.Join(root, "openclaw.json")
	raw, err := os.ReadFile(configPath)
	if err != nil {
		return nil, "", err
	}
	cfg := make(map[string]any)
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return nil, "", fmt.Errorf("parse openclaw config: %w", err)
		}
	}
	return cfg, configPath, nil
}

func saveOpenClawJSONConfig(configPath string, cfg map[string]any) error {
	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal openclaw config: %w", err)
	}
	if err := os.WriteFile(configPath, out, 0o644); err != nil {
		return fmt.Errorf("write openclaw config: %w", err)
	}
	return nil
}

func configBindings(cfg map[string]any) []any {
	bindings, _ := cfg["bindings"].([]any)
	if bindings == nil {
		return []any{}
	}
	return append([]any(nil), bindings...)
}

func removeManagedBindings(bindings []any, platform string, channelID int64) []any {
	if len(bindings) == 0 {
		return bindings
	}
	out := make([]any, 0, len(bindings))
	for _, item := range bindings {
		entry, _ := item.(map[string]any)
		if !isManagedChannelBinding(entry, platform, channelID) {
			out = append(out, item)
		}
	}
	return out
}

func isManagedChannelBinding(entry map[string]any, platform string, channelID int64) bool {
	if entry == nil {
		return false
	}
	match, _ := entry["match"].(map[string]any)
	if match == nil {
		return false
	}
	if bindingType, _ := entry["type"].(string); strings.TrimSpace(bindingType) == "acp" {
		return false
	}
	channelValue, _ := match["channel"].(string)
	accountValue, _ := match["accountId"].(string)
	if strings.TrimSpace(channelValue) != strings.TrimSpace(platform) {
		return false
	}
	if strings.TrimSpace(accountValue) != "default" {
		return false
	}
	if _, ok := match["peer"]; ok {
		return false
	}
	if _, ok := match["guildId"]; ok {
		return false
	}
	if _, ok := match["teamId"]; ok {
		return false
	}
	if roles, ok := match["roles"].([]any); ok && len(roles) > 0 {
		return false
	}
	_ = channelID
	return true
}

func ensurePerChannelPeerDMScope(cfg map[string]any) {
	session, _ := cfg["session"].(map[string]any)
	if session == nil {
		session = map[string]any{}
		cfg["session"] = session
	}
	scope, _ := session["dmScope"].(string)
	if strings.TrimSpace(scope) == "" || strings.EqualFold(strings.TrimSpace(scope), "main") {
		session["dmScope"] = "per-channel-peer"
	}
}

// VerifyChannelConfig verifies credentials for Feishu/WeCom based on config payload.
func (s *OpenClawChannelService) VerifyChannelConfig(extraConfig string) error {
	platform := openClawPlatformFromInput("", extraConfig)
	if platform == channels.PlatformWeCom {
		ctx, cancel := context.WithTimeout(context.Background(), openClawChannelSyncTimeout)
		defer cancel()
		if err := s.ensureOpenClawWeComPluginInstalled(ctx); err != nil {
			return errs.Wrap("error.channel_verify_failed", err)
		}
	}
	return s.channelSvc.VerifyChannelConfig(platform, extraConfig)
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
	Platform    string `json:"platform,omitempty"`
	Name        string `json:"name"`
	Avatar      string `json:"avatar"`
	ExtraConfig string `json:"extra_config"`
	// AgentID is optional: when 0, the channel is created unbound; bind later via BindAgent.
	AgentID int64 `json:"agent_id"`
}

// appCredentialsJSON is used to parse/build extra_config.
type appCredentialsJSON struct {
	Platform          string `json:"platform,omitempty"`
	AppID             string `json:"app_id"`
	AppSecret         string `json:"app_secret"`
	OpenClawChannelID string `json:"openclaw_channel_id,omitempty"`
	StreamOutput      *bool  `json:"stream_output_enabled,omitempty"`
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

func parseOpenClawPlatform(extraConfig string) string {
	extraConfig = strings.TrimSpace(extraConfig)
	if extraConfig == "" {
		return ""
	}
	var cfg appCredentialsJSON
	if err := json.Unmarshal([]byte(extraConfig), &cfg); err != nil {
		return ""
	}
	return strings.TrimSpace(cfg.Platform)
}

func openClawPlatformFromInput(explicitPlatform, extraConfig string) string {
	platform := strings.ToLower(strings.TrimSpace(explicitPlatform))
	if platform == "" {
		platform = strings.ToLower(parseOpenClawPlatform(extraConfig))
	}
	switch platform {
	case channels.PlatformWeCom:
		return channels.PlatformWeCom
	case channels.PlatformFeishu:
		return channels.PlatformFeishu
	default:
		return channels.PlatformFeishu
	}
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

// openClawChannelAccountKey returns the OpenClaw config account key for a channel row.
// The key is stored in extra_config.openclaw_channel_id when available, otherwise
// derived from the local DB channel ID.
func openClawChannelAccountKey(channelID int64, extraConfig string) string {
	if id := extractOpenClawChannelID(extraConfig); id != "" {
		return id
	}
	return fmt.Sprintf("channel_%d", channelID)
}

func (s *OpenClawChannelService) syncOpenClawChannelConfig(ctx context.Context, platform, accountKey, name, extraConfig string, enabled bool) error {
	switch platform {
	case channels.PlatformWeCom:
		if err := s.ensureOpenClawWeComPluginInstalled(ctx); err != nil {
			return err
		}
		if err := s.setOpenClawWeComConfig(ctx, name, extraConfig, enabled); err != nil {
			return err
		}
		if err := s.restartOpenClawGateway(); err != nil {
			return err
		}
		return nil
	default:
		return s.setOpenClawFeishuAccount(ctx, accountKey, name, extraConfig, enabled)
	}
}

func (s *OpenClawChannelService) removeOpenClawChannelConfig(ctx context.Context, platform, accountKey string) error {
	switch platform {
	case channels.PlatformWeCom:
		if err := s.ensureOpenClawWeComPluginInstalled(ctx); err != nil {
			return err
		}
		if err := s.removeOpenClawWeComConfig(ctx); err != nil {
			return err
		}
		if err := s.restartOpenClawGateway(); err != nil {
			return err
		}
		return nil
	default:
		return s.removeOpenClawFeishuAccount(ctx, accountKey)
	}
}

func (s *OpenClawChannelService) syncOpenClawPlatformConfigAfterDelete(ctx context.Context, platform string) error {
	switch platform {
	case channels.PlatformWeCom:
		if err := s.ensureOpenClawWeComPluginInstalled(ctx); err != nil {
			return err
		}
		if err := s.syncOpenClawWeComDefaultConfig(ctx); err != nil {
			return err
		}
		return s.restartOpenClawGateway()
	default:
		return s.syncOpenClawFeishuDefaultAccount(ctx)
	}
}

// setOpenClawFeishuAccount writes a feishu account into the OpenClaw config via CLI.
// It uses `openclaw config set --batch-json` to atomically set appId, appSecret,
// name and enabled in one call. The gateway file watcher hot-applies channel changes
// without restart.
func (s *OpenClawChannelService) setOpenClawFeishuAccount(ctx context.Context, accountKey, name, extraConfig string, enabled bool) error {
	appID, appSecret := parseAppCredentialsPair(extraConfig)
	if appID == "" {
		return fmt.Errorf("feishu appId is required")
	}

	prefix := "channels.feishu.accounts." + accountKey

	type batchEntry struct {
		Path  string `json:"path"`
		Value any    `json:"value"`
	}
	batch := []batchEntry{
		{Path: prefix + ".appId", Value: appID},
		{Path: prefix + ".appSecret", Value: appSecret},
	}
	if name = strings.TrimSpace(name); name != "" {
		batch = append(batch, batchEntry{Path: prefix + ".name", Value: name})
	}
	batch = append(batch,
		batchEntry{Path: "channels.feishu.enabled", Value: enabled},
		batchEntry{Path: "channels.feishu.defaultAccount", Value: accountKey},
		batchEntry{Path: "channels.feishu.dmPolicy", Value: "open"},
		batchEntry{Path: "channels.feishu.allowFrom", Value: []string{"*"}},
	)

	batchJSON, err := json.Marshal(batch)
	if err != nil {
		return fmt.Errorf("marshal config batch: %w", err)
	}
	_, err = s.execOpenClawCLIWithRetry(ctx, "config", "set", "--batch-json", string(batchJSON))
	if err != nil {
		return fmt.Errorf("openclaw config set feishu account %s: %w", accountKey, err)
	}
	return nil
}

func (s *OpenClawChannelService) ensureOpenClawWeComPluginInstalled(ctx context.Context) error {
	out, err := s.execOpenClawPluginCLI(ctx, "plugins", "list")
	if err == nil && containsWeComPluginMarker(string(out)) {
		return nil
	}
	if _, installErr := s.execOpenClawPluginCLI(ctx, "plugins", "install", wecomPluginPackage); installErr != nil {
		installMsg := strings.ToLower(installErr.Error())
		installedButInterrupted := strings.Contains(installMsg, "installed plugin:") && containsWeComPluginMarker(installMsg)
		if installedButInterrupted {
			s.app.Logger.Warn("openclaw plugin install interrupted after success marker; will verify by listing plugins", "plugin", wecomPluginPackage, "error", installErr)
		}
		if !strings.Contains(installMsg, "plugin already exists") && !installedButInterrupted {
			return fmt.Errorf("openclaw plugins install %s: %w", wecomPluginPackage, installErr)
		}
	}
	verifyOut, verifyErr := s.execOpenClawPluginCLI(ctx, "plugins", "list")
	if verifyErr != nil {
		return fmt.Errorf("verify installed plugin %s: %w", wecomPluginPackage, verifyErr)
	}
	if !containsWeComPluginMarker(string(verifyOut)) {
		return fmt.Errorf("plugin %s not found after installation", wecomPluginPackage)
	}
	return nil
}

func containsWeComPluginMarker(out string) bool {
	out = strings.ToLower(out)
	return strings.Contains(out, strings.ToLower(wecomPluginPackage)) || strings.Contains(out, strings.ToLower(wecomPluginID))
}

// execOpenClawPluginCLI uses the same retry strategy as other OpenClaw CLI calls (install/list can be slow).
func (s *OpenClawChannelService) execOpenClawPluginCLI(ctx context.Context, args ...string) ([]byte, error) {
	return s.execOpenClawCLIWithRetry(ctx, args...)
}

func isContextDeadlineExceededError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "context deadline exceeded")
}

// wrapOpenClawSyncErr maps CLI/plugin timeout to a dedicated i18n message; other errors use key (optional data for Wrapf).
func wrapOpenClawSyncErr(err error, key string, data map[string]any) error {
	if err == nil {
		return nil
	}
	if isContextDeadlineExceededError(err) {
		return errs.Wrap("error.channel_openclaw_sync_timeout", err)
	}
	if data != nil {
		return errs.Wrapf(key, err, data)
	}
	return errs.Wrap(key, err)
}

// execOpenClawCLIWithRetry runs the OpenClaw CLI; on deadline exceeded retries once with a longer timeout (same as plugin install).
func (s *OpenClawChannelService) execOpenClawCLIWithRetry(ctx context.Context, args ...string) ([]byte, error) {
	out, err := s.openclawManager.ExecCLI(ctx, args...)
	if err == nil {
		return out, nil
	}
	if !isContextDeadlineExceededError(err) {
		return out, err
	}
	retryCtx, cancel := context.WithTimeout(context.Background(), openClawCLIRetryTimeout)
	defer cancel()
	s.app.Logger.Warn("openclaw CLI timed out with caller context; retrying with longer timeout",
		"args", strings.Join(args, " "), "error", err)
	return s.openclawManager.ExecCLI(retryCtx, args...)
}

func (s *OpenClawChannelService) setOpenClawWeComConfig(ctx context.Context, name, extraConfig string, enabled bool) error {
	botID, secret := parseAppCredentialsPair(extraConfig)
	if botID == "" {
		return fmt.Errorf("wecom botId is required")
	}

	type batchEntry struct {
		Path  string `json:"path"`
		Value any    `json:"value"`
	}
	batch := []batchEntry{
		{Path: "channels.wecom.botId", Value: botID},
		{Path: "channels.wecom.secret", Value: secret},
		{Path: "channels.wecom.enabled", Value: enabled},
		{Path: "channels.wecom.dmPolicy", Value: "open"},
		{Path: "channels.wecom.groupPolicy", Value: "open"},
	}
	if strings.TrimSpace(name) != "" {
		batch = append(batch, batchEntry{Path: "channels.wecom.name", Value: strings.TrimSpace(name)})
	}

	batchJSON, err := json.Marshal(batch)
	if err != nil {
		return fmt.Errorf("marshal wecom config batch: %w", err)
	}
	if _, err := s.execOpenClawCLIWithRetry(ctx, "config", "set", "--batch-json", string(batchJSON)); err != nil {
		return fmt.Errorf("openclaw config set wecom account: %w", err)
	}
	return nil
}

func (s *OpenClawChannelService) removeOpenClawWeComConfig(ctx context.Context) error {
	type batchEntry struct {
		Path  string `json:"path"`
		Value any    `json:"value"`
	}
	batch := []batchEntry{
		{Path: "channels.wecom.enabled", Value: false},
		{Path: "channels.wecom.dmPolicy", Value: "open"},
		{Path: "channels.wecom.groupPolicy", Value: "open"},
	}
	batchJSON, err := json.Marshal(batch)
	if err != nil {
		return fmt.Errorf("marshal wecom disable config batch: %w", err)
	}
	if _, err := s.execOpenClawCLIWithRetry(ctx, "config", "set", "--batch-json", string(batchJSON)); err != nil {
		return fmt.Errorf("openclaw config disable wecom channel: %w", err)
	}
	_, _ = s.execOpenClawCLIWithRetry(ctx, "config", "unset", "channels.wecom.botId")
	_, _ = s.execOpenClawCLIWithRetry(ctx, "config", "unset", "channels.wecom.secret")
	return nil
}

func (s *OpenClawChannelService) syncOpenClawWeComDefaultConfig(ctx context.Context) error {
	channelList, err := s.listAllChannelsByPlatform(channels.PlatformWeCom)
	if err != nil {
		return err
	}
	var selected *channels.Channel
	for i := range channelList {
		botID, _ := parseAppCredentialsPair(channelList[i].ExtraConfig)
		if botID == "" {
			continue
		}
		if selected == nil || channelList[i].Enabled {
			candidate := channelList[i]
			selected = &candidate
		}
	}
	if selected == nil {
		return s.removeOpenClawWeComConfig(ctx)
	}
	return s.setOpenClawWeComConfig(ctx, selected.Name, selected.ExtraConfig, selected.Enabled)
}

func (s *OpenClawChannelService) restartOpenClawGateway() error {
	if s.openclawManager == nil {
		return fmt.Errorf("openclaw manager is not initialized")
	}
	if _, err := s.openclawManager.RestartGateway(); err != nil {
		return fmt.Errorf("restart openclaw gateway: %w", err)
	}
	return nil
}

// removeOpenClawFeishuAccount removes a feishu account from the OpenClaw config.
func (s *OpenClawChannelService) removeOpenClawFeishuAccount(ctx context.Context, accountKey string) error {
	path := "channels.feishu.accounts." + accountKey
	_, err := s.execOpenClawCLIWithRetry(ctx, "config", "unset", path)
	if err != nil {
		return fmt.Errorf("openclaw config unset feishu account %s: %w", accountKey, err)
	}
	return nil
}

// setOpenClawFeishuEnabled toggles the channels.feishu.enabled flag.
func (s *OpenClawChannelService) setOpenClawFeishuEnabled(ctx context.Context, enabled bool) error {
	val := "false"
	if enabled {
		val = "true"
	}
	_, err := s.execOpenClawCLIWithRetry(ctx, "config", "set", "channels.feishu.enabled", val, "--strict-json")
	if err != nil {
		return fmt.Errorf("openclaw config set channels.feishu.enabled: %w", err)
	}
	return nil
}

// syncOpenClawFeishuDefaultAccount recalculates which feishu account should be
// the default and whether feishu should be enabled, then writes the result.
func (s *OpenClawChannelService) syncOpenClawFeishuDefaultAccount(ctx context.Context) error {
	channelList, err := s.ListAllFeishuChannels()
	if err != nil {
		return err
	}

	defaultAccount := ""
	anyEnabled := false
	for _, ch := range channelList {
		appID, _ := parseAppCredentialsPair(ch.ExtraConfig)
		if appID == "" {
			continue
		}
		key := openClawChannelAccountKey(ch.ID, ch.ExtraConfig)
		if defaultAccount == "" || ch.Enabled {
			defaultAccount = key
		}
		if ch.Enabled {
			anyEnabled = true
		}
	}

	type batchEntry struct {
		Path  string `json:"path"`
		Value any    `json:"value"`
	}
	batch := []batchEntry{
		{Path: "channels.feishu.enabled", Value: anyEnabled && defaultAccount != ""},
	}
	if defaultAccount != "" {
		batch = append(batch, batchEntry{Path: "channels.feishu.defaultAccount", Value: defaultAccount})
	}
	batchJSON, err := json.Marshal(batch)
	if err != nil {
		return err
	}
	_, err = s.execOpenClawCLIWithRetry(ctx, "config", "set", "--batch-json", string(batchJSON))
	return err
}
