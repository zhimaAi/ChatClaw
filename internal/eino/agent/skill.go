package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"gopkg.in/yaml.v3"
)

// buildSkillHandler creates the skill middleware using a local filesystem backend.
func buildSkillHandler(ctx context.Context, logger *slog.Logger) adk.ChatModelAgentMiddleware {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		logger.Warn("[agent] failed to get home dir for skills", "error", err)
		return nil
	}

	skillsDir := filepath.Join(homeDir, skillsRelDir)
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
