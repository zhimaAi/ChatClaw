//go:build windows

package winutil

import (
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
	"golang.org/x/sys/windows"
)

var (
	modUser32   = windows.NewLazySystemDLL("user32.dll")
	modKernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procSetForegroundWindow   = modUser32.NewProc("SetForegroundWindow")
	procGetWindowThreadProcId = modUser32.NewProc("GetWindowThreadProcessId")
	procAttachThreadInput     = modUser32.NewProc("AttachThreadInput")
	procGetCurrentThreadId    = modKernel32.NewProc("GetCurrentThreadId")
	procShowWindow            = modUser32.NewProc("ShowWindow")
	procBringWindowToTop      = modUser32.NewProc("BringWindowToTop")
	procGetForegroundWindow   = modUser32.NewProc("GetForegroundWindow")
)

const swRestore = 9

// ForceActivateWindow uses Windows API to activate window
// (doesn't call Wails Focus to avoid WebView2 error).
func ForceActivateWindow(w *application.WebviewWindow) {
	if w == nil {
		return
	}

	// Get the window's native handle directly
	nativeHandle := w.NativeWindow()
	if nativeHandle == nil {
		return
	}
	targetHwnd := uintptr(nativeHandle)
	if targetHwnd == 0 {
		return
	}

	// Get foreground window's thread
	foregroundHwnd, _, _ := procGetForegroundWindow.Call()
	var attached bool
	var foregroundTid, currentTid uintptr

	if foregroundHwnd != 0 {
		var foregroundPid uint32
		foregroundTid, _, _ = procGetWindowThreadProcId.Call(
			foregroundHwnd,
			uintptr(unsafe.Pointer(&foregroundPid)),
		)
		currentTid, _, _ = procGetCurrentThreadId.Call()

		// Attach thread input queue to allow SetForegroundWindow
		if foregroundTid != currentTid {
			ret, _, _ := procAttachThreadInput.Call(currentTid, foregroundTid, 1)
			attached = ret != 0
		}
	}

	// Activate window
	procShowWindow.Call(targetHwnd, swRestore)
	procSetForegroundWindow.Call(targetHwnd)
	procBringWindowToTop.Call(targetHwnd)

	// Detach thread
	if attached {
		procAttachThreadInput.Call(currentTid, foregroundTid, 0)
	}
}
