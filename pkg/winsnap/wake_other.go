//go:build !windows && !darwin

package winsnap

import (
	"errors"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func EnsureWindowVisible(_ *application.WebviewWindow) error {
	return nil
}

// WakeAttachedWindow is not supported on this platform.
func WakeAttachedWindow(_ *application.WebviewWindow, _ string) error {
	return errors.New("winsnap: wake is not supported on this platform")
}

// WakeAttachedWindowWithRefocus is not supported on this platform.
func WakeAttachedWindowWithRefocus(_ *application.WebviewWindow, _ string) error {
	return errors.New("winsnap: wake is not supported on this platform")
}

// WakeStandaloneWindow brings the winsnap window to front when it's in standalone state.
// Fallback implementation: just show and focus the window.
func WakeStandaloneWindow(window *application.WebviewWindow) error {
	if window == nil {
		return errors.New("winsnap: window is nil")
	}
	window.Show()
	window.Focus()
	return nil
}

// BringWinsnapToFront brings the winsnap window to front without stealing focus.
// Fallback implementation: just show the window.
func BringWinsnapToFront(window *application.WebviewWindow) error {
	if window == nil {
		return errors.New("winsnap: window is nil")
	}
	window.Show()
	return nil
}

// SyncAttachedZOrderNoActivate is a no-op on unsupported platforms.
func SyncAttachedZOrderNoActivate(_ *application.WebviewWindow, _ string) error {
	return nil
}

// ShowTargetWindowNoActivate is not supported on this platform.
// Returns nil to allow caller to proceed without error.
func ShowTargetWindowNoActivate(_ *application.WebviewWindow, _ string) error {
	return nil
}
