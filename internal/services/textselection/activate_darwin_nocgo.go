//go:build darwin && !cgo

package textselection

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

// forceActivateWindow fallback when CGO is disabled on macOS.
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

// ActivateAppByPid is a no-op when CGO is disabled.
func ActivateAppByPid(pid int32) {}
