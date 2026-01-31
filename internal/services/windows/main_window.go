package windows

import "github.com/wailsapp/wails/v3/pkg/application"

// NewMainWindow 创建主窗口
func NewMainWindow(app *application.App) *application.WebviewWindow {
	return app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:      "main",
		Title:     "WillChat",
		MinWidth:  1064,
		MinHeight: 628,
		Width:     1064,
		Height:    628,
		Frameless: true,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 40,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		BackgroundColour: application.NewRGB(27, 38, 54),
		URL:              "/",
	})
}
