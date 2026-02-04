//go:build darwin && !cgo

package textselection

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

// forceActivateWindow fallback when CGO is disabled on macOS.
func forceActivateWindow(w *application.WebviewWindow) {
	if w != nil {
		w.Focus()
	}
}

// ActivateAppByPid is a no-op when CGO is disabled.
func ActivateAppByPid(pid int32) {}
