package bootstrap

import (
	"fmt"
	"io/fs"

	"willchat/internal/services/greet"
	"willchat/internal/services/i18n"
	"willchat/internal/services/windows"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type Options struct {
	Assets fs.FS
	Icon   []byte
	Locale string // 语言设置: "zh-CN" 或 "en-US"
}

func NewApp(opts Options) (*application.App, error) {
	// 创建多语言服务
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
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

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
	systrayMenu.Add(i18nService.T("systray.show")).OnClick(func(ctx *application.Context) {
		mainWindow.Show()
		mainWindow.Focus()
	})
	systrayMenu.Add(i18nService.T("systray.quit")).OnClick(func(ctx *application.Context) {
		app.Quit()
	})
	app.SystemTray.New().SetIcon(opts.Icon).SetMenu(systrayMenu)

	return app, nil
}
