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
