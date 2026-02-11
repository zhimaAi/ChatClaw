package app

import (
	"sync"

	"chatclaw/internal/define"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// AppService 应用服务（暴露给前端调用）
type AppService struct {
	app        *application.App
	mainWindow *application.WebviewWindow

	showOnce sync.Once
}

func NewAppService(app *application.App, mainWindow *application.WebviewWindow) *AppService {
	return &AppService{
		app:        app,
		mainWindow: mainWindow,
	}
}

// GetVersion 获取应用版本号
func (s *AppService) GetVersion() string {
	return define.Version
}

// GetRunMode returns the current run mode: "gui" (desktop) or "server" (HTTP).
// Determined at compile time via build tags.
func (s *AppService) GetRunMode() string {
	return define.RunMode
}

// ShowMainWindow shows the main window (called by frontend after Vue app is mounted).
// This is used on Windows to avoid black screen flash during webview loading.
// Safe to call multiple times; only the first call has effect.
func (s *AppService) ShowMainWindow() {
	s.showOnce.Do(func() {
		if s.mainWindow != nil {
			s.mainWindow.Show()
			s.mainWindow.Focus()
		}
	})
}

