package chat

import (
	"context"
	"encoding/json"
	"log"
	"math"
	"os"
	"path/filepath"

	"willchat/internal/errs"
	"willchat/internal/services/tools"

	"github.com/cloudwego/eino-ext/components/model/claude"
	einogemini "github.com/cloudwego/eino-ext/components/model/gemini"
	"github.com/cloudwego/eino-ext/components/model/ollama"
	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/adk"
	fsmw "github.com/cloudwego/eino/adk/middlewares/filesystem"
	"github.com/cloudwego/eino/adk/middlewares/reduction"
	"github.com/cloudwego/eino/adk/middlewares/skill"

	localfs "willchat/internal/services/filesystem"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"google.golang.org/genai"
)

// unlimitedIterations effectively removes the ReAct loop iteration limit.
// The eino ADK defaults to 20 when MaxIterations <= 0, so we use math.MaxInt32 instead.
var unlimitedIterations = math.MaxInt32

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
	Temperature     *float64
	TopP            *float64
	MaxTokens       *int
	EnableTemp      bool
	EnableTopP      bool
	EnableMaxTokens bool
}

// applyOpenAIModelParams applies optional Temperature/TopP/MaxTokens to an openai.ChatModelConfig.
func applyOpenAIModelParams(cfg *openai.ChatModelConfig, config AgentConfig) {
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
	applyOpenAIModelParams(cfg, config)

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
	applyOpenAIModelParams(cfg, config)

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

// toolCallingInstruction is appended to the system prompt to improve tool call reliability.
// Bilingual (Chinese + English) to cover a wider range of models.
const toolCallingInstruction = "\n\n" +
	"Tool calling rules (VERY IMPORTANT): When calling any tool, `tool_calls[].function.arguments` " +
	"MUST be a strictly valid JSON object, e.g. {\"query\":\"...\"} or {\"expression\":\"1+2\"}. " +
	"Do NOT output key=value, plain text, or unquoted fields.\n" +
	"工具调用规则（非常重要）：当你调用任何工具时，必须让 tool_calls[].function.arguments 是严格合法的 JSON（对象），" +
	"例如：{\"query\":\"...\"} 或 {\"expression\":\"1+2\"}。不要输出 key=value、不要输出纯文本、不要输出不带引号的字段。\n"

// createChatModelAgent creates an ADK ChatModelAgent with tools and middlewares.
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

	// Replace BrowserUse tool with one configured with ExtractChatModel
	baseTools := make([]tool.BaseTool, 0, len(enabledTools))
	for _, t := range enabledTools {
		info, _ := t.Info(ctx)
		if info != nil && info.Name == tools.ToolIDBrowserUse {
			// Create BrowserUse tool with ExtractChatModel for intelligent content extraction
			browserTool, err := tools.NewBrowserUseTool(ctx, &tools.BrowserUseConfig{
				ExtractChatModel: chatModel,
			})
			if err != nil {
				log.Printf("[chat] failed to create BrowserUse with ExtractChatModel, using default: %v", err)
				baseTools = append(baseTools, t)
			} else {
				baseTools = append(baseTools, browserTool)
			}
		} else {
			baseTools = append(baseTools, t)
		}
	}

	// Build instruction with tool calling reliability note
	instruction := config.Instruction + toolCallingInstruction

	agentConfig := &adk.ChatModelAgentConfig{
		Name:          config.Name,
		Description:   "AI Assistant",
		Instruction:   instruction,
		Model:         chatModel,
		MaxIterations: unlimitedIterations,
	}

	// Add tools if any
	if len(baseTools) > 0 {
		agentConfig.ToolsConfig = adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: baseTools,
				// Add error-catching middleware to continue ReAct loop on tool errors
				ToolCallMiddlewares: []compose.ToolMiddleware{
					errorCatchingToolMiddleware(),
				},
			},
		}
	}

	// Build middlewares
	agentConfig.Middlewares = buildMiddlewares(ctx)

	return adk.NewChatModelAgent(ctx, agentConfig)
}

// buildMiddlewares creates and returns the agent middleware stack:
//   - filesystem: provides file operation tools (ls, read_file, write_file, edit_file, glob, grep)
//   - reduction:  clears old tool results and offloads large results to the filesystem backend
//   - skill:      provides a skill tool for on-demand skill loading from SKILL.md files
func buildMiddlewares(ctx context.Context) []adk.AgentMiddleware {
	var middlewares []adk.AgentMiddleware

	// Create local filesystem backend (rooted at user's home directory)
	fsBackend, err := localfs.NewLocalBackend(&localfs.LocalBackendConfig{})
	if err != nil {
		log.Printf("[chat] failed to create local filesystem backend: %v", err)
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
		log.Printf("[chat] failed to create filesystem middleware: %v", err)
	} else {
		middlewares = append(middlewares, filesystemMw)
	}

	// 2. Reduction middleware — clearing old tool results + offloading large individual results
	reductionMw, err := reduction.NewToolResultMiddleware(ctx, &reduction.ToolResultConfig{
		Backend: fsBackend,
	})
	if err != nil {
		log.Printf("[chat] failed to create reduction middleware: %v", err)
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
// Skills are stored under <UserConfigDir>/claude/skills/<skill-name>/SKILL.md.
// Using "claude" as base directory for broader compatibility with Claude-style skills.
func buildSkillMiddleware(ctx context.Context) (adk.AgentMiddleware, bool) {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		log.Printf("[chat] failed to get user config dir for skills: %v", err)
		return adk.AgentMiddleware{}, false
	}

	// Use "claude" instead of app-specific directory for broader skill compatibility
	skillsDir := filepath.Join(cfgDir, "claude", "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		log.Printf("[chat] failed to create skills directory %s: %v", skillsDir, err)
		return adk.AgentMiddleware{}, false
	}

	skillBackend, err := skill.NewLocalBackend(&skill.LocalBackendConfig{
		BaseDir: skillsDir,
	})
	if err != nil {
		log.Printf("[chat] failed to create skill backend: %v", err)
		return adk.AgentMiddleware{}, false
	}

	skillMw, err := skill.New(ctx, &skill.Config{
		Backend:    skillBackend,
		UseChinese: true,
	})
	if err != nil {
		log.Printf("[chat] failed to create skill middleware: %v", err)
		return adk.AgentMiddleware{}, false
	}

	return skillMw, true
}

// errorCatchingToolMiddleware creates a middleware that catches tool execution errors
// and returns the error message as a tool result, allowing the ReAct loop to continue.
// This enables the LLM to understand what went wrong and try alternative approaches.
func errorCatchingToolMiddleware() compose.ToolMiddleware {
	return compose.ToolMiddleware{
		Invokable: func(next compose.InvokableToolEndpoint) compose.InvokableToolEndpoint {
			return func(ctx context.Context, input *compose.ToolInput) (*compose.ToolOutput, error) {
				output, err := next(ctx, input)
				if err != nil {
					// Log the error for debugging
					log.Printf("[chat] tool %q execution error (will continue ReAct loop): %v", input.Name, err)
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
					log.Printf("[chat] streaming tool %q execution error (will continue ReAct loop): %v", input.Name, err)
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
