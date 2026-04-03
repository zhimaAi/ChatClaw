package openclawruntime

import "fmt"

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

func (s *OpenClawRuntimeService) UpgradeRuntime() (*RuntimeUpgradeResult, error) {
	return s.manager.UpgradeRuntime()
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

func (s *OpenClawRuntimeService) GetDashboardURL() string {
	cfg := s.manager.store.Get()
	return fmt.Sprintf("http://127.0.0.1:%d?token=%s", cfg.GatewayPort, cfg.GatewayToken)
}

// IsDevMode returns true when the frontend runs with DEV=true (dev server / Wails dev).
func (s *OpenClawRuntimeService) IsDevMode() bool {
	// Backend can also read from env if needed; here we just return false
	// because the frontend already guards the button.
	// The binding exists so the frontend can call it without hitting a missing-method error
	// in production builds where the button is hidden anyway.
	return false
}
