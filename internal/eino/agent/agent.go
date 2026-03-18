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
	"sync"
	"sync/atomic"

	"chatclaw/internal/define"
	"chatclaw/internal/eino/tools"
	feishutools "chatclaw/internal/eino/tools/im/feishu"
	qqtools "chatclaw/internal/eino/tools/im/qq"
	wecomtools "chatclaw/internal/eino/tools/im/wecom"
	"chatclaw/internal/errs"
	"chatclaw/internal/services/channels"

	"errors"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/middlewares/patchtoolcalls"
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

	IMGateway          *channels.Gateway // Gateway for IM tools (nil = no IM tools)
	IMDefaultChannelID int64             // Auto-filled from channel source context (0 = not set)
	IMDefaultTargetID  string            // Auto-filled from channel source context ("" = not set)
}

// AgentResult holds the created agent and a cleanup function that should be
// called (typically via defer) when the agent is no longer needed.
type AgentResult struct {
	Agent   adk.ResumableAgent
	Cleanup func()
}

// NewChatModelAgent creates the main agent with two sub-agents (general-purpose,
// bash) registered as AgentTools, following DeerFlow-style orchestration. messageCount is the number of
// historical messages in the conversation; pass 1 (first user message only)
// to enable one-time system prompt logging.
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

	fsTools := buildFilesystemTools(backend, bgMgr, logger)
	imTools := buildIMTools(config, logger)

	// Full toolset for sub-agents (everything available)
	subAgentTools := make([]tool.BaseTool, 0, len(fsTools)+len(imTools)+len(enabledTools)+len(extraTools)+3)
	subAgentTools = append(subAgentTools, fsTools...)
	subAgentTools = append(subAgentTools, imTools...)
	subAgentTools = append(subAgentTools, enabledTools...)
	subAgentTools = append(subAgentTools, browserTool)
	subAgentTools = append(subAgentTools, NewConfirmExecutionTool())
	subAgentTools = append(subAgentTools, extraTools...)

	// Prepare skill resources
	var skillBackend *filteringSkillBackend
	if config.SkillsEnabled {
		skillBackend = newFilteringSkillBackend(backend, logger)
		subAgentTools = append(subAgentTools, &readSkillTool{backend: skillBackend})
	}

	// Sub-agents get the full toolset
	generalPurpose, gpErr := newGeneralPurposeSubAgent(ctx, chatModel, subAgentTools, backend, config, skillBackend, logger)
	if gpErr != nil {
		logger.Warn("[agent] failed to create general_purpose sub-agent", "error", gpErr)
	}

	bashAgent, bashErr := newBashSubAgent(ctx, chatModel, subAgentTools, backend, config, logger)
	if bashErr != nil {
		logger.Warn("[agent] failed to create bash sub-agent", "error", bashErr)
	}

	// Lead agent gets only read-only tools + sub-agents (DeerFlow-style: orchestrator, not executor)
	leadTools := buildLeadAgentTools(subAgentTools, extraTools, config.SkillsEnabled)
	if gpErr == nil {
		leadTools = append(leadTools, generalPurpose)
	}
	if bashErr == nil {
		leadTools = append(leadTools, bashAgent)
	}

	toolsConfig := adk.ToolsConfig{
		ToolsNodeConfig: compose.ToolsNodeConfig{
			Tools:               leadTools,
			ToolCallMiddlewares: []compose.ToolMiddleware{ErrorCatchingToolMiddleware(leadTools, logger)},
			UnknownToolsHandler: unknownToolsHandler(leadTools, logger),
		},
		EmitInternalEvents: true,
	}

	handlers := buildHandlers(ctx, backend, config, chatModel, extraHandlers, logger, messageCount)

	agent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:          config.Name,
		Description:   "AI Assistant",
		Instruction:   config.Instruction,
		Model:         chatModel,
		ToolsConfig:   toolsConfig,
		Handlers:      handlers,
		MaxIterations: UnlimitedIterations,
	})
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

// buildLeadAgentTools selects the minimal read-only toolset for the lead agent.
// The lead agent is an orchestrator — it delegates execution to sub-agents.
// It only keeps tools needed for quick reads and lightweight management.
func buildLeadAgentTools(allTools []tool.BaseTool, extraTools []tool.BaseTool, skillsEnabled bool) []tool.BaseTool {
	// Tools the lead agent is allowed to use directly
	allowedNames := map[string]bool{
		"read_file":           true,
		"ls":                  true,
		"confirm_execution":   true,
		"sequential_thinking": true,
		"memory_retriever":    true,
		"library_retriever":   true,
	}

	if skillsEnabled {
		allowedNames["skill_list"] = true
		allowedNames["skill_search"] = true
		allowedNames["skill_install"] = true
		allowedNames["skill_enable"] = true
		allowedNames["read_skill"] = true
	}

	// IM sender tools should be directly available to the lead agent
	allowedNames[tools.ToolIDFeishuSender] = true
	allowedNames[tools.ToolIDWeComSender] = true
	allowedNames[tools.ToolIDQQSender] = true

	// MCP tools (mcp__*) should be directly available to the lead agent
	for _, t := range extraTools {
		info, err := t.Info(context.Background())
		if err != nil || info == nil {
			continue
		}
		if strings.HasPrefix(info.Name, "mcp__") {
			allowedNames[info.Name] = true
		}
	}

	var result []tool.BaseTool
	for _, t := range allTools {
		info, err := t.Info(context.Background())
		if err != nil || info == nil {
			continue
		}
		if allowedNames[info.Name] {
			result = append(result, t)
		}
	}
	return result
}

// buildHandlers creates all ChatModelAgentMiddleware handlers for the main agent.
func buildHandlers(ctx context.Context, b *tools.Backend, config Config, chatModel model.BaseChatModel, extraHandlers []adk.ChatModelAgentMiddleware, logger *slog.Logger, messageCount int) []adk.ChatModelAgentMiddleware {
	var handlers []adk.ChatModelAgentMiddleware

	// 1. Core environment prompt
	sessionsDir := buildSessionsDir(config, b)
	handlers = append(handlers, NewInstructionHandler(buildCorePrompt(b.HomeDir(), b.WorkDir(), sessionsDir)))

	// 2. Extra handlers from caller (e.g. memory core profile)
	handlers = append(handlers, extraHandlers...)

	// 3. Tools & sandbox prompt
	handlers = append(handlers, NewInstructionHandler(
		buildToolsPrompt(b.WorkDir(), b.SandboxEnabled(),
			b.SandboxEnabled() && config.SandboxNetwork, b.ToolchainBinDir())))

	// 4. Sub-agent usage guide
	handlers = append(handlers, NewInstructionHandler(buildSubAgentPrompt(config)))

	// 5. Scheduled task management guide
	handlers = append(handlers, NewInstructionHandler(buildScheduledTaskPrompt()))

	// 6. PatchToolCalls
	if h := buildPatchToolCallsHandler(ctx, logger); h != nil {
		handlers = append(handlers, h)
	}

	// 7. Reduction + Summarization (main agent paths)
	einoDir := filepath.Join(b.WorkDir(), einoMetaDir)
	_ = os.MkdirAll(einoDir, 0o755)

	reductionPath := filepath.Join(einoDir, reductionDir)
	_ = os.MkdirAll(reductionPath, 0o755)
	if h := buildReductionHandler(ctx, b, reductionPath, logger); h != nil {
		handlers = append(handlers, h)
	}

	transcriptPath := filepath.Join(einoDir, transcriptFile)
	if h := buildSummarizationHandler(ctx, chatModel, transcriptPath, logger); h != nil {
		handlers = append(handlers, h)
	}

	// 7. Skill middleware + guidance prompt
	if config.SkillsEnabled {
		if h := buildSkillHandler(ctx, b, logger); h != nil {
			handlers = append(handlers, h)
		}
		handlers = append(handlers, NewInstructionHandler(buildSkillGuidancePrompt()))
	}

	// 8. Logging handler goes last so BeforeAgent sees the fully-assembled instruction.
	handlers = append(handlers, newLoggingHandler(logger, messageCount <= 1))

	return handlers
}

// buildFilesystemTools creates all filesystem tools from the backend.
func buildFilesystemTools(backend *tools.Backend, bgMgr *tools.BgProcessManager, logger *slog.Logger) []tool.BaseTool {
	type toolFactory struct {
		name string
		fn   func() (tool.BaseTool, error)
	}
	factories := []toolFactory{
		{"read_file", func() (tool.BaseTool, error) { return tools.NewReadFileTool(backend) }},
		{"ls", func() (tool.BaseTool, error) { return tools.NewLsTool(backend) }},
		{"write_file", func() (tool.BaseTool, error) { return tools.NewWriteFileTool(backend) }},
		{"edit_file", func() (tool.BaseTool, error) { return tools.NewEditFileTool(backend) }},
		{"patch_file", func() (tool.BaseTool, error) { return tools.NewPatchFileTool(backend) }},
		{"glob", func() (tool.BaseTool, error) { return tools.NewGlobTool(backend) }},
		{"grep", func() (tool.BaseTool, error) { return tools.NewGrepTool(backend) }},
		{"execute", func() (tool.BaseTool, error) { return tools.NewExecuteTool(backend, nil) }},
	}

	var result []tool.BaseTool
	for _, f := range factories {
		t, err := f.fn()
		if err != nil {
			logger.Warn("[agent] failed to create filesystem tool", "tool", f.name, "error", err)
			continue
		}
		result = append(result, t)
	}

	bgTool, err := tools.NewBgExecuteTool(backend, bgMgr)
	if err != nil {
		logger.Warn("[agent] failed to create execute_background tool", "error", err)
	} else {
		result = append(result, bgTool)
	}

	return result
}

// buildIMTools creates IM sender tools when a Gateway is configured.
func buildIMTools(config Config, logger *slog.Logger) []tool.BaseTool {
	if config.IMGateway == nil {
		return nil
	}
	type toolFactory struct {
		name string
		fn   func() (tool.BaseTool, error)
	}
	factories := []toolFactory{
		{"feishu_sender", func() (tool.BaseTool, error) {
			return feishutools.NewFeishuSenderTool(&feishutools.FeishuSenderConfig{
				Gateway: config.IMGateway, DefaultChannelID: config.IMDefaultChannelID, DefaultTargetID: config.IMDefaultTargetID,
			})
		}},
		{"wecom_sender", func() (tool.BaseTool, error) {
			return wecomtools.NewWeComSenderTool(&wecomtools.WeComSenderConfig{
				Gateway: config.IMGateway, DefaultChannelID: config.IMDefaultChannelID, DefaultTargetID: config.IMDefaultTargetID,
			})
		}},
		{"qq_sender", func() (tool.BaseTool, error) {
			return qqtools.NewQQSenderTool(&qqtools.QQSenderConfig{
				Gateway: config.IMGateway, DefaultChannelID: config.IMDefaultChannelID, DefaultTargetID: config.IMDefaultTargetID,
			})
		}},
	}
	var result []tool.BaseTool
	for _, f := range factories {
		t, err := f.fn()
		if err != nil {
			logger.Warn("[agent] failed to create IM tool", "tool", f.name, "error", err)
			continue
		}
		result = append(result, t)
		logger.Info("[agent] IM tool created", "tool", f.name)
	}
	return result
}

// newFilteringSkillBackend creates a filteringSkillBackend for reading skill content.
func newFilteringSkillBackend(backend *tools.Backend, logger *slog.Logger) *filteringSkillBackend {
	baseDir := filepath.Join(backend.HomeDir(), ".chatclaw", "skills")
	_ = os.MkdirAll(baseDir, 0o755)
	return &filteringSkillBackend{
		fsBackend: backend,
		baseDir:   baseDir,
		logger:    logger,
	}
}

// buildSessionsDir returns the sessions directory for this agent, or "" if
// AgentID is not set.
func buildSessionsDir(config Config, b *tools.Backend) string {
	if config.AgentID <= 0 {
		return ""
	}
	base := config.WorkDir
	if base == "" {
		base = filepath.Join(b.HomeDir(), appDataDir)
	}
	return filepath.Join(base, sessionsSubdir, idHash(config.AgentID))
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
		Callback: func(_ context.Context, before, _ adk.ChatModelAgentState) error {
			if err := writeTranscript(transcriptPath, before.Messages); err != nil {
				logger.Warn("[agent] failed to write transcript before summarization", "path", transcriptPath, "error", err)
			}
			return nil
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

// ErrorCatchingToolMiddleware catches tool execution errors and returns the error
// message as a tool result, allowing the ReAct loop to continue.
// When a tool fails, the error message includes the tool's parameter schema so
// the model can self-correct. After 3 consecutive failures on the same tool,
// a suggestion to try an alternative approach is appended.
// Interrupt signals are not caught — they must propagate to the ADK framework.
func ErrorCatchingToolMiddleware(allTools []tool.BaseTool, logger *slog.Logger) compose.ToolMiddleware {
	schemaMap := buildToolSchemaMap(allTools)
	var failureCounters sync.Map // tool name -> *int32

	subAgentNames := map[string]bool{"general_purpose": true, "bash": true}

	formatError := func(toolName string, err error) string {
		if errors.Is(err, adk.ErrExceedMaxIterations) && subAgentNames[toolName] {
			return fmt.Sprintf("Sub-agent %q has completed its work (reached iteration limit). "+
				"Use whatever partial results were streamed above. "+
				"Do NOT call %q again for the same request — summarize available information and respond to the user directly.", toolName, toolName)
		}

		msg := "Error: " + err.Error()

		if hint, ok := schemaMap[toolName]; ok {
			msg += "\n\nExpected parameters:\n" + hint
		}

		if strings.Contains(err.Error(), "permission denied") || strings.Contains(err.Error(), "not permitted") {
			msg += "\n\nHint: This may be a sandbox restriction. Ensure you are writing within the working directory."
		}

		counterVal, _ := failureCounters.LoadOrStore(toolName, new(int32))
		counter := counterVal.(*int32)
		count := atomic.AddInt32(counter, 1)
		if count >= 3 {
			msg += fmt.Sprintf("\n\nWarning: Tool %q has failed %d times consecutively. Consider trying a different approach or tool.", toolName, count)
		}

		return msg
	}

	resetCounter := func(toolName string) {
		if counterVal, ok := failureCounters.Load(toolName); ok {
			atomic.StoreInt32(counterVal.(*int32), 0)
		}
	}

	return compose.ToolMiddleware{
		Invokable: func(next compose.InvokableToolEndpoint) compose.InvokableToolEndpoint {
			return func(ctx context.Context, input *compose.ToolInput) (*compose.ToolOutput, error) {
				output, err := next(ctx, input)
				if err != nil {
					if isInterruptErr(err) {
						return nil, err
					}
					logger.Warn("[agent] tool error", "tool", input.Name, "error", err)
					return &compose.ToolOutput{Result: formatError(input.Name, err)}, nil
				}
				resetCounter(input.Name)
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
						Result: schema.StreamReaderFromArray([]string{formatError(input.Name, err)}),
					}, nil
				}
				resetCounter(input.Name)
				return output, nil
			}
		},
	}
}

// buildToolSchemaMap pre-loads tool parameter schemas into a name→hint map.
func buildToolSchemaMap(allTools []tool.BaseTool) map[string]string {
	m := make(map[string]string, len(allTools))
	for _, t := range allTools {
		info, err := t.Info(context.Background())
		if err != nil || info == nil {
			continue
		}
		if info.ParamsOneOf == nil {
			continue
		}

		js, err := info.ParamsOneOf.ToJSONSchema()
		if err != nil || js == nil || js.Properties == nil {
			continue
		}

		var sb strings.Builder
		for pair := js.Properties.Oldest(); pair != nil; pair = pair.Next() {
			name := pair.Key
			prop := pair.Value
			required := ""
			for _, r := range js.Required {
				if r == name {
					required = " (required)"
					break
				}
			}
			sb.WriteString(fmt.Sprintf("  - %s: %s%s — %s\n", name, prop.Type, required, prop.Description))
		}
		if sb.Len() > 0 {
			m[info.Name] = sb.String()
		}
	}
	return m
}
