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

func (s *OpenClawRuntimeService) UpgradeRuntime() (*RuntimeUpgradeResult, error) {
	return s.manager.UpgradeRuntime()
}

// InstallAndStartRuntime downloads and installs the OpenClaw runtime from OSS, then
// starts the gateway. This mirrors the install-then-activate flow of UpgradeRuntime,
// but downloads the full OSS bundle instead of running npm install.
func (s *OpenClawRuntimeService) InstallAndStartRuntime() (*RuntimeUpgradeResult, error) {
	return s.manager.InstallAndStartRuntime()
}

func (s *OpenClawRuntimeService) GetDashboardURL() string {
	cfg := s.manager.store.Get()
	return fmt.Sprintf("http://127.0.0.1:%d?token=%s", cfg.GatewayPort, cfg.GatewayToken)
}
