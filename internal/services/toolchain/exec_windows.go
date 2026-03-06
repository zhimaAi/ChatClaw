//go:build windows

package toolchain

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// hideWindow suppresses the console window that would otherwise flash
// when running external binaries (e.g. for version detection) on Windows.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NO_WINDOW,
	}
}
