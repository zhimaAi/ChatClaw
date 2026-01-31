package app

import (
	"willchat/internal/define"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// AppService 应用服务（暴露给前端调用）
type AppService struct {
	app *application.App
}

func NewAppService(app *application.App) *AppService {
	return &AppService{app: app}
}

// GetVersion 获取应用版本号
func (s *AppService) GetVersion() string {
	return define.Version
}
