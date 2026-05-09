package openclawruntime

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"chatclaw/internal/define"
	"chatclaw/internal/errs"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// OpenClawRuntimeService is the Wails-bound service that exposes
// OpenClaw runtime management to the frontend.
type OpenClawRuntimeService struct {
	manager *Manager
}

func NewOpenClawRuntimeService(manager *Manager) *OpenClawRuntimeService {
	return &OpenClawRuntimeService{manager: manager}
}

func (s *OpenClawRuntimeService) GetStatus() RuntimeStatus {
	return s.manager.GetStatus()
}

func (s *OpenClawRuntimeService) GetGatewayState() GatewayConnectionState {
	return s.manager.GetGatewayState()
}

func (s *OpenClawRuntimeService) RestartGateway() (RuntimeStatus, error) {
	return s.manager.RestartGateway()
}

// StartGateway starts the OpenClaw gateway when it is stopped.
func (s *OpenClawRuntimeService) StartGateway() (RuntimeStatus, error) {
	return s.manager.StartGateway()
}

// StopGateway stops the OpenClaw gateway (used in DEV for quick testing).
func (s *OpenClawRuntimeService) StopGateway() {
	s.manager.Shutdown()
}

// GetAutoStart returns the current auto-start preference.
func (s *OpenClawRuntimeService) GetAutoStart() bool {
	return s.manager.store.Get().AutoStart
}

// SetAutoStart persists the auto-start preference and applies it immediately:
//   - true:  saves the preference; starts/reconciles when there is no process or status is idle/error.
//   - false: saves the preference; if gateway is running, stops it.
func (s *OpenClawRuntimeService) SetAutoStart(v bool) {
	s.manager.store.SetAutoStart(v)
	if v {
		s.manager.mu.RLock()
		running := s.manager.process != nil
		phase := s.manager.status.Phase
		s.manager.mu.RUnlock()
		if !running || phase == PhaseError || phase == PhaseIdle {
			go func() { _ = s.manager.reconcile(false) }()
		}
	} else {
		s.manager.Shutdown()
	}
}

// SetSystemMode is called by the frontend to notify the backend of the current
// system mode ('openclaw' or 'chatclaw'). When the mode is 'openclaw' and the
// runtime is available and AutoStart is enabled, the gateway is started.
func (s *OpenClawRuntimeService) SetSystemMode(isOpenClawMode bool) {
	s.manager.SetSystemMode(isOpenClawMode)
	// Auto-start the gateway immediately when switching to openclaw mode.
	// Start() already handles the initial case; this handles subsequent mode switches.
	if isOpenClawMode && s.manager.store.Get().AutoStart {
		s.manager.mu.RLock()
		running := s.manager.process != nil
		phase := s.manager.status.Phase
		s.manager.mu.RUnlock()
		if !running || phase == PhaseError || phase == PhaseIdle {
			go func() { _ = s.manager.reconcile(false) }()
		}
	}
}

func (s *OpenClawRuntimeService) UpgradeRuntime() (*RuntimeUpgradeResult, error) {
	return s.manager.UpgradeRuntime()
}

// CancelUpgrade cancels the currently running upgrade.
// If rollback is possible, restores the previous version.
func (s *OpenClawRuntimeService) CancelUpgrade() error {
	return s.manager.CancelUpgrade()
}

// ContinueUpgrade resumes a previously interrupted upgrade for the given version.
func (s *OpenClawRuntimeService) ContinueUpgrade(version string) (*RuntimeUpgradeResult, error) {
	return s.manager.ContinueUpgrade(version)
}

// InstallAndStartRuntime downloads and installs the OpenClaw runtime from OSS, then
// starts the gateway. This mirrors the install-then-activate flow of UpgradeRuntime,
// but downloads the full OSS bundle instead of running npm install.
func (s *OpenClawRuntimeService) InstallAndStartRuntime() (*RuntimeUpgradeResult, error) {
	return s.manager.InstallAndStartRuntime()
}

// RunDoctorCommand executes an openclaw doctor command and returns the result.
// The command is run in the OpenClaw runtime directory.
func (s *OpenClawRuntimeService) RunDoctorCommand(command string, fix bool) (*DoctorCommandResult, error) {
	return s.manager.RunDoctorCommand(command, fix)
}

// GetGatewayStatus executes `openclaw gateway status` to get the authoritative gateway state.
// This is more reliable than port-based checks during transitional states like startup.
func (s *OpenClawRuntimeService) GetGatewayStatus(ctx context.Context) (*GatewayStatusResult, error) {
	return s.manager.GetGatewayStatusViaCLI(ctx)
}

func (s *OpenClawRuntimeService) GetDashboardURL() string {
	cfg := s.manager.store.Get()
	return fmt.Sprintf("http://127.0.0.1:%d?token=%s", cfg.GatewayPort, cfg.GatewayToken)
}

// IsDevMode returns true when the application is running in development mode.
func (s *OpenClawRuntimeService) IsDevMode() bool {
	return define.IsDev()
}

// IsRuntimeAvailable checks whether a valid OpenClaw runtime is present.
// This is a fast pre-check that does not verify the CLI binary or open a port.
func (s *OpenClawRuntimeService) IsRuntimeAvailable() bool {
	return IsOpenClawRuntimeAvailable()
}

// PortOccupiedResult contains information about port occupation status.
type PortOccupiedResult struct {
	Occupied    bool   `json:"occupied"`
	Port        int    `json:"port"`
	ProcessName string `json:"processName,omitempty"`
	PID         int    `json:"pid,omitempty"`
}

// CheckPortOccupied checks if the gateway port is currently occupied and returns details.
func (s *OpenClawRuntimeService) CheckPortOccupied() PortOccupiedResult {
	cfg := s.manager.store.Get()
	port := cfg.GatewayPort

	if !gatewayPortOccupied(port) {
		return PortOccupiedResult{
			Occupied: false,
			Port:     port,
		}
	}

	pid := getOccupyingProcessPID(port)
	processName := ""
	if pid > 0 {
		processName = getProcessNameByPID(pid)
	}

	return PortOccupiedResult{
		Occupied:    true,
		Port:        port,
		ProcessName: processName,
		PID:         pid,
	}
}

// ClearGatewayLog truncates the gateway log file before starting.
func (s *OpenClawRuntimeService) ClearGatewayLog() error {
	return s.manager.ClearGatewayLog()
}

// GatewayLogTail returns the last n lines of the gateway log.
func (s *OpenClawRuntimeService) GatewayLogTail(n int) (string, error) {
	return s.manager.GatewayLogTail(n)
}

// GatewayLogPath returns the absolute path to the gateway log file.
func (s *OpenClawRuntimeService) GatewayLogPath() (string, error) {
	return s.manager.GatewayLogPath()
}

// ResetToFactory deletes the OpenClaw data directory and restarts the application.
// This is intended for users who want to reset their OpenClaw environment to a clean state.
func (s *OpenClawRuntimeService) ResetToFactory() error {
	// Shutdown the gateway first
	s.manager.Shutdown()

	// Get the OpenClaw data directory
	dataDir, err := define.OpenClawDataRootDir()
	if err != nil {
		return errs.Wrap("error.openclaw_reset_failed", err)
	}

	// Delete the entire openclaw data directory
	if err := os.RemoveAll(dataDir); err != nil {
		return errs.Wrap("error.openclaw_reset_delete_failed", err)
	}

	// Restart the application
	return restartApp(s.manager.app)
}

// restartApp launches a new instance of the application and exits the current one.
func restartApp(app *application.App) error {
	exe, err := os.Executable()
	if err != nil {
		return errs.Wrap("error.update_restart_failed", err)
	}

	// Windows needs special handling: SingleInstance rejects a second process,
	// and the .old binary cleanup requires cd + relative path. Use a bat script
	// that waits for this process to exit, cleans up, then launches the new exe.
	if runtime.GOOS == "windows" {
		exeDir := filepath.Dir(exe)
		exeName := filepath.Base(exe)
		oldName := "." + exeName + ".old"
		batPath := filepath.Join(os.TempDir(), "chatclaw_restart.bat")
		batContent := fmt.Sprintf(
			"@echo off\r\n"+
				"ping localhost -n 3 >nul\r\n"+
				"cd /D \"%s\"\r\n"+
				"attrib -H \"%s\" >nul 2>&1\r\n"+
				"del /F \"%s\" >nul 2>&1\r\n"+
				"start \"\" \"%s\"\r\n"+
				"del \"%%~f0\"\r\n",
			exeDir, oldName, oldName, exe,
		)
		if err := os.WriteFile(batPath, []byte(batContent), 0o644); err != nil {
			return errs.Wrap("error.update_restart_failed", err)
		}

		cmd := exec.Command("cmd", "/C", batPath)
		cmd.SysProcAttr = &syscall.SysProcAttr{
			HideWindow: true,
		}

		if err := cmd.Start(); err != nil {
			return errs.Wrap("error.update_restart_failed", err)
		}

		go func() {
			time.Sleep(200 * time.Millisecond)
			app.Quit()
		}()

		return nil
	}

	// macOS / Linux
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return errs.Wrap("error.update_restart_failed", err)
	}

	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		appPath := exe
		for i := 0; i < 3; i++ {
			appPath = filepath.Dir(appPath)
		}
		if filepath.Ext(appPath) == ".app" {
			cmd = exec.Command("open", "-n", appPath)
		} else {
			cmd = exec.Command(exe)
		}
	default:
		cmd = exec.Command(exe)
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return errs.Wrap("error.update_restart_failed", err)
	}

	go func() {
		time.Sleep(500 * time.Millisecond)
		os.Exit(0)
	}()

	return nil
}

// getProcessNameByPID returns the process name for a given PID on Windows.
func getProcessNameByPID(pid int) string {
	if runtime.GOOS != "windows" {
		return ""
	}
	cmd := exec.Command("tasklist", "/FI", "PID eq "+strconv.Itoa(pid), "/FO", "CSV", "/NH")
	setCmdHideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	// CSV format: "processname","pid","sessionname","session#","memusage"
	fields := strings.Split(strings.TrimSpace(string(out)), ",")
	if len(fields) > 0 {
		return strings.Trim(fields[0], "\"")
	}
	return ""
}
