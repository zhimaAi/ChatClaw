package tray

import (
	"sync"

	"willclaw/internal/services/settings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// TrayService 托盘服务（暴露给前端调用）
type TrayService struct {
	app     *application.App
	systray *application.SystemTray

	mu                    sync.RWMutex
	trayIconEnabled       bool
	minimizeToTrayEnabled bool
}

func NewTrayService(app *application.App, systray *application.SystemTray) *TrayService {
	return &TrayService{
		app:     app,
		systray: systray,
		trayIconEnabled:       true,
		minimizeToTrayEnabled: true,
	}
}

// SetVisible 设置托盘图标是否可见
func (s *TrayService) SetVisible(visible bool) {
	s.mu.Lock()
	s.trayIconEnabled = visible
	s.mu.Unlock()

	if visible {
		s.systray.Show()
	} else {
		s.systray.Hide()
	}
}

// SetMinimizeToTrayEnabled 设置是否启用"关闭时最小化到托盘"
func (s *TrayService) SetMinimizeToTrayEnabled(enabled bool) {
	s.mu.Lock()
	s.minimizeToTrayEnabled = enabled
	s.mu.Unlock()
}

// IsMinimizeToTrayEnabled 检查是否启用了"关闭时最小化到托盘"
func (s *TrayService) IsMinimizeToTrayEnabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.minimizeToTrayEnabled
}

// IsTrayIconEnabled 检查是否启用了"显示托盘图标"
func (s *TrayService) IsTrayIconEnabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.trayIconEnabled
}

// InitFromSettings 根据设置初始化托盘状态
func (s *TrayService) InitFromSettings() {
	// 从 settings 内存缓存读取（不走 DB）
	trayVisible := settings.GetBool("show_tray_icon", true)
	minimizeEnabled := settings.GetBool("minimize_to_tray_on_close", true)

	// 安全兜底：没有托盘图标时不允许“关闭时最小化到托盘”（否则用户可能无法找回窗口）。
	if !trayVisible && minimizeEnabled {
		minimizeEnabled = false
		s.app.Logger.Warn("tray icon disabled in settings; forcing minimize-to-tray=false to avoid unrecoverable state")
	}

	s.mu.Lock()
	s.trayIconEnabled = trayVisible
	s.minimizeToTrayEnabled = minimizeEnabled
	s.mu.Unlock()

	// 记录初始化日志
	s.app.Logger.Info("tray settings initialized",
		"trayVisible", trayVisible,
		"minimizeEnabled", minimizeEnabled,
	)

	if trayVisible {
		s.systray.Show()
	} else {
		s.systray.Hide()
	}
}
