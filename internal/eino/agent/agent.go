// Package agent provides utilities for creating eino ADK agents with tools and middlewares.
package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"

	"chatclaw/internal/eino/tools"
	"chatclaw/internal/errs"

	"github.com/cloudwego/eino-ext/components/model/claude"
	einogemini "github.com/cloudwego/eino-ext/components/model/gemini"
	"github.com/cloudwego/eino-ext/components/model/ollama"
	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/filesystem"
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

	SandboxMode    string // "codex" or "native"
	SandboxNetwork bool   // Allow network access in sandbox
	WorkDir        string // Per-agent working directory (base path; actual = WorkDir/sessions/<agent_hash>/<conv_hash>)

	AgentID        int64 // Agent database ID (used to generate session subdirectory)
	ConversationID int64 // Conversation database ID (used to generate session subdirectory)
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
func NewChatModelAgent(ctx context.Context, config Config, toolRegistry *tools.ToolRegistry, bgMgr *tools.BgProcessManager, extraTools []tool.BaseTool, extraMiddlewares []adk.AgentMiddleware, beforeChatModel BeforeChatModelFunc) (*AgentResult, error) {
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

	// Shared InMemoryBackend for reduction offloading — filesystem tools
	// dispatch to it when they see virtual paths (/large_tool_result/*).
	memBackend := filesystem.NewInMemoryBackend()

	// Build independent filesystem tools (ls, read_file, write_file, etc.).
	fsTools := BuildFsTools(config, memBackend, bgMgr)

	baseTools := make([]tool.BaseTool, 0, len(enabledTools)+len(fsTools)+len(extraTools)+1)
	baseTools = append(baseTools, enabledTools...)
	baseTools = append(baseTools, browserTool)
	baseTools = append(baseTools, fsTools...)
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

	agentConfig.Middlewares = BuildMiddlewares(ctx, config, memBackend)
	agentConfig.Middlewares = append(agentConfig.Middlewares, extraMiddlewares...)

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
// the OS environment, working directory, sandbox constraints, and available tools.
// sessionsDir is the agent-level sessions directory (parent of the current conversation dir)
// so the LLM knows where sibling conversations live.
func buildFilesystemSystemPrompt(homeDir, workDir, sessionsDir string, sandboxEnabled, sandboxNetworkEnabled bool) string {
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
- **Working directory: %s**
- All tools use real OS absolute paths.
- When the user mentions "working directory" or asks to write/create files, **always use the working directory** as the base path. For example: write_file(file_path="%s/foo.txt"), ls(path="%s").
- When the user mentions "user directory" or "home directory", it refers to: %s
`, osName, shell, homeDir, workDir, workDir, workDir, homeDir)

	if sessionsDir != "" {
		prompt += fmt.Sprintf(`
# Session Directory Structure

Each conversation has its own isolated working directory. The current conversation's directory is: %s
The parent directory (%s) contains sibling conversations from the same AI assistant. If the user mentions files or work from a **previous conversation**, you can use ls and read_file to browse sibling directories under "%s" to locate those files. Write operations should still target the current working directory.
`, workDir, sessionsDir, sessionsDir)
	}

	if sandboxEnabled {
		networkDesc := "Network access is **disabled** for executed commands. Commands like curl, npm install, pip install will fail."
		if sandboxNetworkEnabled {
			networkDesc = "Network access is **enabled** for executed commands (e.g. npm install, curl, pip install will work)."
		}
		prompt += fmt.Sprintf(`
# Sandbox Mode

You are running inside an OS-level sandbox. Understand these constraints **before** choosing commands:

## Write Restrictions
- All write operations are restricted to the working directory: %s
- write_file, edit_file, patch_file can only write to paths within the working directory.
- execute runs commands with the working directory as cwd; writing to paths outside it will be denied by the OS.
- read_file, ls, glob, grep can read any path on the filesystem (read is unrestricted).

## Network
- %s

## Best Practices in Sandbox
- **Never use global installs** (e.g. "npm install -g", "pip install --user"). Global paths are outside the working directory and writes will be rejected. Use local/project-level installs instead (e.g. "npm install" in the project directory, "pip install --target .").
- **Use npx / bunx** to run CLI tools without global installs (e.g. "npx create-vue@latest my-app" instead of installing @vue/cli globally).
- **Always pass non-interactive flags** to avoid commands hanging on stdin: use "--yes", "--default", "-y", or pipe "echo" as needed (e.g. "npx create-vue@latest my-app --default", "npm init -y").
- **All project files must be created inside the working directory.** Do not attempt to create files elsewhere.
- If a command fails due to permission denied, it is likely trying to write outside the working directory. Retry with a local/project-scoped alternative.
`, workDir, networkDesc)
	}

	prompt += fmt.Sprintf(`
# Filesystem Tools

- ls: list files in a directory (use absolute path, e.g. "%s")
- read_file: read a file from the filesystem
- write_file: write/create a file (prefer this over shell echo for creating files with code). **Default to the working directory: %s**
- edit_file: edit a file in the filesystem (string replacement based)
- patch_file: apply line-based patch operations (insert/delete/replace by line numbers). More precise than edit_file for multi-line changes.
- glob: find files matching a pattern (e.g., "%s/**/*.py")
- grep: search for text within files (supports regex, context lines, case-insensitive, output modes)

# Execute Tool (synchronous)

- **action="run"** (default): execute a shell command synchronously in the working directory (%s).
  - Returns combined stdout/stderr output with exit code.
  - Default timeout: 60s. Set `+"`timeout`"+` (max 300s) for slow commands (e.g. npm install).
  - **For short-lived commands only**: build, test, install, compile, lint, etc.
  - **Do NOT use for dev servers or long-running processes** — use execute_background to start them.
  - Avoid using cat/head/tail (use read_file), find (use glob), grep command (use grep tool).
- **action="stop"**: synchronously kill a background process by pid and wait for it to fully exit. Returns success or error if the process cannot be killed within the timeout.
  - Pass `+"`pid`"+` (from execute_background) and optional `+"`timeout`"+` (default 10s, max 300s).
  - If the process does not exit in time, an error is returned so you can take further action.
- **action="status"**: check if a background process is still alive and read its latest output.
  - Pass `+"`pid`"+` (from execute_background).

# Execute Background Tool (asynchronous, start only)

- Use **only** for **starting** long-running processes like dev servers ("npm run dev", "python manage.py runserver", etc.).
- Returns pid and initial output. The process is auto-killed after timeout (default 300s, max 600s).
- After starting a dev server, use execute with action="status" to confirm it's running and check its output.
- To stop a background process, use execute with action="stop" (do NOT use execute_background for stopping).
`, workDir, workDir, workDir, workDir)

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
//   - system prompt: environment info + tool usage instructions (via AdditionalInstruction)
//   - reduction: clears old tool results + offloads large results to InMemoryBackend
//   - skill: on-demand skill loading from SKILL.md files
//
// Filesystem tools (ls, read_file, write_file, edit_file, patch_file, glob, grep, execute)
// are registered as independent tools via BuildFsTools, not through the filesystem middleware.
func BuildMiddlewares(ctx context.Context, config Config, memBackend *filesystem.InMemoryBackend) []adk.AgentMiddleware {
	var middlewares []adk.AgentMiddleware

	fsCfg := buildFsToolsConfig(config, memBackend)

	// Compute the agent-level sessions directory (parent of the conversation dir)
	// so the LLM can browse sibling conversations when needed.
	var sessionsDir string
	if config.AgentID > 0 {
		base := config.WorkDir
		if base == "" {
			base = filepath.Join(fsCfg.HomeDir, ".chatclaw")
		}
		sessionsDir = filepath.Join(base, "sessions", idHash(config.AgentID))
	}

	// System prompt middleware — inject environment info and tool instructions.
	systemPrompt := buildFilesystemSystemPrompt(fsCfg.HomeDir, fsCfg.WorkDir, sessionsDir, fsCfg.SandboxEnabled, fsCfg.SandboxNetworkEnabled)
	middlewares = append(middlewares, adk.AgentMiddleware{
		AdditionalInstruction: systemPrompt,
	})

	// Reduction middleware — uses InMemoryBackend for large result offloading.
	reductionMw, err := reduction.NewToolResultMiddleware(ctx, &reduction.ToolResultConfig{
		Backend: memBackend,
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

// BuildFsTools creates all filesystem tools as independent tool instances.
// These are registered alongside other tools in the agent config, decoupled
// from the filesystem middleware.
// bgMgr is the shared BgProcessManager from ChatService — background processes
// survive across individual generation rounds.
func BuildFsTools(config Config, memBackend *filesystem.InMemoryBackend, bgMgr *tools.BgProcessManager) []tool.BaseTool {
	cfg := buildFsToolsConfig(config, memBackend)

	var fsTools []tool.BaseTool
	builders := []func(*tools.FsToolsConfig) (tool.BaseTool, error){
		tools.NewLsTool,
		tools.NewReadFileTool,
		tools.NewWriteFileTool,
		tools.NewEditFileTool,
		tools.NewPatchFileTool,
		tools.NewGlobTool,
		tools.NewGrepTool,
	}
	for _, build := range builders {
		t, err := build(cfg)
		if err != nil {
			log.Printf("[agent] failed to create fs tool: %v", err)
			continue
		}
		fsTools = append(fsTools, t)
	}

	execTool, err := tools.NewExecuteTool(cfg, bgMgr)
	if err != nil {
		log.Printf("[agent] failed to create execute tool: %v", err)
	} else {
		fsTools = append(fsTools, execTool)
	}

	bgTool, err := tools.NewBgExecuteTool(cfg, bgMgr)
	if err != nil {
		log.Printf("[agent] failed to create execute_background tool: %v", err)
	} else {
		fsTools = append(fsTools, bgTool)
	}

	return fsTools
}

// idHash returns a short (12-char) hex hash derived from a numeric ID.
// This avoids exposing raw database primary keys in filesystem paths.
func idHash(id int64) string {
	h := sha256.Sum256([]byte("chatclaw:" + strconv.FormatInt(id, 10)))
	return hex.EncodeToString(h[:6])
}

// buildSessionWorkDir constructs the per-conversation working directory under
// the base work dir: <baseWorkDir>/sessions/<agentHash>/<convHash>.
// When agentID or conversationID is 0 the corresponding level is omitted.
func buildSessionWorkDir(baseWorkDir string, agentID, conversationID int64) string {
	dir := filepath.Join(baseWorkDir, "sessions")
	if agentID > 0 {
		dir = filepath.Join(dir, idHash(agentID))
	}
	if conversationID > 0 {
		dir = filepath.Join(dir, idHash(conversationID))
	}
	return dir
}

// buildFsToolsConfig constructs the shared config for all filesystem tools
// using per-agent workspace settings from Config.
func buildFsToolsConfig(config Config, memBackend *filesystem.InMemoryBackend) *tools.FsToolsConfig {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Printf("[agent] failed to get user home dir: %v", err)
		homeDir = "/"
	}

	sandboxEnabled := config.SandboxMode == "codex"
	var codexBin string
	if sandboxEnabled {
		codexBin = resolveCodexBin()
	}

	baseWorkDir := config.WorkDir
	if baseWorkDir == "" {
		baseWorkDir = filepath.Join(homeDir, ".chatclaw")
	}

	workDir := buildSessionWorkDir(baseWorkDir, config.AgentID, config.ConversationID)
	_ = os.MkdirAll(workDir, 0o755)

	return &tools.FsToolsConfig{
		HomeDir:               homeDir,
		WorkDir:               workDir,
		SandboxEnabled:        sandboxEnabled,
		SandboxNetworkEnabled: config.SandboxNetwork,
		CodexBin:              codexBin,
		MemBackend:            memBackend,
	}
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

// resolveCodexBin finds the codex binary in the toolchain bin directory.
func resolveCodexBin() string {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	binName := "codex"
	if runtime.GOOS == "windows" {
		binName = "codex.exe"
	}
	candidate := filepath.Join(cfgDir, "chatclaw", "bin", binName)
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	return ""
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
