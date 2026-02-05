//go:build !windows

package floatingball

import "github.com/wailsapp/wails/v3/pkg/application"

func floatingBallBackgroundType() application.BackgroundType {
	return application.BackgroundTypeTransparent
}

