package i18n

// 支持的语言
const (
	LocaleZhCN = "zh-CN"
	LocaleEnUS = "en-US"
)

// 翻译文本
var translations = map[string]map[string]string{
	LocaleZhCN: {
		"systray.show": "显示",
		"systray.quit": "退出",
	},
	LocaleEnUS: {
		"systray.show": "Show",
		"systray.quit": "Quit",
	},
}
