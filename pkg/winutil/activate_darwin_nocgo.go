//go:build darwin && !cgo

package winutil

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

// ForceActivateWindow fallback when CGO is disabled on macOS.
// Safely checks if the window is still valid before calling Focus.
func ForceActivateWindow(w *application.WebviewWindow) {
	if w == nil {
		return
	}
	// Check if window is still valid before calling Focus
	nativeHandle := w.NativeWindow()
	if nativeHandle == nil || uintptr(nativeHandle) == 0 {
		return
	}
	w.Focus()
}
