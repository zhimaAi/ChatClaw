// Package agent provides utilities for creating eino ADK agents with tools and middlewares.
package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"chatclaw/internal/define"
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

const (
	appDataDir     = ".chatclaw"        // top-level app data directory under $HOME
	einoMetaDir    = ".eino"            // per-session metadata directory under WorkDir
	sessionsSubdir = "sessions"         // subdirectory for per-agent/conversation working dirs
	reductionDir   = "reduction"        // reduction middleware offload directory
	tasksDir       = "tasks"            // plantask middleware data directory
	transcriptFile = "transcript.jsonl" // summarization transcript filename
	skillsRelDir   = ".chatclaw/skills"  // skills directory relative to $HOME
	codexBinName   = "codex"            // codex sandbox binary name (without .exe)
	sandboxCodex   = "codex"            // SandboxMode value for codex sandbox
)

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
	SkillsEnabled   bool   // Global skills toggle from settings
}

// AgentResult holds the created agent and a cleanup function that should be
// called (typically via defer) when the agent is no longer needed.
type AgentResult struct {
	Agent   adk.Agent
	Cleanup func()
}

// NewChatModelAgent creates an ADK ChatModelAgent with tools and handlers.
// messageCount is the number of historical messages in the conversation;
// pass 1 (first user message only) to enable one-time system prompt logging.
func NewChatModelAgent(ctx context.Context, config Config, toolRegistry *tools.ToolRegistry, bgMgr *tools.BgProcessManager, extraTools []tool.BaseTool, extraHandlers []adk.ChatModelAgentMiddleware, logger *slog.Logger, messageCount int) (*AgentResult, error) {
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

	backend := buildBackend(config, logger)
	fsTools := BuildFsTools(backend, bgMgr, logger)

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
				UnknownToolsHandler: unknownToolsHandler(baseTools, logger),
			},
		}
	}

	agentConfig.Handlers = buildHandlers(ctx, backend, config, chatModel, extraHandlers, logger, messageCount)

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

// buildHandlers creates all ChatModelAgentMiddleware handlers.
func buildHandlers(ctx context.Context, b *tools.Backend, config Config, chatModel model.BaseChatModel, extraHandlers []adk.ChatModelAgentMiddleware, logger *slog.Logger, messageCount int) []adk.ChatModelAgentMiddleware {
	var handlers []adk.ChatModelAgentMiddleware

	// System prompt injection
	var sessionsDir string
	if config.AgentID > 0 {
		base := config.WorkDir
		if base == "" {
			base = filepath.Join(b.HomeDir(), appDataDir)
		}
		sessionsDir = filepath.Join(base, sessionsSubdir, idHash(config.AgentID))
	}
	systemPrompt := buildFilesystemSystemPrompt(b.HomeDir(), b.WorkDir(), sessionsDir, b.ToolchainBinDir(), b.SandboxEnabled(), b.SandboxEnabled() && config.SandboxNetwork)
	handlers = append(handlers, NewInstructionHandler(systemPrompt))

	// Extra handlers from caller (e.g. memory core profile)
	handlers = append(handlers, extraHandlers...)

	einoDir := filepath.Join(b.WorkDir(), einoMetaDir)
	_ = os.MkdirAll(einoDir, 0o755)

	if h := buildPatchToolCallsHandler(ctx, logger); h != nil {
		handlers = append(handlers, h)
	}

	reductionPath := filepath.Join(einoDir, reductionDir)
	_ = os.MkdirAll(reductionPath, 0o755)
	if h := buildReductionHandler(ctx, b, reductionPath, logger); h != nil {
		handlers = append(handlers, h)
	}

	transcriptPath := filepath.Join(einoDir, transcriptFile)
	if h := buildSummarizationHandler(ctx, chatModel, transcriptPath, logger); h != nil {
		handlers = append(handlers, h)
	}

	taskPath := filepath.Join(einoDir, tasksDir)
	_ = os.MkdirAll(taskPath, 0o755)
	if h := buildPlantaskHandler(ctx, b, taskPath, logger); h != nil {
		handlers = append(handlers, h)
	}

	if config.SkillsEnabled {
		if h := buildSkillHandler(ctx, logger); h != nil {
			handlers = append(handlers, h)
		}
	}

	// Logging handler goes last so BeforeAgent sees the fully-assembled instruction.
	handlers = append(handlers, newLoggingHandler(logger, messageCount <= 1))

	return handlers
}

func buildPatchToolCallsHandler(ctx context.Context, logger *slog.Logger) adk.ChatModelAgentMiddleware {
	mw, err := patchtoolcalls.New(ctx, nil)
	if err != nil {
		logger.Warn("[agent] failed to create patchtoolcalls handler", "error", err)
		return nil
	}
	return mw
}

func buildReductionHandler(ctx context.Context, b *tools.Backend, rootDir string, logger *slog.Logger) adk.ChatModelAgentMiddleware {
	mw, err := reduction.New(ctx, &reduction.Config{
		Backend:           b,
		RootDir:           rootDir,
		MaxLengthForTrunc: 30000,
		MaxTokensForClear: 50000,
	})
	if err != nil {
		logger.Warn("[agent] failed to create reduction handler", "error", err)
		return nil
	}
	return mw
}

func buildSummarizationHandler(ctx context.Context, chatModel model.BaseChatModel, transcriptPath string, logger *slog.Logger) adk.ChatModelAgentMiddleware {
	mw, err := summarization.New(ctx, &summarization.Config{
		Model: chatModel,
		Trigger: &summarization.TriggerCondition{
			ContextTokens: 100000,
		},
		TranscriptFilePath: transcriptPath,
		Prepare: func(_ context.Context, originalMessages []adk.Message) ([]adk.Message, error) {
			if err := writeTranscript(transcriptPath, originalMessages); err != nil {
				logger.Warn("[agent] failed to write transcript before summarization", "path", transcriptPath, "error", err)
			}
			var toSummarize []adk.Message
			for _, msg := range originalMessages {
				if msg.Role != schema.System {
					toSummarize = append(toSummarize, msg)
				}
			}
			return toSummarize, nil
		},
	})
	if err != nil {
		logger.Warn("[agent] failed to create summarization handler", "error", err)
		return nil
	}
	return mw
}

func buildPlantaskHandler(ctx context.Context, b *tools.Backend, taskDir string, logger *slog.Logger) adk.ChatModelAgentMiddleware {
	mw, err := plantask.New(ctx, &plantask.Config{
		Backend: b,
		BaseDir: taskDir,
	})
	if err != nil {
		logger.Warn("[agent] failed to create plantask handler", "error", err)
		return nil
	}
	return mw
}

// BuildFsTools creates all filesystem tools backed by a single Backend.
func BuildFsTools(b *tools.Backend, bgMgr *tools.BgProcessManager, logger *slog.Logger) []tool.BaseTool {
	var fsTools []tool.BaseTool
	builders := []func(*tools.Backend) (tool.BaseTool, error){
		tools.NewLsTool,
		tools.NewReadFileTool,
		tools.NewWriteFileTool,
		tools.NewEditFileTool,
		tools.NewPatchFileTool,
		tools.NewGlobTool,
		tools.NewGrepTool,
	}
	for _, build := range builders {
		t, err := build(b)
		if err != nil {
			logger.Warn("[agent] failed to create fs tool", "error", err)
			continue
		}
		fsTools = append(fsTools, t)
	}

	execTool, err := tools.NewExecuteTool(b, bgMgr)
	if err != nil {
		logger.Warn("[agent] failed to create execute tool", "error", err)
	} else {
		fsTools = append(fsTools, execTool)
	}

	bgTool, err := tools.NewBgExecuteTool(b, bgMgr)
	if err != nil {
		logger.Warn("[agent] failed to create execute_background tool", "error", err)
	} else {
		fsTools = append(fsTools, bgTool)
	}

	return fsTools
}

// buildBackend creates the unified filesystem backend.
func buildBackend(config Config, logger *slog.Logger) *tools.Backend {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		logger.Warn("[agent] failed to get user home dir", "error", err)
		homeDir = "/"
	}

	var codexBin string
	if config.SandboxMode == sandboxCodex {
		codexBin = resolveCodexBin()
		if codexBin == "" {
			logger.Warn("[agent] codex sandbox requested but codex not installed, falling back to native execution")
		}
	}

	baseWorkDir := config.WorkDir
	if baseWorkDir == "" {
		baseWorkDir = filepath.Join(homeDir, appDataDir)
	}

	workDir := buildSessionWorkDir(baseWorkDir, config.AgentID, config.ConversationID)
	_ = os.MkdirAll(workDir, 0o755)

	return tools.NewBackend(&tools.BackendConfig{
		HomeDir:         homeDir,
		WorkDir:         workDir,
		CodexBin:        codexBin,
		NetworkEnabled:  config.SandboxNetwork,
		ToolchainBinDir: config.ToolchainBinDir,
	})
}

func idHash(id int64) string {
	h := sha256.Sum256([]byte("chatclaw:" + strconv.FormatInt(id, 10)))
	return hex.EncodeToString(h[:6])
}

func buildSessionWorkDir(baseWorkDir string, agentID, conversationID int64) string {
	dir := filepath.Join(baseWorkDir, sessionsSubdir)
	if agentID > 0 {
		dir = filepath.Join(dir, idHash(agentID))
	}
	if conversationID > 0 {
		dir = filepath.Join(dir, idHash(conversationID))
	}
	return dir
}

func resolveCodexBin() string {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	binName := codexBinName
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	candidate := filepath.Join(cfgDir, define.AppID, "bin", binName)
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	return ""
}

// instructionHandler appends additional instructions to the agent's system prompt.
type instructionHandler struct {
	adk.BaseChatModelAgentMiddleware
	instruction string
}

func (h *instructionHandler) BeforeAgent(ctx context.Context, runCtx *adk.ChatModelAgentContext) (context.Context, *adk.ChatModelAgentContext, error) {
	runCtx.Instruction += h.instruction
	return ctx, runCtx, nil
}

// NewInstructionHandler creates a handler that appends the given text to the
// agent's system instruction. Exported so callers can inject extra prompts
// (e.g. memory core profile) using the same mechanism.
func NewInstructionHandler(instruction string) adk.ChatModelAgentMiddleware {
	return &instructionHandler{instruction: instruction}
}

// unknownToolsHandler returns a handler for tool calls where the model hallucinates
// a tool name that doesn't exist. Instead of crashing the agent loop, it returns
// an informative error message so the model can self-correct and retry.
func unknownToolsHandler(registeredTools []tool.BaseTool, logger *slog.Logger) func(ctx context.Context, name, input string) (string, error) {
	var toolNames []string
	for _, t := range registeredTools {
		info, _ := t.Info(context.Background())
		if info != nil {
			toolNames = append(toolNames, info.Name)
		}
	}
	return func(ctx context.Context, name, input string) (string, error) {
		logger.Warn("[agent] unknown tool called", "tool", name)
		return fmt.Sprintf("Error: tool %q does not exist. Available tools: %s. Please use one of the available tools.",
			name, strings.Join(toolNames, ", ")), nil
	}
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
