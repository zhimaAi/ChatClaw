//go:build windows

package textselection

// Windows API proc references used by other files in this package
// (e.g., mouse_hook_windows.go).
var (
	procGetWindowThreadProcId = modUser32.NewProc("GetWindowThreadProcessId")
	procGetCurrentProcessId   = modKernel32.NewProc("GetCurrentProcessId")
	procGetForegroundWindow   = modUser32.NewProc("GetForegroundWindow")
)
