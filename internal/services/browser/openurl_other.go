//go:build !windows

package browser

import "os/exec"

// openURLWindows is only implemented for Windows. This stub satisfies the compiler
// when building for other platforms (never called due to runtime.GOOS check).
func openURLWindows(_ string) error {
	panic("openURLWindows should only be called on Windows")
}

// setCmdHideWindow is a no-op on non-Windows platforms.
func setCmdHideWindow(cmd *exec.Cmd) {
	_ = cmd
}
