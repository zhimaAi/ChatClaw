package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"
)

//go:embed locales/*.json
var localesFS embed.FS

// 支持的语言
const (
	LocaleZhCN = "zh-CN"
	LocaleEnUS = "en-US"
)

// Service 多语言服务
type Service struct {
	locale       string
	translations map[string]map[string]any // locale -> nested map
}

// NewService 创建多语言服务
func NewService(locale string) *Service {
	s := &Service{
		translations: make(map[string]map[string]any),
	}
	// 加载所有语言文件
	s.loadLocale(LocaleZhCN)
	s.loadLocale(LocaleEnUS)
	// 设置当前语言
	s.SetLocale(locale)
	return s
}

func (s *Service) loadLocale(locale string) {
	data, err := localesFS.ReadFile("locales/" + locale + ".json")
	if err != nil {
		return
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return
	}
	s.translations[locale] = m
}

// GetLocale 获取当前语言
func (s *Service) GetLocale() string {
	return s.locale
}

// SetLocale 设置语言
func (s *Service) SetLocale(locale string) {
	if locale == LocaleZhCN || locale == LocaleEnUS {
		s.locale = locale
	} else {
		s.locale = LocaleZhCN // 默认中文
	}
}

// T 获取翻译文本，支持嵌套 key（如 "systray.show" 或 "error.app_required"）
func (s *Service) T(key string) string {
	if text := s.lookup(s.locale, key); text != "" {
		return text
	}
	// 回退到英文
	if s.locale != LocaleEnUS {
		if text := s.lookup(LocaleEnUS, key); text != "" {
			return text
		}
	}
	return key
}

// Tf 获取翻译文本（支持 fmt.Sprintf 格式化参数）
func (s *Service) Tf(key string, args ...any) string {
	return fmt.Sprintf(s.T(key), args...)
}

// lookup 从指定语言的翻译中查找 key（支持嵌套，如 "error.app_required"）
func (s *Service) lookup(locale, key string) string {
	m, ok := s.translations[locale]
	if !ok {
		return ""
	}

	parts := strings.Split(key, ".")
	var current any = m

	for _, part := range parts {
		switch v := current.(type) {
		case map[string]any:
			current, ok = v[part]
			if !ok {
				return ""
			}
		default:
			return ""
		}
	}

	if str, ok := current.(string); ok {
		return str
	}
	return ""
}
