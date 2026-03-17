package define

// BuiltinProviderConfig 内置供应商配置
type BuiltinProviderConfig struct {
	ProviderID  string
	Name        string
	Type        string
	Icon        string
	SortOrder   int
	APIEndpoint string
}

// BuiltinModelConfig 内置模型配置
type BuiltinModelConfig struct {
	ProviderID   string
	ModelID      string
	Name         string
	Type         string
	SortOrder    int
	Capabilities []string // 支持的输入类型: text, image, audio, video, file
}

// BuiltinProviders 内置供应商列表（chatclaw 默认置顶，将通过接口查询信息）
var BuiltinProviders = []BuiltinProviderConfig{
	{ProviderID: "chatclaw", Name: "ChatClaw", Type: "openai", Icon: "chatclaw", SortOrder: 0, APIEndpoint: ServerURL},
	{ProviderID: "chatwiki", Name: "Chatwiki", Type: "openai", Icon: "chatclaw", SortOrder: 1, APIEndpoint: ""},
	{ProviderID: "openai", Name: "OpenAI", Type: "openai", Icon: "openai", SortOrder: 2, APIEndpoint: "https://api.openai.com/v1"},
	{ProviderID: "azure", Name: "Azure OpenAI", Type: "azure", Icon: "azure", SortOrder: 3, APIEndpoint: ""},
	{ProviderID: "anthropic", Name: "Anthropic", Type: "anthropic", Icon: "anthropic", SortOrder: 4, APIEndpoint: "https://api.anthropic.com/v1"},
	{ProviderID: "google", Name: "Google Gemini", Type: "gemini", Icon: "google", SortOrder: 5, APIEndpoint: "https://generativelanguage.googleapis.com"},
	{ProviderID: "grok", Name: "Grok", Type: "openai", Icon: "grok", SortOrder: 6, APIEndpoint: "https://api.x.ai/v1"},
	{ProviderID: "deepseek", Name: "DeepSeek", Type: "openai", Icon: "deepseek", SortOrder: 7, APIEndpoint: "https://api.deepseek.com/v1"},
	{ProviderID: "zhipu", Name: "智谱 GLM", Type: "openai", Icon: "zhipu", SortOrder: 8, APIEndpoint: "https://open.bigmodel.cn/api/paas/v4"},
	{ProviderID: "qwen", Name: "通义千问", Type: "qwen", Icon: "qwen", SortOrder: 9, APIEndpoint: "https://dashscope.aliyuncs.com/compatible-mode/v1"},
	{ProviderID: "doubao", Name: "豆包", Type: "openai", Icon: "doubao", SortOrder: 10, APIEndpoint: "https://ark.cn-beijing.volces.com/api/v3"},
	{ProviderID: "baidu", Name: "百度文心", Type: "openai", Icon: "baidu", SortOrder: 11, APIEndpoint: "https://qianfan.baidubce.com/v2"},
	{ProviderID: "ollama", Name: "Ollama", Type: "ollama", Icon: "ollama", SortOrder: 12, APIEndpoint: "http://localhost:11434"},
}

// BuiltinModels 内置模型列表（初始化时写入 models 表；ChatClaw 默认几条，与本地/服务器常见免费模型一致，后续可由 SyncChatClawModels 增删改）
var BuiltinModels = []BuiltinModelConfig{
	// ChatClaw（免费模型，初始化即写入；与 custom-model/list 返回的 modelName 一致时可被同步逻辑更新）
	{ProviderID: "chatclaw", ModelID: "Qwen/Qwen3-8B", Name: "Qwen/Qwen3-8B", Type: "llm", SortOrder: 0, Capabilities: []string{"text", "image"}},
	{ProviderID: "chatclaw", ModelID: "deepseek-ai/DeepSeek-R1-0528-Qwen3-8B", Name: "deepseek-ai/DeepSeek-R1-0528-Qwen3-8B", Type: "llm", SortOrder: 1, Capabilities: []string{"text", "image"}},
	{ProviderID: "chatclaw", ModelID: "BAAI/bge-m3", Name: "BAAI/bge-m3", Type: "embedding", SortOrder: 0, Capabilities: []string{"text"}},
	{ProviderID: "chatclaw", ModelID: "BAAI/bge-reranker-v2-m3", Name: "BAAI/bge-reranker-v2-m3", Type: "rerank", SortOrder: 0, Capabilities: []string{"text"}},

	// OpenAI
	{ProviderID: "openai", ModelID: "gpt-5.4", Name: "GPT-5.4", Type: "llm", SortOrder: 100, Capabilities: []string{"text", "image", "file"}},
	{ProviderID: "openai", ModelID: "gpt-5.4-pro", Name: "GPT-5.4 Pro", Type: "llm", SortOrder: 101, Capabilities: []string{"text", "image", "file"}},
	{ProviderID: "openai", ModelID: "gpt-5.2", Name: "GPT-5.2", Type: "llm", SortOrder: 102, Capabilities: []string{"text", "image", "file"}},
	{ProviderID: "openai", ModelID: "gpt-5.1", Name: "GPT-5.1", Type: "llm", SortOrder: 103, Capabilities: []string{"text", "image", "file"}},
	{ProviderID: "openai", ModelID: "gpt-5", Name: "GPT-5", Type: "llm", SortOrder: 104, Capabilities: []string{"text", "image", "file"}},
	{ProviderID: "openai", ModelID: "gpt-5-mini", Name: "GPT-5 mini", Type: "llm", SortOrder: 105, Capabilities: []string{"text", "image", "file"}},
	{ProviderID: "openai", ModelID: "gpt-5.2-nano", Name: "GPT-5.2 nano", Type: "llm", SortOrder: 106, Capabilities: []string{"text", "image", "file"}},
	{ProviderID: "openai", ModelID: "gpt-5.2-pro", Name: "GPT-5.2 Pro", Type: "llm", SortOrder: 107, Capabilities: []string{"text", "image", "file"}},
	{ProviderID: "openai", ModelID: "text-embedding-3-large", Name: "Text Embedding 3 Large", Type: "embedding", SortOrder: 100, Capabilities: []string{"text"}},
	{ProviderID: "openai", ModelID: "text-embedding-3-small", Name: "Text Embedding 3 Small", Type: "embedding", SortOrder: 101, Capabilities: []string{"text"}},

	// Anthropic
	{ProviderID: "anthropic", ModelID: "claude-opus-4-6", Name: "Claude Opus 4.6", Type: "llm", SortOrder: 100, Capabilities: []string{"text", "image"}},
	{ProviderID: "anthropic", ModelID: "claude-sonnet-4-6", Name: "Claude Sonnet 4.6", Type: "llm", SortOrder: 101, Capabilities: []string{"text", "image"}},
	{ProviderID: "anthropic", ModelID: "claude-haiku-4-5", Name: "Claude Haiku 4.5", Type: "llm", SortOrder: 102, Capabilities: []string{"text", "image"}},

	// Google
	{ProviderID: "google", ModelID: "gemini-3.1-pro-preview", Name: "Gemini 3.1 Pro", Type: "llm", SortOrder: 99, Capabilities: []string{"text", "image", "audio", "video", "file"}},
	{ProviderID: "google", ModelID: "gemini-3-pro-preview", Name: "Gemini 3 Pro", Type: "llm", SortOrder: 100, Capabilities: []string{"text", "image", "audio", "video", "file"}},
	{ProviderID: "google", ModelID: "gemini-3-flash-preview", Name: "Gemini 3 Flash", Type: "llm", SortOrder: 101, Capabilities: []string{"text", "image", "audio", "video", "file"}},
	{ProviderID: "google", ModelID: "gemini-2.5-flash", Name: "Gemini 2.5 Flash", Type: "llm", SortOrder: 102, Capabilities: []string{"text", "image", "audio", "video", "file"}},
	{ProviderID: "google", ModelID: "gemini-2.5-flash-lite", Name: "Gemini 2.5 Flash-Lite", Type: "llm", SortOrder: 103, Capabilities: []string{"text", "image", "audio", "video", "file"}},
	{ProviderID: "google", ModelID: "gemini-2.5-pro", Name: "Gemini 2.5 Pro", Type: "llm", SortOrder: 104, Capabilities: []string{"text", "image", "audio", "video", "file"}},

	// DeepSeek
	{ProviderID: "deepseek", ModelID: "deepseek-chat", Name: "DeepSeek V3", Type: "llm", SortOrder: 100, Capabilities: []string{"text"}},
	{ProviderID: "deepseek", ModelID: "deepseek-reasoner", Name: "DeepSeek R1", Type: "llm", SortOrder: 101, Capabilities: []string{"text"}},

	// 智谱
	{ProviderID: "zhipu", ModelID: "glm-5", Name: "glm-5", Type: "llm", SortOrder: 99, Capabilities: []string{"text"}},
	{ProviderID: "zhipu", ModelID: "glm-4.7", Name: "glm-4.7", Type: "llm", SortOrder: 100, Capabilities: []string{"text"}},
	{ProviderID: "zhipu", ModelID: "glm-4.7-flash", Name: "glm-4.7-flash", Type: "llm", SortOrder: 101, Capabilities: []string{"text"}},
	{ProviderID: "zhipu", ModelID: "glm-4.7-flashx", Name: "glm-4.7-flashx", Type: "llm", SortOrder: 102, Capabilities: []string{"text"}},
	{ProviderID: "zhipu", ModelID: "glm-4.6", Name: "glm-4.6", Type: "llm", SortOrder: 103, Capabilities: []string{"text"}},
	{ProviderID: "zhipu", ModelID: "glm-4.5-air", Name: "glm-4.5-air", Type: "llm", SortOrder: 104, Capabilities: []string{"text"}},
	{ProviderID: "zhipu", ModelID: "glm-4.5-airx", Name: "glm-4.5-airx", Type: "llm", SortOrder: 105, Capabilities: []string{"text"}},
	{ProviderID: "zhipu", ModelID: "glm-4.5-flash", Name: "glm-4.5-flash", Type: "llm", SortOrder: 106, Capabilities: []string{"text"}},
	{ProviderID: "zhipu", ModelID: "glm-4-flash-250414", Name: "glm-4-flash-250414", Type: "llm", SortOrder: 107, Capabilities: []string{"text"}},
	{ProviderID: "zhipu", ModelID: "glm-4-flashx-250414", Name: "glm-4-flashx-250414", Type: "llm", SortOrder: 108, Capabilities: []string{"text"}},
	{ProviderID: "zhipu", ModelID: "glm-4v-flash", Name: "GLM-4V Flash", Type: "llm", SortOrder: 109, Capabilities: []string{"text", "image"}},
	{ProviderID: "zhipu", ModelID: "glm-4v-plus", Name: "GLM-4V Plus", Type: "llm", SortOrder: 110, Capabilities: []string{"text", "image"}},
	{ProviderID: "zhipu", ModelID: "embedding-3", Name: "Embedding-3", Type: "embedding", SortOrder: 100, Capabilities: []string{"text"}},

	// 通义千问
	{ProviderID: "qwen", ModelID: "qwen3.5-plus", Name: "通义千问 3.5 Plus", Type: "llm", SortOrder: 98, Capabilities: []string{"text", "image"}},
	{ProviderID: "qwen", ModelID: "qwen3.5-flash", Name: "通义千问 3.5 Flash", Type: "llm", SortOrder: 99, Capabilities: []string{"text", "image"}},
	{ProviderID: "qwen", ModelID: "qwen3-max", Name: "通义千问 Max", Type: "llm", SortOrder: 100, Capabilities: []string{"text"}},
	{ProviderID: "qwen", ModelID: "qwen-plus", Name: "通义千问 Plus", Type: "llm", SortOrder: 101, Capabilities: []string{"text"}},
	{ProviderID: "qwen", ModelID: "qwen-flash", Name: "通义千问 Flash", Type: "llm", SortOrder: 102, Capabilities: []string{"text"}},
	{ProviderID: "qwen", ModelID: "qwen-long", Name: "通义千问 Long", Type: "llm", SortOrder: 103, Capabilities: []string{"text"}},
	{ProviderID: "qwen", ModelID: "qwen3-vl-plus", Name: "通义千问 vl Plus", Type: "llm", SortOrder: 103, Capabilities: []string{"text", "image"}},
	{ProviderID: "qwen", ModelID: "qwen3-vl-flash", Name: "通义千问 vl flash", Type: "llm", SortOrder: 103, Capabilities: []string{"text", "image"}},
	{ProviderID: "qwen", ModelID: "text-embedding-v4", Name: "Text Embedding V4", Type: "embedding", SortOrder: 100, Capabilities: []string{"text"}},
	{ProviderID: "qwen", ModelID: "qwen3-rerank", Name: "Qwen3 Rerank", Type: "rerank", SortOrder: 100, Capabilities: []string{"text"}},

	// 百度文心
	{ProviderID: "baidu", ModelID: "ernie-5.0-thinking-latest", Name: "ERNIE 5.0", Type: "llm", SortOrder: 100, Capabilities: []string{"text", "image"}},
	{ProviderID: "baidu", ModelID: "ernie-4.5-turbo-vl-32k", Name: "ERNIE 4.5 Turbo VL", Type: "llm", SortOrder: 101, Capabilities: []string{"text", "image"}},
	{ProviderID: "baidu", ModelID: "ernie-speed-pro-128k", Name: "ERNIE Speed", Type: "llm", SortOrder: 102, Capabilities: []string{"text"}},
	{ProviderID: "baidu", ModelID: "ernie-lite-pro-128k", Name: "ERNIE Lite", Type: "llm", SortOrder: 103, Capabilities: []string{"text"}},

	// 豆包
	{ProviderID: "doubao", ModelID: "doubao-pro-32k", Name: "Doubao Pro 32K", Type: "llm", SortOrder: 100, Capabilities: []string{"text", "image"}},
	{ProviderID: "doubao", ModelID: "doubao-lite-32k", Name: "Doubao Lite 32K", Type: "llm", SortOrder: 101, Capabilities: []string{"text"}},
	{ProviderID: "doubao", ModelID: "doubao-1.5-vision-pro", Name: "Doubao 1.5 Vision Pro", Type: "llm", SortOrder: 102, Capabilities: []string{"text", "image"}},
	{ProviderID: "doubao", ModelID: "doubao-1.5-vision-lite", Name: "Doubao 1.5 Vision Lite", Type: "llm", SortOrder: 103, Capabilities: []string{"text", "image"}},

	// Grok
	{ProviderID: "grok", ModelID: "grok-4-1-fast-reasoning", Name: "Grok 4.1 Fast Reasoning", Type: "llm", SortOrder: 100, Capabilities: []string{"text", "image"}},
	{ProviderID: "grok", ModelID: "grok-4-1-fast-reasoning-pro", Name: "Grok 4.1 Fast Reasoning Pro", Type: "llm", SortOrder: 101, Capabilities: []string{"text", "image"}},
	{ProviderID: "grok", ModelID: "grok-4-fast-reasoning", Name: "Grok 4 Fast Reasoning", Type: "llm", SortOrder: 102, Capabilities: []string{"text", "image"}},
	{ProviderID: "grok", ModelID: "grok-4-fast-non-reasoning", Name: "Grok 4 Fast Non-Reasoning", Type: "llm", SortOrder: 103, Capabilities: []string{"text", "image"}},
}

// GetBuiltinProviderDefaultEndpoint 获取内置供应商的默认 API 地址
// ChatClaw 直接使用 ServerURL
func GetBuiltinProviderDefaultEndpoint(providerID string) (string, bool) {
	if providerID == "chatclaw" {
		return ServerURL, true
	}
	if providerID == "chatwiki" {
		return "", true
	}
	for _, p := range BuiltinProviders {
		if p.ProviderID == providerID {
			return p.APIEndpoint, true
		}
	}
	return "", false
}

// GetBuiltinModelCapabilities 获取内置模型的默认能力
func GetBuiltinModelCapabilities(providerID, modelID string) []string {
	for _, m := range BuiltinModels {
		if m.ProviderID == providerID && m.ModelID == modelID {
			return m.Capabilities
		}
	}
	return []string{"text"} // 默认返回 text
}
