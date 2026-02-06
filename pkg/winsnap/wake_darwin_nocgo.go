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

// WakeAttachedWindowWithRefocus is not available without CGO on darwin.
func WakeAttachedWindowWithRefocus(_ *application.WebviewWindow, _ string) error {
	return errors.New("winsnap: wake requires cgo on darwin")
}

// WakeStandaloneWindow brings the winsnap window to front when it's in standalone state.
// Fallback implementation without CGO: just show and focus the window.
func WakeStandaloneWindow(window *application.WebviewWindow) error {
	if window == nil {
		return ErrWinsnapWindowInvalid
	}
	window.Show()
	window.Focus()
	return nil
}

// BringWinsnapToFront brings the winsnap window to front without stealing focus.
// Fallback implementation without CGO: just show the window.
func BringWinsnapToFront(window *application.WebviewWindow) error {
	if window == nil {
		return ErrWinsnapWindowInvalid
	}
	window.Show()
	return nil
}

// SyncAttachedZOrderNoActivate is not available without CGO on darwin.
func SyncAttachedZOrderNoActivate(_ *application.WebviewWindow, _ string) error {
	return errors.New("winsnap: z-order sync requires cgo on darwin")
}

// ShowTargetWindowNoActivate is not available without CGO on darwin.
// Returns nil to allow caller to proceed without error.
func ShowTargetWindowNoActivate(_ *application.WebviewWindow, _ string) error {
	return nil
}
