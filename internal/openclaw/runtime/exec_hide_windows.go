//go:build windows

package openclawruntime

import (
	"os/exec"
	"syscall"
)

// CREATE_NO_WINDOW prevents a console window from being created.
const createNoWindow = 0x08000000

// setCmdHideWindow hides the console window for subprocesses on Windows
// so bundled openclaw.cmd / node does not flash a CMD window.
func setCmdHideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: createNoWindow,
	}
}
