package embedding

import (
	"context"
	"encoding/json"
	"time"

	ollamaembed "github.com/cloudwego/eino-ext/components/embedding/ollama"
	openaiembed "github.com/cloudwego/eino-ext/components/embedding/openai"
	"github.com/cloudwego/eino/components/embedding"
)

// ProviderConfig 创建 Embedder 所需的配置
type ProviderConfig struct {
	// ProviderType 供应商类型（openai, azure, ollama, gemini, anthropic）
	ProviderType string
	// APIKey 供应商的 API 密钥
	APIKey string
	// APIEndpoint 供应商 API 的基础 URL
	APIEndpoint string
	// ModelID 嵌入模型的 ID
	ModelID string
	// Dimension 向量维度（可选，某些模型支持）
	Dimension int
	// ExtraConfig 供应商特定的配置（JSON 格式）
	ExtraConfig string
	// Timeout 请求超时时间
	Timeout time.Duration
}

// NewEmbedder 根据供应商配置创建新的 Embedder
func NewEmbedder(ctx context.Context, cfg *ProviderConfig) (embedding.Embedder, error) {
	if cfg.Timeout == 0 {
		cfg.Timeout = 60 * time.Second
	}

	switch cfg.ProviderType {
	case "openai":
		return newOpenAIEmbedder(ctx, cfg)
	case "azure":
		return newAzureEmbedder(ctx, cfg)
	case "ollama":
		return newOllamaEmbedder(ctx, cfg)
	default:
		// 默认使用 OpenAI 兼容 API
		return newOpenAIEmbedder(ctx, cfg)
	}
}

// newOpenAIEmbedder 创建 OpenAI Embedder
func newOpenAIEmbedder(ctx context.Context, cfg *ProviderConfig) (embedding.Embedder, error) {
	config := &openaiembed.EmbeddingConfig{
		APIKey:  cfg.APIKey,
		Model:   cfg.ModelID,
		Timeout: cfg.Timeout,
	}
	if cfg.APIEndpoint != "" {
		config.BaseURL = cfg.APIEndpoint
	}
	// 设置向量维度（某些模型如 text-embedding-v3、text-embedding-3-large 支持）
	if cfg.Dimension > 0 {
		dim := cfg.Dimension
		config.Dimensions = &dim
	}
	return openaiembed.NewEmbedder(ctx, config)
}

// newAzureEmbedder 创建 Azure OpenAI Embedder
func newAzureEmbedder(ctx context.Context, cfg *ProviderConfig) (embedding.Embedder, error) {
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

	config := &openaiembed.EmbeddingConfig{
		APIKey:     cfg.APIKey,
		Model:      cfg.ModelID,
		BaseURL:    cfg.APIEndpoint,
		ByAzure:    true,
		APIVersion: extraConfig.APIVersion,
		Timeout:    cfg.Timeout,
	}
	return openaiembed.NewEmbedder(ctx, config)
}

// newOllamaEmbedder 创建 Ollama Embedder
func newOllamaEmbedder(ctx context.Context, cfg *ProviderConfig) (embedding.Embedder, error) {
	baseURL := cfg.APIEndpoint
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}

	config := &ollamaembed.EmbeddingConfig{
		BaseURL: baseURL,
		Model:   cfg.ModelID,
		Timeout: cfg.Timeout,
	}
	return ollamaembed.NewEmbedder(ctx, config)
}
