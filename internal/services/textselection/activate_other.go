//go:build !windows && !darwin

package textselection

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

// forceActivateWindow on non-Windows/macOS platforms directly calls Focus.
// Safely checks if the window is still valid before calling Focus.
func forceActivateWindow(w *application.WebviewWindow) {
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
