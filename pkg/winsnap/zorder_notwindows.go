//go:build !windows && !darwin

package winsnap

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

func TopMostVisibleProcessName(_ []string) (string, bool, error) {
	return "", false, ErrNotSupported
}

func MoveOffscreen(_ *application.WebviewWindow) error {
	return ErrNotSupported
}

// MoveToStandalone moves the window to a standalone position.
// Not supported on this platform.
func MoveToStandalone(_ *application.WebviewWindow) error {
	return ErrNotSupported
}
