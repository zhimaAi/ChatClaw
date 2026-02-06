package windows

import (
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// DefaultDefinitions 返回子窗口定义
func DefaultDefinitions() []WindowDefinition {
	return []WindowDefinition{
		{
			Name: WindowWinsnap,
			CreateOptions: func() application.WebviewWindowOptions {
				return application.WebviewWindowOptions{
					Name:   WindowWinsnap,
					Title:  "WillChat",
					Width:  400,
					Height: 720,
					Hidden: true,
					// Use native titlebar for proper close/minimize/fullscreen buttons.
					Frameless: false,
					// Let z-order follow the attached target window (not global top-most).
					// - Windows: we insert after the target hwnd (see pkg/winsnap/winsnap_windows.go)
					// - macOS: we order above the target window number on activation (see pkg/winsnap/winsnap_darwin.go)
					AlwaysOnTop: runtime.GOOS != "windows" && runtime.GOOS != "darwin",
					URL:         "/winsnap.html",
					// Windows specific: hide from taskbar
					Windows: application.WindowsWindow{
						HiddenOnTaskbar: true,
					},
					Mac: application.MacWindow{
						// Use normal window level; ordering is handled dynamically to stay above the target window only.
						WindowLevel: application.MacWindowLevelNormal,
						CollectionBehavior: application.MacWindowCollectionBehaviorCanJoinAllSpaces |
							application.MacWindowCollectionBehaviorTransient |
							application.MacWindowCollectionBehaviorIgnoresCycle,
					},
				}
			},
			// Side window should not steal focus when shown.
			FocusOnShow: false,
		},
		{
			Name: WindowTextSelection,
			CreateOptions: func() application.WebviewWindowOptions {
				return application.WebviewWindowOptions{
					Name:                       WindowTextSelection,
					Title:                      "TextSelection",
					Width:                      140,
					Height:                     50,
					Hidden:                     true,
					Frameless:                  true,
					AlwaysOnTop:                true,
					DisableResize:              true,
					BackgroundType:             application.BackgroundTypeTransparent,
					DefaultContextMenuDisabled: true,
					InitialPosition:            application.WindowXY,
					URL:                        "/selection.html",
					// Windows specific: hide from taskbar
					Windows: application.WindowsWindow{
						HiddenOnTaskbar: true,
					},
					Mac: application.MacWindow{
						Backdrop:    application.MacBackdropTransparent,
						WindowLevel: application.MacWindowLevelFloating,
						CollectionBehavior: application.MacWindowCollectionBehaviorCanJoinAllSpaces |
							application.MacWindowCollectionBehaviorTransient |
							application.MacWindowCollectionBehaviorIgnoresCycle,
					},
				}
			},
			// Popup should not steal focus when shown.
			FocusOnShow: false,
		},
	}
}
