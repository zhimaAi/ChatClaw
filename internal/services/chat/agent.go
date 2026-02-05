package chat

import (
	"context"
	"encoding/json"

	"willchat/internal/errs"
	"willchat/internal/services/tools"

	"github.com/cloudwego/eino-ext/components/model/claude"
	einogemini "github.com/cloudwego/eino-ext/components/model/gemini"
	"github.com/cloudwego/eino-ext/components/model/ollama"
	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"google.golang.org/genai"
)

// ProviderConfig contains the configuration for a provider
type ProviderConfig struct {
	ProviderID  string
	Type        string // openai, azure, anthropic, gemini, ollama
	APIKey      string
	APIEndpoint string
	ExtraConfig string
}

// AgentConfig contains the configuration for creating an agent
type AgentConfig struct {
	Name        string
	Instruction string
	ModelID     string
	Provider    ProviderConfig

	// Optional model parameters
	Temperature    *float64
	TopP           *float64
	MaxTokens      *int
	EnableTemp     bool
	EnableTopP     bool
	EnableMaxTokens bool
}

// createChatModel creates a ChatModel based on the provider type
func createChatModel(ctx context.Context, config AgentConfig) (model.ToolCallingChatModel, error) {
	switch config.Provider.Type {
	case "openai":
		return createOpenAIChatModel(ctx, config)
	case "azure":
		return createAzureChatModel(ctx, config)
	case "anthropic":
		return createClaudeChatModel(ctx, config)
	case "gemini":
		return createGeminiChatModel(ctx, config)
	case "ollama":
		return createOllamaChatModel(ctx, config)
	default:
		return nil, errs.Newf("error.chat_unsupported_provider", map[string]any{"Type": config.Provider.Type})
	}
}

func createOpenAIChatModel(ctx context.Context, config AgentConfig) (model.ToolCallingChatModel, error) {
	cfg := &openai.ChatModelConfig{
		APIKey:  config.Provider.APIKey,
		Model:   config.ModelID,
		BaseURL: config.Provider.APIEndpoint,
	}

	if config.EnableTemp && config.Temperature != nil {
		temp := float32(*config.Temperature)
		cfg.Temperature = &temp
	}
	if config.EnableTopP && config.TopP != nil {
		topP := float32(*config.TopP)
		cfg.TopP = &topP
	}
	if config.EnableMaxTokens && config.MaxTokens != nil {
		cfg.MaxTokens = config.MaxTokens
	}

	return openai.NewChatModel(ctx, cfg)
}

func createAzureChatModel(ctx context.Context, config AgentConfig) (model.ToolCallingChatModel, error) {
	var extraConfig struct {
		APIVersion string `json:"api_version"`
	}
	if config.Provider.ExtraConfig != "" {
		if err := json.Unmarshal([]byte(config.Provider.ExtraConfig), &extraConfig); err != nil {
			return nil, errs.Wrap("error.chat_invalid_extra_config", err)
		}
	}

	cfg := &openai.ChatModelConfig{
		APIKey:     config.Provider.APIKey,
		Model:      config.ModelID,
		BaseURL:    config.Provider.APIEndpoint,
		ByAzure:    true,
		APIVersion: extraConfig.APIVersion,
	}

	if config.EnableTemp && config.Temperature != nil {
		temp := float32(*config.Temperature)
		cfg.Temperature = &temp
	}
	if config.EnableTopP && config.TopP != nil {
		topP := float32(*config.TopP)
		cfg.TopP = &topP
	}
	if config.EnableMaxTokens && config.MaxTokens != nil {
		cfg.MaxTokens = config.MaxTokens
	}

	return openai.NewChatModel(ctx, cfg)
}

func createClaudeChatModel(ctx context.Context, config AgentConfig) (model.ToolCallingChatModel, error) {
	var baseURL *string
	if config.Provider.APIEndpoint != "" {
		baseURL = &config.Provider.APIEndpoint
	}

	cfg := &claude.Config{
		APIKey:  config.Provider.APIKey,
		Model:   config.ModelID,
		BaseURL: baseURL,
	}

	if config.EnableTemp && config.Temperature != nil {
		temp := float32(*config.Temperature)
		cfg.Temperature = &temp
	}
	if config.EnableTopP && config.TopP != nil {
		topP := float32(*config.TopP)
		cfg.TopP = &topP
	}
	if config.EnableMaxTokens && config.MaxTokens != nil {
		cfg.MaxTokens = *config.MaxTokens
	} else {
		cfg.MaxTokens = 4096 // Default for Claude
	}

	return claude.NewChatModel(ctx, cfg)
}

func createGeminiChatModel(ctx context.Context, config AgentConfig) (model.ToolCallingChatModel, error) {
	clientConfig := &genai.ClientConfig{
		APIKey: config.Provider.APIKey,
	}
	if config.Provider.APIEndpoint != "" {
		clientConfig.HTTPOptions = genai.HTTPOptions{
			BaseURL: config.Provider.APIEndpoint,
		}
	}

	client, err := genai.NewClient(ctx, clientConfig)
	if err != nil {
		return nil, errs.Wrap("error.chat_gemini_client_failed", err)
	}

	cfg := &einogemini.Config{
		Client: client,
		Model:  config.ModelID,
	}

	if config.EnableTemp && config.Temperature != nil {
		temp := float32(*config.Temperature)
		cfg.Temperature = &temp
	}
	if config.EnableTopP && config.TopP != nil {
		topP := float32(*config.TopP)
		cfg.TopP = &topP
	}
	// Note: Gemini config doesn't have MaxOutputTokens in this version

	return einogemini.NewChatModel(ctx, cfg)
}

func createOllamaChatModel(ctx context.Context, config AgentConfig) (model.ToolCallingChatModel, error) {
	cfg := &ollama.ChatModelConfig{
		BaseURL: config.Provider.APIEndpoint,
		Model:   config.ModelID,
	}
	// Note: Ollama config temperature/topP are set via Options, not direct fields

	return ollama.NewChatModel(ctx, cfg)
}

// createChatModelAgent creates an ADK ChatModelAgent with tools
func createChatModelAgent(ctx context.Context, config AgentConfig, toolRegistry *tools.ToolRegistry) (adk.Agent, error) {
	// Create the chat model
	chatModel, err := createChatModel(ctx, config)
	if err != nil {
		return nil, err
	}

	// Get enabled tools
	enabledTools, err := toolRegistry.GetAllTools(ctx)
	if err != nil {
		return nil, errs.Wrap("error.chat_tools_failed", err)
	}

	// Convert to BaseTool slice
	baseTools := make([]tool.BaseTool, len(enabledTools))
	copy(baseTools, enabledTools)

	// Create the agent
	instruction := config.Instruction
	// Tool calling reliability note:
	// Some OpenAI-compatible providers require tool call "function.arguments" to be a valid JSON object string.
	// Adding an explicit instruction reduces the chance of malformed arguments and 400 errors.
	instruction = instruction + "\n\n" +
		"工具调用规则（非常重要）：当你调用任何工具时，必须让 tool_calls[].function.arguments 是严格合法的 JSON（对象），" +
		"例如：{\"query\":\"...\"} 或 {\"expression\":\"1+2\"}。不要输出 key=value、不要输出纯文本、不要输出不带引号的字段。\n"

	agentConfig := &adk.ChatModelAgentConfig{
		Name:        config.Name,
		Description: "AI Assistant",
		Instruction: instruction,
		Model:       chatModel,
		MaxIterations: 20,
	}

	// Add tools if any
	if len(baseTools) > 0 {
		agentConfig.ToolsConfig = adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: baseTools,
			},
		}
	}

	return adk.NewChatModelAgent(ctx, agentConfig)
}
