//go:build !windows
// +build !windows

package mcp

import "os/exec"

// setCmdHideWindow is a no-op on non-Windows platforms.
func setCmdHideWindow(cmd *exec.Cmd) {
	_ = cmd
}

