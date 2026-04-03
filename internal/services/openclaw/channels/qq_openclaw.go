package openclawchannels

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"chatclaw/internal/errs"
	"chatclaw/internal/services/channels"
)

const (
	openClawQQPluginPackage = "@tencent-connect/openclaw-qqbot@latest"
	openClawQQPluginMarker  = "@tencent-connect/openclaw-qqbot"
	openClawQQPluginID      = "openclaw-qqbot"
	openClawQQChannelID     = "qqbot"
	qqPluginVerifyAttempts  = 5
	qqPluginVerifyDelay     = 1500 * time.Millisecond
	qqPluginFallbackTimeout = 2 * time.Minute
)

func (s *OpenClawChannelService) ensureOpenClawQQPluginInstalled(ctx context.Context) error {
	installed, err := s.isOpenClawQQPluginInstalled(ctx)
	if err == nil && installed {
		return nil
	}

	if _, installErr := s.execOpenClawPluginCLI(ctx, "plugins", "install", openClawQQPluginPackage); installErr != nil {
		installMsg := strings.ToLower(installErr.Error())
		if isQQPluginInstallRateLimited(installMsg) {
			s.app.Logger.Warn("openclaw: qq plugin install rate limited by ClawHub, falling back to npm pack install",
				"plugin", openClawQQPluginPackage, "error", installErr)
			fallbackCtx, fallbackCancel := context.WithTimeout(context.Background(), qqPluginFallbackTimeout)
			defer fallbackCancel()
			if fallbackErr := s.installOpenClawQQPluginFromLocalPackage(fallbackCtx); fallbackErr != nil {
				return fmt.Errorf("openclaw plugins install %s rate-limited, fallback failed: %w", openClawQQPluginPackage, fallbackErr)
			}
			return s.verifyOpenClawQQPluginInstalled(ctx)
		}
		installedButInterrupted := strings.Contains(installMsg, "installed plugin:") && containsOpenClawQQPluginMarker(installMsg)
		if installedButInterrupted {
			s.app.Logger.Warn("openclaw qq plugin install interrupted after success marker; will verify by listing plugins",
				"plugin", openClawQQPluginPackage, "error", installErr)
		}
		alreadyInstalled := strings.Contains(installMsg, "plugin already exists")
		if !alreadyInstalled && !installedButInterrupted {
			return fmt.Errorf("openclaw plugins install %s: %w", openClawQQPluginPackage, installErr)
		}
	}
	return s.verifyOpenClawQQPluginInstalled(ctx)
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
	return strings.Contains(out, strings.ToLower(openClawQQPluginMarker)) ||
		strings.Contains(out, strings.ToLower(openClawQQPluginID)) ||
		strings.Contains(out, openClawQQChannelID)
}

func isQQPluginInstallRateLimited(msg string) bool {
	m := strings.ToLower(msg)
	return strings.Contains(m, "rate limit") || strings.Contains(m, "429")
}

func (s *OpenClawChannelService) installOpenClawQQPluginFromLocalPackage(ctx context.Context) error {
	if s.openclawManager == nil {
		return fmt.Errorf("openclaw manager not available")
	}
	tmpDir, err := os.MkdirTemp("", "openclaw-qqbot-pack-*")
	if err != nil {
		return fmt.Errorf("create qq plugin temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	registries := []string{
		"https://registry.npmjs.org",
		"https://mirrors.cloud.tencent.com/npm/",
	}
	var tgzPath string
	var lastErr error
	for _, registry := range registries {
		out, packErr := s.openclawManager.ExecNpx(
			ctx,
			"-y",
			"npm@latest",
			"pack",
			openClawQQPluginPackage,
			"--pack-destination",
			tmpDir,
			"--registry",
			registry,
		)
		if packErr == nil {
			tgzPath = strings.TrimSpace(string(out))
			if tgzPath != "" {
				tgzPath = filepath.Join(tmpDir, filepath.Base(tgzPath))
			} else {
				matches, _ := filepath.Glob(filepath.Join(tmpDir, "*.tgz"))
				if len(matches) > 0 {
					tgzPath = matches[0]
				}
			}
			if tgzPath != "" {
				break
			}
			lastErr = fmt.Errorf("npm pack succeeded but no tgz found in %s", tmpDir)
			continue
		}
		lastErr = fmt.Errorf("npm pack from %s: %w\n%s", registry, packErr, string(out))
	}
	if tgzPath == "" {
		if lastErr != nil {
			return lastErr
		}
		return fmt.Errorf("npm pack did not produce a package archive")
	}

	packageDir := filepath.Join(tmpDir, "package")
	if err := extractTarGz(tgzPath, tmpDir); err != nil {
		return fmt.Errorf("extract qq plugin package: %w", err)
	}
	if _, statErr := os.Stat(filepath.Join(packageDir, "package.json")); statErr != nil {
		return fmt.Errorf("qq plugin package missing package.json after extract: %w", statErr)
	}

	if _, err := s.execOpenClawPluginCLI(ctx, "plugins", "install", packageDir); err != nil {
		return fmt.Errorf("install qq plugin from local package dir: %w", err)
	}
	return nil
}

func extractTarGz(archivePath string, targetDir string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if header == nil {
			continue
		}

		name := filepath.Clean(header.Name)
		if name == "." || name == string(filepath.Separator) {
			continue
		}
		destPath := filepath.Join(targetDir, name)
		if !strings.HasPrefix(destPath, filepath.Clean(targetDir)+string(filepath.Separator)) && filepath.Clean(destPath) != filepath.Clean(targetDir) {
			return fmt.Errorf("illegal archive path: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(destPath, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(destPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			if err := out.Close(); err != nil {
				return err
			}
		}
	}
}

func (s *OpenClawChannelService) verifyOpenClawQQPluginInstalled(ctx context.Context) error {
	var lastListOutput string
	var lastErr error
	for attempt := 1; attempt <= qqPluginVerifyAttempts; attempt++ {
		out, err := s.execOpenClawPluginCLI(ctx, "plugins", "list")
		if err == nil {
			lastListOutput = strings.TrimSpace(string(out))
			if containsOpenClawQQPluginMarker(lastListOutput) {
				return nil
			}
			lastErr = nil
		} else {
			lastErr = err
		}

		if attempt == qqPluginVerifyAttempts {
			break
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("verify installed plugin %s: %w", openClawQQPluginMarker, ctx.Err())
		case <-time.After(qqPluginVerifyDelay):
		}
	}

	if lastErr != nil {
		return fmt.Errorf("verify installed plugin %s: %w", openClawQQPluginMarker, lastErr)
	}
	s.app.Logger.Warn("openclaw: qq plugin still missing after install verification retries",
		"plugin", openClawQQPluginMarker, "list_output", lastListOutput)
	return fmt.Errorf("plugin %s not found after installation", openClawQQPluginMarker)
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
