//go:build windows

package browser

import (
	"os/exec"
	"syscall"
)

// CREATE_NO_WINDOW prevents a console window from being created.
const CREATE_NO_WINDOW = 0x08000000

// openURLWindows opens the given URL in the default browser without showing a command prompt window.
// Uses rundll32 url.dll,FileProtocolHandler to bypass cmd.exe entirely (cmd /c start would flash a console).
func openURLWindows(u string) error {
	cmd := exec.Command("rundll32", "url.dll,FileProtocolHandler", u)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: CREATE_NO_WINDOW}
	return cmd.Start()
}

// setCmdHideWindow hides the console window for subprocesses on Windows.
func setCmdHideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: CREATE_NO_WINDOW,
	}
}
