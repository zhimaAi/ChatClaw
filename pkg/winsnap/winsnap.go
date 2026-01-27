package winsnap

import (
	"errors"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

var (
	ErrTargetWindowNotFound = errors.New("winsnap: target window not found")
)

// AttachOptions 吸附窗口的配置选项
type AttachOptions struct {
	// TargetProcessName 目标进程名称，如 "WXWork.exe" 或 "企业微信"
	TargetProcessName string

	// Gap 目标窗口右边缘与吸附窗口左边缘之间的间隙（像素）
	Gap int

	// FindTimeout 查找目标窗口的超时时间，0 表示使用默认值
	FindTimeout time.Duration

	// App Wails应用实例，用于获取屏幕信息和窗口操作
	App *application.App

	// Window 要吸附的窗口实例
	Window *application.WebviewWindow
}

// Controller 吸附控制器
type Controller interface {
	Stop() error
}

// AttachRightOfProcess 将窗口吸附到目标进程的主窗口右侧
func AttachRightOfProcess(opts AttachOptions) (Controller, error) {
	return attachRightOfProcess(opts)
}
