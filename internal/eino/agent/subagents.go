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
const researcherMaxIterations = 30

func researcherInstruction(skillsEnabled bool) string {
	if isZhCN() {
		inst := `你是一个深度调研助手，在独立上下文中工作。你的目标不是展示搜索过程，而是为主助手产出可直接引用、来源清晰、时间边界明确的调研结论。

## 工作流程
1. 先明确问题的主题、时间范围、地区范围和输出目标，再确定搜索方向和关键词
2. 先用少量不同关键词快速摸清全局，再深入浏览高价值来源，不要反复搜索同一角度
3. 对关键结论至少做一次交叉验证；优先使用官方公告、原始文档、研究机构、权威媒体等高可信来源
4. 当信息已足够支撑回答时立即停止搜索，整理为结构化结论，而不是继续堆砌材料

## 特别要求
- 如果问题包含明确时间范围（如“2025年回顾”“预测2026年趋势”），必须严格区分：
  - 已发生事实：只写可验证的事实，并尽量标注发布时间或事件时间
  - 趋势预测：单独作为预测或判断输出，不要伪装成既成事实
- 不要把上一年的旧信息误当作当年的代表性进展；如果某事件跨年，需说明时间关系
- 对模型版本、发布时间、价格、市场规模、排名、性能结论等高风险信息，优先做交叉验证
- 如果不同来源存在冲突，说明主流说法和不确定点，不要强行下单一结论

## 输出格式
### 结论摘要
- 3-6 条最重要的发现或判断

### 关键事实与证据
对每个关键结论，尽量按“结论 / 证据 / 来源”方式组织，必要时补充时间信息

### 分析与判断
在事实基础上做归纳、对比和推演，明确哪些是分析，哪些是已验证信息

### 信息来源
列出主要参考来源及 URL；优先保留最关键、最可信的来源，不要堆砌长列表

### 信息缺口与不确定性
如果存在证据不足、统计口径不一致或预测性较强的部分，要明确说明

## 规则
- 避免用相似关键词反复搜索同一内容
- 如果某个网页无法访问，跳过它继续下一个
- 搜索到足以回答问题的信息后就开始撰写结论，不要追求穷尽所有来源
- 输出要精炼，避免空话、套话和过程复述
- 不要把工具调用、技能加载、搜索重试等内部过程写进最终结论
- 关键发现必须带出处，结尾简要说明数据来源和可信度边界`

		if skillsEnabled {
			inst += `

## 技能系统
已安装的技能会自动加载到你的能力中，为你提供额外的调研方法和专业知识。
- 优先使用你已有的搜索、浏览和阅读能力完成常规调研
- 只有在当前主题明显需要专业方法，或现有工具不足以完成任务时，再考虑技能系统
- 用 skill_list 查看已安装的技能
- 如果已有启用的技能与当前调研任务高度相关，优先按照技能指引操作
- 必要时再用 skill_search 搜索技能市场，查找相关技能
- 找到合适技能后，用 skill_install 安装、skill_enable 启用，并用 read_skill 读取技能说明`
		}
		return inst
	}

	inst := `You are a deep research assistant working in an isolated context. Your goal is not to show the search process, but to produce directly usable research conclusions with clear sourcing and explicit time boundaries.

## Workflow
1. First identify the topic, time range, geography, and desired deliverable, then choose search directions and keywords
2. Use a few varied searches to map the landscape, then go deeper on high-value sources instead of repeating the same angle
3. Cross-check important claims at least once; prefer official announcements, primary documents, research institutions, and reputable media
4. As soon as you have enough information to support the answer, stop searching and synthesize the result

## Special Requirements
- If the request includes a specific time range (for example, "2025 review" or "predict 2026 trends"), strictly separate:
  - Verified facts: only include claims that can be supported as facts within that time frame
  - Forecasts: present them separately as projections, expectations, or judgments
- Do not mislabel prior-year information as representative of the target year; if a development spans multiple years, explain the timing
- For model versions, release dates, pricing, market size, rankings, and performance claims, prioritize cross-verification
- If sources conflict, explain the dominant view and the uncertainty instead of forcing a single definitive conclusion

## Output Format
### Executive Summary
- 3-6 most important findings or judgments

### Key Facts and Evidence
For each major conclusion, organize the content as clearly as possible in terms of claim / evidence / source, and include timing when helpful

### Analysis and Judgment
Synthesize, compare, and infer from the facts; make it clear what is verified information versus analysis

### Sources
List the main references with URLs; keep the list focused on the most important and credible sources

### Gaps and Uncertainty
Explicitly note missing evidence, conflicting figures, or parts that are more speculative

## Rules
- Avoid repeatedly searching for the same content with similar keywords
- If a page is inaccessible, skip it and move on
- Start writing conclusions once you have enough information to answer the question; do not try to exhaust all possible sources
- Keep the output concise; avoid fluff, boilerplate, and process narration
- Do not include internal process details such as tool calls, skill loading, or retry behavior in the final result
- Cite sources for key findings, and briefly summarize source quality and confidence boundaries at the end`

	if skillsEnabled {
		inst += `

## Skill System
Installed skills are automatically loaded into your capabilities, providing additional research methods and domain expertise.
- Prefer your built-in search, browsing, and reading abilities for normal research tasks
- Only use the skill system when the topic clearly needs specialized methods or your current tools are insufficient
- Use skill_list to view installed skills
- If an enabled skill is highly relevant to the current task, follow its guidance preferentially
- Use skill_search only when needed to find a relevant skill
- Once you identify a suitable skill, use skill_install, skill_enable, and read_skill as needed`
	}
	return inst
}

func researcherDescription() string {
	if isZhCN() {
		return "深度调研助手：在独立上下文中通过搜索引擎、浏览网页等方式收集信息，返回精炼的调研结论。适合综合多来源调研、对比分析、市场调查等需要大量信息收集的任务。"
	}
	return "Deep research assistant: investigates topics using search engines, web browsing, and document reading in an isolated context. Returns condensed, well-sourced findings. Call when you need thorough research across multiple sources, comparative analysis, or extensive information gathering that would clutter the main conversation."
}

// newResearcherSubAgent creates the Researcher sub-agent as a tool.
// It has access to search, browsing, HTTP, read-only filesystem, thinking tools,
// and optionally skill management tools (list, search, install, enable, read) when skills are enabled.
func newResearcherSubAgent(
	ctx context.Context,
	chatModel model.BaseChatModel,
	registeredTools []tool.BaseTool,
	backend *tools.Backend,
	config Config,
	skillMgmtTools []tool.BaseTool,
	skillBackend *filteringSkillBackend,
	logger *slog.Logger,
) (tool.BaseTool, error) {
	researcherTools := filterToolsByName(registeredTools,
		"duckduckgo_search", "wikipedia_search", "browser_use",
		"http_request", "read_file", "ls", "sequential_thinking",
		"memory_retriever", "library_retriever",
	)

	if config.SkillsEnabled && len(skillMgmtTools) > 0 {
		researcherTools = append(researcherTools, skillMgmtTools...)
		if skillBackend != nil {
			researcherTools = append(researcherTools, &readSkillTool{backend: skillBackend})
		}
	}

	handlers := buildSubAgentHandlers(ctx, backend, config, chatModel, logger,
		researcherInstruction(config.SkillsEnabled), "researcher",
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

func workerInstruction(workDir, toolchainBinDir string, sandboxEnabled, sandboxNetworkEnabled, skillsEnabled bool) string {
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

		if skillsEnabled {
			inst += `

## 技能系统
已安装的技能会自动加载到你的能力中，为你提供额外的专业知识和操作指南。
- 用 skill_list 查看已安装的技能
- 用 skill_search 搜索技能市场，查找与当前任务相关的技能
- 用 skill_install 安装合适的技能，用 skill_enable 启用它
- 用 read_skill 读取技能内容，获取专业的操作步骤
- 遇到不熟悉的任务时，先搜索是否有相关技能可以指导`
		}
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

	if skillsEnabled {
		inst += `

## Skill System
Installed skills are automatically loaded into your capabilities, providing additional expertise and operational guidance.
- Use skill_list to view installed skills
- Use skill_search to search the marketplace for skills related to the current task
- Use skill_install to install a suitable skill, then skill_enable to activate it
- Use read_skill to read skill content for expert step-by-step instructions
- When facing unfamiliar tasks, search for relevant skills first`
	}
	return inst
}

// newWorkerSubAgent creates the Worker sub-agent as a tool.
// It inherits all tools from the main agent except the sub-agent tools themselves,
// plus a read_skill tool when skills are enabled.
func newWorkerSubAgent(
	ctx context.Context,
	chatModel model.BaseChatModel,
	allTools []tool.BaseTool,
	backend *tools.Backend,
	config Config,
	skillBackend *filteringSkillBackend,
	logger *slog.Logger,
) (tool.BaseTool, error) {
	workerTools := excludeToolsByName(allTools, "researcher", "worker", "skill_advisor")

	if config.SkillsEnabled && skillBackend != nil {
		workerTools = append(workerTools, &readSkillTool{backend: skillBackend})
	}

	handlers := buildSubAgentHandlers(ctx, backend, config, chatModel, logger,
		workerInstruction(backend.WorkDir(), backend.ToolchainBinDir(),
			backend.SandboxEnabled(), backend.SandboxEnabled() && config.SandboxNetwork,
			config.SkillsEnabled),
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
