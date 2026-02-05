//go:build darwin && !cgo

package winsnap

import (
	"errors"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// EnsureWindowVisible shows the winsnap window on macOS.
// Since MoveOffscreen uses Hide() on macOS, we need to use Show() to make it visible again.
func EnsureWindowVisible(window *application.WebviewWindow) error {
	if window == nil {
		return ErrWinsnapWindowInvalid
	}
	window.Show()
	return nil
}

func WakeAttachedWindow(_ *application.WebviewWindow, _ string) error {
	return errors.New("winsnap: wake requires cgo on darwin")
}
