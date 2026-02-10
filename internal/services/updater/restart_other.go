//go:build !windows

package updater

import (
	"os"
	"os/exec"
)

// setDetachedProcess is a no-op on non-Windows platforms.
func setDetachedProcess(_ *exec.Cmd) {}

// removeHiddenFile removes a file. On non-Windows platforms there is no hidden
// attribute, so a plain os.Remove is sufficient.
func removeHiddenFile(path string) error {
	return os.Remove(path)
}
