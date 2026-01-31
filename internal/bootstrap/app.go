package bootstrap

import (
	"fmt"
	"io/fs"

	appservice "willchat/internal/services/app"
	"willchat/internal/services/browser"
	"willchat/internal/services/greet"
	"willchat/internal/services/i18n"
	"willchat/internal/services/settings"
	"willchat/internal/services/tray"
	"willchat/internal/services/windows"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

type Options struct {
	Assets fs.FS
	Icon   []byte
	Locale string // 语言设置: "zh-CN" 或 "en-US"
}

func NewApp(opts Options) (*application.App, error) {
	// 初始化多语言（设置全局语言）
	i18nService := i18n.NewService(opts.Locale)

	// 创建应用实例
	app := application.New(application.Options{
		Name:        "WillChat",
		Description: "WillChat Desktop App",
		Services: []application.Service{
			application.NewService(greet.NewGreetService("Hello, ")),
			application.NewService(i18nService),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(opts.Assets),
		},
		Mac: application.MacOptions{
			// 设置为 false，允许应用在窗口隐藏到托盘后继续运行
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
	})

	// 注册设置服务
	app.RegisterService(application.NewService(settings.NewSettingsService(app)))

	// 注册浏览器服务
	app.RegisterService(application.NewService(browser.NewBrowserService(app)))

	// 注册应用服务
	app.RegisterService(application.NewService(appservice.NewAppService(app)))

	// 创建主窗口
	mainWindow := windows.NewMainWindow(app)

	// 创建子窗口服务
	windowService, err := windows.NewWindowService(app, windows.DefaultDefinitions())
	if err != nil {
		return nil, fmt.Errorf("init window service: %w", err)
	}
	app.RegisterService(application.NewService(windowService))

	// 创建系统托盘
	systrayMenu := app.NewMenu()
	systrayMenu.Add(i18n.T("systray.show")).OnClick(func(ctx *application.Context) {
		mainWindow.Show()
		mainWindow.Focus()
	})
	systrayMenu.Add(i18n.T("systray.quit")).OnClick(func(ctx *application.Context) {
		app.Quit()
	})
	systray := app.SystemTray.New().SetIcon(opts.Icon).SetMenu(systrayMenu)

	// 创建托盘服务（用于前端动态控制 show/hide + 缓存关闭策略）
	trayService := tray.NewTrayService(app, systray)
	app.RegisterService(application.NewService(trayService))
	// 应用启动后再加载设置并应用 Show/Hide（确保 sqlite 已初始化）
	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(_ *application.ApplicationEvent) {
		trayService.InitFromSettings()
	})

	// 监听主窗口关闭事件，实现"关闭时最小化"
	mainWindow.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		minimizeEnabled := trayService.IsMinimizeToTrayEnabled()
		if minimizeEnabled {
			app.Logger.Info("WindowClosing: hiding window to tray")
			mainWindow.Hide()
			e.Cancel()
		} else {
			app.Quit()
		}
	})

	// 点击 Dock 图标时显示窗口
	app.Event.OnApplicationEvent(events.Mac.ApplicationShouldHandleReopen, func(event *application.ApplicationEvent) {
		mainWindow.UnMinimise()
		mainWindow.Show()
		mainWindow.Focus()
	})

	return app, nil
}
