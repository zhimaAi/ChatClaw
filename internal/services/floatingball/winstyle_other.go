//go:build !windows

package floatingball

import "github.com/wailsapp/wails/v3/pkg/application"

func enableWindowsPopupStyle(_ *application.WebviewWindow, _ *FloatingBallService) {}

