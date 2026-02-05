//go:build windows

package winsnap

import (
	"errors"
	"strings"

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
