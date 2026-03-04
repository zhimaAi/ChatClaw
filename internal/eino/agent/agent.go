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
	"github.com/cloudwego/eino/adk/middlewares/reduction"
	"github.com/cloudwego/eino/adk/middlewares/summarization"
	"github.com/cloudwego/eino/adk/prebuilt/deep"
	"github.com/cloudwego/eino/adk/prebuilt/planexecute"
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
	transcriptFile = "transcript.jsonl" // summarization transcript filename
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
	Agent   adk.ResumableAgent
	Cleanup func()
}

// NewChatModelAgent creates a DeepAgent with built-in WriteTodos, TaskTool,
// and a general-purpose SubAgent, backed by the project's filesystem Backend.
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

	// Build user-provided tools (registry + browser + bg-execute + extras).
	// Filesystem tools (read_file, write_file, edit_file, glob, grep, execute)
	// are handled internally by deep.Config.Backend/Shell.
	bgTool, bgErr := tools.NewBgExecuteTool(backend, bgMgr)

	userTools := make([]tool.BaseTool, 0, len(enabledTools)+len(extraTools)+3)
	userTools = append(userTools, enabledTools...)
	userTools = append(userTools, browserTool)
	if bgErr == nil {
		userTools = append(userTools, bgTool)
	} else {
		logger.Warn("[agent] failed to create execute_background tool", "error", bgErr)
	}
	userTools = append(userTools, NewConfirmExecutionTool())
	userTools = append(userTools, extraTools...)

	var toolsConfig adk.ToolsConfig
	if len(userTools) > 0 {
		toolsConfig = adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools:               userTools,
				ToolCallMiddlewares: []compose.ToolMiddleware{ErrorCatchingToolMiddleware(logger)},
				UnknownToolsHandler: unknownToolsHandler(userTools, logger),
			},
		}
	}

	handlers := buildHandlers(ctx, backend, config, chatModel, extraHandlers, logger, messageCount)

	var subAgents []adk.Agent
	if peAgent, peErr := buildPlanExecuteSubAgent(ctx, chatModel, toolsConfig, logger); peErr != nil {
		logger.Warn("[agent] failed to create plan-execute subagent, skipping", "error", peErr)
	} else {
		subAgents = append(subAgents, peAgent)
	}

	deepCfg := &deep.Config{
		Name:         config.Name,
		Description:  "AI Assistant",
		Instruction:  config.Instruction,
		ChatModel:    chatModel,
		MaxIteration: UnlimitedIterations,
		Backend:      backend,
		Shell:        backend,
		ToolsConfig:  toolsConfig,
		Handlers:     handlers,
		SubAgents:    subAgents,
	}

	agent, err := deep.New(ctx, deepCfg)
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

	if config.SkillsEnabled {
		if h := buildSkillHandler(ctx, b, logger); h != nil {
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

// isInterruptErr returns true if the error is an interrupt signal that must
// propagate to the ADK framework rather than being caught.
func isInterruptErr(err error) bool {
	if _, ok := compose.IsInterruptRerunError(err); ok {
		return true
	}
	if _, ok := compose.ExtractInterruptInfo(err); ok {
		return true
	}
	return false
}

const (
	planExecuteMaxIterations   = 8
	planExecuteExecutorMaxIter = 10
)

// namedAgent wraps an adk.Agent with a custom name and description so that
// the DeepAgent TaskTool can display meaningful info to the main agent LLM.
type namedAgent struct {
	adk.Agent
	name string
	desc string
}

func (a *namedAgent) Name(_ context.Context) string        { return a.name }
func (a *namedAgent) Description(_ context.Context) string  { return a.desc }

// buildPlanExecuteSubAgent creates a Plan-Execute agent that can be registered
// as a DeepAgent SubAgent. The main agent delegates complex multi-step tasks
// to this agent via the built-in TaskTool.
//
// Architecture: Planner (generate plan) → Executor (execute steps with tools)
//               → Replanner (evaluate progress, replan or finish)
func buildPlanExecuteSubAgent(ctx context.Context, chatModel model.ToolCallingChatModel, toolsConfig adk.ToolsConfig, logger *slog.Logger) (adk.Agent, error) {
	planner, err := planexecute.NewPlanner(ctx, &planexecute.PlannerConfig{
		ToolCallingChatModel: chatModel,
	})
	if err != nil {
		return nil, fmt.Errorf("create planner: %w", err)
	}

	executor, err := planexecute.NewExecutor(ctx, &planexecute.ExecutorConfig{
		Model:         chatModel,
		ToolsConfig:   toolsConfig,
		MaxIterations: planExecuteExecutorMaxIter,
	})
	if err != nil {
		return nil, fmt.Errorf("create executor: %w", err)
	}

	replanner, err := planexecute.NewReplanner(ctx, &planexecute.ReplannerConfig{
		ChatModel: chatModel,
	})
	if err != nil {
		return nil, fmt.Errorf("create replanner: %w", err)
	}

	agent, err := planexecute.New(ctx, &planexecute.Config{
		Planner:       planner,
		Executor:      executor,
		Replanner:     replanner,
		MaxIterations: planExecuteMaxIterations,
	})
	if err != nil {
		return nil, fmt.Errorf("create plan-execute agent: %w", err)
	}

	desc := selectPlanExecuteDescription()

	logger.Info("[agent] plan-execute subagent created successfully")
	return &namedAgent{Agent: agent, name: "plan-execute", desc: desc}, nil
}

func selectPlanExecuteDescription() string {
	if isZhCN() {
		return "基于「规划-执行-反思」范式的智能体，适用于需要多步骤推理和结构化规划的复杂任务。" +
			"它先将目标拆解为有序步骤，逐步执行并利用可用工具完成每一步，然后反思执行结果并动态调整计划。" +
			"适合以下场景：多步骤调研与分析、需要系统性规划的复杂问题、涉及多个工具协同的任务、" +
			"需要根据中间结果动态调整策略的工作流。（工具：与主代理相同）"
	}
	return "Agent based on the plan-execute-replan paradigm for complex tasks requiring multi-step reasoning and structured planning. " +
		"It decomposes objectives into ordered steps, executes each step using available tools, then reflects on results and dynamically adjusts the plan. " +
		"Best for: multi-step research & analysis, complex problems needing systematic planning, tasks involving multiple tool coordination, " +
		"workflows that require dynamic strategy adjustment based on intermediate results. (Tools: same as main agent)"
}

// ErrorCatchingToolMiddleware catches tool execution errors and returns the error
// message as a tool result, allowing the ReAct loop to continue.
// Interrupt signals are not caught — they must propagate to the ADK framework.
func ErrorCatchingToolMiddleware(logger *slog.Logger) compose.ToolMiddleware {
	return compose.ToolMiddleware{
		Invokable: func(next compose.InvokableToolEndpoint) compose.InvokableToolEndpoint {
			return func(ctx context.Context, input *compose.ToolInput) (*compose.ToolOutput, error) {
				output, err := next(ctx, input)
				if err != nil {
					if isInterruptErr(err) {
						return nil, err
					}
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
					if isInterruptErr(err) {
						return nil, err
					}
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
