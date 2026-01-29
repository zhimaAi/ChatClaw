package errs

import "willchat/internal/services/i18n"

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

// New 创建 i18n 业务错误
func New(key string) error {
	return &I18nError{
		Key:     key,
		Message: i18n.T(key),
	}
}

// Newf 创建带参数的 i18n 业务错误
func Newf(key string, data map[string]any) error {
	return &I18nError{
		Key:     key,
		Message: i18n.Tf(key, data),
	}
}

// Wrap 包装底层错误为 i18n 业务错误
func Wrap(key string, cause error) error {
	return &I18nError{
		Key:     key,
		Message: i18n.T(key),
		Cause:   cause,
	}
}

// Wrapf 包装底层错误为带参数的 i18n 业务错误
func Wrapf(key string, cause error, data map[string]any) error {
	return &I18nError{
		Key:     key,
		Message: i18n.Tf(key, data),
		Cause:   cause,
	}
}
