package bootstrap

import (
	"fmt"
	"io/fs"
	"time"

	"willchat/internal/services/agents"
	"willchat/internal/services/conversations"
	"willchat/internal/define"
	appservice "willchat/internal/services/app"
	"willchat/internal/services/browser"
	"willchat/internal/services/document"
	"willchat/internal/services/greet"
	"willchat/internal/services/i18n"
	"willchat/internal/services/library"
	"willchat/internal/services/providers"
	"willchat/internal/services/settings"
	"willchat/internal/services/textselection"
	"willchat/internal/services/tray"
	"willchat/internal/services/windows"
	"willchat/internal/services/winsnapchat"
	"willchat/internal/sqlite"
	"willchat/internal/taskmanager"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

type Options struct {
	Assets fs.FS
	Icon   []byte
	Locale string // 语言设置: "zh-CN" 或 "en-US"
}

// NewApp 创建并初始化应用
// 返回 app 实例和 cleanup 函数（用于关闭数据库等资源）
func NewApp(opts Options) (app *application.App, cleanup func(), err error) {
	// 初始化多语言（设置全局语言）
	i18nService := i18n.NewService(opts.Locale)

	// 声明主窗口变量，用于单实例回调
	var mainWindow *application.WebviewWindow

	// 创建应用实例
	app = application.New(application.Options{
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
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
		// 单实例配置：防止多个应用实例同时运行
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: define.SingleInstanceUniqueID,
			OnSecondInstanceLaunch: func(data application.SecondInstanceData) {
				// 当第二个实例启动时，聚焦主窗口
				if mainWindow != nil {
					mainWindow.Restore()
					mainWindow.Show()
					mainWindow.Focus()
				}
			},
		},
	})

	// ========== 初始化基础设施 ==========

	// 初始化数据库
	if err := sqlite.Init(app); err != nil {
		return nil, nil, fmt.Errorf("sqlite init: %w", err)
	}

	// 初始化设置缓存
	if err := settings.InitCache(app); err != nil {
		sqlite.Close()
		return nil, nil, fmt.Errorf("settings cache init: %w", err)
	}

	// 初始化任务管理器（基于 goqite 的持久化消息队列）
	if err := taskmanager.Init(app, sqlite.DB().DB, taskmanager.Config{
		Queues: map[string]taskmanager.QueueConfig{
			taskmanager.QueueThumbnail: {Workers: 8, PollInterval: 50 * time.Millisecond},  // 缩略图：快任务
			taskmanager.QueueDocument:  {Workers: 2, PollInterval: 100 * time.Millisecond}, // 文档处理：慢任务
		},
	}); err != nil {
		sqlite.Close()
		return nil, nil, fmt.Errorf("init task manager: %w", err)
	}

	// ========== 注册应用服务 ==========

	// 注册设置服务
	app.RegisterService(application.NewService(settings.NewSettingsService(app)))
	// 注册供应商服务
	app.RegisterService(application.NewService(providers.NewProvidersService(app)))
	// 注册浏览器服务
	app.RegisterService(application.NewService(browser.NewBrowserService(app)))
	// 注册助手服务
	app.RegisterService(application.NewService(agents.NewAgentsService(app)))
	// 注册会话服务
	app.RegisterService(application.NewService(conversations.NewConversationsService(app)))
	// 注册应用服务
	app.RegisterService(application.NewService(appservice.NewAppService(app)))
	// 注册知识库服务
	app.RegisterService(application.NewService(library.NewLibraryService(app)))
	// 注册文档服务
	app.RegisterService(application.NewService(document.NewDocumentService(app)))

	// ========== 创建窗口 ==========

	// 创建主窗口
	mainWindow = windows.NewMainWindow(app)

	// 创建子窗口服务
	windowService, err := windows.NewWindowService(app, windows.DefaultDefinitions())
	if err != nil {
		sqlite.Close()
		return nil, nil, fmt.Errorf("init window service: %w", err)
	}
	app.RegisterService(application.NewService(windowService))

	// 创建吸附（winsnap）服务
	snapService, err := windows.NewSnapService(app, windowService)
	if err != nil {
		sqlite.Close()
		return nil, nil, fmt.Errorf("init snap service: %w", err)
	}
	app.RegisterService(application.NewService(snapService))

	// winsnap AI chat stream service
	app.RegisterService(application.NewService(winsnapchat.NewWinsnapChatService(app)))

	// 创建划词弹窗服务
	textSelectionService := textselection.NewWithSnapStateGetter(func() windows.SnapState {
		return snapService.GetStatus().State
	})
	app.RegisterService(application.NewService(textSelectionService))

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
		// 根据 settings 中的开关状态启动/停止吸附功能
		_, _ = snapService.SyncFromSettings()
		// 根据 settings 中的开关状态启动/停止划词功能
		textSelectionService.Attach(app, mainWindow, application.WebviewWindowOptions{
			Name:                       textselection.WindowTextSelection,
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
		})
		_, _ = textSelectionService.SyncFromSettings()
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

	return app, func() {
		// Stop task manager before closing database
		if tm := taskmanager.Get(); tm != nil {
			tm.StopNow()
		}
		sqlite.Close()
	}, nil
}
