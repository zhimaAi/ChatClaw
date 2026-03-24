package openclawruntime

const (
	EventStatus       = "openclaw:status"
	EventGatewayState = "openclaw:gateway-state"
)

const (
	PhaseIdle       = "idle"
	PhaseStarting   = "starting"
	PhaseConnecting = "connecting"
	PhaseConnected  = "connected"
	PhaseRestarting = "restarting"
	PhaseError      = "error"
)

type RuntimeStatus struct {
	Phase            string `json:"phase"`
	Message          string `json:"message,omitempty"`
	InstalledVersion string `json:"installedVersion,omitempty"`
	GatewayPID       int    `json:"gatewayPid,omitempty"`
	GatewayURL       string `json:"gatewayURL,omitempty"`
}

type GatewayConnectionState struct {
	Connected     bool   `json:"connected"`
	Authenticated bool   `json:"authenticated"`
	Reconnecting  bool   `json:"reconnecting"`
	LastError     string `json:"lastError,omitempty"`
}
