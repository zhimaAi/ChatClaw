//go:build !windows && !darwin

package textselection

import "github.com/wailsapp/wails/v3/pkg/application"

// showPopupNative shows the popup window.
// On non-Windows/macOS platforms, just use Wails' Show().
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

func forcePopupTopMostNoActivate(_ *application.WebviewWindow) {}

func setPopupPositionCocoa(_ *application.WebviewWindow, _, _ int) {}

func showPopupClampedCocoa(_ *application.WebviewWindow, _, _, _, _ int) {}

func setPopupPositionPhysical(_ *application.WebviewWindow, _, _, _, _ int) {}

func getPopupWindowRect(_ *application.WebviewWindow) (int32, int32, int32, int32) {
	return 0, 0, 0, 0
}
