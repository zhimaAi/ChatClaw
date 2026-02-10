package app

import (
	"sync"

	"willchat/internal/define"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// CheckUpdateResult represents the result of a version check
type CheckUpdateResult struct {
	// HasUpdate indicates whether a new version is available
	HasUpdate bool `json:"has_update"`
	// CurrentVersion is the current application version
	CurrentVersion string `json:"current_version"`
	// LatestVersion is the latest available version (empty if no update)
	LatestVersion string `json:"latest_version"`
	// Message is a human-readable message about the check result
	Message string `json:"message"`
}

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

// CheckForUpdate checks if there is a newer version available.
// Currently always returns "already up to date". Real update logic will be added later.
func (s *AppService) CheckForUpdate() (*CheckUpdateResult, error) {
	currentVersion := define.Version
	return &CheckUpdateResult{
		HasUpdate:      false,
		CurrentVersion: currentVersion,
		LatestVersion:  currentVersion,
	}, nil
}
