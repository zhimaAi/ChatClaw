//go:build !darwin

package floatingball

import "github.com/wailsapp/wails/v3/pkg/application"

func getNativeQuartzFrame(win *application.WebviewWindow) (application.Rect, bool) {
	return application.Rect{}, false
}

func setNativeQuartzFrame(win *application.WebviewWindow, x, y, w, h int) bool {
	return false
}

