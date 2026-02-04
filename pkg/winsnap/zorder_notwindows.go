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
