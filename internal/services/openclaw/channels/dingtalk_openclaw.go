package openclawchannels

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"chatclaw/internal/define"
	"chatclaw/internal/errs"
	"chatclaw/internal/services/channels"
	"chatclaw/internal/sqlite"
)

const (
	dingTalkPluginName             = "@dingtalk-real-ai/dingtalk-connector"
	dingTalkPluginChannelID        = "dingtalk-connector" // channel identifier used in openclaw.json bindings
	dingTalkPluginInstallTimeout   = 3 * time.Minute
	dingTalkPluginExtensionSubdir  = "extensions/dingtalk-connector"
)

// ensureDingTalkPluginInstalled checks whether the dingtalk-connector plugin is installed.
func (s *OpenClawChannelService) ensureDingTalkPluginInstalled(ctx context.Context) error {
	if s.isDingTalkPluginInstalledLocally() {
		return nil
	}

	s.app.Logger.Info("openclaw: dingtalk-connector plugin not found, installing", "plugin", dingTalkPluginName)

	const maxAttempts = 4
	baseDelay := 3 * time.Second
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			delay := baseDelay * time.Duration(1<<uint(attempt-1))
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
			return fmt.Errorf("install %s: %w", dingTalkPluginName, err)
		}

		s.app.Logger.Info("openclaw: dingtalk-connector plugin installed successfully")
		return nil
	}

	return fmt.Errorf("install %s: ClawHub rate limit exceeded after %d attempts, please try again later: %w",
		dingTalkPluginName, maxAttempts, lastErr)
}

// installDingTalkPluginBackground installs the DingTalk connector plugin in a goroutine.
func (s *OpenClawChannelService) installDingTalkPluginBackground(channelID int64) {
	ctx, cancel := context.WithTimeout(context.Background(), dingTalkPluginInstallTimeout)
	defer cancel()

	if err := s.ensureDingTalkPluginInstalled(ctx); err != nil {
		s.app.Logger.Warn("openclaw: background dingtalk plugin installation failed", "channelId", channelID, "error", err)
		return
	}

	m, err := s.getChannelModel(ctx, channelID)
	if err != nil {
		s.app.Logger.Warn("openclaw: failed to fetch channel after dingtalk plugin install", "channelId", channelID, "error", err)
		return
	}

	accountKey := openClawChannelAccountKey(channelID, m.ExtraConfig)
	openclawAgentID := s.lookupOpenClawAgentID(ctx, m.AgentID)
	if err := s.setOpenClawDingTalkAccount(ctx, accountKey, m.Name, m.ExtraConfig, openclawAgentID, false); err != nil {
		s.app.Logger.Warn("openclaw: failed to write initial dingtalk config after background install", "channelId", channelID, "error", err)
	}
}

func (s *OpenClawChannelService) isDingTalkPluginInstalledLocally() bool {
	root, err := define.OpenClawDataRootDir()
	if err != nil {
		return false
	}
	pluginDir := filepath.Join(root, dingTalkPluginExtensionSubdir)
	info, err := os.Stat(pluginDir)
	return err == nil && info.IsDir()
}

// IsDingTalkPluginInstalled reports whether the dingtalk-connector extension directory exists (Wails UI).
func (s *OpenClawChannelService) IsDingTalkPluginInstalled() bool {
	return s.isDingTalkPluginInstalledLocally()
}

// EnsureDingTalkPluginIfNeeded runs in the background: if OpenClaw has at least one visible DingTalk channel
// but the dingtalk-connector extension is missing, installs the plugin and syncs account config (Wails: channels page).
func (s *OpenClawChannelService) EnsureDingTalkPluginIfNeeded() {
	go s.compensateDingTalkPluginInstall()
}

func (s *OpenClawChannelService) compensateDingTalkPluginInstall() {
	ctx, cancel := context.WithTimeout(context.Background(), dingTalkPluginInstallTimeout)
	defer cancel()

	if err := s.ensureOpenClawReady(); err != nil {
		s.app.Logger.Debug("openclaw: skip dingtalk plugin compensate", "reason", "openclaw_not_ready", "error", err)
		return
	}

	db, err := s.db()
	if err != nil {
		return
	}

	listCtx, listCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer listCancel()
	var probe []channelModel
	if err := db.NewSelect().Model(&probe).
		Where("ch.platform = ?", channels.PlatformDingTalk).
		Where(openClawChannelVisibilitySQL).
		Limit(1).
		Scan(listCtx); err != nil {
		s.app.Logger.Warn("openclaw: dingtalk compensate probe failed", "error", err)
		return
	}
	if len(probe) == 0 {
		return
	}
	if s.isDingTalkPluginInstalledLocally() {
		return
	}

	s.app.Logger.Info("openclaw: compensating dingtalk-connector plugin install (has DingTalk channels, extension missing)")

	if err := s.ensureDingTalkPluginInstalled(ctx); err != nil {
		s.app.Logger.Warn("openclaw: dingtalk compensate install failed", "error", err)
		return
	}
	if !s.isDingTalkPluginInstalledLocally() {
		return
	}

	s.syncDingTalkAccountsAfterPluginInstall(ctx)
}

func (s *OpenClawChannelService) syncDingTalkAccountsAfterPluginInstall(ctx context.Context) {
	db, err := s.db()
	if err != nil {
		return
	}

	listCtx, listCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer listCancel()
	var models []channelModel
	if err := db.NewSelect().Model(&models).
		Where("ch.platform = ?", channels.PlatformDingTalk).
		Where(openClawChannelVisibilitySQL).
		Scan(listCtx); err != nil {
		s.app.Logger.Warn("openclaw: dingtalk compensate list channels failed", "error", err)
		return
	}

	for i := range models {
		m := &models[i]
		accountKey := openClawChannelAccountKey(m.ID, m.ExtraConfig)
		openclawAgentID := s.lookupOpenClawAgentID(ctx, m.AgentID)
		if err := s.setOpenClawDingTalkAccount(ctx, accountKey, m.Name, m.ExtraConfig, openclawAgentID, m.Enabled); err != nil {
			s.app.Logger.Warn("openclaw: dingtalk compensate sync account failed", "channelId", m.ID, "error", err)
		}
	}
	if err := s.syncOpenClawDingTalkDefaultAccount(ctx); err != nil {
		s.app.Logger.Warn("openclaw: dingtalk compensate sync default failed", "error", err)
	}
}

func (s *OpenClawChannelService) setOpenClawDingTalkAccount(ctx context.Context, accountKey, name, extraConfig, openclawAgentID string, enabled bool) error {
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
	if openclawAgentID = strings.TrimSpace(openclawAgentID); openclawAgentID != "" {
		batch = append(batch, batchEntry{Path: prefix + ".agentId", Value: openclawAgentID})
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

	// Sync route binding so the dingtalk-connector plugin routes messages to the correct agent.
	// The plugin reads cfg.bindings (not the account-level agentId field) for per-account routing.
	if openclawAgentID != "" {
		if err := s.upsertManagedRouteBinding(dingTalkPluginChannelID, accountKey, openclawAgentID); err != nil {
			s.app.Logger.Warn("openclaw: failed to upsert dingtalk route binding", "accountKey", accountKey, "error", err)
		}
	}
	return nil
}

func (s *OpenClawChannelService) lookupOpenClawAgentID(ctx context.Context, agentID int64) string {
	if agentID <= 0 {
		return ""
	}
	db, err := s.db()
	if err != nil {
		return ""
	}
	var openclawAgentID string
	_ = db.NewSelect().
		Table("openclaw_agents").
		ColumnExpr("openclaw_agent_id").
		Where("id = ?", agentID).
		Limit(1).
		Scan(ctx, &openclawAgentID)
	return strings.TrimSpace(openclawAgentID)
}

func (s *OpenClawChannelService) removeOpenClawDingTalkAccount(ctx context.Context, accountKey string) error {
	path := "channels.dingtalk-connector.accounts." + accountKey
	if _, err := s.openclawManager.ExecCLI(ctx, "config", "unset", path); err != nil {
		return fmt.Errorf("openclaw config unset dingtalk-connector account %s: %w", accountKey, err)
	}
	// Remove the corresponding route binding.
	if err := s.removeManagedRouteBinding(dingTalkPluginChannelID, accountKey); err != nil {
		s.app.Logger.Warn("openclaw: failed to remove dingtalk route binding", "accountKey", accountKey, "error", err)
	}
	return nil
}

func (s *OpenClawChannelService) syncOpenClawDingTalkDefaultAccount(ctx context.Context) error {
	if !s.isDingTalkPluginInstalledLocally() {
		return nil
	}
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

// connectDingTalkViaPlugin installs the DingTalk OpenClaw plugin, writes config, and marks the channel online.
func (s *OpenClawChannelService) connectDingTalkViaPlugin(id int64, m *channelModel) error {
	if m.AgentID == 0 {
		return errs.New("error.channel_connect_requires_agent")
	}

	ctx, cancel := context.WithTimeout(context.Background(), dingTalkPluginInstallTimeout)
	defer cancel()

	if err := s.ensureDingTalkPluginInstalled(ctx); err != nil {
		return errs.Wrap("error.channel_connect_failed", err)
	}

	accountKey := openClawChannelAccountKey(id, m.ExtraConfig)
	openclawAgentID := s.lookupOpenClawAgentID(ctx, m.AgentID)
	if err := s.setOpenClawDingTalkAccount(ctx, accountKey, m.Name, m.ExtraConfig, openclawAgentID, true); err != nil {
		return errs.Wrap("error.channel_connect_failed", err)
	}

	extraConfigWithID, encodeErr := withOpenClawChannelID(m.ExtraConfig, accountKey)
	if encodeErr == nil {
		if _, updateErr := s.channelSvc.UpdateChannel(id, channels.UpdateChannelInput{ExtraConfig: &extraConfigWithID}); updateErr != nil {
			return updateErr
		}
	}

	return s.setChannelOnlineStatus(ctx, id, true)
}
