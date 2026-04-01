package openclawchannels

import (
	"context"
	"fmt"
	"strings"

	"chatclaw/internal/errs"
	"chatclaw/internal/services/channels"
)

const (
	openClawQQPluginPackage = "@tencent-connect/openclaw-qqbot@latest"
	openClawQQPluginMarker  = "@tencent-connect/openclaw-qqbot"
	openClawQQChannelID     = "qqbot"
)

func (s *OpenClawChannelService) ensureOpenClawQQPluginInstalled(ctx context.Context) error {
	installed, err := s.isOpenClawQQPluginInstalled(ctx)
	if err == nil && installed {
		return nil
	}

	if _, installErr := s.execOpenClawPluginCLI(ctx, "plugins", "install", openClawQQPluginPackage); installErr != nil {
		installMsg := strings.ToLower(installErr.Error())
		alreadyInstalled := strings.Contains(installMsg, "plugin already exists") || containsOpenClawQQPluginMarker(installMsg)
		if !alreadyInstalled {
			return fmt.Errorf("openclaw plugins install %s: %w", openClawQQPluginPackage, installErr)
		}
	}

	verifyInstalled, verifyErr := s.isOpenClawQQPluginInstalled(ctx)
	if verifyErr != nil {
		return fmt.Errorf("verify installed plugin %s: %w", openClawQQPluginMarker, verifyErr)
	}
	if !verifyInstalled {
		return fmt.Errorf("plugin %s not found after installation", openClawQQPluginMarker)
	}
	return nil
}

func (s *OpenClawChannelService) isOpenClawQQPluginInstalled(ctx context.Context) (bool, error) {
	out, err := s.execOpenClawPluginCLI(ctx, "plugins", "list")
	if err != nil {
		return false, err
	}
	return containsOpenClawQQPluginMarker(string(out)), nil
}

func containsOpenClawQQPluginMarker(out string) bool {
	out = strings.ToLower(out)
	return strings.Contains(out, strings.ToLower(openClawQQPluginMarker)) || strings.Contains(out, openClawQQChannelID)
}

func isOpenClawUnknownQQChannelErr(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "unknown channel: "+openClawQQChannelID)
}

func (s *OpenClawChannelService) addOpenClawQQChannel(ctx context.Context, accountID, name, token string) error {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return fmt.Errorf("qq openclaw account id is required")
	}
	args := []string{
		"channels", "add",
		"--channel", openClawQQChannelID,
		"--account", accountID,
		"--token", token,
	}
	if name = strings.TrimSpace(name); name != "" {
		args = append(args, "--name", name)
	}

	if _, err := s.execOpenClawCLIWithRetry(ctx, args...); err != nil {
		if isOpenClawUnknownQQChannelErr(err) {
			s.app.Logger.Info("openclaw: qqbot channel not registered yet, restarting gateway before retry")
			if restartErr := s.restartOpenClawGateway(); restartErr != nil {
				return fmt.Errorf("restart gateway before retrying qqbot channel add: %w", restartErr)
			}
			if _, retryErr := s.execOpenClawCLIWithRetry(ctx, args...); retryErr != nil {
				return fmt.Errorf("openclaw channels add --channel %s after restart: %w", openClawQQChannelID, retryErr)
			}
			return nil
		}
		return fmt.Errorf("openclaw channels add --channel %s: %w", openClawQQChannelID, err)
	}
	return nil
}

func (s *OpenClawChannelService) setOpenClawQQChannel(ctx context.Context, channelID int64, name, extraConfig string) error {
	if err := s.ensureOpenClawQQPluginInstalled(ctx); err != nil {
		return err
	}

	installed, err := s.isOpenClawQQPluginInstalled(ctx)
	if err != nil {
		return fmt.Errorf("verify installed plugin %s: %w", openClawQQPluginMarker, err)
	}
	if !installed {
		return fmt.Errorf("plugin %s not found after installation", openClawQQPluginMarker)
	}

	appID, appSecret := parseAppCredentialsPair(extraConfig)
	if appID == "" || appSecret == "" {
		return fmt.Errorf("qq appId and appSecret are required")
	}

	accountID := openClawChannelAccountKey(channelID, extraConfig)
	return s.addOpenClawQQChannel(ctx, accountID, name, appID+":"+appSecret)
}

func (s *OpenClawChannelService) removeOpenClawQQChannel(ctx context.Context, channelID int64, extraConfig string) error {
	installed, err := s.isOpenClawQQPluginInstalled(ctx)
	if err != nil {
		return fmt.Errorf("list plugins before removing %s channel: %w", openClawQQChannelID, err)
	}
	if !installed {
		return nil
	}
	accountID := strings.TrimSpace(openClawChannelAccountKey(channelID, extraConfig))
	if accountID == "" {
		return fmt.Errorf("qq openclaw account id is required")
	}
	args := []string{
		"channels", "remove",
		"--channel", openClawQQChannelID,
		"--account", accountID,
		"--delete",
	}
	if _, err := s.execOpenClawCLIWithRetry(ctx, args...); err != nil {
		return fmt.Errorf("openclaw channels remove --channel %s --account %s --delete: %w", openClawQQChannelID, accountID, err)
	}
	return nil
}

func (s *OpenClawChannelService) connectQQViaPlugin(id int64, m *channelModel) error {
	if m.AgentID == 0 {
		return errs.New("error.channel_connect_requires_agent")
	}

	ctx, cancel := context.WithTimeout(context.Background(), openClawChannelSyncTimeout)
	defer cancel()

	if err := s.setOpenClawQQChannel(ctx, id, m.Name, m.ExtraConfig); err != nil {
		return wrapOpenClawSyncErr(err, "error.channel_connect_failed", map[string]any{"Name": m.Name})
	}
	if err := s.syncChannelRoutingBinding(id, m.AgentID); err != nil {
		return wrapOpenClawSyncErr(err, "error.channel_connect_failed", map[string]any{"Name": m.Name})
	}

	accountKey := openClawChannelAccountKey(id, m.ExtraConfig)
	if extraConfigWithID, encodeErr := withOpenClawChannelID(m.ExtraConfig, accountKey); encodeErr == nil {
		if _, updateErr := s.channelSvc.UpdateChannel(id, channels.UpdateChannelInput{ExtraConfig: &extraConfigWithID}); updateErr != nil {
			return updateErr
		}
	}

	enabled := true
	if _, err := s.channelSvc.UpdateChannel(id, channels.UpdateChannelInput{Enabled: &enabled}); err != nil {
		return err
	}
	return s.setOpenClawPluginChannelStatus(ctx, id, true)
}

func (s *OpenClawChannelService) disconnectQQViaPlugin(id int64, m *channelModel) error {
	ctx, cancel := context.WithTimeout(context.Background(), openClawChannelSyncTimeout)
	defer cancel()

	if err := s.removeOpenClawQQChannel(ctx, id, m.ExtraConfig); err != nil {
		return wrapOpenClawSyncErr(err, "error.channel_disconnect_failed", nil)
	}
	accountID := openClawManagedAccountID(channels.PlatformQQ, id, m.ExtraConfig)
	if err := s.removeManagedRouteBinding(channels.PlatformQQ, accountID); err != nil {
		return errs.Wrap("error.channel_disconnect_failed", err)
	}
	if err := s.restartOpenClawGateway(); err != nil {
		return errs.Wrap("error.channel_disconnect_failed", err)
	}

	enabled := false
	if _, err := s.channelSvc.UpdateChannel(id, channels.UpdateChannelInput{Enabled: &enabled}); err != nil {
		return err
	}
	return s.setOpenClawPluginChannelStatus(ctx, id, false)
}
