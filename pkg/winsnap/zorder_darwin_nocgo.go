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

// MoveToStandalone moves the window to a standalone position.
// This is used when the window is no longer attached to any target but should remain visible.
func MoveToStandalone(window *application.WebviewWindow) error {
	if window == nil {
		return ErrWinsnapWindowInvalid
	}
	// Show window first if hidden
	window.Show()

	// Get window size
	width, height := window.Size()
	if width <= 0 {
		width = 400
	}
	if height <= 0 {
		height = 720
	}

	// Get screen bounds from Wails
	screens, err := window.GetScreen()
	if err != nil || screens == nil {
		// Fallback: use reasonable defaults
		window.SetPosition(1920-width-20, (1080-height)/2)
		return nil
	}

	// Position: right side with 20px margin, vertically centered
	workRight := screens.Bounds.X + screens.Bounds.Width
	workTop := screens.Bounds.Y
	workHeight := screens.Bounds.Height

	posX := workRight - width - 20
	posY := workTop + (workHeight-height)/2

	window.SetPosition(posX, posY)
	return nil
}
