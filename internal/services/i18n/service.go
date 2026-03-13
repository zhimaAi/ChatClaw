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
	LocaleArSA = "ar-SA"
	LocaleBnBD = "bn-BD"
	LocaleDeDE = "de-DE"
	LocaleEsES = "es-ES"
	LocaleFrFR = "fr-FR"
	LocaleHiIN = "hi-IN"
	LocaleItIT = "it-IT"
	LocaleJaJP = "ja-JP"
	LocaleKoKR = "ko-KR"
	LocalePtBR = "pt-BR"
	LocaleSlSI = "sl-SI"
	LocaleTrTR = "tr-TR"
	LocaleViVN = "vi-VN"
	LocaleZhTW = "zh-TW"
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

	// 加载所有翻译文件
	bundle.LoadMessageFileFS(localesFS, "locales/zh-CN.json")
	bundle.LoadMessageFileFS(localesFS, "locales/en-US.json")
	bundle.LoadMessageFileFS(localesFS, "locales/ar-SA.json")
	bundle.LoadMessageFileFS(localesFS, "locales/bn-BD.json")
	bundle.LoadMessageFileFS(localesFS, "locales/de-DE.json")
	bundle.LoadMessageFileFS(localesFS, "locales/es-ES.json")
	bundle.LoadMessageFileFS(localesFS, "locales/fr-FR.json")
	bundle.LoadMessageFileFS(localesFS, "locales/hi-IN.json")
	bundle.LoadMessageFileFS(localesFS, "locales/it-IT.json")
	bundle.LoadMessageFileFS(localesFS, "locales/ja-JP.json")
	bundle.LoadMessageFileFS(localesFS, "locales/ko-KR.json")
	bundle.LoadMessageFileFS(localesFS, "locales/pt-BR.json")
	bundle.LoadMessageFileFS(localesFS, "locales/sl-SI.json")
	bundle.LoadMessageFileFS(localesFS, "locales/tr-TR.json")
	bundle.LoadMessageFileFS(localesFS, "locales/vi-VN.json")
	bundle.LoadMessageFileFS(localesFS, "locales/zh-TW.json")

	// 默认使用英文（前端/后端都以英文作为不支持语言时的初始兜底）
	localizer = i18n.NewLocalizer(bundle, LocaleEnUS)
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

var currentLocale = LocaleEnUS

// DetectLocale 检测系统语言，返回支持的语言代码
func DetectLocale() string {
	lang, err := locale.GetLanguage()
	if err != nil {
		// 获取系统语言失败时，直接退回英文
		return LocaleEnUS
	}

	// 系统语言可能是 "zh"、"zh-CN"、"zh_CN"、"zh-Hans" 等格式
	lang = strings.ToLower(lang)

	// 精确匹配
	switch lang {
	case "zh", "zh-cn", "zh_cn", "zh-hans", "zh-hant":
		return LocaleZhCN
	case "zh-tw", "zh-hk", "zh-mo", "zh-hant-tw":
		return LocaleZhTW
	case "en", "en-us", "en-gb", "en-au", "en-ca", "en-nz":
		return LocaleEnUS
	case "ar", "ar-sa", "ar-ae", "ar-eg":
		return LocaleArSA
	case "bn", "bn-bd", "bn-in":
		return LocaleBnBD
	case "de", "de-de", "de-at", "de-ch":
		return LocaleDeDE
	case "es", "es-es", "es-mx", "es-ar":
		return LocaleEsES
	case "fr", "fr-fr", "fr-ca", "fr-be":
		return LocaleFrFR
	case "hi", "hi-in":
		return LocaleHiIN
	case "it", "it-it":
		return LocaleItIT
	case "ja", "ja-jp":
		return LocaleJaJP
	case "ko", "ko-kr":
		return LocaleKoKR
	case "pt", "pt-br", "pt-pt":
		return LocalePtBR
	case "sl", "sl-si":
		return LocaleSlSI
	case "tr", "tr-tr":
		return LocaleTrTR
	case "vi", "vi-vn":
		return LocaleViVN
	}

	// 检查前缀匹配
	if strings.HasPrefix(lang, "zh") {
		return LocaleZhCN
	}
	if strings.HasPrefix(lang, "en") {
		return LocaleEnUS
	}
	if strings.HasPrefix(lang, "ar") {
		return LocaleArSA
	}
	if strings.HasPrefix(lang, "bn") {
		return LocaleBnBD
	}
	if strings.HasPrefix(lang, "de") {
		return LocaleDeDE
	}
	if strings.HasPrefix(lang, "es") {
		return LocaleEsES
	}
	if strings.HasPrefix(lang, "fr") {
		return LocaleFrFR
	}
	if strings.HasPrefix(lang, "hi") {
		return LocaleHiIN
	}
	if strings.HasPrefix(lang, "it") {
		return LocaleItIT
	}
	if strings.HasPrefix(lang, "ja") {
		return LocaleJaJP
	}
	if strings.HasPrefix(lang, "ko") {
		return LocaleKoKR
	}
	if strings.HasPrefix(lang, "pt") {
		return LocalePtBR
	}
	if strings.HasPrefix(lang, "sl") {
		return LocaleSlSI
	}
	if strings.HasPrefix(lang, "tr") {
		return LocaleTrTR
	}
	if strings.HasPrefix(lang, "vi") {
		return LocaleViVN
	}

	// 不支持的语言默认使用英文
	return LocaleEnUS
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
	} else {
		// 验证语言是否支持
		supported := map[string]bool{
			LocaleZhCN: true,
			LocaleEnUS: true,
			LocaleArSA: true,
			LocaleBnBD: true,
			LocaleDeDE: true,
			LocaleEsES: true,
			LocaleFrFR: true,
			LocaleHiIN: true,
			LocaleItIT: true,
			LocaleJaJP: true,
			LocaleKoKR: true,
			LocalePtBR: true,
			LocaleSlSI: true,
			LocaleTrTR: true,
			LocaleViVN: true,
			LocaleZhTW: true,
		}
		if !supported[locale] {
			// 非支持语言统一退回英文
			locale = LocaleEnUS
		}
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
