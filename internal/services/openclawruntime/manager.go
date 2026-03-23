package openclawruntime

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"chatclaw/internal/services/settings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type Manager struct {
	app   *application.App
	store *configStore

	opMu sync.Mutex
	mu   sync.RWMutex

	status       RuntimeStatus
	gatewayState GatewayConnectionState
	client       *GatewayClient
	readyAt      time.Time
	readyHooks   []func()
	process      *exec.Cmd
	processPID   int
	processDone  chan error
	processLog   *os.File

	expectedStopPID int
	shuttingDown    bool
	reconnecting    atomic.Bool
}

func NewManager(app *application.App, settingsSvc *settings.SettingsService) *Manager {
	store := newConfigStore(settingsSvc)
	cfg := store.Get()
	return &Manager{
		app:   app,
		store: store,
		status: RuntimeStatus{
			Phase:      PhaseIdle,
			GatewayURL: gatewayURL(cfg.GatewayPort),
		},
	}
}

func (m *Manager) Start() {
	go func() { _ = m.reconcile(false) }()
}

func (m *Manager) Shutdown() {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	m.mu.Lock()
	m.shuttingDown = true
	m.mu.Unlock()
	m.closeClient()
	m.stopProcess()
}

func (m *Manager) GetStatus() RuntimeStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status
}

func (m *Manager) GetGatewayState() GatewayConnectionState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.gatewayState
}

func (m *Manager) RestartGateway() (RuntimeStatus, error) {
	err := m.reconcile(true)
	return m.GetStatus(), err
}

// reconcile is the single entry point for lifecycle management:
// resolve bundle → verify install → start process → connect WebSocket.
func (m *Manager) reconcile(restart bool) error {
	m.opMu.Lock()
	defer m.opMu.Unlock()

	if m.isShuttingDown() {
		return fmt.Errorf("runtime is shutting down")
	}

	cfg := m.store.Get()

	fail := func(msg string, err error, version string, pid int) error {
		m.app.Logger.Error("openclaw: "+msg, "error", err)
		m.broadcastStatus(RuntimeStatus{
			Phase: PhaseError, Message: err.Error(),
			InstalledVersion: version, GatewayPID: pid,
			GatewayURL: gatewayURL(cfg.GatewayPort),
		})
		return err
	}

	// Fast path: already running and connected
	if !restart {
		m.mu.RLock()
		ready := m.process != nil && m.client != nil
		m.mu.RUnlock()
		if ready {
			return nil
		}
	}

	bundle, err := resolveBundledRuntime()
	if err != nil {
		return fail("resolveBundledRuntime", err, "", 0)
	}

	m.broadcastStatus(RuntimeStatus{
		Phase: PhaseStarting, Message: "Checking bundled OpenClaw runtime",
		GatewayURL: gatewayURL(cfg.GatewayPort),
	})

	if restart {
		m.closeClient()
		m.stopProcess()
	}

	version, err := verifyInstalled(bundle)
	if err != nil {
		return fail("verifyInstalled", err, "", 0)
	}

	if err := ensureOpenResponsesEnabled(bundle); err != nil {
		return fail("ensureOpenResponsesEnabled", err, version, 0)
	}

	// Start process if needed
	m.mu.RLock()
	needProcess := m.process == nil
	pid := m.processPID
	m.mu.RUnlock()

	if needProcess {
		if err := m.startProcess(cfg, bundle); err != nil {
			return fail("startProcess", err, version, 0)
		}
		m.mu.RLock()
		pid = m.processPID
		m.mu.RUnlock()
	}

	// Connect client if needed
	m.mu.RLock()
	needClient := m.client == nil
	m.mu.RUnlock()

	if needClient {
		m.broadcastStatus(RuntimeStatus{
			Phase: PhaseConnecting, Message: "Connecting to OpenClaw Gateway",
			InstalledVersion: version, GatewayPID: pid,
			GatewayURL: gatewayURL(cfg.GatewayPort),
		})
		if err := m.connectClient(cfg, bundle); err != nil {
			return fail("connectClient", err, version, pid)
		}
	}

	m.mu.Lock()
	m.readyAt = time.Now()
	m.mu.Unlock()

	m.broadcastStatus(RuntimeStatus{
		Phase: PhaseConnected, Message: "OpenClaw Gateway connected",
		InstalledVersion: version, GatewayPID: pid,
		GatewayURL: gatewayURL(cfg.GatewayPort),
	})
	m.broadcastGatewayState(GatewayConnectionState{Connected: true, Authenticated: true})
	m.notifyReadyHooks()

	return nil
}

// --- Process management ---

func (m *Manager) startProcess(cfg OpenClawConfig, bundle *bundledRuntime) error {
	logFile, err := openGatewayLogFile(bundle.LogsDir)
	if err != nil {
		return err
	}

	cmd := exec.Command(bundle.CLIPath,
		"gateway", "run",
		"--allow-unconfigured",
		"--port", strconv.Itoa(cfg.GatewayPort),
		"--bind", "loopback",
		"--auth", "token",
		"--token", cfg.GatewayToken,
		"--force",
	)
	cmd.Env = buildGatewayEnv(cfg, bundle)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Dir = bundle.Root

	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return fmt.Errorf("start openclaw gateway: %w", err)
	}

	done := make(chan error, 1)
	pid := cmd.Process.Pid
	m.mu.Lock()
	m.process = cmd
	m.processPID = pid
	m.processDone = done
	m.processLog = logFile
	m.mu.Unlock()

	go func() {
		waitErr := cmd.Wait()
		done <- waitErr
		_ = logFile.Close()
		m.handleProcessExit(pid, waitErr)
	}()

	m.broadcastStatus(RuntimeStatus{
		Phase: PhaseStarting, Message: "Starting OpenClaw Gateway",
		GatewayPID: pid, GatewayURL: gatewayURL(cfg.GatewayPort),
	})
	return nil
}

func (m *Manager) stopProcess() {
	m.mu.Lock()
	if m.process == nil {
		m.mu.Unlock()
		return
	}
	proc := m.process
	done := m.processDone
	m.expectedStopPID = m.processPID
	m.process = nil
	m.processPID = 0
	m.processDone = nil
	m.processLog = nil
	m.mu.Unlock()

	if proc.Process != nil {
		if runtime.GOOS == "windows" {
			_ = proc.Process.Kill()
		} else {
			_ = proc.Process.Signal(os.Interrupt)
		}
	}
	if done != nil {
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			if proc.Process != nil {
				_ = proc.Process.Kill()
			}
			select {
			case <-done:
			case <-time.After(2 * time.Second):
			}
		}
	}
}

func (m *Manager) handleProcessExit(pid int, exitErr error) {
	m.app.Logger.Info("openclaw: process exited", "pid", pid, "error", exitErr)
	m.mu.Lock()
	intentional := pid == m.expectedStopPID
	if intentional {
		m.expectedStopPID = 0
	}
	if m.processPID == pid {
		m.process = nil
		m.processPID = 0
		m.processDone = nil
		m.processLog = nil
		m.client = nil
		m.readyAt = time.Time{}
	}
	shuttingDown := m.shuttingDown
	m.mu.Unlock()

	if shuttingDown || intentional {
		return
	}

	if !m.reconnecting.CompareAndSwap(false, true) {
		m.app.Logger.Info("openclaw: skipping process-exit reconnect, already in progress", "pid", pid)
		return
	}

	m.broadcastStatus(RuntimeStatus{
		Phase: PhaseRestarting, Message: "OpenClaw Gateway exited, restarting",
		GatewayURL: gatewayURL(m.store.Get().GatewayPort),
	})
	go func() {
		defer m.reconnecting.Store(false)
		time.Sleep(1500 * time.Millisecond)
		_ = m.reconcile(false)
	}()
}

// --- WebSocket client management ---

func (m *Manager) connectClient(cfg OpenClawConfig, bundle *bundledRuntime) error {
	identity, err := loadOrCreateDeviceIdentity(bundle.StateDir)
	if err != nil {
		return err
	}
	storedTok, _ := loadStoredDeviceToken(bundle.StateDir, clientRole)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var lastErr error
	for {
		if ctx.Err() != nil {
			if lastErr != nil {
				return lastErr
			}
			return ctx.Err()
		}

		m.mu.RLock()
		alive := m.process != nil
		m.mu.RUnlock()
		if !alive {
			if lastErr != nil {
				return fmt.Errorf("gateway process exited: %w", lastErr)
			}
			return fmt.Errorf("gateway process exited before connection established")
		}

		client := NewGatewayClient(gatewayClientOptions{
			URL:             gatewayWebSocketURL(cfg.GatewayPort),
			Token:           cfg.GatewayToken,
			DeviceIdentity:  identity,
			StoredDeviceTok: storedTok,
			Scopes:          []string{"operator.read", "operator.write", "operator.admin"},
			OnEvent:         func(GatewayEventFrame) {},
			OnDisconnect:    m.handleGatewayDisconnect,
		})
		hello, err := client.Connect(ctx)
		if err == nil {
			if hello.Auth != nil && hello.Auth.DeviceToken != "" {
				_ = storeDeviceToken(bundle.StateDir, hello.Auth.Role, hello.Auth.DeviceToken, hello.Auth.Scopes)
			}
			m.mu.Lock()
			m.client = client
			m.mu.Unlock()
			return nil
		}
		lastErr = err
		if !shouldRetryConnect(err) {
			return err
		}
		select {
		case <-ctx.Done():
			return lastErr
		case <-time.After(300 * time.Millisecond):
		}
	}
}

func (m *Manager) closeClient() {
	m.mu.Lock()
	client := m.client
	m.client = nil
	m.readyAt = time.Time{}
	m.mu.Unlock()
	if client != nil {
		_ = client.Close()
	}
}

func (m *Manager) handleGatewayDisconnect(err error) {
	m.app.Logger.Info("openclaw: gateway disconnected", "error", err)
	m.mu.Lock()
	if m.shuttingDown {
		m.mu.Unlock()
		return
	}
	m.client = nil
	m.readyAt = time.Time{}
	m.mu.Unlock()

	m.broadcastGatewayState(GatewayConnectionState{Reconnecting: true, LastError: errStr(err)})

	if !m.reconnecting.CompareAndSwap(false, true) {
		m.app.Logger.Info("openclaw: skipping disconnect reconnect, already in progress")
		return
	}

	go func() {
		defer m.reconnecting.Store(false)
		time.Sleep(500 * time.Millisecond)
		_ = m.reconcile(false)
	}()
}

// --- Status broadcasting ---

func (m *Manager) broadcastStatus(s RuntimeStatus) {
	m.mu.Lock()
	m.status = s
	m.mu.Unlock()
	if m.app != nil {
		m.app.Event.Emit(EventStatus, s)
	}
}

func (m *Manager) broadcastGatewayState(gs GatewayConnectionState) {
	m.mu.Lock()
	m.gatewayState = gs
	m.mu.Unlock()
	if m.app != nil {
		m.app.Event.Emit(EventGatewayState, gs)
	}
}

func (m *Manager) isShuttingDown() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.shuttingDown
}

func (m *Manager) IsReady() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.client != nil && !m.readyAt.IsZero()
}

// GatewayURL returns the HTTP base URL of the running OpenClaw Gateway.
func (m *Manager) GatewayURL() string {
	return gatewayURL(m.store.Get().GatewayPort)
}

// GatewayToken returns the auth token for the running OpenClaw Gateway.
func (m *Manager) GatewayToken() string {
	return m.store.Get().GatewayToken
}

func (m *Manager) Request(ctx context.Context, method string, params any, out any) error {
	m.mu.RLock()
	client := m.client
	m.mu.RUnlock()
	if client == nil {
		return errors.New("gateway websocket is not connected")
	}
	return client.Request(ctx, method, params, out)
}

func (m *Manager) RegisterReadyHook(fn func()) {
	if fn == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.readyHooks = append(m.readyHooks, fn)
}

func (m *Manager) notifyReadyHooks() {
	m.mu.RLock()
	hooks := append([]func(){}, m.readyHooks...)
	m.mu.RUnlock()
	for _, fn := range hooks {
		go fn()
	}
}

// --- Helpers ---

func verifyInstalled(bundle *bundledRuntime) (string, error) {
	if _, err := os.Stat(bundle.CLIPath); err != nil {
		return "", fmt.Errorf("verify bundled OpenClaw runtime: %w", err)
	}
	out, err := exec.Command(bundle.CLIPath, "--version").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("check openclaw version: %w", err)
	}
	version, err := parseVersionOutput(string(out))
	if err != nil {
		return "", err
	}
	if bundle.Manifest.OpenClawVersion != "" && bundle.Manifest.OpenClawVersion != version {
		return "", fmt.Errorf("bundled OpenClaw version mismatch: manifest=%s cli=%s",
			bundle.Manifest.OpenClawVersion, version)
	}
	return version, nil
}

// ensureOpenResponsesEnabled uses `openclaw config set` to enable the
// OpenResponses HTTP endpoint via the standard CLI.
func ensureOpenResponsesEnabled(bundle *bundledRuntime) error {
	if err := os.MkdirAll(bundle.StateDir, 0o700); err != nil {
		return fmt.Errorf("create openclaw state dir: %w", err)
	}

	env := []string{
		"OPENCLAW_CONFIG_PATH=" + bundle.ConfigPath,
		"OPENCLAW_STATE_DIR=" + bundle.StateDir,
	}
	for _, entry := range os.Environ() {
		if k, _, ok := strings.Cut(entry, "="); ok {
			if k == "OPENCLAW_CONFIG_PATH" || k == "OPENCLAW_STATE_DIR" {
				continue
			}
		}
		env = append(env, entry)
	}

	cmd := exec.Command(bundle.CLIPath, "config", "set",
		"gateway.http.endpoints.responses.enabled", "true")
	cmd.Env = env
	cmd.Dir = bundle.Root
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("openclaw config set responses.enabled: %w: %s", err, string(out))
	}
	return nil
}

func parseVersionOutput(output string) (string, error) {
	for _, field := range strings.Fields(strings.TrimSpace(output)) {
		candidate := strings.TrimPrefix(strings.Trim(field, "(),"), "v")
		if strings.Count(candidate, ".") >= 2 && isVersionChars(candidate) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("could not parse openclaw version from %q", strings.TrimSpace(output))
}

func isVersionChars(s string) bool {
	for _, r := range s {
		if (r < '0' || r > '9') && r != '.' {
			return false
		}
	}
	return true
}

func gatewayURL(port int) string {
	return fmt.Sprintf("http://127.0.0.1:%d", port)
}

func gatewayWebSocketURL(port int) string {
	return fmt.Sprintf("ws://127.0.0.1:%d/ws", port)
}

func buildGatewayEnv(cfg OpenClawConfig, bundle *bundledRuntime) []string {
	envMap := map[string]string{}
	for _, entry := range os.Environ() {
		if k, v, ok := strings.Cut(entry, "="); ok {
			envMap[k] = v
		}
	}
	envMap["OPENCLAW_STATE_DIR"] = bundle.StateDir
	envMap["OPENCLAW_CONFIG_PATH"] = bundle.ConfigPath
	envMap["OPENCLAW_SKIP_CHANNELS"] = "1"
	envMap["OPENCLAW_SKIP_CANVAS_HOST"] = "1"
	envMap["OPENCLAW_EMBEDDED_IN"] = "ChatClaw"

	var pathKey, nodeBin string
	if runtime.GOOS == "windows" {
		pathKey, nodeBin = "Path", filepath.Join(bundle.Root, "tools", "node")
	} else {
		pathKey, nodeBin = "PATH", filepath.Join(bundle.Root, "tools", "node", "bin")
	}
	if cur := envMap[pathKey]; cur != "" {
		envMap[pathKey] = nodeBin + string(os.PathListSeparator) + cur
	} else {
		envMap[pathKey] = nodeBin
	}

	result := make([]string, 0, len(envMap))
	for k, v := range envMap {
		result = append(result, k+"="+v)
	}
	return result
}

func openGatewayLogFile(logsDir string) (*os.File, error) {
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		return nil, fmt.Errorf("create logs dir: %w", err)
	}
	return os.OpenFile(filepath.Join(logsDir, "openclaw-gateway.log"),
		os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
}

func errStr(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func shouldRetryConnect(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	msg := strings.ToLower(err.Error())
	for _, s := range []string{"connection refused", "connection reset", "broken pipe",
		"unexpected eof", "read connect challenge", "dial gateway websocket"} {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}

