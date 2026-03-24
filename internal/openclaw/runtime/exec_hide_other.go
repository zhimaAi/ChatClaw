//go:build !windows

package openclawruntime

import "os/exec"

// setCmdHideWindow is a no-op on non-Windows platforms.
func setCmdHideWindow(cmd *exec.Cmd) {
	_ = cmd
}
