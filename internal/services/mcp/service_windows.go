//go:build windows
// +build windows

package mcp

import (
	"os/exec"
	"syscall"
)

// CREATE_NO_WINDOW prevents a console window from being created.
const CREATE_NO_WINDOW = 0x08000000

// setCmdHideWindow hides the console window for subprocesses on Windows
// to avoid flashing a CMD popup when launching MCP stdio servers.
func setCmdHideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: CREATE_NO_WINDOW,
	}
}

