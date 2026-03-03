// Package agent provides utilities for creating eino ADK agents with tools and middlewares.
package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"chatclaw/internal/eino/tools"
	"chatclaw/internal/errs"
	"chatclaw/internal/services/toolchain"

	"github.com/cloudwego/eino-ext/components/model/claude"
	einogemini "github.com/cloudwego/eino-ext/components/model/gemini"
	"github.com/cloudwego/eino-ext/components/model/ollama"
	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/filesystem"
	"github.com/cloudwego/eino/adk/middlewares/patchtoolcalls"
	"github.com/cloudwego/eino/adk/middlewares/reduction"
	"github.com/cloudwego/eino/adk/middlewares/summarization"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"google.golang.org/genai"
	"gopkg.in/yaml.v3"
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

	ToolchainBinDir string // Directory containing managed tool binaries (uv, bun, etc.)
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
	if config.EnableThinking {
		cfg.Thinking = &claude.Thinking{
			Enable:       true,
			BudgetTokens: cfg.MaxTokens,
		}
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
	if config.EnableThinking {
		cfg.ThinkingConfig = &genai.ThinkingConfig{
			IncludeThoughts: true,
		}
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
// logger is used for structured logging throughout the agent lifecycle.
func NewChatModelAgent(ctx context.Context, config Config, toolRegistry *tools.ToolRegistry, bgMgr *tools.BgProcessManager, extraTools []tool.BaseTool, extraMiddlewares []adk.AgentMiddleware, logger *slog.Logger) (*AgentResult, error) {
	chatModel, err := CreateChatModel(ctx, config)
	if err != nil {
		return nil, err
	}

	browserTool, err := tools.NewBrowserTool(ctx, &tools.BrowserConfig{
		Headless:         true,
		ExtractChatModel: chatModel,
	})
	if err != nil {
		return nil, errs.Wrap("error.chat_browser_tool_failed", err)
	}

	enabledTools, err := toolRegistry.GetEnabledToolsExcluding(ctx, nil, tools.ToolIDBrowserUse)
	if err != nil {
		browserTool.Close()
		return nil, errs.Wrap("error.chat_tools_failed", err)
	}

	fsTools := BuildFsTools(config, bgMgr, logger)

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
				ToolCallMiddlewares: []compose.ToolMiddleware{ErrorCatchingToolMiddleware(logger)},
			},
		}
	}

	agentConfig.Middlewares = buildSimpleMiddlewares(config, logger)
	agentConfig.Middlewares = append(agentConfig.Middlewares, extraMiddlewares...)
	agentConfig.Handlers = buildHandlers(ctx, config, chatModel, logger)

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
func buildFilesystemSystemPrompt(homeDir, workDir, sessionsDir, toolchainBinDir string, sandboxEnabled, sandboxNetworkEnabled bool) string {
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

	if toolchainBinDir != "" {
		installed := toolchain.InstalledSnapshot()
		var toolSections string

		if installed["uv"] {
			toolSections += `
## uv — Fast Python Package Manager & Runner
- **Always prefer uv over system python/pip/pip3/python3.** Even if the user has Python installed, use uv for better reproducibility and speed.
- Create a new Python project: ` + "`uv init my-project`" + `
- Run a Python script (auto-installs dependencies): ` + "`uv run script.py`" + `
- Add a dependency: ` + "`uv add requests`" + `
- Create a virtual environment: ` + "`uv venv`" + `
- Install from requirements.txt: ` + "`uv pip install -r requirements.txt`" + `
`
		}

		if installed["bun"] {
			toolSections += `
## bun — Fast JavaScript Runtime & Package Manager
- **Always prefer bun over system node/npm/npx.** Even if the user has Node.js installed, use bun for faster execution and installs.
- Initialize a project: ` + "`bun init`" + `
- Install dependencies: ` + "`bun install`" + `
- Run a script: ` + "`bun run script.ts`" + ` (supports TypeScript natively)
- Add a dependency: ` + "`bun add express`" + `
- Execute a package binary: ` + "`bunx create-vite my-app`" + `
`
		}

		if toolSections != "" {
			prompt += fmt.Sprintf(`
# Pre-installed Development Tools

The following tools are **pre-installed and already on PATH** (in %s). You can call them directly by name.
%s
## Important
- These tools are managed by the application and guaranteed to be available. Do NOT ask the user to install Python, Node.js, pip, or npm — use uv and bun instead.
- If a task requires Python work, default to uv. If it requires JavaScript/TypeScript work, default to bun.
`, toolchainBinDir, toolSections)
		}
	}

	return prompt
}

// buildSimpleMiddlewares creates lightweight AgentMiddleware items: system prompt injection.
func buildSimpleMiddlewares(config Config, logger *slog.Logger) []adk.AgentMiddleware {
	var middlewares []adk.AgentMiddleware

	fsCfg := buildFsToolsConfig(config, logger)

	var sessionsDir string
	if config.AgentID > 0 {
		base := config.WorkDir
		if base == "" {
			base = filepath.Join(fsCfg.HomeDir, ".chatclaw")
		}
		sessionsDir = filepath.Join(base, "sessions", idHash(config.AgentID))
	}

	systemPrompt := buildFilesystemSystemPrompt(fsCfg.HomeDir, fsCfg.WorkDir, sessionsDir, fsCfg.ToolchainBinDir, fsCfg.SandboxEnabled, fsCfg.SandboxNetworkEnabled)
	middlewares = append(middlewares, adk.AgentMiddleware{
		AdditionalInstruction: systemPrompt,
	})

	return middlewares
}

// buildHandlers creates ChatModelAgentMiddleware handlers:
//   - patchtoolcalls: patches dangling tool calls in message history
//   - reduction: two-phase tool result compression (truncation + clearing)
//   - summarization: automatic conversation history summarization
//   - skill: on-demand skill loading from SKILL.md files
func buildHandlers(ctx context.Context, config Config, chatModel model.BaseChatModel, logger *slog.Logger) []adk.ChatModelAgentMiddleware {
	var handlers []adk.ChatModelAgentMiddleware

	if h := buildPatchToolCallsHandler(ctx, logger); h != nil {
		handlers = append(handlers, h)
	}

	reductionDir := reductionRootDir(config, logger)
	if h := buildReductionHandler(ctx, reductionDir, logger); h != nil {
		handlers = append(handlers, h)
	}

	transcriptPath := transcriptFilePath(config, logger)
	if h := buildSummarizationHandler(ctx, chatModel, transcriptPath, logger); h != nil {
		handlers = append(handlers, h)
	}

	if h := buildSkillHandler(ctx, logger); h != nil {
		handlers = append(handlers, h)
	}

	return handlers
}

// buildPatchToolCallsHandler creates the middleware that patches dangling tool
// calls in message history — tool calls without corresponding tool result
// messages get a placeholder response so providers don't reject them.
func buildPatchToolCallsHandler(ctx context.Context, logger *slog.Logger) adk.ChatModelAgentMiddleware {
	mw, err := patchtoolcalls.New(ctx, nil)
	if err != nil {
		logger.Warn("[agent] failed to create patchtoolcalls handler", "error", err)
		return nil
	}
	return mw
}

// reductionRootDir returns the on-disk directory used by reduction middleware
// to offload truncated/cleared tool results. Files written here are plain text
// and can be read back via the standard read_file tool.
func reductionRootDir(config Config, logger *slog.Logger) string {
	fsCfg := buildFsToolsConfig(config, logger)
	dir := filepath.Join(fsCfg.WorkDir, ".eino", "reduction")
	_ = os.MkdirAll(dir, 0o755)
	return dir
}

// transcriptFilePath returns the on-disk path where the summarization
// middleware reminds the LLM to read full conversation history.
func transcriptFilePath(config Config, logger *slog.Logger) string {
	fsCfg := buildFsToolsConfig(config, logger)
	dir := filepath.Join(fsCfg.WorkDir, ".eino")
	_ = os.MkdirAll(dir, 0o755)
	return filepath.Join(dir, "transcript.jsonl")
}

// diskBackend implements reduction.Backend by writing files to the local
// filesystem. Offloaded content can be read back via the standard read_file tool.
type diskBackend struct{}

func (d *diskBackend) Write(_ context.Context, req *filesystem.WriteRequest) error {
	dir := filepath.Dir(req.FilePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	return os.WriteFile(req.FilePath, []byte(req.Content), 0o644)
}

// buildReductionHandler creates the v0.8 reduction middleware (ChatModelAgentMiddleware).
// Two-phase: truncation (after tool call) + clearing (before model invocation).
// RootDir points to a real disk directory so offloaded content can be read
// back via the standard read_file tool without virtual path dispatch.
func buildReductionHandler(ctx context.Context, rootDir string, logger *slog.Logger) adk.ChatModelAgentMiddleware {
	reductionCfg := &reduction.Config{
		Backend:           &diskBackend{},
		RootDir:           rootDir,
		MaxLengthForTrunc: 30000,
		MaxTokensForClear: 50000,
	}

	mw, err := reduction.New(ctx, reductionCfg)
	if err != nil {
		logger.Warn("[agent] failed to create reduction handler", "error", err)
		return nil
	}
	return mw
}

// buildSummarizationHandler creates the v0.8 summarization middleware.
// Compresses conversation history when token count exceeds threshold.
// TranscriptFilePath tells the LLM where to find the full history if needed.
func buildSummarizationHandler(ctx context.Context, chatModel model.BaseChatModel, transcriptPath string, logger *slog.Logger) adk.ChatModelAgentMiddleware {
	sumCfg := &summarization.Config{
		Model: chatModel,
		Trigger: &summarization.TriggerCondition{
			ContextTokens: 100000,
		},
		TranscriptFilePath: transcriptPath,
	}

	mw, err := summarization.New(ctx, sumCfg)
	if err != nil {
		logger.Warn("[agent] failed to create summarization handler", "error", err)
		return nil
	}
	return mw
}

// buildSkillHandler creates the skill middleware using a local filesystem backend.
func buildSkillHandler(ctx context.Context, logger *slog.Logger) adk.ChatModelAgentMiddleware {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		logger.Warn("[agent] failed to get home dir for skills", "error", err)
		return nil
	}

	skillsDir := filepath.Join(homeDir, ".agents", "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		logger.Warn("[agent] failed to create skills directory", "dir", skillsDir, "error", err)
		return nil
	}

	backend := &localSkillBackend{baseDir: skillsDir}

	mw, err := newSkillChatModelAgentMiddleware(ctx, backend, logger)
	if err != nil {
		logger.Warn("[agent] failed to create skill handler", "error", err)
		return nil
	}
	return mw
}

// BuildFsTools creates all filesystem tools as independent tool instances.
func BuildFsTools(config Config, bgMgr *tools.BgProcessManager, logger *slog.Logger) []tool.BaseTool {
	cfg := buildFsToolsConfig(config, logger)

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
			logger.Warn("[agent] failed to create fs tool", "error", err)
			continue
		}
		fsTools = append(fsTools, t)
	}

	execTool, err := tools.NewExecuteTool(cfg, bgMgr)
	if err != nil {
		logger.Warn("[agent] failed to create execute tool", "error", err)
	} else {
		fsTools = append(fsTools, execTool)
	}

	bgTool, err := tools.NewBgExecuteTool(cfg, bgMgr)
	if err != nil {
		logger.Warn("[agent] failed to create execute_background tool", "error", err)
	} else {
		fsTools = append(fsTools, bgTool)
	}

	return fsTools
}

// idHash returns a short (12-char) hex hash derived from a numeric ID.
func idHash(id int64) string {
	h := sha256.Sum256([]byte("chatclaw:" + strconv.FormatInt(id, 10)))
	return hex.EncodeToString(h[:6])
}

// buildSessionWorkDir constructs the per-conversation working directory.
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

// buildFsToolsConfig constructs the shared config for all filesystem tools.
func buildFsToolsConfig(config Config, logger *slog.Logger) *tools.FsToolsConfig {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		logger.Warn("[agent] failed to get user home dir", "error", err)
		homeDir = "/"
	}

	sandboxEnabled := config.SandboxMode == "codex"
	var codexBin string
	if sandboxEnabled {
		codexBin = resolveCodexBin()
		if codexBin == "" {
			sandboxEnabled = false
			logger.Warn("[agent] codex sandbox requested but codex not installed, falling back to native execution")
		}
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
		ToolchainBinDir:       config.ToolchainBinDir,
	}
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
func ErrorCatchingToolMiddleware(logger *slog.Logger) compose.ToolMiddleware {
	return compose.ToolMiddleware{
		Invokable: func(next compose.InvokableToolEndpoint) compose.InvokableToolEndpoint {
			return func(ctx context.Context, input *compose.ToolInput) (*compose.ToolOutput, error) {
				output, err := next(ctx, input)
				if err != nil {
					logger.Warn("[agent] tool error", "tool", input.Name, "error", err)
					return &compose.ToolOutput{Result: "Error: " + err.Error()}, nil
				}
				return output, nil
			}
		},
		Streamable: func(next compose.StreamableToolEndpoint) compose.StreamableToolEndpoint {
			return func(ctx context.Context, input *compose.ToolInput) (*compose.StreamToolOutput, error) {
				output, err := next(ctx, input)
				if err != nil {
					logger.Warn("[agent] streaming tool error", "tool", input.Name, "error", err)
					return &compose.StreamToolOutput{
						Result: schema.StreamReaderFromArray([]string{"Error: " + err.Error()}),
					}, nil
				}
				return output, nil
			}
		},
	}
}

// ---------------------------------------------------------------------------
// Local skill backend — reads SKILL.md files from local filesystem directly.
// Replaces the removed skill.NewLocalBackend from v0.7.
// ---------------------------------------------------------------------------

type localSkillBackend struct {
	baseDir string
}

type skillFrontMatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

type localSkill struct {
	Name          string
	Description   string
	Content       string
	BaseDirectory string
}

func (b *localSkillBackend) list() ([]localSkill, error) {
	entries, err := os.ReadDir(b.baseDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read skills directory: %w", err)
	}

	var skills []localSkill
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillPath := filepath.Join(b.baseDir, entry.Name(), "SKILL.md")
		if _, statErr := os.Stat(skillPath); os.IsNotExist(statErr) {
			continue
		}

		data, readErr := os.ReadFile(skillPath)
		if readErr != nil {
			continue
		}

		fm, content, parseErr := parseSkillFrontmatter(string(data))
		if parseErr != nil {
			continue
		}

		absDir, _ := filepath.Abs(filepath.Dir(skillPath))
		skills = append(skills, localSkill{
			Name:          fm.Name,
			Description:   fm.Description,
			Content:       strings.TrimSpace(content),
			BaseDirectory: absDir,
		})
	}
	return skills, nil
}

func parseSkillFrontmatter(data string) (*skillFrontMatter, string, error) {
	const delimiter = "---"
	data = strings.TrimSpace(data)

	if !strings.HasPrefix(data, delimiter) {
		return nil, "", fmt.Errorf("file does not start with frontmatter delimiter")
	}

	rest := data[len(delimiter):]
	endIdx := strings.Index(rest, "\n"+delimiter)
	if endIdx == -1 {
		return nil, "", fmt.Errorf("frontmatter closing delimiter not found")
	}

	frontmatter := strings.TrimSpace(rest[:endIdx])
	content := rest[endIdx+len("\n"+delimiter):]
	if strings.HasPrefix(content, "\n") {
		content = content[1:]
	}

	var fm skillFrontMatter
	if err := yaml.Unmarshal([]byte(frontmatter), &fm); err != nil {
		return nil, "", fmt.Errorf("failed to unmarshal frontmatter: %w", err)
	}

	return &fm, content, nil
}

// newSkillChatModelAgentMiddleware creates a skill handler that integrates with
// the v0.8 ChatModelAgentMiddleware pattern. It registers a "skill" tool and
// injects a system prompt about available skills.
func newSkillChatModelAgentMiddleware(_ context.Context, backend *localSkillBackend, _ *slog.Logger) (adk.ChatModelAgentMiddleware, error) {
	skills, err := backend.list()
	if err != nil {
		return nil, fmt.Errorf("failed to list skills: %w", err)
	}

	if len(skills) == 0 {
		return nil, nil
	}

	var descBuilder strings.Builder
	descBuilder.WriteString("Load a skill by name. Available skills:\n")
	for _, s := range skills {
		descBuilder.WriteString(fmt.Sprintf("- %s: %s\n", s.Name, s.Description))
	}

	skillMap := make(map[string]localSkill, len(skills))
	for _, s := range skills {
		skillMap[s.Name] = s
	}

	return &localSkillHandler{
		backend:  backend,
		skillMap: skillMap,
		desc:     descBuilder.String(),
	}, nil
}

type localSkillHandler struct {
	adk.BaseChatModelAgentMiddleware
	backend  *localSkillBackend
	skillMap map[string]localSkill
	desc     string
}

func (h *localSkillHandler) BeforeAgent(ctx context.Context, runCtx *adk.ChatModelAgentContext) (context.Context, *adk.ChatModelAgentContext, error) {
	instruction := `
# Skills

You have access to a "skill" tool. When a task matches one of the available skills,
call skill(skill="<skill_name>") to load detailed instructions before proceeding.
Always read the skill content and follow its instructions carefully.`

	runCtx.Instruction = runCtx.Instruction + instruction
	runCtx.Tools = append(runCtx.Tools, &localSkillTool{handler: h})
	return ctx, runCtx, nil
}

type localSkillTool struct {
	handler *localSkillHandler
}

func (t *localSkillTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "skill",
		Desc: t.handler.desc,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"skill": {
				Type:     schema.String,
				Desc:     "The skill name to load.",
				Required: true,
			},
		}),
	}, nil
}

type skillInput struct {
	Skill string `json:"skill"`
}

func (t *localSkillTool) InvokableRun(_ context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	var input skillInput
	if err := json.Unmarshal([]byte(argumentsInJSON), &input); err != nil {
		return "", fmt.Errorf("failed to parse skill input: %w", err)
	}

	skill, ok := t.handler.skillMap[input.Skill]
	if !ok {
		return fmt.Sprintf("Skill %q not found. Available: %s", input.Skill, strings.Join(mapKeys(t.handler.skillMap), ", ")), nil
	}

	return fmt.Sprintf("# Skill: %s\nBase directory: %s\n\n%s", skill.Name, skill.BaseDirectory, skill.Content), nil
}

func mapKeys[K comparable, V any](m map[K]V) []K {
	keys := make([]K, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
