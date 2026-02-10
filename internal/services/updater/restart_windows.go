//go:build windows

package updater

import (
	"os/exec"
	"syscall"
)

// setDetachedProcess configures the command to launch as a fully independent
// process (DETACHED_PROCESS | CREATE_NEW_PROCESS_GROUP) so that it survives
// parent exit. This also prevents the intermediate console window from appearing.
func setDetachedProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | 0x00000008, // 0x00000008 = DETACHED_PROCESS
	}
}
