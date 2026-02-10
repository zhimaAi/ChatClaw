//go:build windows

package updater

import (
	"os/exec"
	"syscall"
)

// setDetachedProcess configures the command to run without a visible console
// window and in a new process group so it survives parent exit.
//
// CREATE_NO_WINDOW (0x08000000) prevents cmd.exe from showing a console.
// CREATE_NEW_PROCESS_GROUP detaches it from the parent's console group.
func setDetachedProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | 0x08000000, // 0x08000000 = CREATE_NO_WINDOW
	}
}
