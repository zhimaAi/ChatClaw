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
	"chatclaw/internal/services/channels"
	openclawagents "chatclaw/internal/openclaw/agents"
	openclawruntime "chatclaw/internal/openclaw/runtime"
	"chatclaw/internal/sqlite"

	"github.com/uptrace/bun"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// OpenClawChannelService provides channel management for OpenClaw (Feishu + DingTalk).
// It delegates to the shared channels infrastructure while filtering by OpenClaw agents.
type OpenClawChannelService struct {
	app             *application.App
	gateway         *channels.Gateway
	agentsSvc       *openclawagents.OpenClawAgentsService
	channelSvc      *channels.ChannelService
	openclawManager *openclawruntime.Manager
}

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

// dingTalkPluginName is the npm package name of the official DingTalk OpenClaw connector plugin.
const dingTalkPluginName = "@dingtalk-real-ai/dingtalk-connector"

// dingTalkPluginInstallTimeout is the maximum time allowed for plugin installation (npm download).
const dingTalkPluginInstallTimeout = 3 * time.Minute

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

// GetSupportedPlatforms returns the platform list for OpenClaw (Feishu + DingTalk available; others coming soon).
func (s *OpenClawChannelService) GetSupportedPlatforms() []channels.PlatformMeta {
	return []channels.PlatformMeta{
		{ID: channels.PlatformDingTalk, Name: "DingTalk", AuthType: "token"},
		{ID: channels.PlatformFeishu, Name: "Feishu", AuthType: "token"},
		{ID: channels.PlatformWeCom, Name: "WeCom", AuthType: "token"},
		{ID: channels.PlatformQQ, Name: "QQ", AuthType: "token"},
		{ID: channels.PlatformTwitter, Name: "X (Twitter)", AuthType: "token"},
	}
}

// CreateChannel creates a new channel (Feishu or DingTalk). When agent_id > 0, binds that OpenClaw agent;
// when agent_id is 0, creates an unbound channel (UI binds via BindAgent or auto-generate later).
func (s *OpenClawChannelService) CreateChannel(input CreateChannelInput) (*channels.Channel, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, errs.New("error.channel_name_required")
	}

	platform := strings.TrimSpace(input.Platform)
	if platform == "" {
		platform = channels.PlatformFeishu // legacy default
	}
	if platform != channels.PlatformFeishu && platform != channels.PlatformDingTalk {
		return nil, errs.Newf("error.channel_platform_unsupported", map[string]any{"Platform": platform})
	}

	if err := s.ensureOpenClawReady(); err != nil {
		return nil, err
	}

	// DingTalk requires the OpenClaw plugin to be installed — allow extra time for npm download.
	createTimeout := 15 * time.Second
	if platform == channels.PlatformDingTalk {
		createTimeout = dingTalkPluginInstallTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), createTimeout)
	defer cancel()

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

	accountKey := channelAccountKey(ch.ID, input.ExtraConfig)
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

	if platform == channels.PlatformDingTalk {
		// Ensure plugin installed, then write initial (disabled) config to OpenClaw.
		if err := s.ensureDingTalkPluginInstalled(ctx); err != nil {
			_ = s.channelSvc.DeleteChannel(ch.ID)
			return nil, errs.Wrap("error.channel_create_failed", err)
		}
		if err := s.setOpenClawDingTalkAccount(ctx, accountKey, name, input.ExtraConfig, false); err != nil {
			_ = s.channelSvc.DeleteChannel(ch.ID)
			return nil, errs.Wrap("error.channel_create_failed", err)
		}
	} else {
		if err := s.setOpenClawChannelAccount(ctx, platform, accountKey, name, input.ExtraConfig, false); err != nil {
			_ = s.channelSvc.DeleteChannel(ch.ID)
			return nil, errs.Wrap("error.channel_create_failed", err)
		}
	}

	if input.AgentID > 0 {
		if err := s.channelSvc.BindAgent(ch.ID, input.AgentID); err != nil {
			return nil, err
		}
		ch.AgentID = input.AgentID
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

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
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

	accountKey := channelAccountKey(id, extraConfig)

	enabled := m.Enabled
	if input.Enabled != nil {
		enabled = *input.Enabled
	}

	if m.Platform == channels.PlatformDingTalk {
		if err := s.setOpenClawDingTalkAccount(ctx, accountKey, name, extraConfig, enabled); err != nil {
			return nil, errs.Wrap("error.channel_update_failed", err)
		}
	} else {
		if err := s.setOpenClawChannelAccount(ctx, m.Platform, accountKey, name, extraConfig, enabled); err != nil {
			return nil, errs.Wrap("error.channel_update_failed", err)
		}
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
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	m, err := s.getChannelModel(ctx, id)
	if err != nil {
		return err
	}

	accountKey := channelAccountKey(id, m.ExtraConfig)
	if err := s.ensureOpenClawReady(); err != nil {
		return err
	}

	if m.Platform == channels.PlatformDingTalk {
		if err := s.removeOpenClawDingTalkAccount(ctx, accountKey); err != nil {
			s.app.Logger.Warn("openclaw config unset dingtalk account failed, proceeding with local delete", "account", accountKey, "error", err)
		}
	} else {
		if err := s.removeOpenClawFeishuAccount(ctx, accountKey); err != nil {
			s.app.Logger.Warn("openclaw config unset feishu account failed, proceeding with local delete", "account", accountKey, "error", err)
		}
	}

	if err := s.channelSvc.DeleteChannel(id); err != nil {
		return err
	}

	if m.Platform == channels.PlatformDingTalk {
		if err := s.syncOpenClawDingTalkDefaultAccount(ctx); err != nil {
			s.app.Logger.Warn("openclaw dingtalk default account sync failed after delete", "error", err)
		}
	} else {
		if err := s.syncOpenClawFeishuDefaultAccount(ctx); err != nil {
			s.app.Logger.Warn("openclaw feishu default account sync failed after delete", "error", err)
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

// ConnectChannel connects a channel.
// For DingTalk, the official OpenClaw plugin is used (no Go adapter). The plugin is installed on demand.
// For Feishu, the shared ChannelService (Go adapter + OpenClaw config) is used.
func (s *OpenClawChannelService) ConnectChannel(id int64) error {
	if id <= 0 {
		return errs.New("error.channel_id_required")
	}
	if err := s.ensureOpenClawReady(); err != nil {
		return err
	}

	// Use a short timeout just to fetch the channel model.
	fetchCtx, fetchCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer fetchCancel()
	m, err := s.getChannelModel(fetchCtx, id)
	if err != nil {
		return err
	}

	if m.Platform == channels.PlatformDingTalk {
		return s.connectDingTalkViaPlugin(id, m)
	}

	// Feishu: write OpenClaw config then connect Go adapter.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	accountKey := channelAccountKey(id, m.ExtraConfig)
	if err := s.setOpenClawChannelAccount(ctx, m.Platform, accountKey, m.Name, m.ExtraConfig, true); err != nil {
		return errs.Wrap("error.channel_connect_failed", err)
	}
	extraConfigWithID, encodeErr := withOpenClawChannelID(m.ExtraConfig, accountKey)
	if encodeErr == nil {
		if _, updateErr := s.channelSvc.UpdateChannel(id, channels.UpdateChannelInput{ExtraConfig: &extraConfigWithID}); updateErr != nil {
			return updateErr
		}
	}
	return s.channelSvc.ConnectChannel(id)
}

// connectDingTalkViaPlugin installs the DingTalk OpenClaw plugin (if necessary), writes the channel
// config to the gateway, and marks the channel as enabled + online in the local DB.
// The Go DingTalk adapter is intentionally not used here.
func (s *OpenClawChannelService) connectDingTalkViaPlugin(id int64, m *channelModel) error {
	if m.AgentID == 0 {
		return errs.New("error.channel_connect_requires_agent")
	}

	// Allow extra time for plugin installation (npm download).
	ctx, cancel := context.WithTimeout(context.Background(), dingTalkPluginInstallTimeout)
	defer cancel()

	if err := s.ensureDingTalkPluginInstalled(ctx); err != nil {
		return errs.Wrap("error.channel_connect_failed", err)
	}

	accountKey := channelAccountKey(id, m.ExtraConfig)
	if err := s.setOpenClawDingTalkAccount(ctx, accountKey, m.Name, m.ExtraConfig, true); err != nil {
		return errs.Wrap("error.channel_connect_failed", err)
	}

	// Persist account key into extra_config.
	extraConfigWithID, encodeErr := withOpenClawChannelID(m.ExtraConfig, accountKey)
	if encodeErr == nil {
		if _, updateErr := s.channelSvc.UpdateChannel(id, channels.UpdateChannelInput{ExtraConfig: &extraConfigWithID}); updateErr != nil {
			return updateErr
		}
	}

	// Mark channel as enabled + online (OpenClaw plugin owns the actual connection).
	return s.setChannelOnlineStatus(ctx, id, true)
}

// DisconnectChannel disconnects a channel.
// For DingTalk, the OpenClaw plugin config is disabled and DB status set to offline (no Go adapter).
// For Feishu, the shared ChannelService handles both OpenClaw config and Go adapter teardown.
func (s *OpenClawChannelService) DisconnectChannel(id int64) error {
	if id <= 0 {
		return errs.New("error.channel_id_required")
	}
	if err := s.ensureOpenClawReady(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	m, err := s.getChannelModel(ctx, id)
	if err != nil {
		return err
	}

	accountKey := channelAccountKey(id, m.ExtraConfig)

	if m.Platform == channels.PlatformDingTalk {
		// Write disabled config to OpenClaw plugin; log but don't abort on failure.
		if err := s.setOpenClawDingTalkAccount(ctx, accountKey, m.Name, m.ExtraConfig, false); err != nil {
			s.app.Logger.Warn("openclaw dingtalk config disable failed, proceeding with local disconnect", "error", err)
		}
		return s.setChannelOnlineStatus(ctx, id, false)
	}

	if err := s.setOpenClawChannelAccount(ctx, m.Platform, accountKey, m.Name, m.ExtraConfig, false); err != nil {
		return errs.Wrap("error.channel_disconnect_failed", err)
	}
	return s.channelSvc.DisconnectChannel(id)
}

// VerifyChannelConfig verifies platform credentials (Feishu or DingTalk).
func (s *OpenClawChannelService) VerifyChannelConfig(platform, extraConfig string) error {
	if platform == "" {
		platform = channels.PlatformFeishu
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
	// Platform selects the IM platform: "feishu" (default) or "dingtalk".
	Platform    string `json:"platform"`
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

// channelAccountKey returns the OpenClaw config account key for a channel row.
// The key is stored in extra_config.openclaw_channel_id when available, otherwise
// derived from the local DB channel ID.
func channelAccountKey(channelID int64, extraConfig string) string {
	if id := extractOpenClawChannelID(extraConfig); id != "" {
		return id
	}
	return fmt.Sprintf("channel_%d", channelID)
}

// setOpenClawChannelAccount dispatches to the correct platform-specific account setter.
// DingTalk is handled explicitly in Connect/Disconnect/Update methods — do not call this for DingTalk.
func (s *OpenClawChannelService) setOpenClawChannelAccount(ctx context.Context, platform, accountKey, name, extraConfig string, enabled bool) error {
	return s.setOpenClawFeishuAccount(ctx, accountKey, name, extraConfig, enabled)
}

// setOpenClawFeishuAccount writes a Feishu account into the OpenClaw config via CLI.
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
	_, err = s.openclawManager.ExecCLI(ctx, "config", "set", "--batch-json", string(batchJSON))
	if err != nil {
		return fmt.Errorf("openclaw config set feishu account %s: %w", accountKey, err)
	}
	return nil
}


// removeOpenClawFeishuAccount removes a Feishu account from the OpenClaw config.
func (s *OpenClawChannelService) removeOpenClawFeishuAccount(ctx context.Context, accountKey string) error {
	path := "channels.feishu.accounts." + accountKey
	_, err := s.openclawManager.ExecCLI(ctx, "config", "unset", path)
	if err != nil {
		return fmt.Errorf("openclaw config unset feishu account %s: %w", accountKey, err)
	}
	return nil
}


// syncOpenClawFeishuDefaultAccount recalculates which Feishu account should be
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
		key := channelAccountKey(ch.ID, ch.ExtraConfig)
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
	_, err = s.openclawManager.ExecCLI(ctx, "config", "set", "--batch-json", string(batchJSON))
	return err
}

// ---------------------------------------------------------------------------
// DingTalk plugin helpers
// ---------------------------------------------------------------------------

// dingTalkPluginExtensionSubdir is the subdirectory under the OpenClaw data root
// where the dingtalk-connector extension is installed by `openclaw plugins install`.
const dingTalkPluginExtensionSubdir = "extensions/dingtalk-connector"

// ensureDingTalkPluginInstalled checks whether the @dingtalk-real-ai/dingtalk-connector
// plugin is already installed in the bundled OpenClaw runtime. If not, it installs it
// via `openclaw plugins install` with exponential-backoff retries for ClawHub rate limits.
//
// The check is done via a local filesystem probe first (no network) to avoid unnecessary
// ClawHub API calls. The caller should pass a context with a timeout long enough to cover
// an npm download (use dingTalkPluginInstallTimeout).
func (s *OpenClawChannelService) ensureDingTalkPluginInstalled(ctx context.Context) error {
	// Fast local check: see if the extension directory already exists on disk.
	if s.isDingTalkPluginInstalledLocally() {
		return nil
	}

	s.app.Logger.Info("openclaw: dingtalk-connector plugin not found, installing", "plugin", dingTalkPluginName)

	// Retry with exponential backoff to handle ClawHub rate limiting (HTTP 429).
	const maxAttempts = 4
	baseDelay := 3 * time.Second
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			delay := baseDelay * time.Duration(1<<uint(attempt-1)) // 3s, 6s, 12s
			s.app.Logger.Info("openclaw: retrying plugin installation after rate limit",
				"attempt", attempt+1, "wait", delay.String())
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}

		if _, err := s.openclawManager.ExecCLI(ctx, "plugins", "install", dingTalkPluginName); err != nil {
			lastErr = err
			errStr := strings.ToLower(err.Error())
			if strings.Contains(errStr, "rate limit") || strings.Contains(errStr, "429") {
				s.app.Logger.Warn("openclaw: plugin install rate limited by ClawHub, will retry",
					"attempt", attempt+1, "error", err)
				continue
			}
			// Non-transient error — fail immediately.
			return fmt.Errorf("install %s: %w", dingTalkPluginName, err)
		}

		s.app.Logger.Info("openclaw: dingtalk-connector plugin installed successfully")
		return nil
	}

	return fmt.Errorf("install %s: ClawHub rate limit exceeded after %d attempts, please try again later: %w",
		dingTalkPluginName, maxAttempts, lastErr)
}

// isDingTalkPluginInstalledLocally returns true when the dingtalk-connector extension
// directory exists under the OpenClaw data root — a fast, network-free check.
func (s *OpenClawChannelService) isDingTalkPluginInstalledLocally() bool {
	root, err := define.OpenClawDataRootDir()
	if err != nil {
		return false
	}
	pluginDir := filepath.Join(root, dingTalkPluginExtensionSubdir)
	info, err := os.Stat(pluginDir)
	return err == nil && info.IsDir()
}

// setOpenClawDingTalkAccount writes a DingTalk account into the OpenClaw config via CLI.
// The plugin must already be installed before calling this function.
func (s *OpenClawChannelService) setOpenClawDingTalkAccount(ctx context.Context, accountKey, name, extraConfig string, enabled bool) error {
	appID, appSecret := parseAppCredentialsPair(extraConfig)
	if appID == "" {
		return fmt.Errorf("dingtalk clientId (app_id) is required")
	}

	prefix := "channels.dingtalk-connector.accounts." + accountKey

	type batchEntry struct {
		Path  string `json:"path"`
		Value any    `json:"value"`
	}
	batch := []batchEntry{
		{Path: prefix + ".clientId", Value: appID},
		{Path: prefix + ".clientSecret", Value: appSecret},
		{Path: prefix + ".enabled", Value: enabled},
	}
	if name = strings.TrimSpace(name); name != "" {
		batch = append(batch, batchEntry{Path: prefix + ".name", Value: name})
	}
	batch = append(batch,
		batchEntry{Path: "channels.dingtalk-connector.enabled", Value: enabled},
	)

	batchJSON, err := json.Marshal(batch)
	if err != nil {
		return fmt.Errorf("marshal config batch: %w", err)
	}
	if _, err := s.openclawManager.ExecCLI(ctx, "config", "set", "--batch-json", string(batchJSON)); err != nil {
		return fmt.Errorf("openclaw config set dingtalk-connector account %s: %w", accountKey, err)
	}
	return nil
}

// removeOpenClawDingTalkAccount removes a DingTalk account from the OpenClaw config.
func (s *OpenClawChannelService) removeOpenClawDingTalkAccount(ctx context.Context, accountKey string) error {
	path := "channels.dingtalk-connector.accounts." + accountKey
	if _, err := s.openclawManager.ExecCLI(ctx, "config", "unset", path); err != nil {
		return fmt.Errorf("openclaw config unset dingtalk-connector account %s: %w", accountKey, err)
	}
	return nil
}

// syncOpenClawDingTalkDefaultAccount recalculates whether dingtalk-connector should be
// globally enabled after a channel is deleted, then writes the result.
func (s *OpenClawChannelService) syncOpenClawDingTalkDefaultAccount(ctx context.Context) error {
	db, err := s.db()
	if err != nil {
		return err
	}

	var models []channelModel
	listCtx, listCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer listCancel()
	if err := db.NewSelect().Model(&models).
		Where("ch.platform = ?", channels.PlatformDingTalk).
		Where(openClawChannelVisibilitySQL).
		Scan(listCtx); err != nil {
		return err
	}

	anyEnabled := false
	for _, m := range models {
		if m.Enabled {
			anyEnabled = true
			break
		}
	}

	type batchEntry struct {
		Path  string `json:"path"`
		Value any    `json:"value"`
	}
	batch := []batchEntry{
		{Path: "channels.dingtalk-connector.enabled", Value: anyEnabled},
	}
	batchJSON, err := json.Marshal(batch)
	if err != nil {
		return err
	}
	_, err = s.openclawManager.ExecCLI(ctx, "config", "set", "--batch-json", string(batchJSON))
	return err
}

// setChannelOnlineStatus directly updates a channel's enabled and status fields in the DB.
// Used for DingTalk channels where the Go adapter is not involved.
func (s *OpenClawChannelService) setChannelOnlineStatus(ctx context.Context, channelID int64, online bool) error {
	db, err := s.db()
	if err != nil {
		return err
	}
	status := channels.StatusOffline
	if online {
		status = channels.StatusOnline
	}
	if _, err := db.NewUpdate().
		Model((*channelModel)(nil)).
		Where("id = ?", channelID).
		Set("enabled = ?", online).
		Set("status = ?", status).
		Set("updated_at = ?", sqlite.NowUTC()).
		Exec(ctx); err != nil {
		return errs.Wrap("error.channel_update_failed", err)
	}
	return nil
}
