//go:build windows

package floatingball

import "github.com/wailsapp/wails/v3/pkg/application"

// On Windows, BackgroundTypeTransparent + Frameless triggers WS_EX_TRANSPARENT, which makes the
// window click-through and can break hover/drag interactions.
// BackgroundTypeTranslucent keeps the window visually transparent while remaining interactive.
func floatingBallBackgroundType() application.BackgroundType {
	return application.BackgroundTypeTranslucent
}

