package agent

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"chatclaw/internal/eino/tools"
	"chatclaw/internal/sqlite"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/filesystem"
	"github.com/cloudwego/eino/adk/middlewares/skill"
	"gopkg.in/yaml.v3"
)

const skillFileName = "SKILL.md"

// buildSkillHandler creates the skill middleware using the SDK's skill package.
// It uses a filtering backend that only loads skills enabled in the DB.
func buildSkillHandler(ctx context.Context, b *tools.Backend, logger *slog.Logger) adk.ChatModelAgentMiddleware {
	baseDir := filepath.Join(b.HomeDir(), ".chatclaw", "skills")
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		logger.Warn("[agent] failed to create skills directory", "dir", baseDir, "error", err)
		return nil
	}
	enabledSlugs := queryEnabledSlugs(logger)

	backend, err := newFilteringSkillBackend(ctx, b, baseDir, enabledSlugs)
	if err != nil {
		logger.Warn("[agent] failed to create skill backend", "error", err)
		return nil
	}

	mw, err := skill.NewChatModelAgentMiddleware(ctx, &skill.Config{
		Backend: backend,
	})
	if err != nil {
		logger.Warn("[agent] failed to create skill handler", "error", err)
		return nil
	}
	return mw
}

// queryEnabledSlugs returns the set of enabled skill slugs from the DB.
// If the DB is unavailable, returns nil (load all skills as fallback).
func queryEnabledSlugs(logger *slog.Logger) map[string]bool {
	db := sqlite.DB()
	if db == nil {
		return nil
	}

	rows, err := db.QueryContext(context.Background(), `SELECT slug FROM installed_skills WHERE enabled = 1`)
	if err != nil {
		logger.Warn("[agent] failed to query enabled skills", "error", err)
		return nil
	}
	defer rows.Close()

	slugs := make(map[string]bool)
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			continue
		}
		slugs[slug] = true
	}
	return slugs
}

// filteringSkillBackend implements skill.Backend and filters by enabled slugs.
type filteringSkillBackend struct {
	fsBackend    filesystem.Backend
	baseDir      string
	enabledSlugs map[string]bool // nil = load all
}

func newFilteringSkillBackend(ctx context.Context, fs filesystem.Backend, baseDir string, enabledSlugs map[string]bool) (skill.Backend, error) {
	return &filteringSkillBackend{
		fsBackend:    fs,
		baseDir:      baseDir,
		enabledSlugs: enabledSlugs,
	}, nil
}

func (b *filteringSkillBackend) List(ctx context.Context) ([]skill.FrontMatter, error) {
	skills, err := b.list(ctx)
	if err != nil {
		return nil, err
	}
	matters := make([]skill.FrontMatter, 0, len(skills))
	for _, s := range skills {
		matters = append(matters, s.FrontMatter)
	}
	return matters, nil
}

func (b *filteringSkillBackend) Get(ctx context.Context, name string) (skill.Skill, error) {
	skills, err := b.list(ctx)
	if err != nil {
		return skill.Skill{}, err
	}
	for _, s := range skills {
		if s.Name == name {
			return s, nil
		}
	}
	return skill.Skill{}, nil
}

func (b *filteringSkillBackend) list(ctx context.Context) ([]skill.Skill, error) {
	entries, err := b.fsBackend.GlobInfo(ctx, &filesystem.GlobInfoRequest{
		Pattern: "*/" + skillFileName,
		Path:    b.baseDir,
	})
	if err != nil {
		return nil, err
	}

	var skills []skill.Skill
	for _, entry := range entries {
		filePath := entry.Path
		if !filepath.IsAbs(filePath) {
			filePath = filepath.Join(b.baseDir, filePath)
		}
		slug := filepath.Base(filepath.Dir(filePath))
		if b.enabledSlugs != nil && !b.enabledSlugs[slug] {
			continue
		}

		s, loadErr := b.loadSkill(ctx, filePath)
		if loadErr != nil {
			continue
		}
		skills = append(skills, s)
	}
	return skills, nil
}

func (b *filteringSkillBackend) loadSkill(ctx context.Context, path string) (skill.Skill, error) {
	data, err := b.fsBackend.Read(ctx, &filesystem.ReadRequest{
		FilePath: path,
	})
	if err != nil {
		return skill.Skill{}, err
	}

	fm, content, err := parseSkillFrontmatter(data)
	if err != nil {
		return skill.Skill{}, err
	}

	absDir, _ := filepath.Abs(filepath.Dir(path))
	return skill.Skill{
		FrontMatter:   fm,
		Content:       strings.TrimSpace(content),
		BaseDirectory: absDir,
	}, nil
}

func parseSkillFrontmatter(data string) (skill.FrontMatter, string, error) {
	const delimiter = "---"
	data = strings.TrimSpace(data)

	if !strings.HasPrefix(data, delimiter) {
		return skill.FrontMatter{}, "", fmt.Errorf("file does not start with frontmatter delimiter")
	}

	rest := data[len(delimiter):]
	endIdx := strings.Index(rest, "\n"+delimiter)
	if endIdx == -1 {
		return skill.FrontMatter{}, "", fmt.Errorf("frontmatter closing delimiter not found")
	}

	frontmatter := strings.TrimSpace(rest[:endIdx])
	content := strings.TrimPrefix(rest[endIdx+len("\n"+delimiter):], "\n")

	var fm skill.FrontMatter
	if err := yaml.Unmarshal([]byte(frontmatter), &fm); err != nil {
		return skill.FrontMatter{}, "", fmt.Errorf("failed to unmarshal frontmatter: %w", err)
	}
	return fm, content, nil
}
