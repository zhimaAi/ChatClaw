//go:build windows

package textselection

import "github.com/wailsapp/wails/v3/pkg/application"

const (
	hwndTopMost = ^uintptr(0) // (HWND)-1
)

// forcePopupTopMostNoActivate ensures the popup stays above other top-most windows
// (e.g. winsnap window) without activating/focusing it.
// Safely handles the case when the window has been closed/released.
func forcePopupTopMostNoActivate(w *application.WebviewWindow) {
	if w == nil {
		return
	}
	// Check if window is still valid by getting its native handle
	h := uintptr(w.NativeWindow())
	if h == 0 {
		// Window has been closed or is invalid
		return
	}
	// Bring to the top of the "top-most" z-order group, but do not activate.
	_, _, _ = procSetWindowPosTS.Call(
		h,
		hwndTopMost,
		0, 0, 0, 0,
		uintptr(swpNoMove|swpNoSize|swpNoActivate),
	)
}
