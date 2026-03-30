package openclawchannels

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"chatclaw/internal/errs"
	"chatclaw/internal/services/channels"
)

const (
	wechatCLIPackage           = "@tencent-weixin/openclaw-weixin-cli"
	wechatCLIPackageWithTag    = "@tencent-weixin/openclaw-weixin-cli@latest"
	wechatPluginID             = "openclaw-weixin"
	wechatPluginInstallTimeout = 5 * time.Minute
	wechatQRCodeTimeout        = 30 * time.Second
	wechatLoginWaitTimeout     = 5 * time.Minute
	wechatExtensionSubdir      = "extensions/openclaw-weixin"
)

// isWechatPluginInstalledLocally checks whether the WeChat OpenClaw plugin directory exists.
func (s *OpenClawChannelService) isWechatPluginInstalledLocally() bool {
	stateDir, err := s.openclawManager.BundleStateDir()
	if err != nil {
		return false
	}
	pluginDir := filepath.Join(stateDir, wechatExtensionSubdir)
	info, err := os.Stat(pluginDir)
	return err == nil && info.IsDir()
}

// IsWechatPluginInstalled reports whether the WeChat OpenClaw plugin is installed (Wails UI).
func (s *OpenClawChannelService) IsWechatPluginInstalled() bool {
	return s.isWechatPluginInstalledLocally()
}

// ensureWechatPluginInstalled installs and enables the WeChat plugin, then restarts the
// gateway so the plugin's web login provider is registered for gateway method calls.
func (s *OpenClawChannelService) ensureWechatPluginInstalled(ctx context.Context) error {
	alreadyInstalled := s.isWechatPluginInstalledLocally()

	if !alreadyInstalled {
		s.app.Logger.Info("openclaw: wechat plugin not found, installing", "package", wechatCLIPackage)

		out, err := s.openclawManager.ExecNpx(ctx, "-y", wechatCLIPackageWithTag, "install")
		if err != nil {
			outStr := strings.ToLower(string(out))
			if strings.Contains(outStr, "already installed") || strings.Contains(outStr, "plugin already exists") {
				s.app.Logger.Info("openclaw: wechat plugin already installed (marker in output)")
			} else {
				return fmt.Errorf("install wechat plugin: %w", err)
			}
		} else {
			s.app.Logger.Info("openclaw: wechat plugin installed successfully")
		}
	}

	// Explicitly enable the plugin so its web login provider is registered.
	// This is idempotent and safe to call on every invocation.
	if _, err := s.execOpenClawCLIWithRetry(ctx, "config", "set",
		"plugins.entries.openclaw-weixin.enabled", "true", "--strict-json"); err != nil {
		s.app.Logger.Warn("openclaw: failed to enable wechat plugin in config", "error", err)
	}

	// Restart gateway so it loads/reloads the plugin with the updated config.
	if err := s.restartOpenClawGateway(); err != nil {
		s.app.Logger.Warn("openclaw: gateway restart after wechat plugin setup failed", "error", err)
	} else {
		// Give the gateway a moment to come back online before we make method calls.
		time.Sleep(3 * time.Second)
	}

	return nil
}

// EnsureWechatPluginInstalled installs the WeChat OpenClaw plugin if needed (Wails UI).
func (s *OpenClawChannelService) EnsureWechatPluginInstalled() error {
	ctx, cancel := context.WithTimeout(context.Background(), wechatPluginInstallTimeout)
	defer cancel()
	return s.ensureWechatPluginInstalled(ctx)
}

// WechatQRCodeResult holds the QR code image and a session key for subsequent login polling.
type WechatQRCodeResult struct {
	QRCodeDataURL string `json:"qrcode_data_url"` // base64 data URL or direct HTTPS URL
	SessionKey    string `json:"session_key"`
}

// wechatLoginStartResponse is the expected payload from the gateway's web.login.start method.
type wechatLoginStartResponse struct {
	QRCodeURL  string `json:"qrcodeUrl"`
	SessionKey string `json:"sessionKey"`
	Message    string `json:"message"`
}

// wechatLoginWaitResponse is the expected payload from the gateway's web.login.wait method.
type wechatLoginWaitResponse struct {
	Connected bool   `json:"connected"`
	AccountID string `json:"accountId"`
	BotToken  string `json:"botToken"`
	BaseURL   string `json:"baseUrl"`
	UserID    string `json:"userId"`
	Message   string `json:"message"`
}

// WechatLoginResult holds the result of waiting for the WeChat QR code scan.
type WechatLoginResult struct {
	Connected bool   `json:"connected"`
	AccountID string `json:"account_id"` // ilink_bot_id from WeChat side
	Message   string `json:"message"`
}

// GenerateWechatQRCode installs the plugin if needed and starts a new WeChat QR code
// login session. Returns the QR code as a base64 data URL and a session key for polling.
// This is a Wails-exposed method called by the frontend.
func (s *OpenClawChannelService) GenerateWechatQRCode() (*WechatQRCodeResult, error) {
	if err := s.ensureOpenClawReady(); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), wechatPluginInstallTimeout)
	defer cancel()

	if err := s.ensureWechatPluginInstalled(ctx); err != nil {
		return nil, errs.Wrap("error.channel_connect_failed", err)
	}

	// Call the gateway's web.login.start method registered by the WeChat plugin.
	qrCtx, qrCancel := context.WithTimeout(context.Background(), wechatQRCodeTimeout)
	defer qrCancel()

	var startResp wechatLoginStartResponse
	if err := s.openclawManager.Request(qrCtx, "web.login.start", map[string]any{
		"force": true,
	}, &startResp); err != nil {
		return nil, errs.Wrap("error.channel_connect_failed", err)
	}

	if startResp.QRCodeURL == "" {
		return nil, fmt.Errorf("WeChat plugin returned empty QR code URL: %s", startResp.Message)
	}

	s.app.Logger.Info("openclaw: wechat QR code obtained", "sessionKey", startResp.SessionKey, "qrcodeUrl", startResp.QRCodeURL)

	// Fetch the QR code image and encode as base64 data URL for the frontend.
	dataURL, err := fetchImageAsDataURL(startResp.QRCodeURL)
	if err != nil {
		// Fallback: return the URL directly; the webview can load it via <img src>.
		s.app.Logger.Warn("openclaw: failed to fetch wechat QR code image, returning URL directly", "error", err)
		dataURL = startResp.QRCodeURL
	}

	return &WechatQRCodeResult{
		QRCodeDataURL: dataURL,
		SessionKey:    startResp.SessionKey,
	}, nil
}

// WaitForWechatLogin waits (up to ~5 minutes) for the user to scan the QR code and
// confirm the WeChat login. When login succeeds, it automatically creates a local
// channel record. This is a Wails-exposed method called by the frontend.
func (s *OpenClawChannelService) WaitForWechatLogin(sessionKey string, channelName string) (*WechatLoginResult, error) {
	if err := s.ensureOpenClawReady(); err != nil {
		return nil, err
	}

	waitCtx, cancel := context.WithTimeout(context.Background(), wechatLoginWaitTimeout)
	defer cancel()

	var waitResp wechatLoginWaitResponse
	if err := s.openclawManager.Request(waitCtx, "web.login.wait", map[string]any{
		"sessionKey": sessionKey,
	}, &waitResp); err != nil {
		return nil, errs.Wrap("error.channel_connect_failed", err)
	}

	result := &WechatLoginResult{
		Connected: waitResp.Connected,
		AccountID: waitResp.AccountID,
		Message:   waitResp.Message,
	}

	if !waitResp.Connected {
		return result, nil
	}

	// Login succeeded: create a local channel record so it appears in the channel list.
	name := strings.TrimSpace(channelName)
	if name == "" {
		name = "微信"
	}

	ch, err := s.channelSvc.CreateChannel(channels.CreateChannelInput{
		Platform:       channels.PlatformWechat,
		Name:           name,
		Avatar:         "",
		ConnectionType: channels.ConnTypeGateway,
		ExtraConfig:    fmt.Sprintf(`{"platform":"wechat","account_id":%q}`, waitResp.AccountID),
		OpenClawScope:  true,
	})
	if err != nil {
		s.app.Logger.Warn("openclaw: failed to create wechat channel record after login", "error", err)
	} else if ch != nil {
		s.app.Logger.Info("openclaw: wechat channel record created", "channel_id", ch.ID, "accountId", waitResp.AccountID)
		// Mark the channel as online since login was just completed.
		ctx, cancelSet := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelSet()
		if err := s.setChannelOnlineStatus(ctx, ch.ID, true); err != nil {
			s.app.Logger.Warn("openclaw: failed to set wechat channel online", "channel_id", ch.ID, "error", err)
		}
	}

	return result, nil
}

// connectWechatViaPlugin enables the WeChat channel in OpenClaw config and marks it online.
func (s *OpenClawChannelService) connectWechatViaPlugin(id int64, m *channelModel) error {
	if m.AgentID == 0 {
		return errs.New("error.channel_connect_requires_agent")
	}

	ctx, cancel := context.WithTimeout(context.Background(), openClawChannelSyncTimeout)
	defer cancel()

	if err := s.ensureWechatPluginInstalled(ctx); err != nil {
		return errs.Wrap("error.channel_connect_failed", err)
	}

	accountID := openClawWechatAccountID(id)
	openclawAgentID := s.lookupOpenClawAgentID(ctx, m.AgentID)
	if openclawAgentID != "" {
		if err := s.upsertManagedRouteBinding(wechatPluginID, accountID, openclawAgentID); err != nil {
			s.app.Logger.Warn("openclaw: failed to upsert wechat route binding", "channelId", id, "error", err)
		}
		if err := s.restartOpenClawGateway(); err != nil {
			return errs.Wrap("error.channel_connect_failed", err)
		}
	}

	return s.setChannelOnlineStatus(ctx, id, true)
}

// disconnectWechatViaPlugin marks the WeChat channel offline.
func (s *OpenClawChannelService) disconnectWechatViaPlugin(id int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), openClawChannelSyncTimeout)
	defer cancel()
	return s.setChannelOnlineStatus(ctx, id, false)
}

func (s *OpenClawChannelService) countEnabledWechatChannels(ctx context.Context, excludeID int64) int {
	db, err := s.db()
	if err != nil {
		return 0
	}
	listCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var models []channelModel
	if err := db.NewSelect().Model(&models).
		Where("ch.platform = ?", channels.PlatformWechat).
		Where("ch.enabled = ?", true).
		Where("ch.id != ?", excludeID).
		Where(openClawChannelVisibilitySQL).
		Scan(listCtx); err != nil {
		return 0
	}
	return len(models)
}

func openClawWechatAccountID(channelID int64) string {
	return fmt.Sprintf("channel_%d", channelID)
}

// fetchImageAsDataURL downloads an image from the given URL and returns it as a
// base64-encoded data URL (e.g. "data:image/png;base64,...").
func fetchImageAsDataURL(imageURL string) (string, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(imageURL)
	if err != nil {
		return "", fmt.Errorf("fetch QR code image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch QR code image: HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read QR code image: %w", err)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "image/png"
	}
	// Keep only the media type part, strip charset/params.
	if idx := strings.Index(contentType, ";"); idx != -1 {
		contentType = strings.TrimSpace(contentType[:idx])
	}

	encoded := base64.StdEncoding.EncodeToString(data)
	return "data:" + contentType + ";base64," + encoded, nil
}
