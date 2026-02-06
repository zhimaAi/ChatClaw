//go:build production

package define

// 生产环境默认值（启用 -tags production 时生效）
var (
	Env                 = "production"
	ServerURL           = "https://willchat.chatwiki.com"
	ChatWikiAPIEndpoint = "https://dev3.willchat.chatwiki.com/openapi"
)
