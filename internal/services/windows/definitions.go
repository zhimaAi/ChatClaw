package windows

import "github.com/wailsapp/wails/v3/pkg/application"

// DefaultDefinitions 返回子窗口定义
func DefaultDefinitions() []WindowDefinition {
	return []WindowDefinition{
		{
			Name: WindowWinsnap,
			CreateOptions: func() application.WebviewWindowOptions {
				return application.WebviewWindowOptions{
					Name:   WindowWinsnap,
					Title:  "WinSnap",
					Width:  600,
					Height: 400,
					Hidden: true,
					URL:    "/winsnap.html",
				}
			},
			FocusOnShow: true,
		},
	}
}
