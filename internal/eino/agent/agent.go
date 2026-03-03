// Package agent provides utilities for creating eino ADK agents with tools and middlewares.
package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"

	"chatclaw/internal/eino/tools"
	"chatclaw/internal/errs"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/middlewares/patchtoolcalls"
	"github.com/cloudwego/eino/adk/middlewares/plantask"
	"github.com/cloudwego/eino/adk/middlewares/reduction"
	"github.com/cloudwego/eino/adk/middlewares/summarization"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
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
//   - plantask: structured task planning and progress tracking
//   - skill: on-demand skill loading from SKILL.md files
func buildHandlers(ctx context.Context, config Config, chatModel model.BaseChatModel, logger *slog.Logger) []adk.ChatModelAgentMiddleware {
	var handlers []adk.ChatModelAgentMiddleware

	fsCfg := buildFsToolsConfig(config, logger)
	einoDir := filepath.Join(fsCfg.WorkDir, ".eino")
	_ = os.MkdirAll(einoDir, 0o755)

	backend := &diskBackend{}

	if h := buildPatchToolCallsHandler(ctx, logger); h != nil {
		handlers = append(handlers, h)
	}

	reductionDir := filepath.Join(einoDir, "reduction")
	_ = os.MkdirAll(reductionDir, 0o755)
	if h := buildReductionHandler(ctx, backend, reductionDir, logger); h != nil {
		handlers = append(handlers, h)
	}

	transcriptPath := filepath.Join(einoDir, "transcript.jsonl")
	if h := buildSummarizationHandler(ctx, chatModel, transcriptPath, logger); h != nil {
		handlers = append(handlers, h)
	}

	taskDir := filepath.Join(einoDir, "tasks")
	_ = os.MkdirAll(taskDir, 0o755)
	if h := buildPlantaskHandler(ctx, backend, taskDir, logger); h != nil {
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

// buildReductionHandler creates the v0.8 reduction middleware (ChatModelAgentMiddleware).
// Two-phase: truncation (after tool call) + clearing (before model invocation).
// RootDir points to a real disk directory so offloaded content can be read
// back via the standard read_file tool without virtual path dispatch.
func buildReductionHandler(ctx context.Context, backend *diskBackend, rootDir string, logger *slog.Logger) adk.ChatModelAgentMiddleware {
	reductionCfg := &reduction.Config{
		Backend:           backend,
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
		Prepare: func(_ context.Context, originalMessages []adk.Message) ([]adk.Message, error) {
			if err := writeTranscript(transcriptPath, originalMessages); err != nil {
				logger.Warn("[agent] failed to write transcript before summarization", "path", transcriptPath, "error", err)
			}
			// Filter out system messages — only non-system messages should be summarized.
			var toSummarize []adk.Message
			for _, msg := range originalMessages {
				if msg.Role != schema.System {
					toSummarize = append(toSummarize, msg)
				}
			}
			return toSummarize, nil
		},
	}

	mw, err := summarization.New(ctx, sumCfg)
	if err != nil {
		logger.Warn("[agent] failed to create summarization handler", "error", err)
		return nil
	}
	return mw
}

// buildPlantaskHandler creates the plantask middleware that provides structured
// task planning tools (TaskCreate, TaskGet, TaskUpdate, TaskList) for the agent
// to decompose complex multi-step work and track progress.
func buildPlantaskHandler(ctx context.Context, backend *diskBackend, taskDir string, logger *slog.Logger) adk.ChatModelAgentMiddleware {
	mw, err := plantask.New(ctx, &plantask.Config{
		Backend: backend,
		BaseDir: taskDir,
	})
	if err != nil {
		logger.Warn("[agent] failed to create plantask handler", "error", err)
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

