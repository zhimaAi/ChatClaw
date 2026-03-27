package bootstrap

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"runtime"
	"strings"
	"sync"
	"time"

	"chatclaw/internal/deeplink"
	"chatclaw/internal/define"
	"chatclaw/internal/logger"
	"chatclaw/internal/openclaw/agents"
	"chatclaw/internal/openclaw/runtime"
	openclawskills "chatclaw/internal/openclaw/skills"
	"chatclaw/internal/services/agents"
	appservice "chatclaw/internal/services/app"
	"chatclaw/internal/services/assistantmcp"
	"chatclaw/internal/services/browser"
	"chatclaw/internal/services/channels"
	"chatclaw/internal/services/chat"
	"chatclaw/internal/services/chatwiki"
	"chatclaw/internal/services/conversations"
	"chatclaw/internal/services/document"
	"chatclaw/internal/services/floatingball"
	"chatclaw/internal/services/greet"
	"chatclaw/internal/services/i18n"
	"chatclaw/internal/services/library"
	"chatclaw/internal/services/librarymcp"
	"chatclaw/internal/services/mcp"
	"chatclaw/internal/services/memory"
	"chatclaw/internal/services/multiask"
	openclawchannels "chatclaw/internal/services/openclaw/channels"
	"chatclaw/internal/services/providers"
	"chatclaw/internal/services/scheduledtasks"
	"chatclaw/internal/services/settings"
	"chatclaw/internal/services/skills"
	"chatclaw/internal/services/textselection"
	"chatclaw/internal/services/toolchain"
	"chatclaw/internal/services/tray"
	"chatclaw/internal/services/updater"
	"chatclaw/internal/services/windows"
	"chatclaw/internal/services/winsnapchat"
	"chatclaw/internal/sqlite"
	"chatclaw/internal/taskmanager"
	"chatclaw/pkg/winutil"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
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
	// Migrate $HOME/.chatclaw legacy layout into native/ and openclaw/ before logs and sqlite.
	if err := define.EnsureDataLayout(); err != nil {
		return nil, nil, fmt.Errorf("data layout: %w", err)
	}

	// 初始化日志（文件 + 控制台双写；生产模式仅写文件）
	appLogger, logCleanup, err := logger.New()
	if err != nil {
		// Fallback: if file logger fails, continue without it (Wails will use its default).
		appLogger = nil
		logCleanup = func() {}
	}
	if appLogger != nil {
		slog.SetDefault(appLogger)
	}

	// 初始化多语言（设置全局语言）
	i18nService := i18n.NewService(opts.Locale)

	// 主窗口管理器，用于安全的窗口操作
	mainWinMgr := &mainWindowManager{}

	// 声明悬浮球服务变量，用于回调中恢复悬浮球
	var floatingBallService *floatingball.FloatingBallService

	// 创建应用实例
	app = application.New(application.Options{
		Name:        "ChatClaw",
		Description: "ChatClaw Desktop App",
		Logger:      appLogger,
		Services: []application.Service{
			application.NewService(greet.NewGreetService("Hello, ")),
			application.NewService(i18nService),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(opts.Assets),
		},
		// Server mode: listen on all interfaces by default.
		// Can be overridden by WAILS_SERVER_HOST / WAILS_SERVER_PORT env vars.
		Server: application.ServerOptions{
			Host:         "0.0.0.0",
			Port:         8080,
			WriteTimeout: 30 * time.Second,
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
		// 单实例配置：防止多个应用实例同时运行
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: define.SingleInstanceUniqueID,
			OnSecondInstanceLaunch: func(data application.SecondInstanceData) {
				mainWinMgr.safeWake()
				if floatingBallService != nil && settings.GetBool("show_floating_window", false) && !floatingBallService.IsVisible() {
					_ = floatingBallService.SetVisible(true)
				}
				deeplink.HandleSecondInstance(app, data)
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

	// ChatClaw: default enabled, auto-generate API key at startup
	if err := providers.EnsureChatClawInitialized(); err != nil {
		app.Logger.Warn("EnsureChatClawInitialized failed (non-fatal)", "error", err)
	}
	// ChatClaw: refresh model cache on every app start (add/update/delete). Async, silent; errors only logged.
	go func() {
		if err := providers.NewProvidersService(app).SyncChatClawModels(); err != nil {
			app.Logger.Warn("SyncChatClawModels failed (non-fatal)", "error", err)
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

	// Sync ADK built-in prompt language with app locale.
	if i18n.GetLocale() == i18n.LocaleZhCN {
		_ = adk.SetLanguage(adk.LanguageChinese)
	} else {
		_ = adk.SetLanguage(adk.LanguageEnglish)
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

	// ========== 注册应用服务 ==========

	// 注册设置服务
	app.RegisterService(application.NewService(settings.NewSettingsService(app)))
	chatWikiService := chatwiki.NewChatWikiService(app)
	// 注册供应商服务
	providersSvc := providers.NewProvidersService(app)
	app.RegisterService(application.NewService(providersSvc))
	// 注册浏览器服务
	app.RegisterService(application.NewService(browser.NewBrowserService(app)))
	// 注册助手服务
	agentsService := agents.NewAgentsService(app)
	if err := agentsService.EnsureMainAgent(); err != nil {
		sqlite.Close()
		return nil, nil, fmt.Errorf("ensure main agent: %w", err)
	}
	app.RegisterService(application.NewService(agentsService))
	// 注册 OpenClaw 助手服务
	openClawAgentsService := openclawagents.NewOpenClawAgentsService(app)
	if err := openClawAgentsService.EnsureMainAgent(); err != nil {
		sqlite.Close()
		return nil, nil, fmt.Errorf("ensure openclaw main agent: %w", err)
	}
	app.RegisterService(application.NewService(openClawAgentsService))
	// 注册 OpenClaw Runtime 管理器（供 OpenClaw Agent/Channel 与聊天桥接复用）
	openclawManager := openclawruntime.NewManager(app, settings.NewSettingsService(app))
	// 注册会话服务
	conversationsService := conversations.NewConversationsService(app)
	app.RegisterService(application.NewService(conversationsService))
	// 注册 Skill 管理服务
	skillsService := skills.NewSkillsService(app)
	app.RegisterService(application.NewService(skillsService))
	// 注册 MCP 服务
	app.RegisterService(application.NewService(mcp.NewMCPService(app)))
	// 注册助手 MCP 服务
	assistantMCPService := assistantmcp.NewAssistantMCPService(app)
	app.RegisterService(application.NewService(assistantMCPService))
	// 注册知识库 MCP 服务（全局，自动启动，对外暴露知识库检索能力）
	libraryMCPService := librarymcp.NewService(app)
	// 注册聊天服务
	chatService := chat.NewChatService(app)
	chatService.SetChatWikiService(chatWikiService)
	app.RegisterService(application.NewService(chatService))
	// Wire chat bridge for assistant MCP (avoids cyclic import)
	assistantMCPService.SetChatBridge(assistantmcp.NewChatBridge(
		func(agentID int64, externalID, name string) (int64, error) {
			return conversationsService.FindOrCreateByExternalID(agentID, externalID, name, "")
		},
		func(convID int64, content, tabID string) (string, int64, error) {
			res, err := chatService.SendMessage(chat.SendMessageInput{
				ConversationID: convID,
				Content:        content,
				TabID:          tabID,
			})
			if err != nil {
				return "", 0, err
			}
			return res.RequestID, res.MessageID, nil
		},
		chatService.WaitForGeneration,
		conversationsService.GetLatestAssistantReply,
	))
	// 注册定时任务服务
	scheduledTasksService := scheduledtasks.NewScheduledTasksService(app, conversationsService, chatService)
	chatService.RegisterExtraToolFactory(func() ([]tool.BaseTool, error) {
		return newScheduledTaskManagementTools(agentsService, scheduledTasksService)
	})
	app.RegisterService(application.NewService(scheduledTasksService))
	// 注册记忆服务（OpenClaw workspace 文件读写）
	app.RegisterService(application.NewService(memory.NewMemoryService(app)))
	// 注册知识库服务
	app.RegisterService(application.NewService(library.NewLibraryService(app)))
	// 注册文档服务
	app.RegisterService(application.NewService(document.NewDocumentService(app)))
	// 注册频道网关 + 频道管理服务
	// Use indirection so the handler closure can reference the gateway.
	var channelGW *channels.Gateway
	channelGateway := channels.NewGateway(app.Logger, func(msg channels.IncomingMessage) {
		handleChannelMessage(app, chatService, conversationsService, channelGW, msg)
	})
	channelGW = channelGateway
	wireChannelGateway(chatService, scheduledTasksService, channelGateway)
	channelService := channels.NewChannelService(app, channelGateway, func(channelName string) (int64, error) {
		return ensureChannelAgent(agentsService, channelName)
	})
	app.RegisterService(application.NewService(channelService))
	// 注册 OpenClaw 频道服务（Feishu-focused channel management for OpenClaw）
	openClawChannelService := openclawchannels.NewOpenClawChannelService(app, channelGateway, openClawAgentsService, channelService, conversationsService, openclawManager)
	app.RegisterService(application.NewService(openClawChannelService))
	// 注册自动更新服务
	app.RegisterService(application.NewService(updater.NewUpdaterService(app)))
	// 注册工具链服务（管理 uv、bun 等外部工具的安装/更新，前端可调用）
	toolchainService := toolchain.NewToolchainService(app)
	app.RegisterService(application.NewService(toolchainService))
	// 注册 OpenClaw Runtime 服务（管理 OpenClaw Gateway 进程的生命周期）
	configSvc := openclawruntime.NewConfigService(openclawManager)
	configSvc.Register("responses", openclawruntime.ResponsesEndpointSection())
	configSvc.Register("models", openclawruntime.NewModelsSectionBuilder(providersSvc))
	configSvc.Register("mcp", func(ctx context.Context) (map[string]any, error) {
		if !libraryMCPService.IsRunning() {
			return nil, nil
		}
		mcpURL, token := libraryMCPService.ConnectionInfo()
		if mcpURL == "" {
			return nil, nil
		}
		scriptPath := libraryMCPService.BridgeScriptPath()
		if scriptPath == "" {
			return nil, nil
		}
		if err := libraryMCPService.EnsureBridgeScript(); err != nil {
			return nil, fmt.Errorf("write mcp bridge script: %w", err)
		}
		return map[string]any{
			"mcp": map[string]any{
				"servers": map[string]any{
					"chatclaw-knowledge": map[string]any{
						"command": "node",
						"args":    []string{scriptPath},
						"env": map[string]string{
							"CHATCLAW_MCP_URL":   mcpURL,
							"CHATCLAW_MCP_TOKEN": token,
						},
					},
				},
			},
		}, nil
	})
	agentGWSvc := openclawruntime.NewAgentService(app, openclawManager, openClawAgentsService, configSvc)
	openclawManager.RegisterReadyHook(agentGWSvc.OnGatewayReady)
	openClawAgentsService.SetGateway(agentGWSvc)
	chatService.SetOpenClawGateway(openclawManager)
	app.RegisterService(application.NewService(openclawruntime.NewOpenClawRuntimeService(openclawManager)))
	app.RegisterService(application.NewService(openclawskills.NewOpenClawSkillsService(openClawAgentsService, openclawManager)))
	app.Event.On("providers:config-changed", func(e *application.CustomEvent) {
		go configSvc.Sync(context.Background())
	})
	// 注册 ChatWiki 绑定服务
	app.RegisterService(application.NewService(chatWikiService))

	// ========== macOS 应用菜单 ==========
	if runtime.GOOS == "darwin" {
		appMenu := app.NewMenu()
		appMenu.AddRole(application.AppMenu)
		appMenu.AddRole(application.EditMenu)
		windowMenu := appMenu.AddSubmenu("Window")
		windowMenu.Add("Minimize").
			SetAccelerator("CmdOrCtrl+M").
			OnClick(func(ctx *application.Context) {
				if w := app.Window.Current(); w != nil {
					w.Minimise()
				}
			})
		windowMenu.AddRole(application.Zoom)
		windowMenu.AddSeparator()
		windowMenu.AddRole(application.Front)

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

	// macOS 使用模板图标，自动适应深色/浅色模式
	var systray *application.SystemTray
	if runtime.GOOS == "darwin" {
		systray = app.SystemTray.New().SetTemplateIcon(opts.Icon).SetMenu(systrayMenu).
			OnClick(func() {
				go func() {
					mainWinMgr.safeShow()
					if floatingBallService != nil && settings.GetBool("show_floating_window", true) && !floatingBallService.IsVisible() {
						_ = floatingBallService.SetVisible(true)
					}
				}()
			})
	} else {
		systray = app.SystemTray.New().SetIcon(opts.Icon).SetMenu(systrayMenu).
			OnClick(func() {
				go func() {
					mainWinMgr.safeShow()
					if floatingBallService != nil && settings.GetBool("show_floating_window", true) && !floatingBallService.IsVisible() {
						_ = floatingBallService.SetVisible(true)
					}
				}()
			})
	}
	systray.SetTooltip("ChatClaw")

	// 创建托盘服务（用于前端动态控制 show/hide + 缓存关闭策略）
	trayService := tray.NewTrayService(app, systray)
	app.RegisterService(application.NewService(trayService))
	// macOS: URL Scheme is delivered via Apple Event, not via command-line args.
	// Listen for ApplicationLaunchedWithUrl to handle chatclaw:// deep links.
	app.Event.OnApplicationEvent(events.Common.ApplicationLaunchedWithUrl, func(event *application.ApplicationEvent) {
		urlStr := event.Context().URL()
		app.Logger.Info("ApplicationLaunchedWithUrl received", "url", urlStr)
		mainWinMgr.safeWake()
		deeplink.HandleURL(app, urlStr)
	})

	// 应用启动后再加载设置并应用 Show/Hide（确保 sqlite 已初始化）
	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(_ *application.ApplicationEvent) {
		// Register chatclaw:// URL scheme on Windows so browser can launch app after OAuth.
		// Fixes "scheme does not have a registered handler" when installer did not run or registration was lost.
		if err := windows.RegisterChatClawProtocol(); err != nil {
			app.Logger.Warn("Failed to register chatclaw protocol", "error", err)
		}
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
			if err := multiaskService.Initialize("ChatClaw"); err != nil {
				app.Logger.Error("Failed to initialize multiask service", "error", err)
			}
		}()
		floatingBallService.InitFromSettings()
		// Ensure external toolchain binaries (uv, bun) are installed/updated in background.
		go toolchainService.EnsureAll()
		// Start OpenClaw Gateway in background.
		openclawManager.Start()
		// Ensure builtin skills are installed in background.
		go skillsService.EnsureBuiltinSkills()
		// Warm Chatwiki model cache when account is already bound.
		go func() {
			if _, err := chatWikiService.RefreshModelCatalog(); err != nil {
				app.Logger.Warn("RefreshModelCatalog failed (non-fatal)", "error", err)
			}
		}()
		// Start all enabled channel gateway connections in background.
		go channelGateway.StartAll(context.Background())
		go func() {
			if err := scheduledTasksService.Start(); err != nil {
				app.Logger.Error("Failed to start scheduled tasks service", "error", err)
			}
		}()
		// Start all enabled assistant MCP servers in background.
		go assistantMCPService.StartEnabledServers()
		// Start global library MCP server and sync its config to OpenClaw Gateway.
		libraryMCPService.OnStarted(func() {
			go configSvc.Sync(context.Background())
		})
		go libraryMCPService.Start()
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
		openclawManager.Shutdown()
		assistantMCPService.StopAllServers()
		libraryMCPService.Stop()
		channelGateway.StopAll(context.Background())
		scheduledTasksService.Stop()
		chatService.Shutdown()
		// Stop task manager before closing database
		if tm := taskmanager.Get(); tm != nil {
			tm.StopNow()
		}
		sqlite.Close()
		// Close log file last so all shutdown logs are captured.
		logCleanup()
	}, nil
}
