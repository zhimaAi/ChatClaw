//go:build windows

package winsnap

import (
	"errors"
	"syscall"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
	"golang.org/x/sys/windows"
)

var (
	procSetForegroundWindowWake   = modUser32.NewProc("SetForegroundWindow")
	procGetWindowThreadProcIdWake = modUser32.NewProc("GetWindowThreadProcessId")
	procAttachThreadInputWake     = modUser32.NewProc("AttachThreadInput")
	procGetCurrentThreadIdWake    = modKernel32.NewProc("GetCurrentThreadId")
	procShowWindowWake            = modUser32.NewProc("ShowWindow")
	procBringWindowToTopWake      = modUser32.NewProc("BringWindowToTop")
	procGetForegroundWindowWake   = modUser32.NewProc("GetForegroundWindow")
	procSetWindowPosWake          = modUser32.NewProc("SetWindowPos")
)

const swRestoreWake = 9
const hwndTopWake = 0
const (
	swpNoMoveWake     = 0x0002
	swpNoSizeWake     = 0x0001
	swpNoActivateWake = 0x0010
)

// EnsureWindowVisible restores the window if it was minimized (e.g. by Win+D)
// so it becomes visible again. Does not activate or steal focus.
func EnsureWindowVisible(window *application.WebviewWindow) error {
	if window == nil {
		return errors.New("winsnap: Window is nil")
	}
	h := uintptr(window.NativeWindow())
	if h == 0 {
		return errors.New("winsnap: native window handle is 0")
	}
	procShowWindowWake.Call(h, swRestoreWake)
	return nil
}

// WakeAttachedWindow brings the target window and the winsnap window to the front,
// keeping winsnap ordered directly above the target (same-level behavior).
func WakeAttachedWindow(self *application.WebviewWindow, targetProcessName string) error {
	if self == nil {
		return errors.New("winsnap: Window is nil")
	}
	selfH := uintptr(self.NativeWindow())
	if selfH == 0 {
		return errors.New("winsnap: native window handle is 0")
	}

	targetNames := expandWindowsTargetNames(targetProcessName)
	if len(targetNames) == 0 {
		return errors.New("winsnap: TargetProcessName is empty")
	}
	var targetHwnd windows.HWND
	var err error
	for _, n := range targetNames {
		targetHwnd, err = findMainWindowByProcessName(n)
		if err == nil && targetHwnd != 0 {
			break
		}
	}
	if err != nil || targetHwnd == 0 {
		return ErrTargetWindowNotFound
	}

	// Bring target to front, then our window, then ensure z-order relationship.
	activateHwnd(targetHwnd)
	activateHwnd(windows.HWND(selfH))

	// Best-effort visibility: even if SetForegroundWindow is denied, push windows to the top of normal z-order.
	_, _, _ = procSetWindowPosWake.Call(uintptr(targetHwnd), hwndTopWake, 0, 0, 0, 0, uintptr(swpNoMoveWake|swpNoSizeWake|swpNoActivateWake))
	_, _, _ = procSetWindowPosWake.Call(selfH, hwndTopWake, 0, 0, 0, 0, uintptr(swpNoMoveWake|swpNoSizeWake|swpNoActivateWake))

	// Keep winsnap just above target without moving/resizing.
	if isTopMost(targetHwnd) {
		_ = setWindowTopMostNoActivate(windows.HWND(selfH))
	} else {
		_ = setWindowNoTopMostNoActivate(windows.HWND(selfH))
	}
	_ = setWindowPosAfterNoMoveNoSizeNoActivate(windows.HWND(selfH), targetHwnd)
	return nil
}

func activateHwnd(hwnd windows.HWND) {
	if hwnd == 0 {
		return
	}

	foregroundHwnd, _, _ := procGetForegroundWindowWake.Call()
	var attached bool
	var foregroundTid, currentTid uintptr

	if foregroundHwnd != 0 {
		var foregroundPid uint32
		foregroundTid, _, _ = procGetWindowThreadProcIdWake.Call(
			foregroundHwnd,
			uintptr(unsafe.Pointer(&foregroundPid)),
		)
		currentTid, _, _ = procGetCurrentThreadIdWake.Call()
		if foregroundTid != currentTid {
			ret, _, _ := procAttachThreadInputWake.Call(currentTid, foregroundTid, 1)
			attached = ret != 0
		}
	}

	procShowWindowWake.Call(uintptr(hwnd), swRestoreWake)
	procSetForegroundWindowWake.Call(uintptr(hwnd))
	procBringWindowToTopWake.Call(uintptr(hwnd))

	if attached {
		procAttachThreadInputWake.Call(currentTid, foregroundTid, 0)
	}
}

func setWindowPosAfterNoMoveNoSizeNoActivate(hwnd windows.HWND, insertAfter windows.HWND) error {
	flags := uintptr(swpNoMove | swpNoSize | swpNoActivate)
	r1, _, errNo := procSetWindowPos.Call(
		uintptr(hwnd),
		uintptr(insertAfter),
		0, 0, 0, 0,
		flags,
	)
	if r1 == 0 {
		if errNo != nil && errNo != syscall.Errno(0) {
			return errNo
		}
		return syscall.EINVAL
	}
	return nil
}
