// Package agent provides utilities for creating eino ADK agents with tools and middlewares.
package agent

import (
	"context"
	"encoding/json"
	"log"
	"math"
	"os"
	"path/filepath"

	"willchat/internal/eino/filesystem"
	"willchat/internal/eino/tools"
	"willchat/internal/errs"

	"github.com/cloudwego/eino-ext/components/model/claude"
	einogemini "github.com/cloudwego/eino-ext/components/model/gemini"
	"github.com/cloudwego/eino-ext/components/model/ollama"
	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/adk"
	fsmw "github.com/cloudwego/eino/adk/middlewares/filesystem"
	"github.com/cloudwego/eino/adk/middlewares/reduction"
	"github.com/cloudwego/eino/adk/middlewares/skill"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"google.golang.org/genai"
)

// UnlimitedIterations effectively removes the ReAct loop iteration limit.
// The eino ADK defaults to 20 when MaxIterations <= 0, so we use math.MaxInt32 instead.
var UnlimitedIterations = math.MaxInt32

// ProviderConfig contains the configuration for a provider
type ProviderConfig struct {
	ProviderID  string
	Type        string // openai, azure, anthropic, gemini, ollama
	APIKey      string
	APIEndpoint string
	ExtraConfig string
}

// Config contains the configuration for creating an agent
type Config struct {
	Name        string
	Instruction string
	ModelID     string
	Provider    ProviderConfig

	// Optional model parameters
	Temperature     *float64
	TopP            *float64
	MaxTokens       *int
	EnableTemp      bool
	EnableTopP      bool
	EnableMaxTokens bool

	// Context and retrieval settings
	ContextCount  int // Maximum number of messages to include in context (0 or >=200 means unlimited)
	RetrievalTopK int // Maximum number of document chunks to retrieve

	// Thinking mode (for providers that support it)
	EnableThinking bool
}

// applyOpenAIModelParams applies optional Temperature/TopP/MaxTokens to an openai.ChatModelConfig.
func applyOpenAIModelParams(cfg *openai.ChatModelConfig, config Config) {
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
}

// CreateChatModel creates a ToolCallingChatModel based on the provider type
func CreateChatModel(ctx context.Context, config Config) (model.ToolCallingChatModel, error) {
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

func createOpenAIChatModel(ctx context.Context, config Config) (model.ToolCallingChatModel, error) {
	cfg := &openai.ChatModelConfig{
		APIKey:  config.Provider.APIKey,
		Model:   config.ModelID,
		BaseURL: config.Provider.APIEndpoint,
	}
	applyOpenAIModelParams(cfg, config)

	// Add thinking mode support via ExtraFields (for providers like Qwen that support enable_thinking)
	if config.EnableThinking {
		if cfg.ExtraFields == nil {
			cfg.ExtraFields = make(map[string]any)
		}
		cfg.ExtraFields["enable_thinking"] = true
	}

	return openai.NewChatModel(ctx, cfg)
}

func createAzureChatModel(ctx context.Context, config Config) (model.ToolCallingChatModel, error) {
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
	applyOpenAIModelParams(cfg, config)

	// Add thinking mode support via ExtraFields (for Azure OpenAI if it supports thinking)
	if config.EnableThinking {
		if cfg.ExtraFields == nil {
			cfg.ExtraFields = make(map[string]any)
		}
		cfg.ExtraFields["enable_thinking"] = true
	}

	return openai.NewChatModel(ctx, cfg)
}

func createClaudeChatModel(ctx context.Context, config Config) (model.ToolCallingChatModel, error) {
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

func createGeminiChatModel(ctx context.Context, config Config) (model.ToolCallingChatModel, error) {
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

func createOllamaChatModel(ctx context.Context, config Config) (model.ToolCallingChatModel, error) {
	cfg := &ollama.ChatModelConfig{
		BaseURL: config.Provider.APIEndpoint,
		Model:   config.ModelID,
	}
	// Note: Ollama config temperature/topP are set via Options, not direct fields

	return ollama.NewChatModel(ctx, cfg)
}

// NewChatModelAgent creates an ADK ChatModelAgent with tools and middlewares.
// extraTools are additional tools that will be added to the agent (e.g., LibraryRetrieverTool).
func NewChatModelAgent(ctx context.Context, config Config, toolRegistry *tools.ToolRegistry, extraTools []tool.BaseTool) (adk.Agent, error) {
	// Create the chat model
	chatModel, err := CreateChatModel(ctx, config)
	if err != nil {
		return nil, err
	}

	// Get enabled tools
	enabledTools, err := toolRegistry.GetAllTools(ctx)
	if err != nil {
		return nil, errs.Wrap("error.chat_tools_failed", err)
	}

	// Replace BrowserUse tool with one configured with ExtractChatModel
	baseTools := make([]tool.BaseTool, 0, len(enabledTools)+len(extraTools))
	for _, t := range enabledTools {
		info, _ := t.Info(ctx)
		if info != nil && info.Name == tools.ToolIDBrowserUse {
			// Create BrowserUse tool with ExtractChatModel for intelligent content extraction
			browserTool, err := tools.NewBrowserUseTool(ctx, &tools.BrowserUseConfig{
				ExtractChatModel: chatModel,
			})
			if err != nil {
				log.Printf("[agent] failed to create BrowserUse with ExtractChatModel, using default: %v", err)
				baseTools = append(baseTools, t)
			} else {
				baseTools = append(baseTools, browserTool)
			}
		} else {
			baseTools = append(baseTools, t)
		}
	}

	// Add extra tools (e.g., LibraryRetrieverTool)
	baseTools = append(baseTools, extraTools...)

	agentConfig := &adk.ChatModelAgentConfig{
		Name:          config.Name,
		Description:   "AI Assistant",
		Instruction:   config.Instruction,
		Model:         chatModel,
		MaxIterations: UnlimitedIterations,
	}

	// Add tools if any
	if len(baseTools) > 0 {
		agentConfig.ToolsConfig = adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: baseTools,
				// Add error-catching middleware to continue ReAct loop on tool errors
				ToolCallMiddlewares: []compose.ToolMiddleware{
					ErrorCatchingToolMiddleware(),
				},
			},
		}
	}

	// Build middlewares
	agentConfig.Middlewares = BuildMiddlewares(ctx)

	return adk.NewChatModelAgent(ctx, agentConfig)
}

// BuildMiddlewares creates and returns the agent middleware stack:
//   - filesystem: provides file operation tools (ls, read_file, write_file, edit_file, glob, grep)
//   - reduction:  clears old tool results and offloads large results to the filesystem backend
//   - skill:      provides a skill tool for on-demand skill loading from SKILL.md files
func BuildMiddlewares(ctx context.Context) []adk.AgentMiddleware {
	var middlewares []adk.AgentMiddleware

	// Create local filesystem backend (rooted at user's home directory)
	fsBackend, err := filesystem.NewLocalBackend(&filesystem.LocalBackendConfig{})
	if err != nil {
		log.Printf("[agent] failed to create local filesystem backend: %v", err)
		// Fall back to only using reduction clearing without filesystem offloading
		reductionMw, err := reduction.NewToolResultMiddleware(ctx, &reduction.ToolResultConfig{
			Backend: nil, // No backend means clearing only, no offloading
		})
		if err == nil {
			middlewares = append(middlewares, reductionMw)
		}
		if skillMw, ok := buildSkillMiddleware(ctx); ok {
			middlewares = append(middlewares, skillMw)
		}
		return middlewares
	}

	// 1. Filesystem middleware — provides file tools using the local filesystem backend
	filesystemMw, err := fsmw.NewMiddleware(ctx, &fsmw.Config{
		Backend:                          fsBackend,
		WithoutLargeToolResultOffloading: true, // Handled by reduction middleware
	})
	if err != nil {
		log.Printf("[agent] failed to create filesystem middleware: %v", err)
	} else {
		middlewares = append(middlewares, filesystemMw)
	}

	// 2. Reduction middleware — clearing old tool results + offloading large individual results
	reductionMw, err := reduction.NewToolResultMiddleware(ctx, &reduction.ToolResultConfig{
		Backend: fsBackend,
	})
	if err != nil {
		log.Printf("[agent] failed to create reduction middleware: %v", err)
	} else {
		middlewares = append(middlewares, reductionMw)
	}

	// 3. Skill middleware — loads skills from local SKILL.md files
	if skillMw, ok := buildSkillMiddleware(ctx); ok {
		middlewares = append(middlewares, skillMw)
	}

	return middlewares
}

// buildSkillMiddleware creates the skill middleware with a local backend.
// Skills are stored under <UserConfigDir>/.agents/skills/<skill-name>/SKILL.md.
// Using "claude" as base directory for broader compatibility with Claude-style skills.
func buildSkillMiddleware(ctx context.Context) (adk.AgentMiddleware, bool) {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		log.Printf("[agent] failed to get user config dir for skills: %v", err)
		return adk.AgentMiddleware{}, false
	}

	// Use "claude" instead of app-specific directory for broader skill compatibility
	skillsDir := filepath.Join(cfgDir, ".agents", "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		log.Printf("[agent] failed to create skills directory %s: %v", skillsDir, err)
		return adk.AgentMiddleware{}, false
	}

	skillBackend, err := skill.NewLocalBackend(&skill.LocalBackendConfig{
		BaseDir: skillsDir,
	})
	if err != nil {
		log.Printf("[agent] failed to create skill backend: %v", err)
		return adk.AgentMiddleware{}, false
	}

	skillMw, err := skill.New(ctx, &skill.Config{
		Backend:    skillBackend,
		UseChinese: true,
	})
	if err != nil {
		log.Printf("[agent] failed to create skill middleware: %v", err)
		return adk.AgentMiddleware{}, false
	}

	return skillMw, true
}

// ErrorCatchingToolMiddleware creates a middleware that catches tool execution errors
// and returns the error message as a tool result, allowing the ReAct loop to continue.
// This enables the LLM to understand what went wrong and try alternative approaches.
func ErrorCatchingToolMiddleware() compose.ToolMiddleware {
	return compose.ToolMiddleware{
		Invokable: func(next compose.InvokableToolEndpoint) compose.InvokableToolEndpoint {
			return func(ctx context.Context, input *compose.ToolInput) (*compose.ToolOutput, error) {
				output, err := next(ctx, input)
				if err != nil {
					// Log the error for debugging
					log.Printf("[agent] tool %q execution error (will continue ReAct loop): %v", input.Name, err)
					// Return the error message as a tool result instead of failing
					return &compose.ToolOutput{
						Result: "Error: " + err.Error(),
					}, nil
				}
				return output, nil
			}
		},
		Streamable: func(next compose.StreamableToolEndpoint) compose.StreamableToolEndpoint {
			return func(ctx context.Context, input *compose.ToolInput) (*compose.StreamToolOutput, error) {
				output, err := next(ctx, input)
				if err != nil {
					// Log the error for debugging
					log.Printf("[agent] streaming tool %q execution error (will continue ReAct loop): %v", input.Name, err)
					// Return the error message as a streaming result
					errorMsg := "Error: " + err.Error()
					return &compose.StreamToolOutput{
						Result: schema.StreamReaderFromArray([]string{errorMsg}),
					}, nil
				}
				return output, nil
			}
		},
	}
}
