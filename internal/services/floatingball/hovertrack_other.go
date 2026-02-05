//go:build !darwin || ios

package floatingball

import "github.com/wailsapp/wails/v3/pkg/application"

func enableMacHoverTracking(_ *application.WebviewWindow) {}

