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
					Width:  400,
					Height: 720,
					Hidden: true,
					// Use custom titlebar inside the webview.
					Frameless: true,
					URL:       "/winsnap.html",
				}
			},
			// Side window should not steal focus when shown.
			FocusOnShow: false,
		},
	}
}
