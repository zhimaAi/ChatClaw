//go:build darwin && !cgo

package textselection

import "github.com/wailsapp/wails/v3/pkg/application"

// showPopupNative shows the popup window.
// On macOS (no-cgo), just use Wails' Show().
func showPopupNative(w *application.WebviewWindow) {
	if w == nil {
		return
	}
	w.Show()
}

// hidePopupNative hides the popup window using the platform's native hide mechanism.
func hidePopupNative(w *application.WebviewWindow) {
	if w == nil {
		return
	}
	w.Hide()
}

// setPopupPositionPhysical is only used on Windows; no-op on macOS.
func setPopupPositionPhysical(_ *application.WebviewWindow, _, _, _, _ int) {}

// getPopupWindowRect is only used on Windows; returns zero on macOS.
func getPopupWindowRect(_ *application.WebviewWindow) (int32, int32, int32, int32) {
	return 0, 0, 0, 0
}

// forcePopupTopMostNoActivate is a no-op without CGO on macOS.
func forcePopupTopMostNoActivate(_ *application.WebviewWindow) {}

// setPopupPositionCocoa is a no-op without CGO on macOS.
func setPopupPositionCocoa(_ *application.WebviewWindow, _, _ int) {}

// showPopupClampedCocoa is a no-op without CGO on macOS.
func showPopupClampedCocoa(_ *application.WebviewWindow, _, _, _, _ int) {}
