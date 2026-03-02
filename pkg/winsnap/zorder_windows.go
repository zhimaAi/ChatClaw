//go:build windows

package winsnap

import (
	"errors"
	"strings"
	"sync"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
	"golang.org/x/sys/windows"
)

// Package-level state for the EnumWindows callback.
// The callback is created once (via sync.Once) to avoid exhausting the
// Go runtime's fixed-size callback table (~2000 slots) on Windows.
// Access is serialised by enumMu; EnumWindows invokes the callback
// synchronously so the lock is held for the entire enumeration.
var (
	enumCBOnce  sync.Once
	enumCB      uintptr
	enumMu      sync.Mutex
	enumTargets map[string]struct{}
	enumResult  string
)

var procGetForegroundWindowZOrder = modUser32.NewProc("GetForegroundWindow")

// ForegroundProcessName returns current foreground process executable name.
// If current app is foreground, isSelf=true.
func ForegroundProcessName() (processName string, isSelf bool, err error) {
	foregroundHWND, _, _ := procGetForegroundWindowZOrder.Call()
	if foregroundHWND == 0 {
		return "", false, nil
	}
	foregroundPID, ferr := getWindowProcessID(windows.HWND(foregroundHWND))
	if ferr != nil || foregroundPID == 0 {
		return "", false, ferr
	}
	if foregroundPID == windows.GetCurrentProcessId() {
		return "", true, nil
	}
	foregroundEXE, eerr := getProcessImageBaseName(foregroundPID)
	if eerr != nil {
		return "", false, eerr
	}
	return foregroundEXE, false, nil
}

func buildWindowsTargetSet(targetProcessNames []string) map[string]struct{} {
	targets := make(map[string]struct{}, len(targetProcessNames))
	for _, raw := range targetProcessNames {
		for _, n := range expandWindowsTargetNames(raw) {
			if n == "" {
				continue
			}
			targets[strings.ToLower(n)] = struct{}{}
		}
	}
	return targets
}

func frontMostTargetFromSet(targets map[string]struct{}) (processName string, found bool, err error) {
	if len(targets) == 0 {
		return "", false, nil
	}
	foregroundEXE, isSelf, ferr := ForegroundProcessName()
	if ferr != nil || foregroundEXE == "" && !isSelf {
		return "", false, nil
	}
	if isSelf {
		return "", false, ErrSelfIsFrontmost
	}
	if _, ok := targets[strings.ToLower(foregroundEXE)]; !ok {
		return "", false, nil
	}
	return foregroundEXE, true, nil
}

// FrontMostTargetProcessName returns the foreground target process among targetProcessNames.
// If foreground app is not one of targets, found=false.
// Returns ErrSelfIsFrontmost when our own app is foreground.
func FrontMostTargetProcessName(targetProcessNames []string) (processName string, found bool, err error) {
	targets := buildWindowsTargetSet(targetProcessNames)
	return frontMostTargetFromSet(targets)
}

// IsTargetProcessVisible reports whether target process currently has a visible main window.
func IsTargetProcessVisible(targetProcessName string) (bool, error) {
	targetNames := expandWindowsTargetNames(targetProcessName)
	if len(targetNames) == 0 {
		return false, nil
	}
	for _, name := range targetNames {
		h, err := findMainWindowByProcessName(name)
		if err == nil && h != 0 {
			return true, nil
		}
	}
	return false, nil
}

// enumWindowsProc is the single, reusable EnumWindows callback.
func enumWindowsProc(hwnd uintptr, _ uintptr) uintptr {
	h := windows.HWND(hwnd)
	if !isTopLevelCandidate(h) {
		return 1
	}
	pid, perr := getWindowProcessID(h)
	if perr != nil || pid == 0 {
		return 1
	}
	exe, eerr := getProcessImageBaseName(pid)
	if eerr != nil {
		return 1
	}
	if _, ok := enumTargets[strings.ToLower(exe)]; !ok {
		return 1
	}
	enumResult = exe
	return 0 // stop enumeration
}

// TopMostVisibleProcessName returns the process image base name (e.g. "WXWork.exe")
// of the top-most (highest z-order) visible top-level window that belongs to any
// of targetProcessNames. If none matches, found=false.
func TopMostVisibleProcessName(targetProcessNames []string) (processName string, found bool, err error) {
	if len(targetProcessNames) == 0 {
		return "", false, nil
	}

	targets := buildWindowsTargetSet(targetProcessNames)
	if len(targets) == 0 {
		return "", false, nil
	}

	// Prefer the true foreground application when it belongs to targets.
	frontmost, frontFound, frontErr := frontMostTargetFromSet(targets)
	if frontErr != nil {
		return "", false, frontErr
	}
	if frontFound {
		return frontmost, true, nil
	}

	// Serialise access to the shared callback state.
	enumMu.Lock()
	defer enumMu.Unlock()

	enumTargets = targets
	enumResult = ""

	enumCBOnce.Do(func() {
		enumCB = windows.NewCallback(enumWindowsProc)
	})

	_, _, _ = procEnumWindows.Call(enumCB, 0)
	if enumResult == "" {
		return "", false, nil
	}
	return enumResult, true, nil
}

// MoveOffscreen hides the given window using native ShowWindow(SW_HIDE).
// This is used to represent "hidden (snapping) state" without closing the window.
// Using native SW_HIDE instead of moving off-screen avoids the window being
// discovered on multi-monitor setups where off-screen coordinates may be visible.
// Note: We use native ShowWindow instead of Wails' w.Hide() because Wails Hide()
// internally may call Focus(), which crashes WebView2 on HiddenOnTaskbar windows.
func MoveOffscreen(window *application.WebviewWindow) error {
	if window == nil {
		return errors.New("winsnap: Window is nil")
	}
	h := uintptr(window.NativeWindow())
	if h == 0 {
		return errors.New("winsnap: native window handle is 0")
	}
	const swHide = 0 // SW_HIDE
	procShowWindowZOrder.Call(h, swHide)
	return nil
}

// MoveToStandalone moves the window to a standalone position (right side of the screen where the window is located).
// This is used when the window is no longer attached to any target but should remain visible.
// Multi-monitor aware: uses the monitor where the window is currently located.
func MoveToStandalone(window *application.WebviewWindow) error {
	if window == nil {
		return errors.New("winsnap: Window is nil")
	}
	h := uintptr(window.NativeWindow())
	if h == 0 {
		return errors.New("winsnap: native window handle is 0")
	}

	// Show window first if hidden (needed to get correct window rect)
	// Use native API with SW_SHOWNOACTIVATE to avoid stealing focus
	showWindowNoActivate(windows.HWND(h))

	// Get monitor work area (physical pixels) for the monitor where the window is located
	var monitorInfo struct {
		cbSize    uint32
		rcMonitor windows.Rect
		rcWork    windows.Rect
		dwFlags   uint32
	}
	monitorInfo.cbSize = uint32(40) // sizeof(MONITORINFO)

	// Get the monitor where the window is currently located (or nearest if off-screen)
	// MONITOR_DEFAULTTONEAREST = 2
	hMonitor, _, _ := procMonitorFromWindow.Call(h, 2)
	if hMonitor != 0 {
		procGetMonitorInfo.Call(hMonitor, uintptr(unsafe.Pointer(&monitorInfo)))
	}

	// Get window size using GetWindowRect (physical pixels, DPI-aware)
	// This is more reliable than window.Size() which may return logical pixels
	var windowRect rect
	var width, height int32
	if err := getWindowRect(windows.HWND(h), &windowRect); err == nil {
		width = windowRect.Right - windowRect.Left
		height = windowRect.Bottom - windowRect.Top
	}
	if width <= 0 {
		width = 400
	}
	if height <= 0 {
		height = 720
	}

	// Calculate position: right side of screen with some margin
	workLeft := int32(monitorInfo.rcWork.Left)
	workRight := int32(monitorInfo.rcWork.Right)
	workTop := int32(monitorInfo.rcWork.Top)
	workBottom := int32(monitorInfo.rcWork.Bottom)

	// If we couldn't get monitor info, use reasonable defaults
	if workRight == 0 && workLeft == 0 {
		workLeft = 0
		workRight = 1920
		workTop = 0
		workBottom = 1080
	}

	// Position: right side with 20px margin from right edge, vertically centered
	posX := workRight - width - 20
	posY := workTop + (workBottom-workTop-height)/2

	// Clamp to work area bounds to prevent window going off-screen
	if posX < workLeft {
		posX = workLeft + 20
	}
	if posX+width > workRight {
		posX = workRight - width - 20
	}
	if posY < workTop {
		posY = workTop
	}
	if posY+height > workBottom {
		posY = workBottom - height
	}

	return setWindowPosNoSizeNoZ(windows.HWND(h), posX, posY)
}

var (
	procMonitorFromWindow = modUser32.NewProc("MonitorFromWindow")
	procGetMonitorInfo    = modUser32.NewProc("GetMonitorInfoW")
	procShowWindowZOrder  = modUser32.NewProc("ShowWindow")
)

const (
	swShowNoActivateZOrder = 4 // SW_SHOWNOACTIVATE
	swRestoreZOrder        = 9 // SW_RESTORE
)

// showWindowNoActivate shows window without activating it.
// Uses SW_SHOWNOACTIVATE to avoid stealing focus from other apps.
func showWindowNoActivate(hwnd windows.HWND) {
	procShowWindowZOrder.Call(uintptr(hwnd), swShowNoActivateZOrder)
}

// restoreWindowNoActivate restores a minimized window without activating it.
// Uses SW_RESTORE first (to restore from minimized state), but this may activate.
// For best results, only call this when window is minimized.
func restoreWindowNoActivate(hwnd windows.HWND) {
	procShowWindowZOrder.Call(uintptr(hwnd), swRestoreZOrder)
}
