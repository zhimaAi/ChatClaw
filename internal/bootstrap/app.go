package bootstrap

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"willclaw/internal/define"
	"willclaw/internal/services/agents"
	appservice "willclaw/internal/services/app"
	"willclaw/internal/services/browser"
	"willclaw/internal/services/chat"
	"willclaw/internal/services/conversations"
	"willclaw/internal/services/document"
	"willclaw/internal/services/floatingball"
	"willclaw/internal/services/greet"
	"willclaw/internal/services/i18n"
	"willclaw/internal/services/library"
	"willclaw/internal/services/multiask"
	"willclaw/internal/services/providers"
	"willclaw/internal/services/settings"
	"willclaw/internal/services/textselection"
	"willclaw/internal/services/tray"
	"willclaw/internal/services/updater"
	"willclaw/internal/services/windows"
	"willclaw/internal/services/winsnapchat"
	"willclaw/internal/sqlite"
	"willclaw/internal/taskmanager"
	"willclaw/pkg/winutil"

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
	// Use native API to activate window; avoids WebView2 Focus() crash on Windows.
	winutil.ForceActivateWindow(m.window)
}

// safeShow safely shows and focuses the main window, bringing it to the foreground.
func (m *mainWindowManager) safeShow() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.isWindowValid() {
		return
	}

	m.window.Show()
	// Use native API to activate window; avoids WebView2 Focus() crash on Windows.
	winutil.ForceActivateWindow(m.window)
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
	// Use native API to activate window; avoids WebView2 Focus() crash on Windows.
	winutil.ForceActivateWindow(m.window)
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

// NewApp 创建并初始化应用
// 返回 app 实例和 cleanup 函数（用于关闭数据库等资源）
func NewApp(opts Options) (app *application.App, cleanup func(), err error) {
	// 初始化多语言（设置全局语言）
	i18nService := i18n.NewService(opts.Locale)

	// Initialize logger: write to both stderr and a log file in the config directory.
	// The log file is rotated on each launch (overwritten), kept at a reasonable size.
	logger, logFile := initLogger()

	// 主窗口管理器，用于安全的窗口操作
	mainWinMgr := &mainWindowManager{}

	// 声明悬浮球服务变量，用于回调中恢复悬浮球
	var floatingBallService *floatingball.FloatingBallService

	// 创建应用实例
	app = application.New(application.Options{
		Name:        "WillClaw",
		Description: "WillClaw Desktop App",
		Logger:      logger,
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
				// 若悬浮球开关为开启，则在唤醒主窗口时恢复悬浮球
				if floatingBallService != nil && settings.GetBool("show_floating_window", false) && !floatingBallService.IsVisible() {
					_ = floatingBallService.SetVisible(true)
				}
			},
		},
	})

	// Allow i18n to broadcast locale:changed events to all windows
	i18n.SetApp(app)

	// ========== 初始化基础设施 ==========

	// 初始化数据库
	if err := sqlite.Init(app); err != nil {
		return nil, nil, fmt.Errorf("sqlite init: %w", err)
	}

	// ChatWiki: default enabled, auto-generate API key at startup
	if err := providers.EnsureChatWikiInitialized(); err != nil {
		app.Logger.Warn("EnsureChatWikiInitialized failed (non-fatal)", "error", err)
	}
	// ChatWiki: refresh model cache on every app start (add/update/delete). Async, silent; errors only logged.
	go func() {
		if err := providers.NewProvidersService(app).SyncChatWikiModels(); err != nil {
			app.Logger.Warn("SyncChatWikiModels failed (non-fatal)", "error", err)
		}
	}()

	// 初始化设置缓存
	if err := settings.InitCache(app); err != nil {
		sqlite.Close()
		return nil, nil, fmt.Errorf("settings cache init: %w", err)
	}

	// 使用 DB 中持久化的 language 覆盖启动语言（保证重启后语言一致）
	if lang, ok := settings.GetValue("language"); ok && strings.TrimSpace(lang) != "" {
		i18n.SetLocale(lang)
	}

	// 初始化任务管理器（基于 goqite 的持久化消息队列）
	if err := taskmanager.Init(app, sqlite.DB().DB, taskmanager.Config{
		Queues: map[string]taskmanager.QueueConfig{
			taskmanager.QueueThumbnail: {Workers: 10, PollInterval: 50 * time.Millisecond}, // 缩略图任务
			taskmanager.QueueDocument:  {Workers: 3, PollInterval: 100 * time.Millisecond}, // 文档处理任务
		},
	}); err != nil {
		sqlite.Close()
		return nil, nil, fmt.Errorf("init task manager: %w", err)
	}

	// ========== ChatWiki embedding 全局设置对齐 ==========
	// 约束：
	// - embedding provider/model 来自 ChatWiki 的模型列表（已在启动时 SyncChatWikiModels 刷新到本地 models 表）
	// - embedding dimension 固定为 1024
	// - 通过 SettingsService.UpdateEmbeddingConfig 更新 settings +（必要时）重建 doc_vec + 提交全量重嵌入任务
	{
		db := sqlite.DB()
		if db != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			var embeddingModelID sql.NullString
			err := db.NewSelect().
				Table("models").
				Column("model_id").
				Where("provider_id = ?", "chatwiki").
				Where("type = ?", "embedding").
				Where("enabled = ?", true).
				OrderExpr("sort_order ASC").
				Limit(1).
				Scan(ctx, &embeddingModelID)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				app.Logger.Warn("query chatwiki embedding model failed (non-fatal)", "error", err)
			} else if embeddingModelID.Valid && strings.TrimSpace(embeddingModelID.String) != "" {
				if err := settings.NewSettingsService(app).UpdateEmbeddingConfig(settings.UpdateEmbeddingConfigInput{
					ProviderID: "chatwiki",
					ModelID:    embeddingModelID.String,
					Dimension:  1024,
				}); err != nil {
					app.Logger.Warn("sync chatwiki embedding settings failed (non-fatal)", "error", err)
				}
			} else {
				// No embedding models available from ChatWiki cache; keep existing settings.
				app.Logger.Warn("no chatwiki embedding model found; keep existing embedding settings")
			}
		}
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
	// 注册聊天服务
	app.RegisterService(application.NewService(chat.NewChatService(app)))
	// 注册知识库服务
	app.RegisterService(application.NewService(library.NewLibraryService(app)))
	// 注册文档服务
	app.RegisterService(application.NewService(document.NewDocumentService(app)))
	// 注册自动更新服务
	app.RegisterService(application.NewService(updater.NewUpdaterService(app)))

	// ========== macOS 应用菜单 ==========
	// Set up standard macOS application menu so that system shortcuts work:
	// Cmd+M (minimize), Cmd+H (hide), Cmd+Q (quit), Cmd+C/V/X (copy/paste/cut), etc.
	if runtime.GOOS == "darwin" {
		appMenu := app.NewMenu()
		appMenu.AddRole(application.AppMenu)
		appMenu.AddRole(application.EditMenu)
		appMenu.AddRole(application.WindowMenu)
		app.Menu.Set(appMenu)
	}

	// ========== 创建窗口 ==========

	// 创建主窗口
	mainWindow := windows.NewMainWindow(app)
	mainWinMgr.app = app
	mainWinMgr.window = mainWindow

	// 注册多问服务（管理多个 AI WebView 面板，传入主窗口引用）
	multiaskService := multiask.NewMultiaskService(app, mainWindow)
	app.RegisterService(application.NewService(multiaskService))

	// 注册应用服务（传入主窗口引用，用于 ShowMainWindow API）
	app.RegisterService(application.NewService(appservice.NewAppService(app, mainWindow)))

	// 创建悬浮球服务（独立 AlwaysOnTop 小窗）
	floatingBallService = floatingball.NewFloatingBallService(app, mainWindow)
	app.RegisterService(application.NewService(floatingBallService))

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
	// - getSnapState: 获取吸附窗体状态（attached/standalone/hidden/stopped）
	// - wakeSnapWindow: 唤醒吸附窗体（当划词点击时吸附窗体可见则唤醒吸附窗体）
	textSelectionService := textselection.NewWithSnapCallbacks(
		func() windows.SnapState {
			return snapService.GetStatus().State
		},
		func() {
			snapService.WakeWindow()
		},
	)
	app.RegisterService(application.NewService(textSelectionService))

	// 创建系统托盘
	systrayMenu := app.NewMenu()
	systrayMenu.Add(i18n.T("systray.show")).OnClick(func(ctx *application.Context) {
		// 安全地显示主窗口
		mainWinMgr.safeShow()
		// 若悬浮球开关为开启，则在唤醒主窗口时恢复悬浮球
		if floatingBallService != nil && settings.GetBool("show_floating_window", false) && !floatingBallService.IsVisible() {
			_ = floatingBallService.SetVisible(true)
		}
	})
	systrayMenu.Add(i18n.T("systray.quit")).OnClick(func(ctx *application.Context) {
		app.Quit()
	})
	systray := app.SystemTray.New().SetIcon(opts.Icon).SetMenu(systrayMenu).
		OnClick(func() {
			// Run in a goroutine to avoid deadlock: the OnClick handler is called
			// synchronously on the main thread by Wails, but window.Show() internally
			// uses InvokeSync which also dispatches to the main thread and waits.
			// (Menu item callbacks are already wrapped in goroutines by Wails, but
			// SystemTray OnClick is not.)
			go func() {
				mainWinMgr.safeShow()
				if floatingBallService != nil && settings.GetBool("show_floating_window", true) && !floatingBallService.IsVisible() {
					_ = floatingBallService.SetVisible(true)
				}
			}()
		})

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
		// 初始化多问服务（需要窗口已创建，在后台进行以避免阻塞）
		go func() {
			if err := multiaskService.Initialize("WillClaw"); err != nil {
				app.Logger.Error("Failed to initialize multiask service", "error", err)
			}
		}()
		floatingBallService.InitFromSettings()
	})

	// 监听文件拖拽事件，将文件路径转发到前端
	mainWindow.OnWindowEvent(events.Common.WindowFilesDropped, func(event *application.WindowEvent) {
		files := event.Context().DroppedFiles()
		if len(files) == 0 {
			return
		}
		app.Event.Emit("filedrop:files", map[string]any{
			"files": files,
		})
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

	// 主窗口被唤醒/恢复/聚焦时：若悬浮球开关为开启，则恢复悬浮球
	restoreFloatingBall := func(reason string) {
		if floatingBallService == nil {
			return
		}
		if !settings.GetBool("show_floating_window", false) {
			return
		}
		if floatingBallService.IsVisible() {
			return
		}
		_ = floatingBallService.SetVisible(true)
	}
	mainWindow.RegisterHook(events.Common.WindowShow, func(_ *application.WindowEvent) { restoreFloatingBall("main_window_show") })
	mainWindow.RegisterHook(events.Common.WindowRestore, func(_ *application.WindowEvent) { restoreFloatingBall("main_window_restore") })
	// NOTE: Don't restore on WindowFocus. Closing the floating ball shifts focus back to main window,
	// which would immediately re-show the floating ball and make it impossible to close.

	// 点击 Dock 图标时显示窗口
	app.Event.OnApplicationEvent(events.Mac.ApplicationShouldHandleReopen, func(event *application.ApplicationEvent) {
		// 安全地唤醒主窗口
		mainWinMgr.safeUnMinimiseAndShow()
		restoreFloatingBall("mac_reopen")
	})

	return app, func() {
		// Stop task manager before closing database
		if tm := taskmanager.Get(); tm != nil {
			tm.StopNow()
		}
		sqlite.Close()
		if logFile != nil {
			logFile.Close()
		}
	}, nil
}

// initLogger creates a slog.Logger that writes to both stderr and a log file.
// The log file is located at <UserConfigDir>/willclaw/willclaw.log and is
// truncated on each launch so it stays small. Returns the logger and the
// file handle (caller must close it). If the file cannot be created, logging
// falls back to stderr only.
func initLogger() (*slog.Logger, *os.File) {
	level := slog.LevelInfo
	if define.IsDev() {
		level = slog.LevelDebug
	}

	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})), nil
	}

	dir := filepath.Join(cfgDir, define.AppID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})), nil
	}

	logPath := filepath.Join(dir, "willclaw.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})), nil
	}

	w := io.MultiWriter(os.Stderr, f)
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: level})), f
}
