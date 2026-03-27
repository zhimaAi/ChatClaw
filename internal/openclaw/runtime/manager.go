package openclawruntime

import (
	"context"
	"encoding/json"
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

// EventListener receives gateway events. Parameters are event name and raw JSON payload.
type EventListener func(event string, payload json.RawMessage)

// ToolchainServiceIF is the subset of *toolchain.ToolchainService needed by Manager.
// Implemented as an interface to avoid a cyclic import.
type ToolchainServiceIF interface {
	InstallOpenClawRuntime() error
}

type Manager struct {
	app             *application.App
	store           *configStore
	toolchainSvc    ToolchainServiceIF

	opMu sync.Mutex
	mu   sync.RWMutex

	status       RuntimeStatus
	gatewayState GatewayConnectionState
	client       *GatewayClient
	queryClient  *GatewayClient // separate connection for queries during agent runs
	readyAt      time.Time
	readyHooks   []func()
	process      *exec.Cmd
	processPID   int
	processDone  chan error
	processLog   *os.File

	expectedStopPID int
	shuttingDown    bool
	reconnecting    atomic.Bool

	eventListenersMu sync.RWMutex
	eventListeners   map[string]EventListener // keyed by caller-chosen ID
}

func gatewayOperatorScopes() []string {
	return []string{"operator.read", "operator.write", "operator.admin"}
}

func gatewayQueryOperatorScopes() []string {
	return gatewayOperatorScopes()
}

func NewManager(app *application.App, settingsSvc *settings.SettingsService, toolchainSvc ToolchainServiceIF) *Manager {
	store := newConfigStore(settingsSvc)
	cfg := store.Get()
	m := &Manager{
		app:          app,
		store:        store,
		toolchainSvc: toolchainSvc,
		status: RuntimeStatus{
			Phase:      PhaseIdle,
			GatewayURL: gatewayURL(cfg.GatewayPort),
		},
		eventListeners: make(map[string]EventListener),
	}
	return m
}

// SetToolchainService injects the toolchain service after construction.
// Call this before Manager.Start() so the OSS fallback is available.
func (m *Manager) SetToolchainService(svc ToolchainServiceIF) {
	m.toolchainSvc = svc
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

// InstallAndStartRuntime downloads the OpenClaw runtime from OSS and starts the gateway.
// This is the "OSS install" equivalent of UpgradeRuntime: it installs the runtime bundle,
// stops any existing gateway, and starts a new one using the newly installed runtime.
func (m *Manager) InstallAndStartRuntime() (*RuntimeUpgradeResult, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()

	cfg := m.store.Get()

	// Broadcast installing state
	m.broadcastStatus(RuntimeStatus{
		Phase:       PhaseUpgrading,
		Message:     "Downloading OpenClaw runtime from OSS...",
		GatewayURL: gatewayURL(cfg.GatewayPort),
	})
	m.closeClient()
	m.stopProcess()

	if err := m.toolchainSvc.InstallOpenClawRuntime(); err != nil {
		_ = m.reconcileLocked(false)
		return nil, fmt.Errorf("OSS runtime install: %w", err)
	}

	bundle, err := resolveBundledRuntime()
	if err != nil {
		_ = m.reconcileLocked(false)
		return nil, fmt.Errorf("resolveBundledRuntime after OSS install: %w", err)
	}
	installedVersion, err := verifyInstalled(bundle)
	if err != nil {
		_ = m.reconcileLocked(false)
		return nil, fmt.Errorf("verifyInstalled after OSS install: %w", err)
	}

	// Activate the newly installed runtime
	if err := m.reconcileLocked(false); err != nil {
		_ = m.reconcileLocked(false)
		return nil, fmt.Errorf("reconcile after OSS install: %w", err)
	}

	status := m.GetStatus()
	return &RuntimeUpgradeResult{
		PreviousVersion: "",
		CurrentVersion:  installedVersion,
		LatestVersion:   installedVersion,
		Upgraded:        true,
		RuntimeSource:   status.RuntimeSource,
		RuntimePath:     status.RuntimePath,
	}, nil
}

func (m *Manager) UpgradeRuntime() (*RuntimeUpgradeResult, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	return m.upgradeRuntimeLocked()
}

// reconcile is the single entry point for lifecycle management:
// resolve bundle → verify install → start process → connect WebSocket.
func (m *Manager) reconcile(restart bool) error {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	return m.reconcileLocked(restart)
}

func (m *Manager) reconcileLocked(restart bool) error {
	if m.isShuttingDown() {
		return fmt.Errorf("runtime is shutting down")
	}

	cfg := m.store.Get()

	fail := func(msg string, err error, version string, pid int) error {
		m.app.Logger.Error("openclaw: "+msg, "error", err)
		m.broadcastStatus(RuntimeStatus{
			Phase:            PhaseError,
			Message:          err.Error(),
			InstalledVersion: version,
			GatewayPID:       pid,
			GatewayURL:       gatewayURL(cfg.GatewayPort),
		})
		// Disconnect path sets reconnecting=true; if reconcile then fails, clear it so UI does not
		// spin forever on "reconnecting" while phase is error.
		m.broadcastGatewayState(GatewayConnectionState{
			Connected:     false,
			Authenticated: false,
			Reconnecting:  false,
			LastError:     err.Error(),
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
		// No bundled runtime found — try installing from OSS as a fallback
		m.app.Logger.Info("openclaw: no bundled runtime found, attempting OSS install", "error", err)
		m.broadcastStatus(RuntimeStatus{
			Phase:       PhaseUpgrading,
			Message:     "No OpenClaw runtime found, downloading from OSS...",
			GatewayURL: gatewayURL(cfg.GatewayPort),
		})
		if m.toolchainSvc == nil {
			return fail("resolveBundledRuntime", err, "", 0)
		}
		if installErr := m.toolchainSvc.InstallOpenClawRuntime(); installErr != nil {
			return fail("OSS runtime install", installErr, "", 0)
		}
		// Reload bundle after OSS install
		bundle, err = resolveBundledRuntime()
		if err != nil {
			return fail("resolveBundledRuntime after OSS install", err, "", 0)
		}
	}

	if restart {
		m.closeClient()
		m.stopProcess()
	}

	version, err := verifyInstalled(bundle)
	if err != nil {
		return fail("verifyInstalled", err, "", 0)
	}

	m.broadcastStatus(RuntimeStatus{
		Phase:            PhaseStarting,
		Message:          "Preparing OpenClaw Gateway",
		InstalledVersion: version,
		RuntimeSource:    bundle.Source,
		RuntimePath:      bundle.Root,
		GatewayURL:       gatewayURL(cfg.GatewayPort),
	})

	if err := ensureOpenClawStateDir(bundle); err != nil {
		return fail("ensureOpenClawStateDir", err, version, 0)
	}

	ensureSandboxConfigured(bundle)

	// Start process if needed
	m.mu.RLock()
	needProcess := m.process == nil
	pid := m.processPID
	m.mu.RUnlock()

	if needProcess {
		if err := m.startProcess(cfg, bundle, version); err != nil {
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
			Phase:            PhaseConnecting,
			Message:          "Connecting to OpenClaw Gateway",
			InstalledVersion: version,
			RuntimeSource:    bundle.Source,
			RuntimePath:      bundle.Root,
			GatewayPID:       pid,
			GatewayURL:       gatewayURL(cfg.GatewayPort),
		})
		if err := m.connectClient(cfg, bundle); err != nil {
			return fail("connectClient", err, version, pid)
		}
	}

	m.mu.Lock()
	m.readyAt = time.Now()
	m.mu.Unlock()

	m.broadcastStatus(RuntimeStatus{
		Phase:            PhaseConnected,
		Message:          "OpenClaw Gateway connected",
		InstalledVersion: version,
		RuntimeSource:    bundle.Source,
		RuntimePath:      bundle.Root,
		GatewayPID:       pid,
		GatewayURL:       gatewayURL(cfg.GatewayPort),
	})
	m.broadcastGatewayState(GatewayConnectionState{Connected: true, Authenticated: true})
	m.notifyReadyHooks()

	return nil
}

// --- Process management ---

func (m *Manager) startProcess(cfg OpenClawConfig, bundle *bundledRuntime, installedVersion string) error {
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
	setCmdHideWindow(cmd)

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
		Phase:            PhaseStarting,
		Message:          "Starting OpenClaw Gateway",
		InstalledVersion: installedVersion,
		RuntimeSource:    bundle.Source,
		RuntimePath:      bundle.Root,
		GatewayPID:       pid,
		GatewayURL:       gatewayURL(cfg.GatewayPort),
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
		if m.queryClient != nil {
			_ = m.queryClient.Close()
			m.queryClient = nil
		}
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

	m.broadcastStatus(m.runtimeStatusRestarting())
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
			Scopes:          gatewayOperatorScopes(),
			OnEvent:         m.dispatchEvent,
			OnDisconnect:    m.handleGatewayDisconnect,
			OnLateError:     m.handleLateErrorResponse,
		})
		hello, err := client.Connect(ctx)
		if err == nil {
			if hello.Auth != nil && hello.Auth.DeviceToken != "" {
				_ = storeDeviceToken(bundle.StateDir, hello.Auth.Role, hello.Auth.DeviceToken, hello.Auth.Scopes)
			}

			// Create a second connection for queries (sessions.get etc.)
			// so they don't block on long-running agent RPCs.
			qClient := NewGatewayClient(gatewayClientOptions{
				URL:             gatewayWebSocketURL(cfg.GatewayPort),
				Token:           cfg.GatewayToken,
				DeviceIdentity:  identity,
				StoredDeviceTok: storedTok,
				Scopes:          gatewayQueryOperatorScopes(),
			})
			if _, qErr := qClient.Connect(ctx); qErr != nil {
				m.app.Logger.Warn("openclaw: query client connect failed, will use main client", "err", qErr)
				qClient = nil
			}

			m.mu.Lock()
			m.client = client
			m.queryClient = qClient
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
	qClient := m.queryClient
	m.client = nil
	m.queryClient = nil
	m.readyAt = time.Time{}
	m.mu.Unlock()
	if client != nil {
		_ = client.Close()
	}
	if qClient != nil {
		_ = qClient.Close()
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
	if m.queryClient != nil {
		_ = m.queryClient.Close()
		m.queryClient = nil
	}
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
	// Intermediate broadcasts often omit runtime metadata; keep last known values so
	// UI state stays stable during reconnects and errors.
	if s.InstalledVersion == "" && m.status.InstalledVersion != "" {
		switch s.Phase {
		case PhaseStarting, PhaseConnecting, PhaseRestarting, PhaseConnected, PhaseError:
			s.InstalledVersion = m.status.InstalledVersion
		}
	}
	if s.RuntimeSource == "" && m.status.RuntimeSource != "" {
		switch s.Phase {
		case PhaseStarting, PhaseConnecting, PhaseRestarting, PhaseConnected, PhaseError:
			s.RuntimeSource = m.status.RuntimeSource
		}
	}
	if s.RuntimePath == "" && m.status.RuntimePath != "" {
		switch s.Phase {
		case PhaseStarting, PhaseConnecting, PhaseRestarting, PhaseConnected, PhaseError:
			s.RuntimePath = m.status.RuntimePath
		}
	}
	if s.GatewayURL == "" && m.status.GatewayURL != "" {
		s.GatewayURL = m.status.GatewayURL
	}
	m.status = s
	m.mu.Unlock()
	if m.app != nil {
		m.app.Event.Emit(EventStatus, s)
	}
}

// runtimeStatusRestarting builds a restarting status while preserving the last known CLI version label.
func (m *Manager) runtimeStatusRestarting() RuntimeStatus {
	m.mu.RLock()
	prev := m.status
	m.mu.RUnlock()
	cfg := m.store.Get()
	return RuntimeStatus{
		Phase:            PhaseRestarting,
		Message:          "OpenClaw Gateway exited, restarting",
		InstalledVersion: prev.InstalledVersion,
		RuntimeSource:    prev.RuntimeSource,
		RuntimePath:      prev.RuntimePath,
		GatewayURL:       gatewayURL(cfg.GatewayPort),
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

// CLICommand returns the bundled OpenClaw CLI path and the isolated environment used by ChatClaw.
func (m *Manager) CLICommand() (string, []string, error) {
	bundle, err := resolveBundledRuntime()
	if err != nil {
		return "", nil, err
	}
	return bundle.CLIPath, buildGatewayEnv(m.store.Get(), bundle), nil
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

// QueryRequest sends a request over the dedicated query connection,
// which is not blocked by long-running agent RPCs on the main connection.
// Falls back to the main client if the query client is unavailable.
func (m *Manager) QueryRequest(ctx context.Context, method string, params any, out any) error {
	m.mu.RLock()
	qc := m.queryClient
	mc := m.client
	m.mu.RUnlock()

	if qc != nil {
		return qc.Request(ctx, method, params, out)
	}
	if mc != nil {
		return mc.Request(ctx, method, params, out)
	}
	return errors.New("gateway websocket is not connected")
}

// SkillsStatus calls the OpenClaw Gateway RPC "skills.status" (protocol schema: SkillsStatusParams).
// Pass empty agentID for the default scope; pass an OpenClaw agent id for that agent's workspace view.
func (m *Manager) SkillsStatus(ctx context.Context, agentID string) (json.RawMessage, error) {
	params := map[string]any{}
	if strings.TrimSpace(agentID) != "" {
		params["agentId"] = strings.TrimSpace(agentID)
	}
	var raw json.RawMessage
	if err := m.QueryRequest(ctx, "skills.status", params, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// ExecCLI runs an openclaw CLI subcommand (e.g. "channels", "add", "--channel", "feishu")
// and returns its combined stdout+stderr output. The command inherits the same
// environment as the gateway process so config paths, node path, etc. are correct.
// The gateway does NOT need to restart — channel config changes hot-apply via
// file watcher (see docs/gateway/configuration: "Channels → No restart needed").
func (m *Manager) ExecCLI(ctx context.Context, args ...string) ([]byte, error) {
	bundle, err := resolveBundledRuntime()
	if err != nil {
		return nil, fmt.Errorf("resolve openclaw runtime for CLI exec: %w", err)
	}
	cmd := exec.CommandContext(ctx, bundle.CLIPath, args...)
	cmd.Env = buildGatewayEnv(m.store.Get(), bundle)
	cmd.Dir = bundle.Root
	setCmdHideWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("openclaw CLI %v: %w\n%s", args, err, string(out))
	}
	return out, nil
}

// AddEventListener registers a listener for gateway events with the given key.
// The caller is responsible for removing it when done via RemoveEventListener.
func (m *Manager) AddEventListener(key string, fn func(event string, payload json.RawMessage)) {
	m.eventListenersMu.Lock()
	defer m.eventListenersMu.Unlock()
	m.eventListeners[key] = fn
}

// RemoveEventListener removes the listener registered under key.
func (m *Manager) RemoveEventListener(key string) {
	m.eventListenersMu.Lock()
	defer m.eventListenersMu.Unlock()
	delete(m.eventListeners, key)
}

// handleLateErrorResponse is called when a second (error) response arrives for
// a request whose initial OK response was already consumed. This happens when
// the Gateway sends an early ack followed by an async error (e.g. sandbox failure).
// We re-dispatch it as a synthetic "agent_late_error" event so chat listeners
// can detect and report the failure.
func (m *Manager) handleLateErrorResponse(resp gatewayResponseFrame) {
	errMsg := ""
	errCode := ""
	if resp.Error != nil {
		errMsg = resp.Error.Message
		errCode = resp.Error.Code
	}
	m.app.Logger.Warn("openclaw: late error response for completed request",
		"id", resp.ID, "code", errCode, "error", errMsg)

	synth := map[string]any{"error": errMsg, "code": errCode}
	if len(resp.Payload) > 0 {
		var extra map[string]any
		if json.Unmarshal(resp.Payload, &extra) == nil {
			for k, v := range extra {
				synth[k] = v
			}
		}
	}
	payload, _ := json.Marshal(synth)
	m.dispatchEvent(GatewayEventFrame{
		Event:   "agent_late_error",
		Payload: payload,
	})
}

func (m *Manager) dispatchEvent(ev GatewayEventFrame) {
	m.eventListenersMu.RLock()
	listeners := make([]EventListener, 0, len(m.eventListeners))
	for _, fn := range m.eventListeners {
		listeners = append(listeners, fn)
	}
	listenerCount := len(m.eventListeners)
	m.eventListenersMu.RUnlock()

	if ev.Event != "tick" && ev.Event != "health" {
		fmt.Printf("[openclaw-gateway] event received: %s (listeners=%d, payloadLen=%d)\n",
			ev.Event, listenerCount, len(ev.Payload))
		if ev.Event == "chat" || ev.Event == "agent" {
			fmt.Printf("[openclaw-gateway] payload: %s\n", string(ev.Payload))
		}
	}

	for _, fn := range listeners {
		fn(ev.Event, ev.Payload)
	}
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
	verCmd := exec.Command(bundle.CLIPath, "--version")
	setCmdHideWindow(verCmd)
	out, err := verCmd.CombinedOutput()
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

// ensureOpenClawStateDir creates OPENCLAW_STATE_DIR. We intentionally do not run
// `openclaw config set` before gateway start — that pre-writes openclaw.json and races with
// the gateway's own persistence of --auth/--token, causing repeated reload restarts; see
// ResponsesEndpointSection + ConfigService.Sync instead.
func ensureOpenClawStateDir(bundle *bundledRuntime) error {
	if err := os.MkdirAll(bundle.StateDir, 0o700); err != nil {
		return fmt.Errorf("create openclaw state dir: %w", err)
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

// ensureSandboxConfigured checks whether Docker is available.
// If Docker is not running, any agent with sandbox.mode="all" is switched to
// "none" so the agent can operate without a container runtime.
func ensureSandboxConfigured(bundle *bundledRuntime) {
	if isDockerAvailable() {
		return
	}

	raw, err := os.ReadFile(bundle.ConfigPath)
	if err != nil {
		return
	}
	var cfg map[string]any
	if json.Unmarshal(raw, &cfg) != nil {
		return
	}

	agents, _ := cfg["agents"].(map[string]any)
	if agents == nil {
		return
	}
	list, _ := agents["list"].([]any)
	if len(list) == 0 {
		return
	}

	modified := false
	for _, item := range list {
		agent, _ := item.(map[string]any)
		if agent == nil {
			continue
		}
		sandbox, _ := agent["sandbox"].(map[string]any)
		if sandbox == nil {
			continue
		}
		if mode, _ := sandbox["mode"].(string); mode == "all" {
			sandbox["mode"] = "off"
			modified = true
		}
	}

	if !modified {
		return
	}

	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(bundle.ConfigPath, out, 0o644)
}

func isDockerAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "info")
	setCmdHideWindow(cmd)
	return cmd.Run() == nil
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
