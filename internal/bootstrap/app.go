package bootstrap

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"runtime"
	"strings"
	"sync"
	"time"

	"chatclaw/internal/define"
	"chatclaw/internal/logger"
	"chatclaw/internal/services/agents"
	appservice "chatclaw/internal/services/app"
	"chatclaw/internal/services/browser"
	"chatclaw/internal/services/channels"
	"chatclaw/internal/services/chat"
	"chatclaw/internal/services/conversations"
	"chatclaw/internal/services/document"
	"chatclaw/internal/services/floatingball"
	"chatclaw/internal/services/greet"
	"chatclaw/internal/services/i18n"
	"chatclaw/internal/services/library"
	"chatclaw/internal/services/memory"
	"chatclaw/internal/services/multiask"
	"chatclaw/internal/services/providers"
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
	"github.com/uptrace/bun"
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
	// 初始化日志（文件 + 控制台双写；生产模式仅写文件）
	appLogger, logCleanup, err := logger.New()
	if err != nil {
		// Fallback: if file logger fails, continue without it (Wails will use its default).
		appLogger = nil
		logCleanup = func() {}
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
			Host: "0.0.0.0",
			Port: 8080,
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

	// 初始化记忆数据库
	if err := memory.InitDB(app); err != nil {
		return nil, nil, fmt.Errorf("memory db init: %w", err)
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

	// ========== ChatClaw embedding 全局设置对齐 ==========
	// 约束：
	// - embedding provider/model 来自 ChatClaw 的模型列表（已在启动时 SyncChatClawModels 刷新到本地 models 表）
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
				Where("provider_id = ?", "chatclaw").
				Where("type = ?", "embedding").
				Where("enabled = ?", true).
				OrderExpr("sort_order ASC").
				Limit(1).
				Scan(ctx, &embeddingModelID)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				app.Logger.Warn("query chatclaw embedding model failed (non-fatal)", "error", err)
			} else if embeddingModelID.Valid && strings.TrimSpace(embeddingModelID.String) != "" {
				if err := settings.NewSettingsService(app).UpdateEmbeddingConfig(settings.UpdateEmbeddingConfigInput{
					ProviderID: "chatclaw",
					ModelID:    embeddingModelID.String,
					Dimension:  1024,
				}); err != nil {
					app.Logger.Warn("sync chatclaw embedding settings failed (non-fatal)", "error", err)
				}
			} else {
				// No embedding models available from ChatClaw cache; keep existing settings.
				app.Logger.Warn("no chatclaw embedding model found; keep existing embedding settings")
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
	agentsService := agents.NewAgentsService(app)
	app.RegisterService(application.NewService(agentsService))
	// 注册会话服务
	conversationsService := conversations.NewConversationsService(app)
	app.RegisterService(application.NewService(conversationsService))
	// 注册 Skill 管理服务
	skillsService := skills.NewSkillsService(app)
	app.RegisterService(application.NewService(skillsService))
	// 注册聊天服务
	chatService := chat.NewChatService(app)
	app.RegisterService(application.NewService(chatService))
	// 注册记忆服务
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
	chatService.SetGateway(channelGateway)
	channelService := channels.NewChannelService(app, channelGateway, func(channelName string) (int64, error) {
		return ensureChannelAgent(agentsService, channelName)
	})
	app.RegisterService(application.NewService(channelService))
	// 注册自动更新服务
	app.RegisterService(application.NewService(updater.NewUpdaterService(app)))
	// 注册工具链服务（管理 uv、bun 等外部工具的安装/更新，前端可调用）
	toolchainService := toolchain.NewToolchainService(app)
	app.RegisterService(application.NewService(toolchainService))

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
	systray.SetTooltip("ChatClaw")

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
			if err := multiaskService.Initialize("ChatClaw"); err != nil {
				app.Logger.Error("Failed to initialize multiask service", "error", err)
			}
		}()
		floatingBallService.InitFromSettings()
		// Ensure external toolchain binaries (uv, bun) are installed/updated in background.
		go toolchainService.EnsureAll()
		// Ensure builtin skills are installed in background.
		go skillsService.EnsureBuiltinSkills()
		// Start all enabled channel gateway connections in background.
		go channelGateway.StartAll(context.Background())
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
		channelGateway.StopAll(context.Background())
		chatService.Shutdown()
		// Stop task manager before closing database
		if tm := taskmanager.Get(); tm != nil {
			tm.StopNow()
		}
		memory.CloseDB()
		sqlite.Close()
		// Close log file last so all shutdown logs are captured.
		logCleanup()
	}, nil
}

// ensureChannelAgent creates a new AI agent for a channel if needed.
// The agent is assigned the first available enabled LLM model as its default.
func ensureChannelAgent(agentsService *agents.AgentsService, channelName string) (int64, error) {
	agent, err := agentsService.CreateAgent(agents.CreateAgentInput{
		Name:   channelName,
		Prompt: "You are a helpful AI assistant connected to a messaging channel. Be concise and friendly in your responses.",
	})
	if err != nil {
		return 0, fmt.Errorf("create agent: %w", err)
	}

	// Try to assign the first available LLM model as default
	db := sqlite.DB()
	if db != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		type modelRow struct {
			ProviderID string `bun:"provider_id"`
			ModelID    string `bun:"model_id"`
		}
		var m modelRow
		err := db.NewSelect().
			Table("models").
			Column("provider_id", "model_id").
			Where("type = ?", "llm").
			Where("enabled = ?", true).
			Where("provider_id IN (SELECT provider_id FROM providers WHERE enabled = true)").
			OrderExpr("sort_order ASC").
			Limit(1).
			Scan(ctx, &m)
		if err == nil && m.ProviderID != "" && m.ModelID != "" {
			providerID := m.ProviderID
			modelID := m.ModelID
			_, _ = agentsService.UpdateAgent(agent.ID, agents.UpdateAgentInput{
				DefaultLLMProviderID: &providerID,
				DefaultLLMModelID:    &modelID,
			})
		}
	}

	return agent.ID, nil
}

// handleChannelMessage processes an incoming message from a channel platform.
// It extracts text content, finds or creates a conversation for the sender,
// generates an AI reply, and sends it back through the channel adapter.
func handleChannelMessage(
	app *application.App,
	chatService *chat.ChatService,
	convService *conversations.ConversationsService,
	gateway *channels.Gateway,
	msg channels.IncomingMessage,
) {
	app.Logger.Info("channel message received",
		"channel_id", msg.ChannelID,
		"platform", msg.Platform,
		"message_id", msg.MessageID,
		"sender", msg.SenderID,
		"sender_name", msg.SenderName,
		"chat_id", msg.ChatID,
		"msg_type", msg.MsgType,
	)

	if gateway == nil {
		app.Logger.Warn("channel gateway not ready, dropping message")
		return
	}

	sendReply := func(text string) {
		replyTarget := msg.ChatID
		if replyTarget == "" {
			replyTarget = msg.SenderID
		}
		if replyTarget == "" {
			app.Logger.Warn("channel message: no reply target available", "channel_id", msg.ChannelID)
			return
		}

		adapter := gateway.GetAdapter(msg.ChannelID)
		if adapter == nil {
			app.Logger.Warn("channel adapter not found, cannot reply", "channel_id", msg.ChannelID)
			return
		}

		replyCtx, replyCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer replyCancel()

		if err := adapter.SendMessage(replyCtx, replyTarget, text); err != nil {
			app.Logger.Error("channel message: send reply failed", "channel_id", msg.ChannelID, "target", replyTarget, "error", err)
			return
		}

		app.Logger.Info("channel reply sent", "channel_id", msg.ChannelID, "target", replyTarget, "response_len", len(text))
	}

	// Extract text content from platform-specific format
	textContent := extractTextContent(msg)
	if textContent == "" {
		app.Logger.Info("channel message has no text content, skipping", "msg_type", msg.MsgType)
		return
	}

	// Look up channel to get agent_id
	db := sqlite.DB()
	if db == nil {
		app.Logger.Error("channel message: database not initialized")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var agentID int64
	err := db.NewSelect().
		Table("channels").
		Column("agent_id").
		Where("id = ?", msg.ChannelID).
		Scan(ctx, &agentID)
	if err != nil || agentID == 0 {
		app.Logger.Warn("channel has no linked agent, dropping message", "channel_id", msg.ChannelID, "error", err)
		return
	}

	// Use ChatID as conversation key so different Feishu chats get separate conversations.
	// Fall back to SenderID for direct messages (where ChatID may be empty).
	convKey := msg.ChatID
	if convKey == "" {
		convKey = msg.SenderID
	}
	externalID := fmt.Sprintf("ch:%d:%s", msg.ChannelID, convKey)

	// Use ChatName (group name) or SenderName (user name) as display name
	displayName := msg.ChatName
	if displayName == "" {
		displayName = msg.SenderName
	}
	if displayName == "" {
		displayName = externalID
	}

	conv, err := findOrCreateConversation(ctx, db, convService, agentID, externalID, displayName)
	if err != nil {
		app.Logger.Error("channel message: find/create conversation failed", "error", err)
		return
	}

	// Notify frontend that conversation list has changed (e.g. new conversation created)
	app.Event.Emit("conversations:changed", map[string]any{
		"agent_id": agentID,
	})

	// Prepend sender name so the AI can distinguish who sent the message in a group chat.
	aiContent := textContent
	if msg.SenderName != "" {
		aiContent = fmt.Sprintf("%s：%s", msg.SenderName, textContent)
	}

	// Wait for the full response to be generated so we can send it back to the channel.
	// We want the frontend to show the streaming process, so we use SendMessage
	// which creates a normal streaming context. We wait for it to complete.
	var finalResponse string
	res, err := chatService.SendMessage(chat.SendMessageInput{
		ConversationID: conv.ID,
		Content:        aiContent,
		TabID:          "channel_backend", // Special tab ID to prevent frontend errors
	})
	if err != nil {
		app.Logger.Error("channel message: AI generation failed to start", "conv", conv.ID, "error", err)
		sendReply(fmt.Sprintf("AI回复失败: %v", err))
		return
	}

	// Notify frontend that there's a new user message
	app.Event.Emit("chat:messages-changed", map[string]any{
		"conversation_id": conv.ID,
	})

	// Wait for the background generation to complete
	if err := chatService.WaitForGeneration(conv.ID, res.RequestID); err != nil {
		app.Logger.Error("channel message: AI generation wait failed", "conv", conv.ID, "error", err)
		sendReply(fmt.Sprintf("AI回复失败: %v", err))
		// Not returning here in case some partial response was generated
	}

	// Fetch the final assistant message from the DB
	var assistantMsg struct {
		Content string `bun:"content"`
	}
	err = db.NewSelect().
		Table("messages").
		Column("content").
		Where("conversation_id = ?", conv.ID).
		Where("role = ?", "assistant").
		OrderExpr("id DESC").
		Limit(1).
		Scan(ctx, &assistantMsg)

	if err == nil {
		finalResponse = assistantMsg.Content
	}

	// Notify frontend again after AI reply is generated
	_, _ = convService.UpdateConversation(conv.ID, conversations.UpdateConversationInput{
		LastMessage: &finalResponse,
	})
	app.Event.Emit("conversations:changed", map[string]any{
		"agent_id": agentID,
	})
	app.Event.Emit("chat:messages-changed", map[string]any{
		"conversation_id": conv.ID,
	})

	if finalResponse == "" {
		app.Logger.Warn("channel message: empty AI response", "conv", conv.ID)
		sendReply("AI回复失败: 生成了空回复")
		return
	}

	sendReply(finalResponse)
}

// extractTextContent extracts plain text from platform-specific message formats.
func extractTextContent(msg channels.IncomingMessage) string {
	if msg.MsgType != "text" && msg.MsgType != "post" && msg.MsgType != "" {
		return ""
	}

	content := strings.TrimSpace(msg.Content)
	if content == "" {
		return ""
	}

	if strings.HasPrefix(content, "{") {
		// Handle Feishu text messages: {"text":"actual message"}
		if msg.MsgType == "text" || msg.MsgType == "" {
			var parsed struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal([]byte(content), &parsed); err == nil && parsed.Text != "" {
				return strings.TrimSpace(parsed.Text)
			}
		}

		// Handle Feishu post (rich text) messages
		if msg.MsgType == "post" {
			var parsed struct {
				Title   string `json:"title"`
				Content [][]struct {
					Tag  string `json:"tag"`
					Text string `json:"text"`
				} `json:"content"`
			}
			if err := json.Unmarshal([]byte(content), &parsed); err == nil {
				var sb strings.Builder
				if parsed.Title != "" {
					sb.WriteString(parsed.Title)
					sb.WriteString("\n")
				}
				for _, line := range parsed.Content {
					for _, item := range line {
						if item.Tag == "text" || item.Tag == "a" {
							sb.WriteString(item.Text)
						}
					}
					sb.WriteString("\n")
				}
				return strings.TrimSpace(sb.String())
			}
		}
	}

	return content
}

// findOrCreateConversation finds an existing conversation by agent_id and external_id,
// or creates a new one if it doesn't exist.
// externalID is a stable key (e.g., "ch:1:oc_xxx") for lookup.
// displayName is a human-readable name (e.g., group/user name) for display.
func findOrCreateConversation(
	ctx context.Context,
	db *bun.DB,
	convService *conversations.ConversationsService,
	agentID int64,
	externalID string,
	displayName string,
) (*conversations.Conversation, error) {
	// Try to find existing conversation by external_id
	type convRow struct {
		ID int64 `bun:"id"`
	}
	var existing convRow
	err := db.NewSelect().
		Table("conversations").
		Column("id").
		Where("agent_id = ?", agentID).
		Where("external_id = ?", externalID).
		Limit(1).
		Scan(ctx, &existing)
	if err == nil && existing.ID > 0 {
		conv, err := convService.GetConversation(existing.ID)
		if err == nil {
			return conv, nil
		}
	}

	// Fall back to legacy lookup by name for backward compatibility
	err = db.NewSelect().
		Table("conversations").
		Column("id").
		Where("agent_id = ?", agentID).
		Where("name = ?", externalID).
		Limit(1).
		Scan(ctx, &existing)
	if err == nil && existing.ID > 0 {
		conv, err := convService.GetConversation(existing.ID)
		if err == nil {
			return conv, nil
		}
	}

	// Create new conversation with chat mode (simple LLM, no tools)
	conv, err := convService.CreateConversation(conversations.CreateConversationInput{
		AgentID:    agentID,
		Name:       displayName,
		ExternalID: externalID,
		ChatMode:   "chat",
	})
	if err != nil {
		return nil, fmt.Errorf("create conversation: %w", err)
	}

	return conv, nil
}
