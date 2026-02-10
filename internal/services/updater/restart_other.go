//go:build !windows

package updater

import "os/exec"

// setDetachedProcess is a no-op on non-Windows platforms.
func setDetachedProcess(_ *exec.Cmd) {}

// unhideFile is a no-op on non-Windows platforms (hidden attribute is Windows-only).
func unhideFile(_ string) {}
