//go:build windows

package winsnap

import (
	"errors"
	"strings"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
	"golang.org/x/sys/windows"
)

// TopMostVisibleProcessName returns the process image base name (e.g. "WXWork.exe")
// of the top-most (highest z-order) visible top-level window that belongs to any
// of targetProcessNames. If none matches, found=false.
func TopMostVisibleProcessName(targetProcessNames []string) (processName string, found bool, err error) {
	if len(targetProcessNames) == 0 {
		return "", false, nil
	}

	targets := make(map[string]struct{}, len(targetProcessNames))
	for _, raw := range targetProcessNames {
		for _, n := range expandWindowsTargetNames(raw) {
			if n == "" {
				continue
			}
			targets[strings.ToLower(n)] = struct{}{}
		}
	}
	if len(targets) == 0 {
		return "", false, nil
	}

	var out string
	cb := windows.NewCallback(func(hwnd uintptr, _ uintptr) uintptr {
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
		if _, ok := targets[strings.ToLower(exe)]; !ok {
			return 1
		}
		out = exe
		return 0
	})
	_, _, _ = procEnumWindows.Call(cb, 0)
	if out == "" {
		return "", false, nil
	}
	return out, true, nil
}

// MoveOffscreen moves the given window far outside the visible desktop area.
// This is used to represent "hidden (snapping) state" without closing the window.
func MoveOffscreen(window *application.WebviewWindow) error {
	if window == nil {
		return errors.New("winsnap: Window is nil")
	}
	h := uintptr(window.NativeWindow())
	if h == 0 {
		return errors.New("winsnap: native window handle is 0")
	}
	// Get window size to ensure we move it completely off-screen.
	// Move by 3x window dimensions to ensure complete hiding.
	width, height := window.Size()
	if width <= 0 {
		width = 2000 // fallback: large enough to ensure hiding
	}
	if height <= 0 {
		height = 2000 // fallback: large enough to ensure hiding
	}
	offX := int32(-(width * 3))
	offY := int32(-(height * 3))
	return setWindowPosNoSizeNoZ(windows.HWND(h), offX, offY)
}

// MoveToStandalone moves the window to a standalone position (right side of primary screen).
// This is used when the window is no longer attached to any target but should remain visible.
func MoveToStandalone(window *application.WebviewWindow) error {
	if window == nil {
		return errors.New("winsnap: Window is nil")
	}
	h := uintptr(window.NativeWindow())
	if h == 0 {
		return errors.New("winsnap: native window handle is 0")
	}

	// Show window first if hidden (needed to get correct window rect)
	window.Show()

	// Get primary monitor work area (physical pixels)
	var monitorInfo struct {
		cbSize    uint32
		rcMonitor windows.Rect
		rcWork    windows.Rect
		dwFlags   uint32
	}
	monitorInfo.cbSize = uint32(40) // sizeof(MONITORINFO)

	// Get primary monitor
	hMonitor, _, _ := procMonitorFromWindow.Call(h, 1) // MONITOR_DEFAULTTOPRIMARY
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
	if workRight == 0 {
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
)
