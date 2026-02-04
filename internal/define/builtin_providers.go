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
	ProviderID string
	ModelID    string
	Name       string
	Type       string
	SortOrder  int
}

// BuiltinProviders 内置供应商列表
var BuiltinProviders = []BuiltinProviderConfig{
	{ProviderID: "openai", Name: "OpenAI", Type: "openai", Icon: "openai", SortOrder: 1, APIEndpoint: "https://api.openai.com/v1"},
	{ProviderID: "azure", Name: "Azure OpenAI", Type: "azure", Icon: "azure", SortOrder: 2, APIEndpoint: ""},
	{ProviderID: "anthropic", Name: "Anthropic", Type: "anthropic", Icon: "anthropic", SortOrder: 3, APIEndpoint: "https://api.anthropic.com/v1"},
	{ProviderID: "google", Name: "Google Gemini", Type: "gemini", Icon: "google", SortOrder: 4, APIEndpoint: "https://generativelanguage.googleapis.com/v1beta"},
	{ProviderID: "grok", Name: "Grok", Type: "openai", Icon: "grok", SortOrder: 5, APIEndpoint: "https://api.x.ai/v1"},
	{ProviderID: "deepseek", Name: "DeepSeek", Type: "openai", Icon: "deepseek", SortOrder: 6, APIEndpoint: "https://api.deepseek.com/v1"},
	{ProviderID: "zhipu", Name: "智谱 GLM", Type: "openai", Icon: "zhipu", SortOrder: 7, APIEndpoint: "https://open.bigmodel.cn/api/paas/v4"},
	{ProviderID: "qwen", Name: "通义千问", Type: "openai", Icon: "qwen", SortOrder: 8, APIEndpoint: "https://dashscope.aliyuncs.com/compatible-mode/v1"},
	{ProviderID: "doubao", Name: "豆包", Type: "openai", Icon: "doubao", SortOrder: 9, APIEndpoint: "https://ark.cn-beijing.volces.com/api/v3"},
	{ProviderID: "baidu", Name: "百度文心", Type: "openai", Icon: "baidu", SortOrder: 10, APIEndpoint: "https://qianfan.baidubce.com/v2"},
	{ProviderID: "ollama", Name: "Ollama", Type: "ollama", Icon: "ollama", SortOrder: 11, APIEndpoint: "http://localhost:11434"},
}

// BuiltinModels 内置模型列表
var BuiltinModels = []BuiltinModelConfig{
	// OpenAI
	{ProviderID: "openai", ModelID: "gpt-5.2", Name: "GPT-5.2", Type: "llm", SortOrder: 100},
	{ProviderID: "openai", ModelID: "gpt-5.1", Name: "GPT-5.1", Type: "llm", SortOrder: 101},
	{ProviderID: "openai", ModelID: "gpt-5", Name: "GPT-5", Type: "llm", SortOrder: 102},
	{ProviderID: "openai", ModelID: "gpt-5-mini", Name: "GPT-5 mini", Type: "llm", SortOrder: 103},
	{ProviderID: "openai", ModelID: "gpt-5.2-nano", Name: "GPT-5.2 nano", Type: "llm", SortOrder: 104},
	{ProviderID: "openai", ModelID: "gpt-5.2-pro", Name: "GPT-5.2 pro", Type: "llm", SortOrder: 105},
	{ProviderID: "openai", ModelID: "text-embedding-3-large", Name: "Text Embedding 3 Large", Type: "embedding", SortOrder: 100},
	{ProviderID: "openai", ModelID: "text-embedding-3-small", Name: "Text Embedding 3 Small", Type: "embedding", SortOrder: 101},

	// Anthropic
	{ProviderID: "anthropic", ModelID: "claude-sonnet-4-5-20250929", Name: "Claude Sonnet 4.5", Type: "llm", SortOrder: 100},
	{ProviderID: "anthropic", ModelID: "claude-haiku-4-5-20251001", Name: "Claude Haiku 4.5", Type: "llm", SortOrder: 101},
	{ProviderID: "anthropic", ModelID: "claude-opus-4-5-20251101", Name: "Claude Opus 4.5", Type: "llm", SortOrder: 102},

	// Google
	{ProviderID: "google", ModelID: "gemini-3-pro-preview", Name: "Gemini 3 Pro", Type: "llm", SortOrder: 100},
	{ProviderID: "google", ModelID: "gemini-3-flash-preview", Name: "Gemini 3 Flash", Type: "llm", SortOrder: 101},
	{ProviderID: "google", ModelID: "gemini-2.5-flash", Name: "Gemini 2.5 Flash", Type: "llm", SortOrder: 102},
	{ProviderID: "google", ModelID: "gemini-2.5-flash-lite", Name: "Gemini 2.5 Flash-Lite", Type: "llm", SortOrder: 103},
	{ProviderID: "google", ModelID: "gemini-2.5-pro", Name: "Gemini 2.5 Pro", Type: "llm", SortOrder: 104},

	// DeepSeek
	{ProviderID: "deepseek", ModelID: "deepseek-chat", Name: "DeepSeek V3", Type: "llm", SortOrder: 100},
	{ProviderID: "deepseek", ModelID: "deepseek-reasoner", Name: "DeepSeek R1", Type: "llm", SortOrder: 101},

	// 智谱
	{ProviderID: "zhipu", ModelID: "glm-4.7", Name: "glm-4.7", Type: "llm", SortOrder: 100},
	{ProviderID: "zhipu", ModelID: "glm-4.7-flash", Name: "glm-4.7-flash", Type: "llm", SortOrder: 101},
	{ProviderID: "zhipu", ModelID: "glm-4.7-flashx", Name: "glm-4.7-flashx", Type: "llm", SortOrder: 102},
	{ProviderID: "zhipu", ModelID: "glm-4.6", Name: "glm-4.6", Type: "llm", SortOrder: 103},
	{ProviderID: "zhipu", ModelID: "glm-4.5-air", Name: "glm-4.5-air", Type: "llm", SortOrder: 104},
	{ProviderID: "zhipu", ModelID: "glm-4.5-airx", Name: "glm-4.5-airx", Type: "llm", SortOrder: 105},
	{ProviderID: "zhipu", ModelID: "glm-4.5-flash", Name: "glm-4.5-flash", Type: "llm", SortOrder: 106},
	{ProviderID: "zhipu", ModelID: "glm-4-flash-250414", Name: "glm-4-flash-250414", Type: "llm", SortOrder: 107},
	{ProviderID: "zhipu", ModelID: "glm-4-flashx-250414", Name: "glm-4-flashx-250414", Type: "llm", SortOrder: 108},
	{ProviderID: "zhipu", ModelID: "embedding-3", Name: "Embedding-3", Type: "embedding", SortOrder: 100},

	// 通义千问
	{ProviderID: "qwen", ModelID: "qwen3-max", Name: "通义千问 Max", Type: "llm", SortOrder: 100},
	{ProviderID: "qwen", ModelID: "qwen-plus", Name: "通义千问 Plus", Type: "llm", SortOrder: 101},
	{ProviderID: "qwen", ModelID: "qwen-flash", Name: "通义千问 Flash", Type: "llm", SortOrder: 102},
	{ProviderID: "qwen", ModelID: "qwen-long", Name: "通义千问 Long", Type: "llm", SortOrder: 103},
	{ProviderID: "qwen", ModelID: "text-embedding-v3", Name: "Text Embedding V3", Type: "embedding", SortOrder: 100},
	{ProviderID: "qwen", ModelID: "qwen3-rerank", Name: "Qwen3 Rerank", Type: "rerank", SortOrder: 100},

	// 百度文心
	{ProviderID: "baidu", ModelID: "ernie-5.0-thinking-latest", Name: "ERNIE 5.0", Type: "llm", SortOrder: 100},
	{ProviderID: "baidu", ModelID: "ernie-4.5-turbo-latest", Name: "ERNIE 4.5 Turbo", Type: "llm", SortOrder: 101},
	{ProviderID: "baidu", ModelID: "ernie-speed-pro-128k", Name: "ERNIE Speed", Type: "llm", SortOrder: 102},
	{ProviderID: "baidu", ModelID: "ernie-lite-pro-128k", Name: "ERNIE Lite", Type: "llm", SortOrder: 103},

	// Grok
	{ProviderID: "grok", ModelID: "grok-4-1-fast-reasoning", Name: "Grok 4.1 Fast Reasoning", Type: "llm", SortOrder: 100},
	{ProviderID: "grok", ModelID: "grok-4-1-fast-reasoning-pro", Name: "Grok 4.1 Fast Reasoning Pro", Type: "llm", SortOrder: 101},
	{ProviderID: "grok", ModelID: "grok-4-fast-reasoning", Name: "Grok 4 Fast Reasoning", Type: "llm", SortOrder: 102},
	{ProviderID: "grok", ModelID: "grok-4-fast-non-reasoning", Name: "Grok 4 Fast Non-Reasoning", Type: "llm", SortOrder: 103},
}

// GetBuiltinProviderDefaultEndpoint 获取内置供应商的默认 API 地址
func GetBuiltinProviderDefaultEndpoint(providerID string) (string, bool) {
	for _, p := range BuiltinProviders {
		if p.ProviderID == providerID {
			return p.APIEndpoint, true
		}
	}
	return "", false
}
