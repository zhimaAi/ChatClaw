//go:build darwin && !cgo

package winsnap

import (
	"errors"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func TopMostVisibleProcessName(_ []string) (string, bool, error) {
	return "", false, errors.New("winsnap: not supported without cgo on darwin")
}

func MoveOffscreen(_ *application.WebviewWindow) error {
	return errors.New("winsnap: not supported without cgo on darwin")
}
