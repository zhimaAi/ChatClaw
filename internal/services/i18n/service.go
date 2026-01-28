package i18n

// Service 多语言服务
type Service struct {
	locale string
}

// NewService 创建多语言服务
func NewService(locale string) *Service {
	// 验证语言是否支持，不支持则使用默认语言
	if locale != LocaleZhCN && locale != LocaleEnUS {
		locale = LocaleZhCN
	}
	return &Service{
		locale: locale,
	}
}

// GetLocale 获取当前语言
func (s *Service) GetLocale() string {
	return s.locale
}

// SetLocale 设置语言
func (s *Service) SetLocale(locale string) {
	if locale == LocaleZhCN || locale == LocaleEnUS {
		s.locale = locale
	}
}

// T 获取翻译文本
func (s *Service) T(key string) string {
	if texts, ok := translations[s.locale]; ok {
		if text, ok := texts[key]; ok {
			return text
		}
	}
	// 回退到英文
	if texts, ok := translations[LocaleEnUS]; ok {
		if text, ok := texts[key]; ok {
			return text
		}
	}
	return key
}
