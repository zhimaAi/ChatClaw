//go:build !windows

package updater

import "os/exec"

// setDetachedProcess is a no-op on non-Windows platforms.
func setDetachedProcess(_ *exec.Cmd) {}
