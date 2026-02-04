//go:build !windows && !darwin

package winsnap

import (
	"errors"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// WakeAttachedWindow is not supported on this platform.
func WakeAttachedWindow(_ *application.WebviewWindow, _ string) error {
	return errors.New("winsnap: wake is not supported on this platform")
}
