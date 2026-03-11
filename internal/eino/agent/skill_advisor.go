package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// readSkillTool reads a skill's SKILL.md content via the filteringSkillBackend.
// Used by main agent and general-purpose sub-agent when skills are enabled.
// (DeerFlow-style: skills are tools, not a separate sub-agent.)
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
