//go:build darwin && !cgo

package winsnap

import (
	"errors"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func EnsureWindowVisible(_ *application.WebviewWindow) error {
	return nil
}

func WakeAttachedWindow(_ *application.WebviewWindow, _ string) error {
	return errors.New("winsnap: wake requires cgo on darwin")
}
