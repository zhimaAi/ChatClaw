//go:build !windows

package openclawruntime

// killListenersOnLocalTCPPort is a no-op on non-Windows builds; port cleanup relies on
// openclaw gateway stop and process management from the Manager.
func killListenersOnLocalTCPPort(port int) error {
	return nil
}
