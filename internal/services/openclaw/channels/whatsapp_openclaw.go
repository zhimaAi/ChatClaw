package openclawchannels

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"chatclaw/internal/define"
	"chatclaw/internal/errs"
	"chatclaw/internal/services/channels"
)

const (
	openClawWhatsappChannelID    = "whatsapp"
	whatsappDefaultChannelName   = "WhatsApp"
	whatsappLoginWaitTimeout     = 8 * time.Minute
	whatsappLoginRetryDelay      = 1500 * time.Millisecond
	whatsappQRFastStartTimeout   = 8 * time.Second
	whatsappQRStartTimeout       = 25 * time.Second
	whatsappPluginInstallTimeout = 5 * time.Minute
	whatsappDefaultAccountID     = "default"
	whatsappConfigKeyEnabled     = "enabled"
	whatsappConfigKeySelfChat    = "selfChatMode"
	whatsappConfigKeyAccounts    = "accounts"
	whatsappConfigKeyAgentID     = "agentId"
)

func normalizeWhatsappAccountID(accountID string) string {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return whatsappDefaultAccountID
	}
	return accountID
}

func shortenWhatsappSessionKey(sessionKey string) string {
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" {
		return ""
	}
	if len(sessionKey) <= 12 {
		return sessionKey
	}
	return sessionKey[:8] + "..." + sessionKey[len(sessionKey)-4:]
}

func nextWhatsappAutoChannelName(existingNames []string) string {
	occupied := make(map[string]struct{}, len(existingNames))
	for _, name := range existingNames {
		normalized := strings.ToLower(strings.TrimSpace(name))
		if normalized == "" {
			continue
		}
		occupied[normalized] = struct{}{}
	}

	candidateIndex := len(existingNames) + 1
	if candidateIndex < 1 {
		candidateIndex = 1
	}

	for {
		candidate := fmt.Sprintf("%s%d", whatsappDefaultChannelName, candidateIndex)
		if _, exists := occupied[strings.ToLower(candidate)]; !exists {
			return candidate
		}
		candidateIndex++
	}
}

func ensureWhatsappConfigEntry(entry map[string]any) (map[string]any, bool) {
	if entry == nil {
		entry = map[string]any{}
	}

	enabled, _ := entry[whatsappConfigKeyEnabled].(bool)
	selfChatMode, _ := entry[whatsappConfigKeySelfChat].(bool)
	if enabled && selfChatMode {
		return entry, false
	}

	entry[whatsappConfigKeyEnabled] = true
	entry[whatsappConfigKeySelfChat] = true
	return entry, true
}

func ensureWhatsappAccountConfigEntry(entry map[string]any) (map[string]any, bool) {
	return ensureWhatsappConfigEntry(entry)
}

func ensureWhatsappChannelConfigEntry(entry map[string]any) (map[string]any, bool) {
	return ensureWhatsappConfigEntry(entry)
}

func whatsappChannelConfigFromRoot(cfg map[string]any) map[string]any {
	if cfg == nil {
		return nil
	}
	channelsCfg, _ := cfg["channels"].(map[string]any)
	if channelsCfg == nil {
		return nil
	}
	whatsappCfg, _ := channelsCfg[openClawWhatsappChannelID].(map[string]any)
	return whatsappCfg
}

func whatsappAccountConfigs(channelCfg map[string]any) map[string]any {
	if channelCfg == nil {
		return nil
	}
	accounts, _ := channelCfg[whatsappConfigKeyAccounts].(map[string]any)
	return accounts
}

func whatsappAccountConfigFromChannel(channelCfg map[string]any, accountID string) map[string]any {
	accountID = normalizeWhatsappAccountID(accountID)
	accounts := whatsappAccountConfigs(channelCfg)
	if accounts == nil {
		return nil
	}
	entry, _ := accounts[accountID].(map[string]any)
	return entry
}

func firstConfiguredWhatsappAccountID(channelCfg map[string]any) string {
	accounts := whatsappAccountConfigs(channelCfg)
	if len(accounts) == 0 {
		return whatsappDefaultAccountID
	}
	if _, ok := accounts[whatsappDefaultAccountID]; ok {
		return whatsappDefaultAccountID
	}
	for key := range accounts {
		if accountID := strings.TrimSpace(key); accountID != "" {
			return accountID
		}
	}
	return whatsappDefaultAccountID
}

func whatsappConfiguredAgentIDFromChannel(channelCfg map[string]any, accountID string) string {
	accountCfg := whatsappAccountConfigFromChannel(channelCfg, accountID)
	if accountCfg == nil {
		return ""
	}
	agentID, _ := accountCfg[whatsappConfigKeyAgentID].(string)
	return strings.TrimSpace(agentID)
}

func whatsappConfiguredAgentIDFromBindings(cfg map[string]any, accountID string) string {
	bindings, _ := cfg["bindings"].([]any)
	for _, raw := range bindings {
		binding, _ := raw.(map[string]any)
		if binding == nil {
			continue
		}
		if strings.TrimSpace(fmt.Sprint(binding["type"])) != "route" {
			continue
		}
		match, _ := binding["match"].(map[string]any)
		if match == nil {
			continue
		}
		if strings.TrimSpace(fmt.Sprint(match["channel"])) != openClawWhatsappChannelID {
			continue
		}
		if accountID != "" && strings.TrimSpace(fmt.Sprint(match["accountId"])) != accountID {
			continue
		}
		if agentID := strings.TrimSpace(fmt.Sprint(binding["agentId"])); agentID != "" {
			return agentID
		}
	}
	return ""
}

func whatsappConfiguredAgentIDFromConfig(cfg map[string]any, accountID string) string {
	if cfg == nil {
		return ""
	}
	accountID = normalizeWhatsappAccountID(accountID)
	if agentID := whatsappConfiguredAgentIDFromChannel(whatsappChannelConfigFromRoot(cfg), accountID); agentID != "" {
		return agentID
	}
	return whatsappConfiguredAgentIDFromBindings(cfg, accountID)
}

func resolveWhatsappConfigBool(channelCfg map[string]any, accountCfg map[string]any, key string) (bool, bool) {
	if accountCfg != nil {
		if v, ok := accountCfg[key].(bool); ok {
			return v, true
		}
	}
	if channelCfg != nil {
		if v, ok := channelCfg[key].(bool); ok {
			return v, true
		}
	}
	return false, false
}

func isWhatsappConfigEnabledForAccount(channelCfg map[string]any, accountID string) bool {
	if channelCfg == nil {
		return false
	}

	entry := whatsappAccountConfigFromChannel(channelCfg, accountID)

	enabled, enabledConfigured := resolveWhatsappConfigBool(channelCfg, entry, whatsappConfigKeyEnabled)
	if !enabledConfigured {
		enabled = true
	}
	selfChatMode, selfChatConfigured := resolveWhatsappConfigBool(channelCfg, entry, whatsappConfigKeySelfChat)
	return enabled && selfChatConfigured && selfChatMode
}

func (s *OpenClawChannelService) isWhatsappPluginConfigured(accountID string) bool {
	accountID = normalizeWhatsappAccountID(accountID)
	cfg, _, err := loadOpenClawJSONConfig()
	if err != nil {
		return false
	}
	return isWhatsappConfigEnabledForAccount(whatsappChannelConfigFromRoot(cfg), accountID)
}

func (s *OpenClawChannelService) isWhatsappPluginReadyForUse() bool {
	return s.isWhatsappPluginConfigured(whatsappDefaultAccountID)
}

type whatsappLoginSession struct {
	accountID string
}

// WhatsappChannelPreparation reports whether the bundled WhatsApp channel is ready before QR login starts.
type WhatsappChannelPreparation struct {
	Ready          bool `json:"ready"`
	Installing     bool `json:"installing"`
	StartedInstall bool `json:"started_install"`
}

// WhatsappQRCodeResult is returned by GenerateWhatsappQRCode for the Wails UI.
type WhatsappQRCodeResult struct {
	QrcodeDataURL string `json:"qrcode_data_url"`
	SessionKey    string `json:"session_key"`
}

// WhatsappLoginResult is returned by WaitForWhatsappLogin.
type WhatsappLoginResult struct {
	Connected bool   `json:"connected"`
	AccountID string `json:"account_id"`
	Message   string `json:"message"`
	ChannelID int64  `json:"channel_id"`
}

func (s *OpenClawChannelService) ensureWhatsappLoginMap() {
	if s.whatsappLogins == nil {
		s.whatsappLogins = make(map[string]*whatsappLoginSession)
	}
}

func (s *OpenClawChannelService) cancelAllWhatsappLoginSessions() {
	s.whatsappLoginMu.Lock()
	defer s.whatsappLoginMu.Unlock()
	s.ensureWhatsappLoginMap()
	for k := range s.whatsappLogins {
		delete(s.whatsappLogins, k)
	}
}

func (s *OpenClawChannelService) ensureOpenClawWhatsappPluginInstalled(ctx context.Context) error {
	accountID := normalizeWhatsappAccountID(s.readFirstWhatsappAccountIDFromOpenClawJSON())
	startedAt := time.Now()
	s.app.Logger.Info("openclaw: ensuring whatsapp channel is enabled",
		"accountId", accountID)
	changed, err := s.ensureWhatsappChannelEnabled(accountID)
	if err != nil {
		s.app.Logger.Warn("openclaw: failed to ensure whatsapp channel is enabled",
			"accountId", accountID,
			"duration", time.Since(startedAt).String(),
			"error", err)
		return err
	}
	if !changed {
		s.app.Logger.Info("openclaw: whatsapp channel already enabled",
			"accountId", accountID,
			"duration", time.Since(startedAt).String())
		return nil
	}
	s.app.Logger.Info("openclaw: whatsapp channel config updated, restarting gateway",
		"accountId", accountID)
	if err := s.restartOpenClawGateway(); err != nil {
		s.app.Logger.Warn("openclaw: failed to restart gateway after enabling whatsapp channel",
			"accountId", accountID,
			"duration", time.Since(startedAt).String(),
			"error", err)
		return fmt.Errorf("restart openclaw gateway after enabling whatsapp channel: %w", err)
	}
	s.app.Logger.Info("openclaw: gateway restarted after enabling whatsapp channel",
		"accountId", accountID,
		"duration", time.Since(startedAt).String())
	// The gateway needs a brief moment to rehydrate the web login surface after config changes.
	select {
	case <-ctx.Done():
		s.app.Logger.Warn("openclaw: waiting for whatsapp channel rehydrate interrupted",
			"accountId", accountID,
			"duration", time.Since(startedAt).String(),
			"error", ctx.Err())
		return ctx.Err()
	case <-time.After(3 * time.Second):
	}
	s.app.Logger.Info("openclaw: whatsapp channel is ready after enable flow",
		"accountId", accountID,
		"duration", time.Since(startedAt).String())
	return nil
}

func (s *OpenClawChannelService) ensureWhatsappChannelEnabled(accountID string) (bool, error) {
	accountID = normalizeWhatsappAccountID(accountID)
	cfg, configPath, err := loadOpenClawJSONConfig()
	if err != nil {
		return false, err
	}

	channelsCfg, _ := cfg["channels"].(map[string]any)
	if channelsCfg == nil {
		channelsCfg = map[string]any{}
	}
	whatsappCfg, _ := channelsCfg[openClawWhatsappChannelID].(map[string]any)
	if whatsappCfg == nil {
		whatsappCfg = map[string]any{}
	}
	whatsappCfg, channelChanged := ensureWhatsappChannelConfigEntry(whatsappCfg)
	accounts := whatsappAccountConfigs(whatsappCfg)
	if accounts == nil {
		accounts = map[string]any{}
	}
	entry, _ := accounts[accountID].(map[string]any)
	if entry == nil {
		entry = map[string]any{}
	}

	entry, accountChanged := ensureWhatsappAccountConfigEntry(entry)
	changed := channelChanged || accountChanged
	if !changed {
		s.app.Logger.Debug("openclaw: whatsapp account already enabled in config",
			"accountId", accountID,
			"config", configPath)
		return false, nil
	}

	accounts[accountID] = entry
	whatsappCfg[whatsappConfigKeyAccounts] = accounts
	channelsCfg[openClawWhatsappChannelID] = whatsappCfg
	cfg["channels"] = channelsCfg
	if err := saveOpenClawJSONConfig(configPath, cfg); err != nil {
		return false, err
	}
	s.app.Logger.Info("openclaw: ensured whatsapp channel config in openclaw.json",
		"accountId", accountID,
		"enabled", true,
		"selfChatMode", true,
		"config", configPath)
	return true, nil
}

// PrepareWhatsappChannel ensures the bundled WhatsApp channel is enabled before QR login starts.
func (s *OpenClawChannelService) PrepareWhatsappChannel() (*WhatsappChannelPreparation, error) {
	startedAt := time.Now()
	accountID := normalizeWhatsappAccountID(s.readFirstWhatsappAccountIDFromOpenClawJSON())
	s.app.Logger.Info("openclaw: preparing whatsapp channel",
		"accountId", accountID)
	if err := s.ensureOpenClawReady(); err != nil {
		s.app.Logger.Warn("openclaw: failed to prepare whatsapp channel because openclaw is not ready",
			"accountId", accountID,
			"duration", time.Since(startedAt).String(),
			"error", err)
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), whatsappPluginInstallTimeout)
	defer cancel()
	if err := s.ensureOpenClawWhatsappPluginInstalled(ctx); err != nil {
		s.app.Logger.Warn("openclaw: failed to prepare whatsapp channel",
			"accountId", accountID,
			"duration", time.Since(startedAt).String(),
			"error", err)
		return nil, err
	}
	s.app.Logger.Info("openclaw: whatsapp channel prepared",
		"accountId", accountID,
		"duration", time.Since(startedAt).String())
	return &WhatsappChannelPreparation{Ready: true}, nil
}

func whatsappLoginOutputSuggestsMissingPluginOrChannel(blob string) bool {
	b := strings.ToLower(blob)
	if b == "" {
		return false
	}
	if strings.Contains(b, "whatsapp") && strings.Contains(b, "unknown channel") {
		return true
	}
	if strings.Contains(b, "provider unavailable") || strings.Contains(b, "provider unsupported") {
		return true
	}
	if strings.Contains(b, "plugin") {
		if strings.Contains(b, "not found") || strings.Contains(b, "not installed") ||
			strings.Contains(b, "missing") || strings.Contains(b, "eplugin") {
			return true
		}
	}
	return false
}

func whatsappLoginOutputSuggestsQRTimeout(blob string) bool {
	b := strings.ToLower(strings.TrimSpace(blob))
	if b == "" {
		return false
	}
	return strings.Contains(b, "timed out waiting for whatsapp qr") ||
		strings.Contains(b, "failed to get qr")
}

func whatsappLoginWaitMessageSuggestsRetry(blob string) bool {
	b := strings.ToLower(strings.TrimSpace(blob))
	if b == "" {
		return false
	}
	return strings.Contains(b, "restart required") ||
		strings.Contains(b, "login ended without a connection") ||
		strings.Contains(b, "still waiting for the qr scan")
}

func whatsappQRStartMessageSuggestsRetry(blob string) bool {
	b := strings.ToLower(strings.TrimSpace(blob))
	if b == "" {
		return true
	}
	return whatsappLoginOutputSuggestsQRTimeout(b) ||
		whatsappLoginWaitMessageSuggestsRetry(b)
}

type whatsappGatewayLoginStartResult struct {
	QRDataURL string `json:"qrDataUrl"`
	Message   string `json:"message"`
}

type whatsappGatewayLoginWaitResult struct {
	Connected bool   `json:"connected"`
	Message   string `json:"message"`
}

func buildWhatsappWebLoginStartParams(accountID string, timeout time.Duration, force bool) map[string]any {
	params := map[string]any{
		"timeoutMs": int(timeout / time.Millisecond),
		"verbose":   true,
	}
	if force {
		params["force"] = true
	}
	if id := strings.TrimSpace(accountID); id != "" {
		params["accountId"] = id
	}
	return params
}

func buildWhatsappWebLoginWaitParams(accountID string, timeout time.Duration) map[string]any {
	params := map[string]any{
		"timeoutMs": int(timeout / time.Millisecond),
	}
	if id := strings.TrimSpace(accountID); id != "" {
		params["accountId"] = id
	}
	return params
}

func wrapWhatsappQRStartError(err error) error {
	if err == nil {
		return nil
	}
	if whatsappLoginOutputSuggestsMissingPluginOrChannel(err.Error()) {
		return errs.Wrap("error.whatsapp_plugin_not_ready", err)
	}
	return errs.Wrap("error.whatsapp_qr_not_found", err)
}

func whatsappQRStartResultError(start *whatsappGatewayLoginStartResult) error {
	if start == nil {
		return errs.New("error.whatsapp_qr_not_found")
	}
	msg := strings.TrimSpace(start.Message)
	if msg == "" {
		return errs.New("error.whatsapp_qr_not_found")
	}
	if whatsappLoginOutputSuggestsMissingPluginOrChannel(msg) {
		return errs.Wrap("error.whatsapp_plugin_not_ready", errors.New(msg))
	}
	return errors.New(msg)
}

// GenerateWhatsappQRCode requests a QR data URL from the OpenClaw gateway's
// web.login.start flow instead of scraping the terminal QR output from the CLI.
func (s *OpenClawChannelService) GenerateWhatsappQRCode() (*WhatsappQRCodeResult, error) {
	startedAt := time.Now()
	if err := s.ensureOpenClawReady(); err != nil {
		s.app.Logger.Warn("openclaw: failed to generate whatsapp qr because openclaw is not ready",
			"duration", time.Since(startedAt).String(),
			"error", err)
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), whatsappPluginInstallTimeout)
	defer cancel()
	if err := s.ensureOpenClawWhatsappPluginInstalled(ctx); err != nil {
		s.app.Logger.Warn("openclaw: failed to generate whatsapp qr because channel is not ready",
			"duration", time.Since(startedAt).String(),
			"error", err)
		return nil, errs.Wrap("error.whatsapp_plugin_not_ready", err)
	}

	s.cancelAllWhatsappLoginSessions()

	accountID := normalizeWhatsappAccountID(s.readFirstWhatsappAccountIDFromOpenClawJSON())
	startQRCode := func(resetSession bool, force bool, qrTimeout time.Duration) (*whatsappGatewayLoginStartResult, error) {
		if resetSession {
			logoutCtx, logoutCancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer logoutCancel()
			s.app.Logger.Info("openclaw: resetting whatsapp web session before qr retry",
				"accountId", accountID)
			if err := s.runOpenClawWhatsappLogout(logoutCtx, accountID); err != nil {
				s.app.Logger.Warn("openclaw: failed to reset whatsapp web session before qr retry",
					"accountId", accountID,
					"error", err)
			} else {
				s.app.Logger.Info("openclaw: reset whatsapp web session before qr retry succeeded",
					"accountId", accountID)
			}
		}

		requestTimeout := qrTimeout + 10*time.Second
		reqCtx, reqCancel := context.WithTimeout(context.Background(), requestTimeout)
		defer reqCancel()
		params := buildWhatsappWebLoginStartParams(accountID, qrTimeout, force)
		s.app.Logger.Info("openclaw: requesting whatsapp qr code",
			"accountId", accountID,
			"request_timeout", requestTimeout.String(),
			"qr_timeout", qrTimeout.String(),
			"force", params["force"],
			"verbose", params["verbose"],
			"resetSession", resetSession)

		var start whatsappGatewayLoginStartResult
		if err := s.openclawManager.QueryRequest(reqCtx, "web.login.start", params, &start); err != nil {
			s.app.Logger.Warn("openclaw: whatsapp qr request failed",
				"accountId", accountID,
				"duration", time.Since(startedAt).String(),
				"deadline_exceeded", errors.Is(err, context.DeadlineExceeded) || errors.Is(reqCtx.Err(), context.DeadlineExceeded),
				"resetSession", resetSession,
				"error", err)
			return nil, err
		}
		s.app.Logger.Info("openclaw: whatsapp qr request returned",
			"accountId", accountID,
			"duration", time.Since(startedAt).String(),
			"has_qr_data_url", strings.TrimSpace(start.QRDataURL) != "",
			"resetSession", resetSession,
			"message", strings.TrimSpace(start.Message))
		return &start, nil
	}

	start, startErr := startQRCode(false, false, whatsappQRFastStartTimeout)
	if startErr != nil {
		if !whatsappLoginOutputSuggestsMissingPluginOrChannel(startErr.Error()) {
			s.app.Logger.Warn("openclaw: fast whatsapp qr request failed, retrying with reset",
				"accountId", accountID,
				"duration", time.Since(startedAt).String(),
				"error", startErr)
			start, startErr = startQRCode(true, true, whatsappQRStartTimeout)
		}
	}
	if startErr != nil {
		return nil, wrapWhatsappQRStartError(startErr)
	}
	if start != nil && strings.TrimSpace(start.QRDataURL) == "" && whatsappQRStartMessageSuggestsRetry(start.Message) {
		s.app.Logger.Warn("openclaw: whatsapp qr start returned transient state, retrying after reset",
			"accountId", accountID,
			"duration", time.Since(startedAt).String(),
			"message", strings.TrimSpace(start.Message))
		start, startErr = startQRCode(true, true, whatsappQRStartTimeout)
		if startErr != nil {
			return nil, wrapWhatsappQRStartError(startErr)
		}
	}
	if start == nil || strings.TrimSpace(start.QRDataURL) == "" {
		s.app.Logger.Warn("openclaw: whatsapp qr response missing qr data url",
			"accountId", accountID,
			"duration", time.Since(startedAt).String(),
			"message", strings.TrimSpace(start.Message))
		return nil, whatsappQRStartResultError(start)
	}

	sessionKey := uuid.NewString()
	s.whatsappLoginMu.Lock()
	s.ensureWhatsappLoginMap()
	s.whatsappLogins[sessionKey] = &whatsappLoginSession{accountID: accountID}
	s.whatsappLoginMu.Unlock()
	s.app.Logger.Info("openclaw: whatsapp qr code generated",
		"accountId", accountID,
		"sessionKey", shortenWhatsappSessionKey(sessionKey),
		"duration", time.Since(startedAt).String())

	return &WhatsappQRCodeResult{
		QrcodeDataURL: strings.TrimSpace(start.QRDataURL),
		SessionKey:    sessionKey,
	}, nil
}

// CancelWhatsappLogin clears the local session mapping for the pending QR flow.
func (s *OpenClawChannelService) CancelWhatsappLogin(sessionKey string) {
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" {
		s.cancelAllWhatsappLoginSessions()
		return
	}
	s.whatsappLoginMu.Lock()
	s.ensureWhatsappLoginMap()
	delete(s.whatsappLogins, sessionKey)
	s.whatsappLoginMu.Unlock()
}

// WaitForWhatsappLogin waits on the OpenClaw gateway's QR-login session, then creates a local channel row.
func (s *OpenClawChannelService) WaitForWhatsappLogin(sessionKey string, channelName string) (*WhatsappLoginResult, error) {
	startedAt := time.Now()
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" {
		s.app.Logger.Warn("openclaw: wait for whatsapp login called without session key")
		return nil, errs.New("error.whatsapp_login_failed")
	}
	s.whatsappLoginMu.Lock()
	s.ensureWhatsappLoginMap()
	sess, ok := s.whatsappLogins[sessionKey]
	s.whatsappLoginMu.Unlock()
	if !ok || sess == nil {
		s.app.Logger.Warn("openclaw: wait for whatsapp login called with unknown session",
			"sessionKey", shortenWhatsappSessionKey(sessionKey))
		return nil, errs.New("error.whatsapp_login_failed")
	}
	accountID := normalizeWhatsappAccountID(sess.accountID)
	defer func() {
		s.whatsappLoginMu.Lock()
		delete(s.whatsappLogins, sessionKey)
		s.whatsappLoginMu.Unlock()
	}()

	s.app.Logger.Info("openclaw: waiting for whatsapp login confirmation",
		"accountId", accountID,
		"sessionKey", shortenWhatsappSessionKey(sessionKey),
		"timeout", whatsappLoginWaitTimeout.String())

	deadline := time.Now().Add(whatsappLoginWaitTimeout)
	attempt := 0
	var waitResp whatsappGatewayLoginWaitResult
	for {
		attempt++
		remaining := time.Until(deadline)
		if remaining <= 0 {
			s.app.Logger.Warn("openclaw: whatsapp login wait exhausted overall timeout",
				"accountId", accountID,
				"sessionKey", shortenWhatsappSessionKey(sessionKey),
				"attempt", attempt,
				"duration", time.Since(startedAt).String())
			return &WhatsappLoginResult{Connected: false, Message: "timeout"}, errs.New("error.whatsapp_login_timeout")
		}

		params := buildWhatsappWebLoginWaitParams(accountID, remaining)
		waitCtx, waitCancel := context.WithTimeout(context.Background(), remaining)
		waitErr := s.openclawManager.QueryRequest(waitCtx, "web.login.wait", params, &waitResp)
		waitCancel()

		if waitErr != nil {
			s.app.Logger.Warn("openclaw: whatsapp login wait failed",
				"accountId", accountID,
				"sessionKey", shortenWhatsappSessionKey(sessionKey),
				"attempt", attempt,
				"duration", time.Since(startedAt).String(),
				"deadline_exceeded", errors.Is(waitErr, context.DeadlineExceeded) || errors.Is(waitCtx.Err(), context.DeadlineExceeded),
				"error", waitErr)
			if waitCtx.Err() == context.DeadlineExceeded {
				return &WhatsappLoginResult{Connected: false, Message: "timeout"}, errs.New("error.whatsapp_login_timeout")
			}
			return &WhatsappLoginResult{
				Connected: false,
				Message:   waitErr.Error(),
			}, nil
		}

		waitMessage := strings.TrimSpace(waitResp.Message)
		s.app.Logger.Info("openclaw: whatsapp login wait returned",
			"accountId", accountID,
			"sessionKey", shortenWhatsappSessionKey(sessionKey),
			"attempt", attempt,
			"duration", time.Since(startedAt).String(),
			"connected", waitResp.Connected,
			"message", waitMessage)
		if waitResp.Connected {
			break
		}
		if !whatsappLoginWaitMessageSuggestsRetry(waitMessage) {
			return &WhatsappLoginResult{
				Connected: false,
				Message:   waitMessage,
			}, nil
		}

		s.app.Logger.Info("openclaw: whatsapp login wait returned transient state, retrying",
			"accountId", accountID,
			"sessionKey", shortenWhatsappSessionKey(sessionKey),
			"attempt", attempt,
			"remaining", time.Until(deadline).String(),
			"message", waitMessage)

		retryDelay := whatsappLoginRetryDelay
		if left := time.Until(deadline); left < retryDelay {
			retryDelay = left
		}
		if retryDelay > 0 {
			time.Sleep(retryDelay)
		}
	}

	accountID = strings.TrimSpace(sess.accountID)
	if accountID == "" {
		accountID = s.readFirstWhatsappAccountIDFromOpenClawJSON()
	}
	name := strings.TrimSpace(channelName)
	extraConfig, extraErr := json.Marshal(appCredentialsJSON{
		Platform:  channels.PlatformWhatsapp,
		AccountID: accountID,
	})
	if extraErr != nil {
		extraConfig = []byte(fmt.Sprintf(`{"platform":"whatsapp","account_id":%q}`, accountID))
	}

	ch, chErr := s.upsertWhatsappChannelRecord(accountID, name, string(extraConfig))
	if chErr != nil {
		s.app.Logger.Warn("openclaw: failed to upsert whatsapp channel after login", "error", chErr)
		return &WhatsappLoginResult{Connected: false, Message: chErr.Error()}, nil
	}
	s.app.Logger.Info("openclaw: whatsapp channel record ready", "channel_id", ch.ID, "accountId", accountID)

	var agentID int64
	targetOpenClawAgentID := strings.TrimSpace(s.readWhatsappConfiguredOpenClawAgentID(accountID))
	if targetOpenClawAgentID == "" {
		targetOpenClawAgentID = define.OpenClawMainAgentID
	}
	targetLocalAgent, ensureAgentErr := s.agentsSvc.EnsureAgentRecordByOpenClawAgentID(targetOpenClawAgentID, "")
	if ensureAgentErr != nil {
		s.app.Logger.Warn("openclaw: failed to ensure whatsapp target agent before bind", "channel_id", ch.ID, "openclaw_agent_id", targetOpenClawAgentID, "error", ensureAgentErr)
	} else if targetLocalAgent != nil {
		if bindErr := s.channelSvc.BindAgent(ch.ID, targetLocalAgent.ID); bindErr != nil {
			s.app.Logger.Warn("openclaw: failed to bind whatsapp channel to configured agent", "channel_id", ch.ID, "agent_id", targetLocalAgent.ID, "openclaw_agent_id", targetOpenClawAgentID, "error", bindErr)
		} else {
			agentID = targetLocalAgent.ID
			s.app.Logger.Info("openclaw: whatsapp channel bound to configured assistant", "channel_id", ch.ID, "agent_id", agentID, "openclaw_agent_id", targetOpenClawAgentID)
		}
	}

	if agentID > 0 {
		if rErr := s.syncChannelRoutingBinding(ch.ID, agentID); rErr != nil {
			s.app.Logger.Warn("openclaw: failed to apply whatsapp openclaw routing after login", "channel_id", ch.ID, "error", rErr)
		}
	} else {
		s.app.Logger.Warn("openclaw: whatsapp channel left unbound after login", "channel_id", ch.ID, "accountId", accountID)
	}

	if err := s.setChannelOnlineStatus(context.Background(), ch.ID, true); err != nil {
		s.app.Logger.Warn("openclaw: failed to set whatsapp channel online", "channel_id", ch.ID, "error", err)
	}

	return &WhatsappLoginResult{
		Connected: true,
		AccountID: accountID,
		ChannelID: ch.ID,
	}, nil
}

func (s *OpenClawChannelService) readFirstWhatsappAccountIDFromOpenClawJSON() string {
	cfg, _, err := loadOpenClawJSONConfig()
	if err != nil {
		return whatsappDefaultAccountID
	}
	return firstConfiguredWhatsappAccountID(whatsappChannelConfigFromRoot(cfg))
}

func (s *OpenClawChannelService) readWhatsappConfiguredOpenClawAgentID(accountID string) string {
	cfg, _, err := loadOpenClawJSONConfig()
	if err != nil {
		return ""
	}
	return whatsappConfiguredAgentIDFromConfig(cfg, accountID)
}

func (s *OpenClawChannelService) findWhatsappChannelByAccountID(accountID string) (*channelModel, error) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return nil, nil
	}

	db, err := s.db()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var models []channelModel
	if err := db.NewSelect().
		Model(&models).
		Where("ch.platform = ?", channels.PlatformWhatsapp).
		Where("ch.openclaw_scope = ?", true).
		OrderExpr("ch.id DESC").
		Scan(ctx); err != nil {
		return nil, errs.Wrap("error.channel_read_failed", err)
	}

	for i := range models {
		if extractWhatsappAccountID(models[i].ExtraConfig) == accountID {
			return &models[i], nil
		}
	}
	return nil, nil
}

func (s *OpenClawChannelService) upsertWhatsappChannelRecord(accountID, name, extraConfig string) (*channels.Channel, error) {
	name = strings.TrimSpace(name)
	existing, err := s.findWhatsappChannelByAccountID(accountID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		if name == "" {
			name, err = s.generateNextWhatsappChannelName()
			if err != nil {
				return nil, err
			}
		}
		return s.channelSvc.CreateChannel(channels.CreateChannelInput{
			Platform:       channels.PlatformWhatsapp,
			Name:           name,
			Avatar:         "",
			ConnectionType: channels.ConnTypeGateway,
			ExtraConfig:    extraConfig,
			OpenClawScope:  true,
		})
	}

	var input channels.UpdateChannelInput
	var changed bool
	if name != "" && strings.TrimSpace(existing.Name) != name {
		input.Name = &name
		changed = true
	}
	if strings.TrimSpace(existing.ExtraConfig) != strings.TrimSpace(extraConfig) {
		extraConfig = strings.TrimSpace(extraConfig)
		input.ExtraConfig = &extraConfig
		changed = true
	}
	if !changed {
		dto := existing.toDTO()
		return &dto, nil
	}
	return s.channelSvc.UpdateChannel(existing.ID, input)
}

func (s *OpenClawChannelService) generateNextWhatsappChannelName() (string, error) {
	db, err := s.db()
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	names := make([]string, 0)
	if err := db.NewSelect().
		Model((*channelModel)(nil)).
		Column("name").
		Where("platform = ?", channels.PlatformWhatsapp).
		Scan(ctx, &names); err != nil {
		return "", errs.Wrap("error.channel_list_failed", err)
	}

	return nextWhatsappAutoChannelName(names), nil
}

func (s *OpenClawChannelService) runOpenClawWhatsappLogout(ctx context.Context, accountID string) error {
	args := []string{"channels", "logout", "--channel", openClawWhatsappChannelID}
	if id := strings.TrimSpace(accountID); id != "" && !strings.EqualFold(id, "default") {
		args = append(args, "--account", id)
	}
	_, err := s.execOpenClawCLIWithRetry(ctx, args...)
	return err
}

func (s *OpenClawChannelService) purgeWhatsappChannelOpenClawIntegration(m *channelModel) error {
	accountID := openClawManagedAccountID(channels.PlatformWhatsapp, m.ID, m.ExtraConfig)
	ctx, cancel := context.WithTimeout(context.Background(), openClawChannelSyncTimeout)
	defer cancel()
	if err := s.runOpenClawWhatsappLogout(ctx, accountID); err != nil {
		s.app.Logger.Warn("openclaw: whatsapp logout during purge failed", "error", err)
	}
	if err := s.removeManagedRouteBinding(openClawWhatsappChannelID, accountID); err != nil {
		return err
	}
	return s.restartOpenClawGateway()
}

func (s *OpenClawChannelService) connectWhatsappViaPlugin(id int64, m *channelModel) error {
	if m.AgentID == 0 {
		return errs.New("error.channel_connect_requires_agent")
	}
	readyCtx, readyCancel := context.WithTimeout(context.Background(), whatsappPluginInstallTimeout)
	defer readyCancel()
	if err := s.ensureOpenClawWhatsappPluginInstalled(readyCtx); err != nil {
		return errs.Wrap("error.whatsapp_plugin_not_ready", err)
	}
	syncCtx, syncCancel := context.WithTimeout(context.Background(), openClawChannelSyncTimeout)
	defer syncCancel()
	if err := s.syncChannelRoutingBinding(id, m.AgentID); err != nil {
		return wrapOpenClawSyncErr(err, "error.channel_connect_failed", map[string]any{"Name": m.Name})
	}
	return s.setChannelOnlineStatus(syncCtx, id, true)
}

func (s *OpenClawChannelService) disconnectWhatsappViaPlugin(id int64, m *channelModel) error {
	ctx, cancel := context.WithTimeout(context.Background(), openClawChannelSyncTimeout)
	defer cancel()
	routeAccount := openClawManagedAccountID(channels.PlatformWhatsapp, id, m.ExtraConfig)
	if err := s.removeManagedRouteBinding(openClawWhatsappChannelID, routeAccount); err != nil {
		s.app.Logger.Warn("openclaw: whatsapp route remove on disconnect failed", "error", err)
	}
	if err := s.restartOpenClawGateway(); err != nil {
		return errs.Wrap("error.channel_disconnect_failed", err)
	}
	return s.setChannelOnlineStatus(ctx, id, false)
}
