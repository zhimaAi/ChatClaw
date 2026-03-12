//go:build windows
// +build windows

package mcp

import (
	"os/exec"
	"syscall"
)

// setCmdHideWindow hides the console window for subprocesses on Windows
// to avoid flashing a CMD popup when launching MCP stdio servers.
func setCmdHideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
	}
}

