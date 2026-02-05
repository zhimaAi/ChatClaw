package chatmodel

import (
	"context"
	"encoding/json"
	"time"

	"github.com/cloudwego/eino-ext/components/model/claude"
	einogemini "github.com/cloudwego/eino-ext/components/model/gemini"
	"github.com/cloudwego/eino-ext/components/model/ollama"
	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"google.golang.org/genai"
)

// ProviderConfig 创建 ChatModel 所需的配置
type ProviderConfig struct {
	// ProviderType 供应商类型（openai, azure, ollama, gemini, anthropic）
	ProviderType string
	// APIKey 供应商的 API 密钥
	APIKey string
	// APIEndpoint 供应商 API 的基础 URL
	APIEndpoint string
	// ModelID LLM 模型的 ID
	ModelID string
	// ExtraConfig 供应商特定的配置（JSON 格式）
	ExtraConfig string
	// Timeout 请求超时时间
	Timeout time.Duration
}

// NewChatModel 根据供应商配置创建新的 ChatModel
func NewChatModel(ctx context.Context, cfg *ProviderConfig) (model.ChatModel, error) {
	if cfg.Timeout == 0 {
		cfg.Timeout = 120 * time.Second
	}

	switch cfg.ProviderType {
	case "openai":
		return newOpenAIChatModel(ctx, cfg)
	case "azure":
		return newAzureChatModel(ctx, cfg)
	case "ollama":
		return newOllamaChatModel(ctx, cfg)
	case "gemini":
		return newGeminiChatModel(ctx, cfg)
	case "anthropic":
		return newClaudeChatModel(ctx, cfg)
	default:
		// 默认使用 OpenAI 兼容 API
		return newOpenAIChatModel(ctx, cfg)
	}
}

// newOpenAIChatModel 创建 OpenAI ChatModel
func newOpenAIChatModel(ctx context.Context, cfg *ProviderConfig) (model.ChatModel, error) {
	config := &openai.ChatModelConfig{
		APIKey: cfg.APIKey,
		Model:  cfg.ModelID,
	}
	if cfg.APIEndpoint != "" {
		config.BaseURL = cfg.APIEndpoint
	}
	return openai.NewChatModel(ctx, config)
}

// newAzureChatModel 创建 Azure OpenAI ChatModel
func newAzureChatModel(ctx context.Context, cfg *ProviderConfig) (model.ChatModel, error) {
	// 解析 Azure 特定的额外配置
	var extraConfig struct {
		APIVersion string `json:"api_version"`
	}
	if cfg.ExtraConfig != "" {
		if err := json.Unmarshal([]byte(cfg.ExtraConfig), &extraConfig); err != nil {
			// 解析失败时使用默认 API 版本
			extraConfig.APIVersion = "2023-05-15"
		}
	}
	if extraConfig.APIVersion == "" {
		extraConfig.APIVersion = "2023-05-15"
	}

	config := &openai.ChatModelConfig{
		APIKey:     cfg.APIKey,
		Model:      cfg.ModelID,
		BaseURL:    cfg.APIEndpoint,
		ByAzure:    true,
		APIVersion: extraConfig.APIVersion,
	}
	return openai.NewChatModel(ctx, config)
}

// newOllamaChatModel 创建 Ollama ChatModel
func newOllamaChatModel(ctx context.Context, cfg *ProviderConfig) (model.ChatModel, error) {
	baseURL := cfg.APIEndpoint
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}

	config := &ollama.ChatModelConfig{
		BaseURL: baseURL,
		Model:   cfg.ModelID,
	}
	return ollama.NewChatModel(ctx, config)
}

// newGeminiChatModel 创建 Gemini ChatModel
func newGeminiChatModel(ctx context.Context, cfg *ProviderConfig) (model.ChatModel, error) {
	clientConfig := &genai.ClientConfig{
		APIKey: cfg.APIKey,
	}
	if cfg.APIEndpoint != "" {
		clientConfig.HTTPOptions = genai.HTTPOptions{
			BaseURL: cfg.APIEndpoint,
		}
	}
	client, err := genai.NewClient(ctx, clientConfig)
	if err != nil {
		return nil, err
	}

	return einogemini.NewChatModel(ctx, &einogemini.Config{
		Client: client,
		Model:  cfg.ModelID,
	})
}

// newClaudeChatModel 创建 Claude ChatModel
func newClaudeChatModel(ctx context.Context, cfg *ProviderConfig) (model.ChatModel, error) {
	var baseURL *string
	if cfg.APIEndpoint != "" {
		baseURL = &cfg.APIEndpoint
	}

	return claude.NewChatModel(ctx, &claude.Config{
		APIKey:    cfg.APIKey,
		Model:     cfg.ModelID,
		BaseURL:   baseURL,
		MaxTokens: 4096,
	})
}
