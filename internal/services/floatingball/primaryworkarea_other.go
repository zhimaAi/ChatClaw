//go:build !darwin

package floatingball

import "github.com/wailsapp/wails/v3/pkg/application"

func primaryWorkAreaNative() (application.Rect, float32, bool) {
	return application.Rect{}, 1, false
}

