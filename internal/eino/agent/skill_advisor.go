package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

// skillAdvisorMaxIterations caps the SkillAdvisor's ReAct loop. Its SOP is
// short (list → search → install → enable → read → respond), so 15 is generous.
const skillAdvisorMaxIterations = 15

func skillAdvisorInstruction() string {
	if isZhCN() {
		return `你是技能顾问，负责帮助用户找到并启用合适的技能。

## 标准流程 (SOP)
1. 理解用户需要什么类型的技能支持
2. 用 skill_list 查看已安装的技能，检查是否已有合适的
3. 如果没有，用 skill_search 搜索技能市场
4. 找到合适的技能后，用 skill_install 安装
5. 用 skill_enable 启用技能
6. 用 read_skill 读取技能内容，理解其用法
7. 生成结构化的执行指南返回

## 输出格式
### 相关技能
列出找到的相关技能及其用途

### 执行步骤
基于技能内容，给出具体的操作步骤

### 关键约束
技能中提到的限制条件和注意事项

## 规则
- 优先使用已安装且已启用的技能
- 安装新技能前确认与用户需求匹配
- read_skill 后要仔细分析内容，提取关键操作步骤
- 如果没有找到合适的技能，明确告知`
	}
	return `You are a Skill Advisor responsible for finding and activating suitable skills for the user.

## Standard Operating Procedure (SOP)
1. Understand what type of skill support the user needs
2. Use skill_list to check already installed skills for a match
3. If none found, use skill_search to search the marketplace
4. Install the right skill with skill_install
5. Enable it with skill_enable
6. Read the skill content with read_skill to understand usage
7. Return a structured execution guide

## Output Format
### Relevant Skills
List found skills and their purposes

### Execution Steps
Concrete steps based on the skill content

### Key Constraints
Limitations and caveats mentioned in the skill

## Rules
- Prefer already installed and enabled skills
- Confirm skill matches user needs before installing
- After read_skill, carefully extract key operational steps
- Clearly state if no suitable skill was found`
}

func skillAdvisorDescription() string {
	if isZhCN() {
		return "技能顾问：搜索技能市场、安装技能、分析技能内容，返回结构化的执行指南。当遇到不熟悉的任务或需要专业能力时调用。"
	}
	return "Skill advisor: searches the skill marketplace, installs skills, analyzes their content, and returns a structured execution guide. Call when facing unfamiliar tasks or needing specialized capabilities."
}

// readSkillTool reads a skill's SKILL.md content via the filteringSkillBackend.
type readSkillTool struct {
	backend *filteringSkillBackend
}

type readSkillInput struct {
	Name string `json:"name"`
}

func (t *readSkillTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	desc := "Read the full content of an installed skill's SKILL.md file. Use after installing/enabling a skill to understand its instructions and capabilities."
	if isZhCN() {
		desc = "读取已安装技能的 SKILL.md 全文内容。在安装/启用技能后使用，了解其指令和能力。"
	}
	return &schema.ToolInfo{
		Name: "read_skill",
		Desc: desc,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"name": {Type: schema.String, Desc: "Skill name (as shown by skill_list)", Required: true},
		}),
	}, nil
}

func (t *readSkillTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var in readSkillInput
	if err := json.Unmarshal([]byte(argsJSON), &in); err != nil {
		return "", fmt.Errorf("parse arguments: %w", err)
	}
	if in.Name == "" {
		return "Error: name is required", nil
	}

	s, err := t.backend.Get(ctx, in.Name)
	if err != nil {
		return fmt.Sprintf("Error: %s", err.Error()), nil
	}

	return fmt.Sprintf("# Skill: %s\n\nBase directory: %s\n\n%s", s.Name, s.BaseDirectory, s.Content), nil
}

// newSkillAdvisorSubAgent creates the SkillAdvisor sub-agent as a tool.
// It has access to skill management tools (list, search, install, enable) plus read_skill.
func newSkillAdvisorSubAgent(
	ctx context.Context,
	chatModel model.BaseChatModel,
	skillMgmtTools []tool.BaseTool,
	skillBackend *filteringSkillBackend,
	logger *slog.Logger,
) (tool.BaseTool, error) {
	advisorTools := make([]tool.BaseTool, 0, len(skillMgmtTools)+1)

	relevantSkillTools := filterToolsByName(skillMgmtTools,
		"skill_list", "skill_search", "skill_install", "skill_enable",
	)
	advisorTools = append(advisorTools, relevantSkillTools...)
	advisorTools = append(advisorTools, &readSkillTool{backend: skillBackend})

	var handlers []adk.ChatModelAgentMiddleware
	handlers = append(handlers, NewInstructionHandler(skillAdvisorInstruction()))
	if h := buildPatchToolCallsHandler(ctx, logger); h != nil {
		handlers = append(handlers, h)
	}

	agent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        "skill_advisor",
		Description: skillAdvisorDescription(),
		Model:       chatModel,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools:               advisorTools,
				ToolCallMiddlewares: []compose.ToolMiddleware{ErrorCatchingToolMiddleware(advisorTools, logger)},
			},
		},
		Handlers:      handlers,
		MaxIterations: skillAdvisorMaxIterations,
	})
	if err != nil {
		return nil, err
	}

	return adk.NewAgentTool(ctx, agent), nil
}
