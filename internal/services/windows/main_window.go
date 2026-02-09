package windows

import (
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// NewMainWindow 创建主窗口
// 使用 frameless 模式，自定义窗口控制按钮
// - macOS: 红黄绿按钮在左侧
// - Windows: 最小化/最大化/关闭按钮在右侧
//
// ⚠️ 已知问题 (Wails v3 alpha.40):
// macOS frameless 窗口的 Minimise() 无效，前端暂时使用 Hide() 替代。
// 详见: https://github.com/wailsapp/wails/issues/4294
//
// Windows 启动黑屏问题:
// 在 Windows 上，webview 加载时会有短暂黑屏（窗口已显示但 HTML 未渲染完成）。
// 解决方案：Windows 上初始隐藏窗口，待前端 Vue 应用挂载后再通过 AppService.ShowMainWindow() 显示。
func NewMainWindow(app *application.App) *application.WebviewWindow {
	return app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             "main",
		Title:            "WillChat",
		MinWidth:         1064,
		MinHeight:        628,
		Width:            1280,
		Height:           800,
		Frameless:        true,
		EnableFileDrop:   true,
		BackgroundColour: application.NewRGB(27, 38, 54),
		URL:              "/",
		// Windows: start hidden to avoid black screen flash during webview loading.
		// Frontend will call AppService.ShowMainWindow() after Vue app is mounted.
		Hidden: runtime.GOOS == "windows",
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 40,
			Backdrop:                application.MacBackdropTranslucent,
		},
	})
}
