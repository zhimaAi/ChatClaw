//go:build !windows

package filesystem

import (
	"os/exec"
	"syscall"
)

// setProcGroup configures the command to run in its own process group
// so that killProcessGroup can terminate all child processes.
func setProcGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcessGroup sends SIGKILL to the entire process group of cmd.
// This ensures that all child processes (e.g. servers spawned by the command)
// are terminated, not just the shell process itself.
func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err == nil {
		// Kill the entire process group (negative PID).
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
	} else {
		// Fallback: kill just the main process.
		_ = cmd.Process.Kill()
	}
}
