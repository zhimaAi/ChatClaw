package settings

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"chatclaw/internal/define"
	"chatclaw/internal/eino/processor"
	"chatclaw/internal/errs"
	"chatclaw/internal/services/document"
	"chatclaw/internal/sqlite"
	"chatclaw/internal/taskmanager"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// reembedMu protects against concurrent embedding config updates
var reembedMu sync.Mutex

// Category 设置分类
type Category string

const (
	CategoryGeneral   Category = "general"   // 常规设置
	CategorySnap      Category = "snap"      // 吸附设置
	CategoryTools     Category = "tools"     // 功能工具
	CategoryWorkspace Category = "workspace" // 工作区设置
	CategorySkills    Category = "skills"    // 技能设置
	CategoryMCP       Category = "mcp"       // MCP 设置
	CategoryOpenClaw  Category = "openclaw"  // OpenClaw 设置
)

type Setting struct {
	Key         string    `json:"key"`
	Value       string    `json:"value"`
	Type        string    `json:"type"`
	Category    Category  `json:"category"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// SettingsService 设置服务（暴露给前端调用）
type SettingsService struct {
	app *application.App
}

func NewSettingsService(app *application.App) *SettingsService {
	return &SettingsService{app: app}
}

func (s *SettingsService) db() (*bun.DB, error) {
	return dbForWrite()
}

func (s *SettingsService) List(category Category) ([]Setting, error) {
	// 读取只走缓存
	if !cacheLoaded() {
		return nil, errs.New("error.setting_cache_not_initialized")
	}

	cat := Category(strings.TrimSpace(string(category)))
	keys := listCachedKeys(cat)

	out := make([]Setting, 0, len(keys))
	for _, k := range keys {
		v, _ := getCachedValue(k)
		c, _ := getCachedCategory(k)
		out = append(out, Setting{
			Key:      k,
			Value:    v,
			Category: c,
		})
	}

	// 保持原先的排序语义：category ASC, key ASC
	sort.Slice(out, func(i, j int) bool {
		if out[i].Category == out[j].Category {
			return out[i].Key < out[j].Key
		}
		return out[i].Category < out[j].Category
	})
	return out, nil
}

func (s *SettingsService) Get(key string) (*Setting, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, errs.New("error.setting_key_required")
	}

	// 读取只走缓存
	if !cacheLoaded() {
		return nil, errs.New("error.setting_cache_not_initialized")
	}
	v, ok := getCachedValue(key)
	if !ok {
		return nil, errs.Newf("error.setting_not_found", map[string]any{"Key": key})
	}
	cat, _ := getCachedCategory(key)
	out := &Setting{
		Key:      key,
		Value:    v,
		Category: cat,
	}
	return out, nil
}

func (s *SettingsService) SetValue(key string, value string) (*Setting, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, errs.New("error.setting_key_required")
	}

	// 写入：先写 DB，再更新缓存
	db, err := dbForWrite()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// 只更新 value 字段（BeforeUpdate hook 会自动设置 updated_at）
	result, err := db.NewUpdate().
		Model((*settingModel)(nil)).
		Set("value = ?", value).
		Where("key = ?", key).
		Exec(ctx)
	if err != nil {
		return nil, errs.Wrap("error.setting_write_failed", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		// Setting doesn't exist, create it with default category based on key prefix
		category := inferCategoryFromKey(key)
		model := &settingModel{
			Key:      key,
			Value:    toNullString(value),
			Type:     "string",
			Category: string(category),
		}
		_, err = db.NewInsert().Model(model).Exec(ctx)
		if err != nil {
			return nil, errs.Wrap("error.setting_write_failed", err)
		}
		// Update cache with category info
		setCachedValueWithCategory(key, value, category)
		return &Setting{
			Key:      key,
			Value:    value,
			Type:     "string",
			Category: category,
		}, nil
	}

	setCachedValue(key, value)
	return s.Get(key)
}

// inferCategoryFromKey determines the category based on the key prefix
func inferCategoryFromKey(key string) Category {
	if strings.HasPrefix(key, "snap_") {
		return CategorySnap
	}
	if strings.HasPrefix(key, "tools_") || strings.HasPrefix(key, "tray_") || strings.HasPrefix(key, "float_") || strings.HasPrefix(key, "selection_") {
		return CategoryTools
	}
	if strings.HasPrefix(key, "workspace_") {
		return CategoryWorkspace
	}
	if strings.HasPrefix(key, "skills_") {
		return CategorySkills
	}
	if strings.HasPrefix(key, "mcp_") {
		return CategoryMCP
	}
	if strings.HasPrefix(key, "openclaw_") {
		return CategoryOpenClaw
	}
	return CategoryGeneral
}

// UpdateWorkspaceSettingsInput holds workspace configuration fields.
type UpdateWorkspaceSettingsInput struct {
	SandboxMode string `json:"sandbox_mode"` // "codex" or "native"
	WorkDir     string `json:"work_dir"`
}

// UpdateWorkspaceSettings saves workspace sandbox mode and working directory.
func (s *SettingsService) UpdateWorkspaceSettings(input UpdateWorkspaceSettingsInput) error {
	mode := strings.TrimSpace(input.SandboxMode)
	if mode == "" {
		mode = "codex"
	}
	if mode != "codex" && mode != "native" {
		return errs.Newf("error.setting_invalid_value", map[string]any{"Key": "workspace_sandbox_mode"})
	}

	workDir := strings.TrimSpace(input.WorkDir)

	db, err := dbForWrite()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	updates := []struct {
		Key string
		Val string
	}{
		{Key: "workspace_sandbox_mode", Val: mode},
		{Key: "workspace_work_dir", Val: workDir},
	}

	if err := db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		for _, u := range updates {
			result, err := tx.NewUpdate().
				Model((*settingModel)(nil)).
				Set("value = ?", u.Val).
				Where("key = ?", u.Key).
				Exec(ctx)
			if err != nil {
				return errs.Wrap("error.setting_write_failed", err)
			}
			rows, _ := result.RowsAffected()
			if rows == 0 {
				category := inferCategoryFromKey(u.Key)
				model := &settingModel{
					Key:      u.Key,
					Value:    toNullString(u.Val),
					Type:     "string",
					Category: string(category),
				}
				if _, err := tx.NewInsert().Model(model).Exec(ctx); err != nil {
					return errs.Wrap("error.setting_write_failed", err)
				}
			}
		}
		return nil
	}); err != nil {
		return err
	}

	for _, u := range updates {
		setCachedValueWithCategory(u.Key, u.Val, inferCategoryFromKey(u.Key))
	}
	return nil
}

// GetWorkspaceSettings returns the current workspace configuration.
func (s *SettingsService) GetWorkspaceSettings() (*UpdateWorkspaceSettingsInput, error) {
	mode, _ := GetValue("workspace_sandbox_mode")
	if mode == "" {
		mode = "codex"
	}
	workDir, _ := GetValue("workspace_work_dir")

	return &UpdateWorkspaceSettingsInput{
		SandboxMode: mode,
		WorkDir:     workDir,
	}, nil
}

// GetSkillsDir returns the fixed skills directory path ($HOME/.chatclaw/native/skills).
func (s *SettingsService) GetSkillsDir() (string, error) {
	appDir, err := define.AppDataDir()
	if err != nil {
		return "", errs.Wrap("error.setting_read_failed", err)
	}
	return filepath.Join(appDir, "skills"), nil
}

// GetMCPDir returns the fixed MCP servers directory path ($HOME/.chatclaw/native/mcp).
func (s *SettingsService) GetMCPDir() (string, error) {
	appDir, err := define.AppDataDir()
	if err != nil {
		return "", errs.Wrap("error.setting_read_failed", err)
	}
	dir := filepath.Join(appDir, "mcp")
	_ = os.MkdirAll(dir, 0o755)
	return dir, nil
}

// toNullString converts a string to sql.NullString
func toNullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{Valid: false}
	}
	return sql.NullString{String: s, Valid: true}
}

// UpdateEmbeddingConfig updates global embedding provider/model/dimension and triggers re-embedding for all documents.
// It rebuilds the vec0 table with the new dimension, then submits embedding-only jobs for every document.
type UpdateEmbeddingConfigInput struct {
	ProviderID string `json:"provider_id"`
	ModelID    string `json:"model_id"`
	Dimension  int    `json:"dimension"`
}

func (s *SettingsService) UpdateEmbeddingConfig(input UpdateEmbeddingConfigInput) error {
	providerID := strings.TrimSpace(input.ProviderID)
	modelID := strings.TrimSpace(input.ModelID)
	if providerID == "" || modelID == "" {
		return errs.New("error.setting_key_required")
	}
	if input.Dimension <= 0 {
		return errs.New("error.setting_value_required")
	}

	// Remember old values to decide whether to trigger rebuild.
	oldProvider, _ := GetValue("embedding_provider_id")
	oldModel, _ := GetValue("embedding_model_id")
	oldDim, _ := GetValue("embedding_dimension")

	db, err := dbForWrite()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := processor.ResolveEmbeddingConfig(ctx, db, providerID, modelID, input.Dimension); err != nil {
		return err
	}

	// Update in a transaction to keep config consistent.
	updates := []struct {
		Key string
		Val string
	}{
		{Key: "embedding_provider_id", Val: providerID},
		{Key: "embedding_model_id", Val: modelID},
		{Key: "embedding_dimension", Val: fmt.Sprintf("%d", input.Dimension)},
	}
	if err := db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		for _, u := range updates {
			result, err := tx.NewUpdate().
				Model((*settingModel)(nil)).
				Set("value = ?", u.Val).
				Where("key = ?", u.Key).
				Exec(ctx)
			if err != nil {
				return errs.Wrap("error.setting_write_failed", err)
			}
			rows, _ := result.RowsAffected()
			if rows == 0 {
				return errs.Newf("error.setting_not_found", map[string]any{"Key": u.Key})
			}
		}
		return nil
	}); err != nil {
		return err
	}

	// Update cache after transaction commits.
	for _, u := range updates {
		setCachedValue(u.Key, u.Val)
	}

	changed := oldProvider != providerID || oldModel != modelID || strings.TrimSpace(oldDim) != fmt.Sprintf("%d", input.Dimension)
	if !changed {
		return nil
	}

	// Fire-and-forget: rebuild vector table and submit re-embedding tasks.
	go s.triggerReembedAllDocuments(input.Dimension)
	return nil
}

func (s *SettingsService) triggerReembedAllDocuments(dimension int) {
	// Acquire lock to prevent concurrent rebuilds (e.g. rapid config changes)
	reembedMu.Lock()
	defer reembedMu.Unlock()

	db := sqlite.DB()
	if db == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 1) Rebuild vec0 table to match the configured dimension.
	// vec0 uses shadow tables under the hood; rename-swap can leave them behind
	// and produce blob-size mismatches later. We therefore fully clean doc_vec*
	// artifacts before recreating the table.
	if err := rebuildDocVecTable(ctx, db, dimension); err != nil {
		s.app.Logger.Error("rebuild doc_vec failed", "error", err)
		return
	}

	// 2) Submit embedding-only jobs for all documents
	type row struct {
		ID        int64 `bun:"id"`
		LibraryID int64 `bun:"library_id"`
	}
	rows := make([]row, 0, 1024)
	if err := db.NewSelect().
		Table("documents").
		Column("id", "library_id").
		OrderExpr("id DESC").
		Scan(ctx, &rows); err != nil {
		s.app.Logger.Error("query documents failed", "error", err)
		return
	}

	tm := taskmanager.Get()
	if tm == nil {
		return
	}

	for _, r := range rows {
		runID := uuid.New().String()
		// update run id + reset embedding fields
		if _, err := db.NewUpdate().
			Table("documents").
			Set("processing_run_id = ?", runID).
			Set("embedding_status = ?", document.StatusPending).
			Set("embedding_progress = ?", 0).
			Set("embedding_error = ?", "").
			Where("id = ?", r.ID).
			Exec(ctx); err != nil {
			s.app.Logger.Error("update document for reembed failed", "docID", r.ID, "error", err)
		}

		jobData, _ := json.Marshal(document.ProcessJobData{
			DocID:     r.ID,
			LibraryID: r.LibraryID,
			RunID:     runID,
		})
		taskKey := fmt.Sprintf("doc:%d", r.ID)
		tm.Submit(taskmanager.QueueDocument, document.JobTypeReembed, taskKey, runID, jobData)
	}
}

// RepairEmbeddingIndexIfNeeded checks whether the sqlite-vec backing tables are
// inconsistent with the current embedding dimension and, if so, rebuilds the
// index and queues a global re-embedding pass.
func (s *SettingsService) RepairEmbeddingIndexIfNeeded() {
	db := sqlite.DB()
	if db == nil {
		return
	}

	dimension := GetInt("embedding_dimension", 0)
	if dimension <= 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	needsRepair, reason, err := inspectDocVecState(ctx, db, dimension)
	if err != nil {
		s.app.Logger.Warn("inspect doc_vec state failed", "error", err)
		return
	}
	if !needsRepair {
		return
	}

	cfg, err := processor.GetEmbeddingConfig(ctx, db)
	if err != nil {
		s.app.Logger.Warn("doc_vec needs repair but embedding config is unavailable", "reason", reason, "error", err)
		return
	}

	s.app.Logger.Warn("detected inconsistent doc_vec index; scheduling rebuild", "reason", reason, "dimension", cfg.Dimension)
	go s.triggerReembedAllDocuments(cfg.Dimension)
}

type sqliteMasterRow struct {
	Name string         `bun:"name"`
	SQL  sql.NullString `bun:"sql"`
}

func inspectDocVecState(ctx context.Context, db *bun.DB, expectedDimension int) (bool, string, error) {
	rows := make([]sqliteMasterRow, 0, 8)
	if err := db.NewRaw(`
		SELECT name, sql
		FROM sqlite_master
		WHERE type = 'table' AND (name = 'doc_vec' OR name LIKE 'doc_vec_%')
		ORDER BY name ASC
	`).Scan(ctx, &rows); err != nil {
		return false, "", err
	}

	presentNames := make(map[string]struct{}, len(rows))
	reasons := make([]string, 0, 4)
	declaredDimension := 0

	for _, row := range rows {
		presentNames[row.Name] = struct{}{}
		if row.Name == "doc_vec" {
			declaredDimension = parseVecDimension(row.SQL.String)
		}
		if strings.HasPrefix(row.Name, "doc_vec_old_") || strings.HasPrefix(row.Name, "doc_vec_tmp_") {
			reasons = append(reasons, fmt.Sprintf("found stale vec artifact %s", row.Name))
		}
	}

	if _, ok := presentNames["doc_vec"]; !ok {
		reasons = append(reasons, "doc_vec table missing")
	}
	if declaredDimension <= 0 {
		reasons = append(reasons, "doc_vec dimension unreadable")
	} else if expectedDimension > 0 && declaredDimension != expectedDimension {
		reasons = append(reasons, fmt.Sprintf("doc_vec dimension=%d expected=%d", declaredDimension, expectedDimension))
	}

	requiredShadowTables := []string{
		"doc_vec_chunks",
		"doc_vec_info",
		"doc_vec_rowids",
		"doc_vec_vector_chunks00",
	}
	for _, name := range requiredShadowTables {
		if _, ok := presentNames[name]; !ok {
			reasons = append(reasons, fmt.Sprintf("missing shadow table %s", name))
		}
	}

	if declaredDimension > 0 {
		expectedChunkBytes := declaredDimension * 4 * 1024
		var badChunk int
		err := db.NewRaw(`
			SELECT EXISTS(
				SELECT 1
				FROM doc_vec_vector_chunks00
				WHERE length(vectors) <> ?
				LIMIT 1
			)
		`, expectedChunkBytes).Scan(ctx, &badChunk)
		if err != nil && !isMissingSQLiteObject(err) {
			return false, "", err
		}
		if badChunk == 1 {
			reasons = append(reasons, fmt.Sprintf("vector chunk blob size does not match dimension %d", declaredDimension))
		}
	}

	if len(reasons) == 0 {
		return false, "", nil
	}
	return true, strings.Join(reasons, "; "), nil
}

func rebuildDocVecTable(ctx context.Context, db *bun.DB, dimension int) error {
	if dimension <= 0 {
		return errors.New("invalid embedding dimension")
	}

	// Drop the main virtual table first so sqlite-vec can clean up the active
	// shadow tables it still tracks internally.
	_, _ = db.ExecContext(ctx, `DROP TABLE IF EXISTS "doc_vec";`)

	rows := make([]sqliteMasterRow, 0, 8)
	if err := db.NewRaw(`
		SELECT name, sql
		FROM sqlite_master
		WHERE type = 'table' AND (name = 'doc_vec' OR name LIKE 'doc_vec_%')
		ORDER BY name DESC
	`).Scan(ctx, &rows); err != nil {
		return fmt.Errorf("list doc_vec artifacts: %w", err)
	}

	for _, row := range rows {
		name := row.Name
		if name == "" {
			continue
		}
		if _, err := db.ExecContext(ctx, fmt.Sprintf(`DROP TABLE IF EXISTS "%s";`, escapeSQLiteIdentifier(name))); err != nil {
			return fmt.Errorf("drop %s: %w", name, err)
		}
	}

	if _, err := db.ExecContext(ctx, fmt.Sprintf(
		`CREATE VIRTUAL TABLE "doc_vec" USING vec0(id INTEGER PRIMARY KEY, content FLOAT[%d]);`,
		dimension,
	)); err != nil {
		return fmt.Errorf("create doc_vec: %w", err)
	}
	return nil
}

func parseVecDimension(createSQL string) int {
	createSQL = strings.TrimSpace(createSQL)
	if createSQL == "" {
		return 0
	}

	upper := strings.ToUpper(createSQL)
	idx := strings.Index(upper, "FLOAT[")
	if idx < 0 {
		return 0
	}
	start := idx + len("FLOAT[")
	end := strings.IndexByte(upper[start:], ']')
	if end < 0 {
		return 0
	}

	dim, err := strconv.Atoi(strings.TrimSpace(createSQL[start : start+end]))
	if err != nil || dim <= 0 {
		return 0
	}
	return dim
}

func escapeSQLiteIdentifier(name string) string {
	return strings.ReplaceAll(name, `"`, `""`)
}

func isMissingSQLiteObject(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such table") || strings.Contains(msg, "no such module")
}
