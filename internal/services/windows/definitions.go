package windows

import "github.com/wailsapp/wails/v3/pkg/application"

// DefaultDefinitions 返回子窗口定义
func DefaultDefinitions() []WindowDefinition {
	return []WindowDefinition{
		{
			Name: WindowSettings,
			CreateOptions: func() application.WebviewWindowOptions {
				return application.WebviewWindowOptions{
					Name:   WindowSettings,
					Title:  "Settings",
					Width:  600,
					Height: 400,
					Hidden: true,
					URL: "/settings.html",
				}
			},
			FocusOnShow: true,
		},
	}
}

