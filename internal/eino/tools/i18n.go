package tools

import "chatclaw/internal/services/i18n"

// selectDesc returns the Chinese description when locale is zh-CN, otherwise English.
// Used for tool descriptions to match the agent's language.
func selectDesc(eng, zh string) string {
	if i18n.GetLocale() == i18n.LocaleZhCN {
		return zh
	}
	return eng
}
