//go:build windows

package winsnap

import (
	"errors"
	"strings"
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
	procGetWindowWake             = modUser32.NewProc("GetWindow")
	procGetCurrentProcessIdWake   = modKernel32.NewProc("GetCurrentProcessId")
)

const swRestoreWake = 9
const hwndTopWake = 0
const (
	swpNoMoveWake     = 0x0002
	swpNoSizeWake     = 0x0001
	swpNoActivateWake = 0x0010
)

// GW_HWNDNEXT = 2: Returns a handle to the window below in z-order
const gwHwndNextWake = 2

// EnsureWindowVisible restores the window if it was minimized (e.g. by Win+D)
// so it becomes visible again. Does not activate or steal focus.
func EnsureWindowVisible(window *application.WebviewWindow) error {
	if window == nil {
		return ErrWinsnapWindowInvalid
	}
	h := uintptr(window.NativeWindow())
	if h == 0 {
		return ErrWinsnapWindowInvalid
	}
	procShowWindowWake.Call(h, swRestoreWake)
	return nil
}

// BringWinsnapToFront brings the winsnap window to front without stealing focus.
// This is used when the target app becomes frontmost and we want winsnap visible
// alongside it, but we don't want to steal focus from the target app.
func BringWinsnapToFront(window *application.WebviewWindow) error {
	if window == nil {
		return ErrWinsnapWindowInvalid
	}
	h := uintptr(window.NativeWindow())
	if h == 0 {
		return ErrWinsnapWindowInvalid
	}
	// Use SetWindowPos with SWP_NOACTIVATE to bring window to top without activating it
	procSetWindowPosWake.Call(h, hwndTopWake, 0, 0, 0, 0, swpNoMoveWake|swpNoSizeWake|swpNoActivateWake)
	return nil
}

// SyncAttachedZOrderNoActivate is a no-op on Windows.
// Windows z-order syncing after attach is handled by the follower and SetWindowPos strategy.
func SyncAttachedZOrderNoActivate(_ *application.WebviewWindow, _ string) error {
	return nil
}

// IsTargetObscured checks if the target window is obscured by other app windows.
// Returns true if there are other app windows between winsnap and target.
// This is used in the step loop to detect when the attached target needs to be woken up.
func IsTargetObscured(self *application.WebviewWindow, targetProcessName string) (bool, error) {
	if self == nil {
		return false, ErrWinsnapWindowInvalid
	}
	selfH := uintptr(self.NativeWindow())
	if selfH == 0 {
		return false, ErrWinsnapWindowInvalid
	}

	targetNames := expandWindowsTargetNames(targetProcessName)
	if len(targetNames) == 0 {
		return false, errors.New("winsnap: TargetProcessName is empty")
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
		return false, ErrTargetWindowNotFound
	}

	// isTargetAdjacent returns true if adjacent (not obscured), false if obscured
	adjacent := isTargetAdjacent(windows.HWND(selfH), targetHwnd, targetNames)
	return !adjacent, nil
}

// BringTargetToFrontNoActivate brings the target window to front without stealing focus.
// On Windows, this uses SetWindowPos with SWP_NOACTIVATE to adjust z-order without activation.
func BringTargetToFrontNoActivate(self *application.WebviewWindow, targetProcessName string) error {
	if self == nil {
		return ErrWinsnapWindowInvalid
	}
	selfH := uintptr(self.NativeWindow())
	if selfH == 0 {
		return ErrWinsnapWindowInvalid
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

	// Bring target to top of z-order without activating
	_, _, _ = procSetWindowPosWake.Call(uintptr(targetHwnd), hwndTopWake, 0, 0, 0, 0,
		uintptr(swpNoMoveWake|swpNoSizeWake|swpNoActivateWake))

	// Then bring winsnap above target
	_, _, _ = procSetWindowPosWake.Call(selfH, hwndTopWake, 0, 0, 0, 0,
		uintptr(swpNoMoveWake|swpNoSizeWake|swpNoActivateWake))

	// Keep winsnap just above target
	if isTopMost(targetHwnd) {
		_ = setWindowTopMostNoActivate(windows.HWND(selfH))
	} else {
		_ = setWindowNoTopMostNoActivate(windows.HWND(selfH))
	}
	_ = setWindowPosAfterNoMoveNoSizeNoActivate(windows.HWND(selfH), targetHwnd)

	return nil
}

// WakeAttachedWindow brings the target window and the winsnap window to the front,
// keeping winsnap ordered directly above the target (same-level behavior).
func WakeAttachedWindow(self *application.WebviewWindow, targetProcessName string) error {
	return wakeAttachedWindowInternal(self, targetProcessName, false)
}

// WakeAttachedWindowWithRefocus is like WakeAttachedWindow but checks if target is already
// adjacent (no other app windows in between). If so, skip waking target to avoid focus flickering.
func WakeAttachedWindowWithRefocus(self *application.WebviewWindow, targetProcessName string) error {
	return wakeAttachedWindowInternal(self, targetProcessName, true)
}

// isTargetAdjacent checks if target window is already directly below self window (no other app windows in between).
// Returns true if target is adjacent (no need to wake), false if target needs to be woken up.
func isTargetAdjacent(selfHwnd windows.HWND, targetHwnd windows.HWND, targetProcessNames []string) bool {
	if selfHwnd == 0 || targetHwnd == 0 {
		return false
	}

	// Get our own process ID
	selfPid, _, _ := procGetCurrentProcessIdWake.Call()

	// Get target process ID
	var targetPid uint32
	procGetWindowThreadProcIdWake.Call(uintptr(targetHwnd), uintptr(unsafe.Pointer(&targetPid)))

	// Build a set of target process names for quick lookup
	targetNames := make(map[string]struct{})
	for _, name := range targetProcessNames {
		for _, n := range expandWindowsTargetNames(name) {
			if n != "" {
				targetNames[strings.ToLower(n)] = struct{}{}
			}
		}
	}

	// Walk z-order from self window downward, looking for target
	// If we encounter any other app's window before finding target, return false
	hwnd := selfHwnd
	for {
		// Get next window in z-order (below current)
		nextHwnd, _, _ := procGetWindowWake.Call(uintptr(hwnd), gwHwndNextWake)
		if nextHwnd == 0 {
			break
		}
		hwnd = windows.HWND(nextHwnd)

		// Skip invisible windows
		if !isWindowVisible(hwnd) {
			continue
		}

		// Skip minimized windows
		if isWindowIconic(hwnd) {
			continue
		}

		// Check if this is a top-level candidate (not a tool window, etc.)
		if !isTopLevelCandidate(hwnd) {
			continue
		}

		// Get this window's process ID
		var pid uint32
		procGetWindowThreadProcIdWake.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&pid)))

		// If this is our own app's window, skip it
		if uint32(selfPid) == pid {
			continue
		}

		// If this is the target window, target is adjacent
		if hwnd == targetHwnd {
			return true
		}

		// Check if this is target app's window (different window but same process)
		if pid == targetPid {
			continue
		}

		// Check if this window belongs to target process by name
		exe, err := getProcessImageBaseName(pid)
		if err == nil && exe != "" {
			if _, ok := targetNames[strings.ToLower(exe)]; ok {
				continue
			}
		}

		// This is another app's window between self and target
		return false
	}

	// Target not found below self - check if target is above self
	// Walk z-order from top to find positions
	hwnd = selfHwnd
	for {
		// Get previous window in z-order (above current)
		prevHwnd, _, _ := procGetWindowWake.Call(uintptr(hwnd), 3) // GW_HWNDPREV = 3
		if prevHwnd == 0 {
			break
		}
		hwnd = windows.HWND(prevHwnd)

		if hwnd == targetHwnd {
			// Target is above self, no need to wake
			return true
		}
	}

	return false
}

// focusSelfOnly brings focus to winsnap without waking target.
func focusSelfOnly(selfHwnd windows.HWND) {
	if selfHwnd == 0 {
		return
	}
	activateHwnd(selfHwnd)
}

func wakeAttachedWindowInternal(self *application.WebviewWindow, targetProcessName string, checkAdjacent bool) error {
	if self == nil {
		return ErrWinsnapWindowInvalid
	}
	selfH := uintptr(self.NativeWindow())
	if selfH == 0 {
		return ErrWinsnapWindowInvalid
	}

	targetNames := expandWindowsTargetNames(targetProcessName)
	if len(targetNames) == 0 {
		return errors.New("winsnap: TargetProcessName is empty")
	}

	// Use findMainWindowByProcessNameIncludingMinimized to find windows even after Win+D.
	// This ensures we can wake minimized windows.
	var targetHwnd windows.HWND
	var err error
	for _, n := range targetNames {
		targetHwnd, err = findMainWindowByProcessNameIncludingMinimized(n)
		if err == nil && targetHwnd != 0 {
			break
		}
	}
	if err != nil || targetHwnd == 0 {
		return ErrTargetWindowNotFound
	}

	// If checking adjacency, skip wake if target is already adjacent
	if checkAdjacent && isTargetAdjacent(windows.HWND(selfH), targetHwnd, targetNames) {
		// Target is already visible and adjacent, just focus winsnap without waking target
		focusSelfOnly(windows.HWND(selfH))
		return nil
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

// WakeStandaloneWindow brings the winsnap window to front when it's in standalone state
// (visible but not attached to any target app).
//
// IMPORTANT: On Windows, we must NOT trigger WM_ACTIVATE or other messages that cause
// Wails to internally call Focus(), which would crash WebView2 for HiddenOnTaskbar windows.
// Instead of using SetForegroundWindow (which triggers WM_ACTIVATE), we use:
// - ShowWindow to ensure visibility (SW_RESTORE if minimized, SW_SHOWNOACTIVATE/SW_SHOW otherwise)
// - SetWindowPos with HWND_TOPMOST then HWND_NOTOPMOST to bring to front without activation
// - SWP_NOACTIVATE flag to prevent activation messages
func WakeStandaloneWindow(window *application.WebviewWindow) error {
	if window == nil {
		return ErrWinsnapWindowInvalid
	}
	h := uintptr(window.NativeWindow())
	if h == 0 {
		return ErrWinsnapWindowInvalid
	}

	// Check window state and use appropriate ShowWindow command:
	// - Minimized (iconic): use SW_RESTORE to bring it back
	// - Hidden (not visible): use SW_SHOW to make it visible
	// - Normal/visible: use SW_SHOWNOACTIVATE to avoid stealing focus
	const swShow = 5           // SW_SHOW
	const swShowNoActivate = 4 // SW_SHOWNOACTIVATE

	if isWindowIconic(windows.HWND(h)) {
		// Minimized: must use SW_RESTORE to bring it back
		procShowWindowWake.Call(h, swRestoreWake)
	} else if !isWindowVisible(windows.HWND(h)) {
		// Hidden (never shown or explicitly hidden): use SW_SHOW to make it visible
		// SW_SHOWNOACTIVATE may not work for windows that have never been shown
		procShowWindowWake.Call(h, swShow)
	} else {
		// Normal visible window: use SW_SHOWNOACTIVATE to avoid stealing focus
		procShowWindowWake.Call(h, swShowNoActivate)
	}

	// Bring to front using SetWindowPos with SWP_NOACTIVATE
	// First set as TOPMOST, then remove TOPMOST (this brings window to front of normal z-order)
	const hwndTopMost = ^uintptr(0)   // -1 = HWND_TOPMOST
	const hwndNoTopMost = ^uintptr(1) // -2 = HWND_NOTOPMOST
	flags := uintptr(swpNoMoveWake | swpNoSizeWake | swpNoActivateWake)

	// Set topmost then remove (brings to front without activation)
	procSetWindowPosWake.Call(h, hwndTopMost, 0, 0, 0, 0, flags)
	procSetWindowPosWake.Call(h, hwndNoTopMost, 0, 0, 0, 0, flags)

	return nil
}

// ShowTargetWindowNoActivate shows the target window without activating it.
// This is used when winsnap gains focus and needs the target visible,
// but focus should remain on winsnap itself.
//
// Behavior:
// 1. Find target window by process name (including minimized windows)
// 2. Use ShowWindow with SW_SHOWNOACTIVATE to make it visible
// 3. Adjust z-order so target is just below winsnap (using SWP_NOACTIVATE)
// 4. DO NOT call SetForegroundWindow or any activation APIs
func ShowTargetWindowNoActivate(self *application.WebviewWindow, targetProcessName string) error {
	if self == nil {
		return ErrWinsnapWindowInvalid
	}
	selfH := uintptr(self.NativeWindow())
	if selfH == 0 {
		return ErrWinsnapWindowInvalid
	}

	targetNames := expandWindowsTargetNames(targetProcessName)
	if len(targetNames) == 0 {
		return errors.New("winsnap: TargetProcessName is empty")
	}

	// Find target window (including minimized ones)
	var targetHwnd windows.HWND
	var err error
	for _, n := range targetNames {
		targetHwnd, err = findMainWindowByProcessNameIncludingMinimized(n)
		if err == nil && targetHwnd != 0 {
			break
		}
	}
	if err != nil || targetHwnd == 0 {
		return ErrTargetWindowNotFound
	}

	const swShowNoActivate = 4 // SW_SHOWNOACTIVATE

	// Show target window without activating
	// If minimized, we still use SW_SHOWNOACTIVATE (SW_RESTORE would activate)
	// Note: SW_SHOWNOACTIVATE on minimized window may not restore it fully,
	// but it's better than stealing focus. If target is minimized, user must
	// explicitly click on it to restore.
	if isWindowIconic(targetHwnd) {
		// For minimized windows, use SW_RESTORE but immediately return focus to winsnap
		procShowWindowWake.Call(uintptr(targetHwnd), swRestoreWake)
	} else {
		procShowWindowWake.Call(uintptr(targetHwnd), swShowNoActivate)
	}

	// Adjust z-order: bring target to top without activating
	flags := uintptr(swpNoMoveWake | swpNoSizeWake | swpNoActivateWake)
	procSetWindowPosWake.Call(uintptr(targetHwnd), hwndTopWake, 0, 0, 0, 0, flags)

	// Ensure winsnap stays above target
	procSetWindowPosWake.Call(selfH, hwndTopWake, 0, 0, 0, 0, flags)

	// Keep winsnap just above target
	if isTopMost(targetHwnd) {
		_ = setWindowTopMostNoActivate(windows.HWND(selfH))
	} else {
		_ = setWindowNoTopMostNoActivate(windows.HWND(selfH))
	}
	_ = setWindowPosAfterNoMoveNoSizeNoActivate(windows.HWND(selfH), targetHwnd)

	return nil
}
