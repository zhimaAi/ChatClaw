// Package tools provides skill management tools for the agent's ReAct loop.
// These tools allow the AI to search, install, uninstall, enable, disable skills,
// and open skill directories.

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"chatclaw/internal/services/skills"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// SkillManagementConfig configures the skill management tools.
type SkillManagementConfig struct {
	SkillsService *skills.SkillsService
}

// NewSkillManagementTools creates all skill management tools.
func NewSkillManagementTools(config *SkillManagementConfig) ([]tool.BaseTool, error) {
	if config == nil || config.SkillsService == nil {
		return nil, fmt.Errorf("SkillsService is required")
	}
	svc := config.SkillsService

	return []tool.BaseTool{
		&skillSearchTool{svc: svc},
		&skillListTool{svc: svc},
		&skillInstallTool{svc: svc},
		&skillUninstallTool{svc: svc},
		&skillEnableTool{svc: svc},
		&skillDisableTool{svc: svc},
		&skillOpenFolderTool{svc: svc},
	}, nil
}

// --- skill_search ---

type skillSearchTool struct {
	svc *skills.SkillsService
}

type skillSearchInput struct {
	Query string `json:"query"`
	Limit int    `json:"limit"`
}

func (t *skillSearchTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "skill_search",
		Desc: "Search skills in the marketplace by keyword. Use when the user wants to find or discover new skills.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"query": {Type: schema.String, Desc: "Search query (e.g. 'pdf', 'twitter')", Required: true},
			"limit": {Type: schema.Integer, Desc: "Max results (default 20)", Required: false},
		}),
	}, nil
}

func (t *skillSearchTool) InvokableRun(_ context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var in skillSearchInput
	if err := json.Unmarshal([]byte(argsJSON), &in); err != nil {
		return "", fmt.Errorf("parse arguments: %w", err)
	}
	if in.Query == "" {
		return "Error: query is required", nil
	}
	if in.Limit <= 0 {
		in.Limit = 20
	}

	results, err := t.svc.SearchSkills(in.Query, in.Limit)
	if err != nil {
		return fmt.Sprintf("Error: %s", err.Error()), nil
	}
	if len(results) == 0 {
		return "No skills found.", nil
	}

	var sb strings.Builder
	for i, r := range results {
		inst := ""
		if r.Installed {
			inst = " [installed]"
		}
		sb.WriteString(fmt.Sprintf("%d. %s (%s)%s - %s\n", i+1, r.DisplayName, r.Slug, inst, r.Summary))
	}
	return sb.String(), nil
}

// --- skill_list ---

type skillListTool struct {
	svc *skills.SkillsService
}

func (t *skillListTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "skill_list",
		Desc: "List installed skills with their enabled status. Use before enabling, disabling, or uninstalling.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"filter": {Type: schema.String, Desc: "Optional: 'all', 'builtin', 'market', 'local' (default: all)", Required: false},
		}),
	}, nil
}

func (t *skillListTool) InvokableRun(_ context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	filter := "all"
	if argsJSON != "" {
		var in struct {
			Filter string `json:"filter"`
		}
		_ = json.Unmarshal([]byte(argsJSON), &in)
		if in.Filter != "" {
			filter = in.Filter
		}
	}

	list, err := t.svc.ListInstalledSkills(filter)
	if err != nil {
		return fmt.Sprintf("Error: %s", err.Error()), nil
	}
	if len(list) == 0 {
		return "No installed skills.", nil
	}

	var sb strings.Builder
	for _, s := range list {
		status := "disabled"
		if s.Enabled {
			status = "enabled"
		}
		name := s.Name
		if name == "" {
			name = s.Slug
		}
		sb.WriteString(fmt.Sprintf("- %s (%s) [%s] %s - %s\n", name, s.Slug, status, s.Source, s.Description))
	}
	return sb.String(), nil
}

// --- skill_install ---

type skillInstallTool struct {
	svc *skills.SkillsService
}

type skillInstallInput struct {
	Slug    string `json:"slug"`
	Version string `json:"version"`
}

func (t *skillInstallTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "skill_install",
		Desc: "Install a skill from the marketplace. Use skill_search first to find the slug.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"slug":    {Type: schema.String, Desc: "Skill slug (e.g. 'intel-search')", Required: true},
			"version": {Type: schema.String, Desc: "Version (default: latest)", Required: false},
		}),
	}, nil
}

func (t *skillInstallTool) InvokableRun(_ context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var in skillInstallInput
	if err := json.Unmarshal([]byte(argsJSON), &in); err != nil {
		return "", fmt.Errorf("parse arguments: %w", err)
	}
	if in.Slug == "" {
		return "Error: slug is required", nil
	}

	err := t.svc.InstallSkill(in.Slug, in.Version)
	if err != nil {
		return fmt.Sprintf("Error: %s", err.Error()), nil
	}
	return fmt.Sprintf("Skill %q installed successfully.", in.Slug), nil
}

// --- skill_uninstall ---

type skillUninstallTool struct {
	svc *skills.SkillsService
}

type skillUninstallInput struct {
	Slug string `json:"slug"`
}

func (t *skillUninstallTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "skill_uninstall",
		Desc: "Uninstall a skill. Only market and local skills can be uninstalled; builtin skills cannot.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"slug": {Type: schema.String, Desc: "Skill slug to uninstall", Required: true},
		}),
	}, nil
}

func (t *skillUninstallTool) InvokableRun(_ context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var in skillUninstallInput
	if err := json.Unmarshal([]byte(argsJSON), &in); err != nil {
		return "", fmt.Errorf("parse arguments: %w", err)
	}
	if in.Slug == "" {
		return "Error: slug is required", nil
	}

	err := t.svc.UninstallSkill(in.Slug)
	if err != nil {
		return fmt.Sprintf("Error: %s", err.Error()), nil
	}
	return fmt.Sprintf("Skill %q uninstalled.", in.Slug), nil
}

// --- skill_enable ---

type skillEnableTool struct {
	svc *skills.SkillsService
}

type skillEnableInput struct {
	Slug string `json:"slug"`
}

func (t *skillEnableTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "skill_enable",
		Desc: "Enable an installed skill so the AI can use it.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"slug": {Type: schema.String, Desc: "Skill slug to enable", Required: true},
		}),
	}, nil
}

func (t *skillEnableTool) InvokableRun(_ context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var in skillEnableInput
	if err := json.Unmarshal([]byte(argsJSON), &in); err != nil {
		return "", fmt.Errorf("parse arguments: %w", err)
	}
	if in.Slug == "" {
		return "Error: slug is required", nil
	}

	err := t.svc.EnableSkill(in.Slug)
	if err != nil {
		return fmt.Sprintf("Error: %s", err.Error()), nil
	}
	return fmt.Sprintf("Skill %q enabled.", in.Slug), nil
}

// --- skill_disable ---

type skillDisableTool struct {
	svc *skills.SkillsService
}

type skillDisableInput struct {
	Slug string `json:"slug"`
}

func (t *skillDisableTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "skill_disable",
		Desc: "Disable an installed skill so the AI will not load it.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"slug": {Type: schema.String, Desc: "Skill slug to disable", Required: true},
		}),
	}, nil
}

func (t *skillDisableTool) InvokableRun(_ context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var in skillDisableInput
	if err := json.Unmarshal([]byte(argsJSON), &in); err != nil {
		return "", fmt.Errorf("parse arguments: %w", err)
	}
	if in.Slug == "" {
		return "Error: slug is required", nil
	}

	err := t.svc.DisableSkill(in.Slug)
	if err != nil {
		return fmt.Sprintf("Error: %s", err.Error()), nil
	}
	return fmt.Sprintf("Skill %q disabled.", in.Slug), nil
}

// --- skill_open_folder ---

type skillOpenFolderTool struct {
	svc *skills.SkillsService
}

type skillOpenFolderInput struct {
	Slug string `json:"slug"`
}

func (t *skillOpenFolderTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "skill_open_folder",
		Desc: "Open the skill's directory in the system file manager. Use when the user wants to view or edit skill files.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"slug": {Type: schema.String, Desc: "Skill slug", Required: true},
		}),
	}, nil
}

func (t *skillOpenFolderTool) InvokableRun(_ context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var in skillOpenFolderInput
	if err := json.Unmarshal([]byte(argsJSON), &in); err != nil {
		return "", fmt.Errorf("parse arguments: %w", err)
	}
	if in.Slug == "" {
		return "Error: slug is required", nil
	}

	path := t.svc.GetSkillPath(in.Slug)
	if err := openDirectory(path); err != nil {
		return fmt.Sprintf("Error opening folder: %s", err.Error()), nil
	}
	return fmt.Sprintf("Opened skill folder: %s", path), nil
}

// openDirectory opens a directory in the system file manager.
func openDirectory(dir string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", dir)
	case "windows":
		cmd = exec.Command("explorer", dir)
	default:
		cmd = exec.Command("xdg-open", dir)
	}
	return cmd.Start()
}
