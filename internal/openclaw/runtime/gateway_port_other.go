//go:build !windows

package openclawruntime

// getOccupyingProcessPID returns 0 on non-Windows builds (not implemented).
func getOccupyingProcessPID(port int) int {
	return 0
}

// killListenersOnLocalTCPPort is a no-op on non-Windows builds; port cleanup relies on
// openclaw gateway stop and process management from the Manager.
func killListenersOnLocalTCPPort(port int) error {
	return nil
}
