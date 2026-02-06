//go:build windows

package winsnap

import (
	"errors"

	"github.com/wailsapp/wails/v3/pkg/application"
	"golang.org/x/sys/windows"
)

// IsWindowMinimized reports whether the given window is minimized (iconic).
// It does not change window state.
func IsWindowMinimized(window *application.WebviewWindow) (bool, error) {
	if window == nil {
		return false, errors.New("winsnap: Window is nil")
	}
	h := uintptr(window.NativeWindow())
	if h == 0 {
		return false, errors.New("winsnap: native window handle is 0")
	}
	return isWindowIconic(windows.HWND(h)), nil
}
