package skills

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"chatclaw/internal/sqlite"

	"github.com/uptrace/bun"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// InstalledSkill is the DB model and the DTO returned to the frontend.
type InstalledSkill struct {
	Slug        string `json:"slug"        bun:"slug,pk"`
	Name        string `json:"name"        bun:"-"`
	Description string `json:"description" bun:"-"`
	Version     string `json:"version"     bun:"version"`
	Source      string `json:"source"      bun:"source"`
	Enabled     bool   `json:"enabled"     bun:"enabled"`
	InstalledAt string `json:"installedAt" bun:"installed_at"`
}

type SkillsService struct {
	app        *application.App
	skillsDir  string
	httpClient *http.Client
}

func NewSkillsService(app *application.App) *SkillsService {
	homeDir, _ := os.UserHomeDir()
	skillsDir := filepath.Join(homeDir, ".chatclaw", "skills")
	_ = os.MkdirAll(skillsDir, 0o755)

	return &SkillsService{
		app:       app,
		skillsDir: skillsDir,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (s *SkillsService) db() *bun.DB {
	return sqlite.DB()
}

// GetSkillsDir returns the local skills directory path.
func (s *SkillsService) GetSkillsDir() string {
	return s.skillsDir
}

// --- Local management ---

func (s *SkillsService) InstallSkill(slug, version string) error {
	return s.installSkill(slug, version, "market")
}

func (s *SkillsService) installSkill(slug, version, source string) error {
	if version == "" || version == "latest" {
		detail, err := s.GetSkillDetail(slug)
		if err != nil {
			return fmt.Errorf("failed to get skill detail: %w", err)
		}
		version = detail.Version
	}

	files, err := s.getVersionFiles(slug, version)
	if err != nil {
		return fmt.Errorf("failed to get version files: %w", err)
	}

	destDir := filepath.Join(s.skillsDir, slug)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("failed to create skill directory: %w", err)
	}

	for _, f := range files {
		content, dlErr := s.getFileContent(slug, version, f.Path)
		if dlErr != nil {
			_ = os.RemoveAll(destDir)
			return fmt.Errorf("failed to download %s: %w", f.Path, dlErr)
		}

		filePath := filepath.Join(destDir, f.Path)
		if dir := filepath.Dir(filePath); dir != destDir {
			_ = os.MkdirAll(dir, 0o755)
		}
		if err := os.WriteFile(filePath, content, 0o644); err != nil {
			_ = os.RemoveAll(destDir)
			return fmt.Errorf("failed to write %s: %w", f.Path, err)
		}
	}

	db := s.db()
	_, err = db.NewInsert().
		TableExpr("installed_skills").
		ModelTableExpr("").
		Value("slug", "?", slug).
		Value("version", "?", version).
		Value("source", "?", source).
		Value("enabled", "?", true).
		Value("installed_at", "CURRENT_TIMESTAMP").
		Exec(context.Background())
	if err != nil {
		// ON CONFLICT: update version and source
		_, err = db.ExecContext(context.Background(),
			`INSERT OR REPLACE INTO installed_skills (slug, version, source, enabled, installed_at)
			 VALUES (?, ?, ?, 1, CURRENT_TIMESTAMP)`, slug, version, source)
		if err != nil {
			return fmt.Errorf("failed to save skill record: %w", err)
		}
	}

	return nil
}

func (s *SkillsService) UninstallSkill(slug string) error {
	db := s.db()

	var source string
	err := db.NewSelect().
		TableExpr("installed_skills").
		Column("source").
		Where("slug = ?", slug).
		Scan(context.Background(), &source)
	if err != nil {
		return fmt.Errorf("skill not found: %w", err)
	}
	if source != "market" {
		return fmt.Errorf("only market skills can be uninstalled")
	}

	destDir := filepath.Join(s.skillsDir, slug)
	if err := os.RemoveAll(destDir); err != nil {
		return fmt.Errorf("failed to remove skill directory: %w", err)
	}

	_, err = db.ExecContext(context.Background(),
		`DELETE FROM installed_skills WHERE slug = ?`, slug)
	return err
}

func (s *SkillsService) EnableSkill(slug string) error {
	_, err := s.db().ExecContext(context.Background(),
		`UPDATE installed_skills SET enabled = 1 WHERE slug = ?`, slug)
	return err
}

func (s *SkillsService) DisableSkill(slug string) error {
	_, err := s.db().ExecContext(context.Background(),
		`UPDATE installed_skills SET enabled = 0 WHERE slug = ?`, slug)
	return err
}

func (s *SkillsService) ListInstalledSkills(filter string) ([]InstalledSkill, error) {
	_ = s.SyncInstalledSkills()

	db := s.db()
	ctx := context.Background()

	query := `SELECT slug, version, source, enabled, installed_at FROM installed_skills`
	switch filter {
	case "builtin":
		query += ` WHERE source = 'builtin'`
	case "market":
		query += ` WHERE source = 'market'`
	case "local":
		query += ` WHERE source = 'local'`
	}
	query += ` ORDER BY installed_at DESC`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var skills []InstalledSkill
	for rows.Next() {
		var sk InstalledSkill
		if err := rows.Scan(&sk.Slug, &sk.Version, &sk.Source, &sk.Enabled, &sk.InstalledAt); err != nil {
			continue
		}
		s.enrichFromDisk(&sk)
		skills = append(skills, sk)
	}
	return skills, nil
}

// enrichFromDisk reads SKILL.md frontmatter to populate Name and Description.
func (s *SkillsService) enrichFromDisk(sk *InstalledSkill) {
	skillPath := filepath.Join(s.skillsDir, sk.Slug, "SKILL.md")
	data, err := os.ReadFile(skillPath)
	if err != nil {
		sk.Name = sk.Slug
		return
	}
	fm, _, parseErr := parseSkillFrontmatter(string(data))
	if parseErr != nil {
		sk.Name = sk.Slug
		return
	}
	sk.Name = fm.Name
	sk.Description = fm.Description
	if sk.Name == "" {
		sk.Name = sk.Slug
	}
}

// --- Sync & startup ---

func (s *SkillsService) SyncInstalledSkills() error {
	db := s.db()
	ctx := context.Background()

	dbSkills := make(map[string]string)
	rows, err := db.QueryContext(ctx, `SELECT slug, source FROM installed_skills`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var slug, source string
		if err := rows.Scan(&slug, &source); err != nil {
			continue
		}
		dbSkills[slug] = source
	}
	rows.Close()

	dirSkills := make(map[string]bool)
	entries, err := os.ReadDir(s.skillsDir)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillPath := filepath.Join(s.skillsDir, entry.Name(), "SKILL.md")
		if _, statErr := os.Stat(skillPath); statErr == nil {
			dirSkills[entry.Name()] = true
		}
	}

	// 1. DB has record but directory missing → delete orphan
	for slug := range dbSkills {
		if !dirSkills[slug] {
			_, _ = db.ExecContext(ctx, `DELETE FROM installed_skills WHERE slug = ?`, slug)
		}
	}

	// 2. Directory exists but no DB record → local skill
	for slug := range dirSkills {
		if _, exists := dbSkills[slug]; !exists {
			_, _ = db.ExecContext(ctx,
				`INSERT OR IGNORE INTO installed_skills (slug, version, source, enabled, installed_at)
				 VALUES (?, '', 'local', 1, CURRENT_TIMESTAMP)`, slug)
		}
	}

	return nil
}

func (s *SkillsService) EnsureBuiltinSkills() {
	if err := s.SyncInstalledSkills(); err != nil {
		s.app.Logger.Warn("[skills] sync failed", "error", err)
	}

	for _, slug := range BuiltinSlugs {
		var exists bool
		err := s.db().NewSelect().
			TableExpr("installed_skills").
			ColumnExpr("1").
			Where("slug = ?", slug).
			Scan(context.Background(), &exists)
		if err == nil && exists {
			continue
		}

		s.app.Logger.Info("[skills] installing builtin skill", "slug", slug)
		if err := s.installSkill(slug, "latest", "builtin"); err != nil {
			s.app.Logger.Warn("[skills] failed to install builtin skill", "slug", slug, "error", err)
		}
	}
}

// ReadSkillMD reads the SKILL.md content for a locally installed skill.
func (s *SkillsService) ReadSkillMD(slug string) (string, error) {
	skillPath := filepath.Join(s.skillsDir, slug, "SKILL.md")
	data, err := os.ReadFile(skillPath)
	if err != nil {
		return "", fmt.Errorf("failed to read SKILL.md: %w", err)
	}
	return string(data), nil
}

// GetSkillPath returns the local directory path for a skill.
func (s *SkillsService) GetSkillPath(slug string) string {
	return filepath.Join(s.skillsDir, slug)
}

// SkillFileInfo represents a file inside a skill directory.
type SkillFileInfo struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

// ListSkillFiles lists all files in a locally installed skill directory.
func (s *SkillsService) ListSkillFiles(slug string) ([]SkillFileInfo, error) {
	baseDir := filepath.Join(s.skillsDir, slug)
	var files []SkillFileInfo

	err := filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		relPath, _ := filepath.Rel(baseDir, path)
		relPath = filepath.ToSlash(relPath)
		files = append(files, SkillFileInfo{Path: relPath, Size: info.Size()})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list skill files: %w", err)
	}

	// Sort: SKILL.md first, then alphabetical
	sortSkillFiles(files)
	return files, nil
}

// ReadSkillFile reads a file from a locally installed skill.
func (s *SkillsService) ReadSkillFile(slug, filePath string) (string, error) {
	fullPath := filepath.Join(s.skillsDir, slug, filepath.FromSlash(filePath))

	// Security: ensure the resolved path is still under the skill directory
	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}
	absBase, _ := filepath.Abs(filepath.Join(s.skillsDir, slug))
	if !strings.HasPrefix(absPath, absBase+string(filepath.Separator)) && absPath != absBase {
		return "", fmt.Errorf("path traversal not allowed")
	}

	data, err := os.ReadFile(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}
	return string(data), nil
}

func sortSkillFiles(files []SkillFileInfo) {
	for i := 0; i < len(files); i++ {
		for j := i + 1; j < len(files); j++ {
			if skillFileLess(files[i].Path, files[j].Path) > 0 {
				files[i], files[j] = files[j], files[i]
			}
		}
	}
}

func skillFileLess(a, b string) int {
	if a == "SKILL.md" {
		return -1
	}
	if b == "SKILL.md" {
		return 1
	}
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

// RefreshSkills syncs the filesystem and returns the full installed list.
func (s *SkillsService) RefreshSkills() ([]InstalledSkill, error) {
	return s.ListInstalledSkills("all")
}

// GetEnabledSlugs returns slugs of all enabled skills (used by agent loader).
func (s *SkillsService) GetEnabledSlugs() ([]string, error) {
	db := s.db()
	ctx := context.Background()

	rows, err := db.QueryContext(ctx, `SELECT slug FROM installed_skills WHERE enabled = 1`)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()

	var slugs []string
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			continue
		}
		slugs = append(slugs, slug)
	}
	return slugs, nil
}

// --- Frontmatter parsing (shared with agent/skill.go) ---

type skillFrontMatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
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
	content := strings.TrimPrefix(rest[endIdx+len("\n"+delimiter):], "\n")

	var fm skillFrontMatter
	for _, line := range strings.Split(frontmatter, "\n") {
		line = strings.TrimSpace(line)
		if k, v, ok := strings.Cut(line, ":"); ok {
			k = strings.TrimSpace(k)
			v = strings.TrimSpace(v)
			switch k {
			case "name":
				fm.Name = v
			case "description":
				fm.Description = v
			}
		}
	}

	return &fm, content, nil
}
