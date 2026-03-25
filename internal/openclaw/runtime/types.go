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
	PhaseUpgrading  = "upgrading"
	PhaseError      = "error"
)

type RuntimeStatus struct {
	Phase            string `json:"phase"`
	Message          string `json:"message,omitempty"`
	InstalledVersion string `json:"installedVersion,omitempty"`
	RuntimeSource    string `json:"runtimeSource,omitempty"`
	RuntimePath      string `json:"runtimePath,omitempty"`
	GatewayPID       int    `json:"gatewayPid,omitempty"`
	GatewayURL       string `json:"gatewayURL,omitempty"`
}

type GatewayConnectionState struct {
	Connected     bool   `json:"connected"`
	Authenticated bool   `json:"authenticated"`
	Reconnecting  bool   `json:"reconnecting"`
	LastError     string `json:"lastError,omitempty"`
}

type RuntimeUpgradeResult struct {
	PreviousVersion string `json:"previousVersion,omitempty"`
	CurrentVersion  string `json:"currentVersion,omitempty"`
	LatestVersion   string `json:"latestVersion,omitempty"`
	Upgraded        bool   `json:"upgraded"`
	RuntimeSource   string `json:"runtimeSource,omitempty"`
	RuntimePath     string `json:"runtimePath,omitempty"`
}
