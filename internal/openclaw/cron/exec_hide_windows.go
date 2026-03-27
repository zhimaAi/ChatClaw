//go:build windows

package openclawcron

import (
	"os/exec"
	"syscall"
)

const createNoWindow = 0x08000000

// setCmdHideWindow hides the transient cmd window for cron CLI calls on Windows.
// setCmdHideWindow 在 Windows 下隐藏 Cron CLI 调用产生的临时控制台窗口。
func setCmdHideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: createNoWindow,
	}
}
