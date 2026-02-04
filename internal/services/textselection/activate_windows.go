//go:build windows

package textselection

import (
	"syscall"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
	"golang.org/x/sys/windows"
)

var (
	procSetForegroundWindow   = modUser32.NewProc("SetForegroundWindow")
	procGetWindowThreadProcId = modUser32.NewProc("GetWindowThreadProcessId")
	procAttachThreadInput     = modUser32.NewProc("AttachThreadInput")
	procGetCurrentThreadId    = modKernel32.NewProc("GetCurrentThreadId")
	procEnumWindows           = modUser32.NewProc("EnumWindows")
	procShowWindow            = modUser32.NewProc("ShowWindow")
	procBringWindowToTop      = modUser32.NewProc("BringWindowToTop")
	procGetCurrentProcessId   = modKernel32.NewProc("GetCurrentProcessId")
	procGetForegroundWindow   = modUser32.NewProc("GetForegroundWindow")
	procGetWindowTextW        = modUser32.NewProc("GetWindowTextW")
)

const swRestore = 9

// Storage for enumeration results
var enumTargetHwnd uintptr
var enumCurrentPid uintptr

// forceActivateWindow uses Windows API to activate window (doesn't call Wails Focus to avoid WebView2 error).
func forceActivateWindow(w *application.WebviewWindow) {
	if w == nil {
		return
	}

	// Get current process ID
	enumCurrentPid, _, _ = procGetCurrentProcessId.Call()
	enumTargetHwnd = 0

	// Enumerate all windows, find main window belonging to current process
	procEnumWindows.Call(
		syscall.NewCallback(enumWindowsCallback),
		0,
	)

	if enumTargetHwnd == 0 {
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
	procShowWindow.Call(enumTargetHwnd, swRestore)
	procSetForegroundWindow.Call(enumTargetHwnd)
	procBringWindowToTop.Call(enumTargetHwnd)

	// Detach thread
	if attached {
		procAttachThreadInput.Call(currentTid, foregroundTid, 0)
	}
}

// enumWindowsCallback EnumWindows callback function.
func enumWindowsCallback(hwnd uintptr, lParam uintptr) uintptr {
	// Get window's owning process
	var pid uint32
	procGetWindowThreadProcId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if uintptr(pid) != enumCurrentPid {
		return 1 // Continue enumeration
	}

	// Get window title
	title := make([]uint16, 256)
	length, _, _ := procGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&title[0])), 256)
	titleStr := windows.UTF16ToString(title[:length])

	// Specifically look for main window (title is "WillChat")
	if titleStr == "WillChat" {
		enumTargetHwnd = hwnd
		return 0 // Stop enumeration
	}

	return 1 // Continue enumeration
}
