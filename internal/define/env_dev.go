//go:build !production

package define

// 开发环境默认值（当未启用 -tags production 时生效）
var (
	Env              = "development"
	ServerURL        = "https://dev1.chatclaw.chatwiki.com/openapi"
	ChatWikiCloudURL = "http://dev13.zhima_chat_ai.applnk.cn"
	ModelChatWikiUrl = "http://dev13.zhima_chat_ai.applnk.cn"
)
