//go:build windows

package textselection

import "github.com/wailsapp/wails/v3/pkg/application"

var (
	procSetWindowPos = modUser32.NewProc("SetWindowPos")
)

const (
	hwndTopMost = ^uintptr(0) // (HWND)-1

	swpNoMove     = 0x0002
	swpNoSize     = 0x0001
	swpNoActivate = 0x0010
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
	_, _, _ = procSetWindowPos.Call(
		h,
		hwndTopMost,
		0, 0, 0, 0,
		uintptr(swpNoMove|swpNoSize|swpNoActivate),
	)
}
