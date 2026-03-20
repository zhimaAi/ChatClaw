//go:build windows

package sysinfo

import (
	"os/exec"
	"syscall"
)

// CREATE_NO_WINDOW prevents a console window from being created.
const CREATE_NO_WINDOW = 0x08000000

// setCmdHideWindow hides the console window for subprocesses on Windows.
func setCmdHideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: CREATE_NO_WINDOW,
	}
}
