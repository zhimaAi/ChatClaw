package bootstrap

import (
	"fmt"
	"io/fs"
	"sync"

	"willchat/internal/define"
	"willchat/internal/services/agents"
	appservice "willchat/internal/services/app"
	"willchat/internal/services/browser"
	"willchat/internal/services/greet"
	"willchat/internal/services/i18n"
	"willchat/internal/services/library"
	"willchat/internal/services/providers"
	"willchat/internal/services/settings"
	"willchat/internal/services/textselection"
	"willchat/internal/services/tray"
	"willchat/internal/services/windows"
	"willchat/internal/services/winsnapchat"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// mainWindowManager handles safe main window operations with validity checks.
// It ensures the main window is valid before performing operations and
// can recreate the window if it becomes invalid.
type mainWindowManager struct {
	mu     sync.Mutex
	app    *application.App
	window *application.WebviewWindow
}

// isWindowValid checks if the window's native handle is still valid.
func (m *mainWindowManager) isWindowValid() bool {
	if m.window == nil {
		return false
	}
	nativeHandle := m.window.NativeWindow()
	return nativeHandle != nil && uintptr(nativeHandle) != 0
}

// safeWake safely wakes the main window, checking validity first.
// If the window is invalid, it does nothing (main window recreation
// would require app restart in most cases).
func (m *mainWindowManager) safeWake() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.isWindowValid() {
		// Main window is invalid, cannot wake
		// In production, main window invalidation typically means app should restart
		return
	}

	m.window.Restore()
	m.window.Show()
	m.window.Focus()
}

// safeShow safely shows and focuses the main window.
func (m *mainWindowManager) safeShow() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.isWindowValid() {
		return
	}

	m.window.Show()
	m.window.Focus()
}

// safeUnMinimiseAndShow safely unminimizes, shows and focuses the main window.
func (m *mainWindowManager) safeUnMinimiseAndShow() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.isWindowValid() {
		return
	}

	m.window.UnMinimise()
	m.window.Show()
	m.window.Focus()
}

// safeHide safely hides the main window.
func (m *mainWindowManager) safeHide() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.isWindowValid() {
		return
	}

	m.window.Hide()
}

// getWindow returns the underlying window (for operations that need direct access).
func (m *mainWindowManager) getWindow() *application.WebviewWindow {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.window
}

type Options struct {
	Assets fs.FS
	Icon   []byte
	Locale string // 语言设置: "zh-CN" 或 "en-US"
}

func NewApp(opts Options) (*application.App, error) {
	// 初始化多语言（设置全局语言）
	i18nService := i18n.NewService(opts.Locale)

	// 主窗口管理器，用于安全的窗口操作
	mainWinMgr := &mainWindowManager{}

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
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
		// 单实例配置：防止多个应用实例同时运行
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: define.SingleInstanceUniqueID,
			OnSecondInstanceLaunch: func(data application.SecondInstanceData) {
				// 当第二个实例启动时，安全地聚焦主窗口
				mainWinMgr.safeWake()
			},
		},
	})

	// 注册设置服务
	app.RegisterService(application.NewService(settings.NewSettingsService(app)))

	// 注册供应商服务
	app.RegisterService(application.NewService(providers.NewProvidersService(app)))
	// 注册浏览器服务
	app.RegisterService(application.NewService(browser.NewBrowserService(app)))

	// 注册助手服务
	app.RegisterService(application.NewService(agents.NewAgentsService(app)))

	// 注册应用服务
	app.RegisterService(application.NewService(appservice.NewAppService(app)))

	// 注册知识库服务
	app.RegisterService(application.NewService(library.NewLibraryService(app)))

	// 创建主窗口
	mainWindow := windows.NewMainWindow(app)
	mainWinMgr.app = app
	mainWinMgr.window = mainWindow

	// 创建子窗口服务
	windowService, err := windows.NewWindowService(app, windows.DefaultDefinitions())
	if err != nil {
		return nil, fmt.Errorf("init window service: %w", err)
	}
	app.RegisterService(application.NewService(windowService))

	// 创建吸附（winsnap）服务
	snapService, err := windows.NewSnapService(app, windowService)
	if err != nil {
		return nil, fmt.Errorf("init snap service: %w", err)
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
		// 安全地显示主窗口
		mainWinMgr.safeShow()
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
			mainWinMgr.safeHide()
			e.Cancel()
		} else {
			app.Quit()
		}
	})

	// 点击 Dock 图标时显示窗口
	app.Event.OnApplicationEvent(events.Mac.ApplicationShouldHandleReopen, func(event *application.ApplicationEvent) {
		// 安全地唤醒主窗口
		mainWinMgr.safeUnMinimiseAndShow()
	})

	return app, nil
}
