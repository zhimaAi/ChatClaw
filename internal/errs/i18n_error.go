package errs

// Localizer 用于生成本地化错误文案（来自 i18n.Service）。
type Localizer interface {
	T(key string) string
	Tf(key string, args ...any) string
}

// I18nError 业务错误：携带 i18n key，并把本地化后的 message 作为 Error() 输出。
//
// 说明：
// - 前端通常只能拿到 error.message（Wails Promise reject），所以这里以 message 为主。
// - key 保留给后端日志/排查（或未来扩展前端结构化错误处理）。
type I18nError struct {
	Key     string
	Message string
	Cause   error
}

func (e *I18nError) Error() string { return e.Message }
func (e *I18nError) Unwrap() error { return e.Cause }

func NewI18n(localizer Localizer, key string, cause error) error {
	msg := key
	if localizer != nil {
		msg = localizer.T(key)
	}
	return &I18nError{
		Key:     key,
		Message: msg,
		Cause:   cause,
	}
}

func NewI18nF(localizer Localizer, key string, cause error, args ...any) error {
	msg := key
	if localizer != nil {
		msg = localizer.Tf(key, args...)
	}
	return &I18nError{
		Key:     key,
		Message: msg,
		Cause:   cause,
	}
}
