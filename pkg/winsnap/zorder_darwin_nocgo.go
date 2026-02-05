//go:build darwin && !cgo

package winsnap

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

func TopMostVisibleProcessName(_ []string) (string, bool, error) {
	return "", false, ErrNotSupported
}

// MoveOffscreen hides the winsnap window on macOS.
// Uses Hide() for reliable hiding instead of moving off-screen.
func MoveOffscreen(window *application.WebviewWindow) error {
	if window == nil {
		return ErrWinsnapWindowInvalid
	}
	window.Hide()
	return nil
}
