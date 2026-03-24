package openclawruntime

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
