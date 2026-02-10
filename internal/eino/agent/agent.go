// Package agent provides utilities for creating eino ADK agents with tools and middlewares.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"runtime"

	"willclaw/internal/eino/filesystem"
	"willclaw/internal/eino/tools"
	"willclaw/internal/errs"

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

// UnlimitedIterations removes the ReAct loop iteration limit (eino defaults to 20).
var UnlimitedIterations = math.MaxInt32

// ProviderConfig contains the configuration for a provider.
type ProviderConfig struct {
	ProviderID  string
	Type        string // openai, azure, anthropic, gemini, ollama
	APIKey      string
	APIEndpoint string
	ExtraConfig string
}

// Config contains the configuration for creating an agent.
type Config struct {
	Name        string
	Instruction string
	ModelID     string
	Provider    ProviderConfig

	Temperature     *float64
	TopP            *float64
	MaxTokens       *int
	EnableTemp      bool
	EnableTopP      bool
	EnableMaxTokens bool

	ContextCount   int  // Max messages in context (0 or >=200 = unlimited)
	RetrievalTopK  int  // Max document chunks to retrieve
	EnableThinking bool // Thinking mode (for providers that support it)
}

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

// CreateChatModel creates a ToolCallingChatModel based on the provider type.
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
		cfg.MaxTokens = 4096
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

	return einogemini.NewChatModel(ctx, cfg)
}

func createOllamaChatModel(ctx context.Context, config Config) (model.ToolCallingChatModel, error) {
	cfg := &ollama.ChatModelConfig{
		BaseURL: config.Provider.APIEndpoint,
		Model:   config.ModelID,
	}
	return ollama.NewChatModel(ctx, cfg)
}

// BeforeChatModelFunc is called before each LLM invocation with the complete
// message list (system prompt + history + user message) that will be sent.
// This is useful for logging the full prompt context.
type BeforeChatModelFunc func(ctx context.Context, messages []*schema.Message)

// AgentResult holds the created agent and a cleanup function that should be
// called (typically via defer) when the agent is no longer needed. Cleanup
// releases per-session resources such as headless Chrome processes.
type AgentResult struct {
	Agent   adk.Agent
	Cleanup func()
}

// NewChatModelAgent creates an ADK ChatModelAgent with tools and middlewares.
// Each call creates its own browserTool instance so that concurrent conversations
// (tabs) do not share or interfere with each other's browser sessions.
// The caller MUST call result.Cleanup() when the agent is no longer needed.
//
// beforeChatModel, if non-nil, is called before every LLM invocation with
// the complete message list that will be sent to the model, including the
// system instruction, middleware additions, and all tool schemas.
func NewChatModelAgent(ctx context.Context, config Config, toolRegistry *tools.ToolRegistry, extraTools []tool.BaseTool, beforeChatModel BeforeChatModelFunc) (*AgentResult, error) {
	chatModel, err := CreateChatModel(ctx, config)
	if err != nil {
		return nil, err
	}

	// Create a per-session browserTool. It is lazily initialized (Chrome only
	// starts if the LLM actually calls the tool), so the cost of creating one
	// per conversation is negligible.
	browserTool, err := tools.NewBrowserTool(ctx, &tools.BrowserConfig{
		Headless:         true,
		ExtractChatModel: chatModel,
	})
	if err != nil {
		return nil, errs.Wrap("error.chat_browser_tool_failed", err)
	}

	// Get shared tools from the registry, excluding browserTool (it's per-session).
	enabledTools, err := toolRegistry.GetEnabledToolsExcluding(ctx, nil, tools.ToolIDBrowserUse)
	if err != nil {
		return nil, errs.Wrap("error.chat_tools_failed", err)
	}

	baseTools := make([]tool.BaseTool, 0, len(enabledTools)+len(extraTools)+1)
	baseTools = append(baseTools, enabledTools...)
	baseTools = append(baseTools, browserTool)
	baseTools = append(baseTools, extraTools...)

	agentConfig := &adk.ChatModelAgentConfig{
		Name:          config.Name,
		Description:   "AI Assistant",
		Instruction:   config.Instruction,
		Model:         chatModel,
		MaxIterations: UnlimitedIterations,
	}

	if len(baseTools) > 0 {
		agentConfig.ToolsConfig = adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools:               baseTools,
				ToolCallMiddlewares: []compose.ToolMiddleware{ErrorCatchingToolMiddleware()},
			},
		}
	}

	agentConfig.Middlewares = BuildMiddlewares(ctx)

	// Append a logging middleware that fires before each LLM call.
	if beforeChatModel != nil {
		agentConfig.Middlewares = append(agentConfig.Middlewares, adk.AgentMiddleware{
			BeforeChatModel: func(ctx context.Context, state *adk.ChatModelAgentState) error {
				beforeChatModel(ctx, state.Messages)
				return nil
			},
		})
	}

	agent, err := adk.NewChatModelAgent(ctx, agentConfig)
	if err != nil {
		browserTool.Close()
		return nil, err
	}

	return &AgentResult{
		Agent: agent,
		Cleanup: func() {
			browserTool.Close()
		},
	}, nil
}

// buildFilesystemSystemPrompt generates a system prompt that tells the LLM about
// the OS environment, home directory, and available filesystem/execute tools.
func buildFilesystemSystemPrompt(baseDir string) string {
	osName := runtime.GOOS
	shell := "/bin/bash"
	switch osName {
	case "windows":
		shell = "powershell"
	case "darwin":
		shell = "/bin/zsh"
	}

	prompt := fmt.Sprintf(`
# Filesystem & Execute Tools — Environment Info

- Operating System: %s
- Shell: %s
- Home directory: %s
- All tools use real OS absolute paths. For example: ls(path="%s"), write_file(file_path="%s/foo.txt").
- The execute tool runs commands with home directory as working directory.
- When the user mentions "user directory" or "home directory", it refers to: %s

# Filesystem Tools

- ls: list files in a directory (use absolute path, e.g. "%s")
- read_file: read a file from the filesystem
- write_file: write/create a file (prefer this over shell echo for creating files with code)
- edit_file: edit a file in the filesystem (string replacement based)
- patch_file: apply line-based patch operations (insert/delete/replace by line numbers). More precise than edit_file for multi-line changes.
- glob: find files matching a pattern (e.g., "%s/**/*.py")
- grep: search for text within files (supports regex, context lines, case-insensitive, output modes)

# Execute Tool

- Working directory: %s
- Returns combined stdout/stderr output with exit code
- Timeout: 60 seconds per command. Commands exceeding this limit are killed automatically.
- **NEVER run long-running or persistent commands** (e.g. "php artisan serve", "npm run dev", "python manage.py runserver", "docker compose up", "tail -f", "watch"). These will block and timeout. If the user needs to start a server, instruct them to run it manually in a separate terminal.
- For build commands that may take long, keep them focused (e.g. "npm run build" is fine, but avoid running dev servers).
- Avoid using cat/head/tail (use read_file), find (use glob), grep command (use grep tool)
`, osName, shell, baseDir, baseDir, baseDir, baseDir, baseDir, baseDir, baseDir)

	if osName == "windows" {
		prompt += `
# PowerShell Notes

- Use semicolons to chain commands: "cd C:\path; command" (NOT "&&" which requires PowerShell 7+)
- Run executables in current directory with ".\" prefix: ".\app.exe" (NOT "app.exe")
- The working directory resets for each execute call — always use "cd targetDir; command" when running commands in a specific directory
`
	}

	return prompt
}

// BuildMiddlewares creates the agent middleware stack:
//   - filesystem: file tools (ls, read_file, write_file, edit_file, glob, grep, execute)
//   - reduction: clears old tool results + offloads large results to filesystem
//   - skill: on-demand skill loading from SKILL.md files
func BuildMiddlewares(ctx context.Context) []adk.AgentMiddleware {
	var middlewares []adk.AgentMiddleware

	fsBackend, err := filesystem.NewLocalBackend(&filesystem.LocalBackendConfig{
		ShellPolicy: &filesystem.ShellPolicy{
			BlockedCommands: []string{
				"rm -rf /", "rm -rf /*", "mkfs", "dd if=",
				":(){:|:&};:", "format c:", "format d:",
			},
		},
	})
	if err != nil {
		log.Printf("[agent] failed to create local filesystem backend: %v", err)
		reductionMw, err := reduction.NewToolResultMiddleware(ctx, &reduction.ToolResultConfig{Backend: nil})
		if err == nil {
			middlewares = append(middlewares, reductionMw)
		}
		if skillMw, ok := buildSkillMiddleware(ctx); ok {
			middlewares = append(middlewares, skillMw)
		}
		return middlewares
	}

	customSystemPrompt := buildFilesystemSystemPrompt(fsBackend.BaseDir())

	filesystemMw, err := fsmw.NewMiddleware(ctx, &fsmw.Config{
		Backend:                          fsBackend,
		WithoutLargeToolResultOffloading: true,
		CustomSystemPrompt:               &customSystemPrompt,
	})
	if err != nil {
		log.Printf("[agent] failed to create filesystem middleware: %v", err)
	} else {
		// Replace the built-in grep tool with our enhanced version and add patch_file.
		grepTool, grepErr := filesystem.NewGrepTool(fsBackend)
		if grepErr != nil {
			log.Printf("[agent] failed to create grep tool: %v", grepErr)
		} else {
			// Remove the built-in grep tool (same name) so ours takes its place.
			filtered := make([]tool.BaseTool, 0, len(filesystemMw.AdditionalTools))
			for _, t := range filesystemMw.AdditionalTools {
				info, infoErr := t.Info(ctx)
				if infoErr != nil || info.Name != filesystem.GrepToolID {
					filtered = append(filtered, t)
				}
			}
			filesystemMw.AdditionalTools = append(filtered, grepTool)
		}

		patchTool, patchErr := filesystem.NewPatchTool(fsBackend)
		if patchErr != nil {
			log.Printf("[agent] failed to create patch_file tool: %v", patchErr)
		} else {
			filesystemMw.AdditionalTools = append(filesystemMw.AdditionalTools, patchTool)
		}

		middlewares = append(middlewares, filesystemMw)
	}

	reductionMw, err := reduction.NewToolResultMiddleware(ctx, &reduction.ToolResultConfig{
		Backend: fsBackend,
	})
	if err != nil {
		log.Printf("[agent] failed to create reduction middleware: %v", err)
	} else {
		middlewares = append(middlewares, reductionMw)
	}

	if skillMw, ok := buildSkillMiddleware(ctx); ok {
		middlewares = append(middlewares, skillMw)
	}

	return middlewares
}

// buildSkillMiddleware creates the skill middleware.
// Skills are stored under $HOME/.agents/skills/<skill-name>/SKILL.md.
func buildSkillMiddleware(ctx context.Context) (adk.AgentMiddleware, bool) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Printf("[agent] failed to get home dir for skills: %v", err)
		return adk.AgentMiddleware{}, false
	}

	skillsDir := filepath.Join(homeDir, ".agents", "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		log.Printf("[agent] failed to create skills directory %s: %v", skillsDir, err)
		return adk.AgentMiddleware{}, false
	}

	skillBackend, err := skill.NewLocalBackend(&skill.LocalBackendConfig{BaseDir: skillsDir})
	if err != nil {
		log.Printf("[agent] failed to create skill backend: %v", err)
		return adk.AgentMiddleware{}, false
	}

	skillMw, err := skill.New(ctx, &skill.Config{Backend: skillBackend, UseChinese: true})
	if err != nil {
		log.Printf("[agent] failed to create skill middleware: %v", err)
		return adk.AgentMiddleware{}, false
	}

	return skillMw, true
}

// ErrorCatchingToolMiddleware catches tool execution errors and returns the error
// message as a tool result, allowing the ReAct loop to continue.
func ErrorCatchingToolMiddleware() compose.ToolMiddleware {
	return compose.ToolMiddleware{
		Invokable: func(next compose.InvokableToolEndpoint) compose.InvokableToolEndpoint {
			return func(ctx context.Context, input *compose.ToolInput) (*compose.ToolOutput, error) {
				output, err := next(ctx, input)
				if err != nil {
					log.Printf("[agent] tool %q error: %v", input.Name, err)
					return &compose.ToolOutput{Result: "Error: " + err.Error()}, nil
				}
				return output, nil
			}
		},
		Streamable: func(next compose.StreamableToolEndpoint) compose.StreamableToolEndpoint {
			return func(ctx context.Context, input *compose.ToolInput) (*compose.StreamToolOutput, error) {
				output, err := next(ctx, input)
				if err != nil {
					log.Printf("[agent] streaming tool %q error: %v", input.Name, err)
					return &compose.StreamToolOutput{
						Result: schema.StreamReaderFromArray([]string{"Error: " + err.Error()}),
					}, nil
				}
				return output, nil
			}
		},
	}
}
