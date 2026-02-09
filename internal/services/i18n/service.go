package i18n

import (
	"embed"
	"encoding/json"
	"strings"
	"sync"

	"github.com/jeandeaual/go-locale"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/wailsapp/wails/v3/pkg/application"
	"golang.org/x/text/language"
)

//go:embed locales/*.json
var localesFS embed.FS

// 支持的语言
const (
	LocaleZhCN = "zh-CN"
	LocaleEnUS = "en-US"
)

var (
	bundle    *i18n.Bundle
	localizer *i18n.Localizer
	mu        sync.RWMutex
	appRef    *application.App
)

// SetApp stores the application reference for cross-window event broadcasting.
// Call after the app is created in bootstrap.
func SetApp(a *application.App) {
	mu.Lock()
	defer mu.Unlock()
	appRef = a
}

func init() {
	bundle = i18n.NewBundle(language.Chinese)
	bundle.RegisterUnmarshalFunc("json", json.Unmarshal)

	// 加载翻译文件
	bundle.LoadMessageFileFS(localesFS, "locales/zh-CN.json")
	bundle.LoadMessageFileFS(localesFS, "locales/en-US.json")

	// 默认使用中文
	localizer = i18n.NewLocalizer(bundle, LocaleZhCN)
}

// Service 多语言服务（暴露给前端调用）
type Service struct{}

// NewService 创建多语言服务
func NewService(locale string) *Service {
	SetLocale(locale)
	return &Service{}
}

// GetLocale 获取当前语言（暴露给前端）
func (s *Service) GetLocale() string {
	return GetLocale()
}

// SetLocale 设置语言（暴露给前端）
func (s *Service) SetLocale(locale string) {
	SetLocale(locale)
}

// ---- 包级便捷函数 ----

var currentLocale = LocaleZhCN

// DetectLocale 检测系统语言，返回支持的语言代码
func DetectLocale() string {
	lang, err := locale.GetLanguage()
	if err != nil {
		return LocaleZhCN
	}

	// 系统语言可能是 "zh"、"zh-CN"、"zh_CN"、"zh-Hans" 等格式
	lang = strings.ToLower(lang)
	if strings.HasPrefix(lang, "zh") {
		return LocaleZhCN
	}
	if strings.HasPrefix(lang, "en") {
		return LocaleEnUS
	}

	// 不支持的语言默认使用中文
	return LocaleZhCN
}

// GetLocale 获取当前语言
func GetLocale() string {
	mu.RLock()
	defer mu.RUnlock()
	return currentLocale
}

// SetLocale 设置语言（空字符串时自动检测系统语言）
func SetLocale(locale string) {
	mu.Lock()

	if locale == "" {
		locale = DetectLocale()
	} else if locale != LocaleZhCN && locale != LocaleEnUS {
		locale = LocaleZhCN
	}
	currentLocale = locale
	localizer = i18n.NewLocalizer(bundle, locale)
	a := appRef
	mu.Unlock()

	// Broadcast to all windows so snap/selection can sync
	if a != nil {
		a.Event.Emit("locale:changed", map[string]string{"locale": locale})
	}
}

// T 获取翻译文本
func T(key string) string {
	mu.RLock()
	defer mu.RUnlock()

	msg, err := localizer.Localize(&i18n.LocalizeConfig{MessageID: key})
	if err != nil {
		return key
	}
	return msg
}

// Tf 获取翻译文本（带参数）
func Tf(key string, data map[string]any) string {
	mu.RLock()
	defer mu.RUnlock()

	msg, err := localizer.Localize(&i18n.LocalizeConfig{
		MessageID:    key,
		TemplateData: data,
	})
	if err != nil {
		return key
	}
	return msg
}
