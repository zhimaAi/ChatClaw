//go:build windows

package browser

import (
	"os/exec"
	"syscall"
)

// openURLWindows opens the given URL in the default browser without showing a command prompt window.
// Uses SysProcAttr.HideWindow to suppress the cmd.exe window on Windows.
func openURLWindows(u string) error {
	cmd := exec.Command("cmd", "/c", "start", "", u)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Start()
}
