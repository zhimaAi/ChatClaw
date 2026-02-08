//go:build windows

package filesystem

import (
	"os/exec"
	"syscall"
)

// setProcGroup configures the command to create a new process group on Windows.
// CREATE_NEW_PROCESS_GROUP allows us to terminate the entire process tree.
func setProcGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}

// killProcessGroup terminates the process tree on Windows.
// Since Windows doesn't have Unix-style process groups, we simply kill the
// main process. The OS will also terminate child processes that were created
// with the same process group.
func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
}
