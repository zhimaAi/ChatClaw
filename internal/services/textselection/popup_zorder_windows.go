//go:build windows

package textselection

import (
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
)

var (
	procSetWindowPos   = modUser32.NewProc("SetWindowPos")
	procGetWindowRect_ = modUser32.NewProc("GetWindowRect")
)

const (
	hwndTopMost = ^uintptr(0) // (HWND)-1

	swpNoMove     = 0x0002
	swpNoSize     = 0x0001
	swpNoActivate = 0x0010
)

var procShowWindowPopup = modUser32.NewProc("ShowWindow")

const (
	swHidePopup = 0 // SW_HIDE
	swShowNA    = 8 // SW_SHOWNA: show window without activating it
)

// showPopupNative shows the popup window without activating it.
// Uses native ShowWindow(SW_SHOWNA) instead of Wails' w.Show() which may
// internally call ShowWindow(SW_SHOW) and trigger window activation, stealing
// focus from the previously active window (e.g. main chat window).
func showPopupNative(w *application.WebviewWindow) {
	if w == nil {
		return
	}
	h := uintptr(w.NativeWindow())
	if h == 0 {
		return
	}
	procShowWindowPopup.Call(h, swShowNA)
}

// hidePopupNative hides the popup window using native ShowWindow(SW_HIDE).
// We use native API instead of Wails' w.Hide() because Wails Hide() internally
// may call Focus(), which crashes WebView2 on popup/tool windows.
// Using native SW_HIDE instead of moving off-screen avoids the window being
// discovered on multi-monitor setups.
func hidePopupNative(w *application.WebviewWindow) {
	if w == nil {
		return
	}
	h := uintptr(w.NativeWindow())
	if h == 0 {
		return
	}
	procShowWindowPopup.Call(h, swHidePopup)
}

// setPopupPositionCocoa is only used on macOS; no-op on Windows.
func setPopupPositionCocoa(_ *application.WebviewWindow, _, _ int) {}

// showPopupClampedCocoa is only used on macOS; no-op on Windows.
func showPopupClampedCocoa(_ *application.WebviewWindow, _, _, _, _ int) {}

// setPopupPositionPhysical positions and sizes the popup at the given physical pixel
// coordinates using native SetWindowPos. This bypasses Wails' DIP coordinate conversion
// which can be inaccurate on multi-monitor setups with different DPI.
// The window is also set to HWND_TOPMOST in the same call.
func setPopupPositionPhysical(w *application.WebviewWindow, x, y, width, height int) {
	if w == nil {
		return
	}
	h := uintptr(w.NativeWindow())
	if h == 0 {
		return
	}
	procSetWindowPos.Call(
		h,
		hwndTopMost,
		uintptr(x), uintptr(y), uintptr(width), uintptr(height),
		uintptr(swpNoActivate),
	)
}

// getPopupWindowRect returns the popup window's actual rectangle in physical screen
// pixels via GetWindowRect. This is always accurate regardless of how the window was
// positioned (Wails DIP or native SetWindowPos).
func getPopupWindowRect(w *application.WebviewWindow) (left, top, right, bottom int32) {
	if w == nil {
		return 0, 0, 0, 0
	}
	h := uintptr(w.NativeWindow())
	if h == 0 {
		return 0, 0, 0, 0
	}
	type rect struct {
		Left, Top, Right, Bottom int32
	}
	var r rect
	procGetWindowRect_.Call(h, uintptr(unsafe.Pointer(&r)))
	return r.Left, r.Top, r.Right, r.Bottom
}

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
