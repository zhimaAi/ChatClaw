package agent

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"chatclaw/internal/eino/tools"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
)

// filterToolsByName extracts tools matching any of the given names.
func filterToolsByName(allTools []tool.BaseTool, names ...string) []tool.BaseTool {
	nameSet := make(map[string]struct{}, len(names))
	for _, n := range names {
		nameSet[n] = struct{}{}
	}

	var result []tool.BaseTool
	for _, t := range allTools {
		info, err := t.Info(context.Background())
		if err != nil || info == nil {
			continue
		}
		if _, ok := nameSet[info.Name]; ok {
			result = append(result, t)
		}
	}
	return result
}

// excludeToolsByName returns tools NOT matching any of the given names.
func excludeToolsByName(allTools []tool.BaseTool, names ...string) []tool.BaseTool {
	nameSet := make(map[string]struct{}, len(names))
	for _, n := range names {
		nameSet[n] = struct{}{}
	}

	var result []tool.BaseTool
	for _, t := range allTools {
		info, err := t.Info(context.Background())
		if err != nil || info == nil {
			continue
		}
		if _, ok := nameSet[info.Name]; !ok {
			result = append(result, t)
		}
	}
	return result
}

// buildSubAgentHandlers creates middleware handlers for a sub-agent.
// Flags control which optional middleware layers are included.
func buildSubAgentHandlers(
	ctx context.Context,
	backend *tools.Backend,
	config Config,
	chatModel model.BaseChatModel,
	logger *slog.Logger,
	instruction string,
	subAgentName string,
	needReduction, needSummarization, needSkill bool,
) []adk.ChatModelAgentMiddleware {
	var handlers []adk.ChatModelAgentMiddleware

	handlers = append(handlers, NewInstructionHandler(instruction))

	if h := buildPatchToolCallsHandler(ctx, logger); h != nil {
		handlers = append(handlers, h)
	}

	if needReduction || needSummarization {
		subEinoDir := filepath.Join(backend.WorkDir(), einoMetaDir, subAgentName)
		_ = os.MkdirAll(subEinoDir, 0o755)

		if needReduction {
			reductionPath := filepath.Join(subEinoDir, reductionDir)
			_ = os.MkdirAll(reductionPath, 0o755)
			if h := buildReductionHandler(ctx, backend, reductionPath, logger); h != nil {
				handlers = append(handlers, h)
			}
		}

		if needSummarization {
			transcriptPath := filepath.Join(subEinoDir, transcriptFile)
			if h := buildSummarizationHandler(ctx, chatModel, transcriptPath, logger); h != nil {
				handlers = append(handlers, h)
			}
		}
	}

	if needSkill && config.SkillsEnabled {
		if h := buildSkillHandler(ctx, backend, logger); h != nil {
			handlers = append(handlers, h)
		}
	}

	return handlers
}

// researcherMaxIterations caps the Researcher's ReAct loop. Each "iteration"
// is one model call that may produce one or more tool calls. A typical research
// task needs ~5-8 iterations (a few search rounds + browsing + conclusion).
// 15 provides ample room while preventing runaway loops with weaker models.
const researcherMaxIterations = 15

func researcherInstruction() string {
	if isZhCN() {
		return `你是一个深度调研助手，在独立上下文中工作。

## 工作流程
1. 分析调研需求，确定搜索方向和关键词
2. 搜索相关信息，浏览关键网页获取详情
3. 当你认为已经收集到足够的信息来回答问题时，停止搜索，开始整理结论

## 输出格式
### 关键发现
- 列出最重要的发现要点

### 详细分析
对每个发现进行展开说明

### 信息来源
列出主要参考来源及 URL

## 规则
- 避免用相似的关键词反复搜索同一内容
- 如果某个网页无法访问，跳过它继续下一个
- 搜索到能回答问题的信息后就开始撰写结论，不要追求穷尽所有来源
- 交叉验证关键信息，区分事实与观点
- 如果信息确实不足，在结论中说明，而不是无限搜索
- 输出要精炼，避免冗余`
	}
	return `You are a deep research assistant working in an isolated context.

## Workflow
1. Analyze the request and determine search directions and keywords
2. Search for information and browse key pages for details
3. When you have gathered enough information to answer the question, stop searching and write your conclusions

## Output Format
### Key Findings
- List the most important findings

### Detailed Analysis
Expand on each finding with supporting details

### Sources
List main references with URLs

## Rules
- Avoid repeatedly searching for the same content with similar keywords
- If a page is inaccessible, skip it and move on
- Start writing conclusions once you have enough information to answer the question — do not try to exhaust all sources
- Cross-verify key information; distinguish facts from opinions
- If information is genuinely insufficient, state so in conclusions rather than searching endlessly
- Keep output concise — avoid redundancy`
}

func researcherDescription() string {
	if isZhCN() {
		return "深度调研助手：在独立上下文中通过搜索引擎、浏览网页等方式收集信息，返回精炼的调研结论。适合综合多来源调研、对比分析、市场调查等需要大量信息收集的任务。"
	}
	return "Deep research assistant: investigates topics using search engines, web browsing, and document reading in an isolated context. Returns condensed, well-sourced findings. Call when you need thorough research across multiple sources, comparative analysis, or extensive information gathering that would clutter the main conversation."
}

// newResearcherSubAgent creates the Researcher sub-agent as a tool.
// It has access to search, browsing, HTTP, read-only filesystem, and thinking tools.
func newResearcherSubAgent(
	ctx context.Context,
	chatModel model.BaseChatModel,
	registeredTools []tool.BaseTool,
	backend *tools.Backend,
	config Config,
	logger *slog.Logger,
) (tool.BaseTool, error) {
	researcherTools := filterToolsByName(registeredTools,
		"duckduckgo_search", "wikipedia_search", "browser_use",
		"http_request", "read_file", "ls", "sequential_thinking",
	)

	handlers := buildSubAgentHandlers(ctx, backend, config, chatModel, logger,
		researcherInstruction(), "researcher",
		true, true, true,
	)

	agent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        "researcher",
		Description: researcherDescription(),
		Model:       chatModel,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools:               researcherTools,
				ToolCallMiddlewares: []compose.ToolMiddleware{ErrorCatchingToolMiddleware(researcherTools, logger)},
			},
		},
		Handlers:      handlers,
		MaxIterations: researcherMaxIterations,
	})
	if err != nil {
		return nil, err
	}

	return adk.NewAgentTool(ctx, agent), nil
}

// workerMaxIterations caps the Worker's ReAct loop. Worker handles complex
// multi-step tasks, so the limit is generous but still prevents runaway loops.
const workerMaxIterations = 50

func workerDescription() string {
	if isZhCN() {
		return "通用执行助手：在独立上下文中自主完成复杂的多步骤任务。拥有所有工具（文件操作、命令执行、浏览器等）。适合需要反复试错、多步调试、或会产生大量中间输出的任务。"
	}
	return "General-purpose worker: handles complex multi-step tasks autonomously in an isolated context. Has access to all tools including file operations, shell commands, and browser. Call when the task requires multiple dependent steps, trial-and-error debugging, or would generate too much intermediate output for the main conversation."
}

func workerInstruction(workDir, toolchainBinDir string, sandboxEnabled, sandboxNetworkEnabled bool) string {
	if isZhCN() {
		inst := fmt.Sprintf(`你是一个通用执行助手，在独立上下文中自主完成用户描述的任务。

## 环境
- 工作目录: %s
- 所有文件操作使用绝对路径，基于工作目录`, workDir)

		if sandboxEnabled {
			inst += fmt.Sprintf(`
- 沙箱模式已启用：写入操作仅限工作目录 %s 内`, workDir)
			if sandboxNetworkEnabled {
				inst += "\n- 网络访问已启用"
			} else {
				inst += "\n- 网络访问已禁用"
			}
		}

		if toolchainBinDir != "" {
			inst += "\n- 已预装 uv（Python）和 bun（JavaScript），优先使用"
		}

		inst += `

## 工具使用
- 创建/写入文件用 write_file，编辑现有文件用 edit_file
- 运行命令用 execute，长时间运行的命令用 execute_background
- 读取文件用 read_file，搜索文件用 glob/grep
- 运行 Python 脚本优先用 uv run，运行 JS/TS 优先用 bun run
- 运行 shell 脚本时用 "sh script.sh"（或 "bash script.sh" / "zsh script.sh"），不要用 "./script.sh"（没有执行权限），也不要尝试 chmod
- 需要确认的危险命令先调用 confirm_execution

## 原则
- 理解任务目标后立即开始执行
- 遇到错误时自行诊断和修复，不要放弃
- 完成后清晰总结：做了什么、生成了哪些文件、结果在哪里
- 如果无法完成，说明原因和已尝试的方法`
		return inst
	}

	inst := fmt.Sprintf(`You are a general-purpose execution assistant working autonomously in an isolated context.

## Environment
- Working directory: %s
- Use absolute paths based on the working directory for all file operations`, workDir)

	if sandboxEnabled {
		inst += fmt.Sprintf(`
- Sandbox mode enabled: write operations restricted to %s`, workDir)
		if sandboxNetworkEnabled {
			inst += "\n- Network access is enabled"
		} else {
			inst += "\n- Network access is disabled"
		}
	}

	if toolchainBinDir != "" {
		inst += "\n- uv (Python) and bun (JavaScript) are pre-installed — prefer them"
	}

	inst += `

## Tool Usage
- Create/write files with write_file, edit existing files with edit_file
- Run commands with execute, long-running commands with execute_background
- Read files with read_file, search with glob/grep
- Run Python scripts with uv run, JS/TS with bun run
- Run shell scripts with "sh script.sh" (or "bash script.sh" / "zsh script.sh") — never use "./script.sh" (no execute permission) and do not attempt chmod
- Call confirm_execution before dangerous commands

## Principles
- Begin execution immediately after understanding the task goal
- Self-diagnose and fix errors — do not give up
- Summarize clearly when done: what was done, files created, where results are
- If unable to complete, explain why and what was attempted`
	return inst
}

// newWorkerSubAgent creates the Worker sub-agent as a tool.
// It inherits all tools from the main agent except the sub-agent tools themselves.
func newWorkerSubAgent(
	ctx context.Context,
	chatModel model.BaseChatModel,
	allTools []tool.BaseTool,
	backend *tools.Backend,
	config Config,
	logger *slog.Logger,
) (tool.BaseTool, error) {
	workerTools := excludeToolsByName(allTools, "researcher", "worker", "skill_advisor")

	handlers := buildSubAgentHandlers(ctx, backend, config, chatModel, logger,
		workerInstruction(backend.WorkDir(), backend.ToolchainBinDir(),
			backend.SandboxEnabled(), backend.SandboxEnabled() && config.SandboxNetwork),
		"worker",
		true, true, true,
	)

	agent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        "worker",
		Description: workerDescription(),
		Model:       chatModel,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools:               workerTools,
				ToolCallMiddlewares: []compose.ToolMiddleware{ErrorCatchingToolMiddleware(workerTools, logger)},
			},
		},
		Handlers:      handlers,
		MaxIterations: workerMaxIterations,
	})
	if err != nil {
		return nil, err
	}

	return adk.NewAgentTool(ctx, agent), nil
}
