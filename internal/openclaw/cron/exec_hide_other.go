//go:build !windows

package openclawcron

import "os/exec"

// setCmdHideWindow is a no-op on non-Windows platforms.
// setCmdHideWindow 在非 Windows 平台无需处理控制台窗口。
func setCmdHideWindow(cmd *exec.Cmd) {
	_ = cmd
}
