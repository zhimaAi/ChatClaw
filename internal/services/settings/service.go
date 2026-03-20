package settings

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"chatclaw/internal/define"
	"chatclaw/internal/eino/embedding"
	"chatclaw/internal/errs"
	"chatclaw/internal/services/document"
	"chatclaw/internal/services/memory"
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

// GetSkillsDir returns the fixed skills directory path ($HOME/.chatclaw/skills).
func (s *SettingsService) GetSkillsDir() (string, error) {
	appDir, err := define.AppDataDir()
	if err != nil {
		return "", errs.Wrap("error.setting_read_failed", err)
	}
	return filepath.Join(appDir, "skills"), nil
}

// GetMCPDir returns the fixed MCP servers directory path ($HOME/.chatclaw/mcp).
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

type UpdateMemorySettingsInput struct {
	Enabled               bool   `json:"enabled"`
	ExtractProviderID     string `json:"extract_provider_id"`
	ExtractModelID        string `json:"extract_model_id"`
	EmbeddingProviderID   string `json:"embedding_provider_id"`
	EmbeddingModelID      string `json:"embedding_model_id"`
	EmbeddingDimension    int    `json:"embedding_dimension"`
}

func (s *SettingsService) UpdateMemorySettings(input UpdateMemorySettingsInput) error {
	// Remember old values to decide whether to trigger rebuild.
	oldDim, _ := GetValue("memory_embedding_dimension")
	oldProvider, _ := GetValue("memory_embedding_provider_id")
	oldModel, _ := GetValue("memory_embedding_model_id")

	db, err := dbForWrite()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	enabledStr := "false"
	if input.Enabled {
		enabledStr = "true"
	}

	updates := []struct {
		Key string
		Val string
	}{
		{Key: "memory_enabled", Val: enabledStr},
		{Key: "memory_extract_provider_id", Val: strings.TrimSpace(input.ExtractProviderID)},
		{Key: "memory_extract_model_id", Val: strings.TrimSpace(input.ExtractModelID)},
		{Key: "memory_embedding_provider_id", Val: strings.TrimSpace(input.EmbeddingProviderID)},
		{Key: "memory_embedding_model_id", Val: strings.TrimSpace(input.EmbeddingModelID)},
		{Key: "memory_embedding_dimension", Val: fmt.Sprintf("%d", input.EmbeddingDimension)},
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

	for _, u := range updates {
		setCachedValue(u.Key, u.Val)
	}

	changed := oldProvider != input.EmbeddingProviderID || oldModel != input.EmbeddingModelID || strings.TrimSpace(oldDim) != fmt.Sprintf("%d", input.EmbeddingDimension)
	if !changed || input.EmbeddingDimension <= 0 || !input.Enabled {
		return nil
	}

	// Trigger rebuild of memory vector tables
	go s.triggerRebuildMemoryVectors(input.EmbeddingDimension)
	return nil
}

func (s *SettingsService) triggerRebuildMemoryVectors(dimension int) {
	reembedMu.Lock()
	defer reembedMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	if err := memory.RebuildVectorTables(ctx, dimension); err != nil {
		s.app.Logger.Error("[memory] rebuild vector tables failed", "error", err)
		return
	}
	s.app.Logger.Info("[memory] vector tables rebuilt", "dimension", dimension)

	memDB := memory.DB()
	if memDB == nil {
		return
	}

	embProviderID, _ := GetValue("memory_embedding_provider_id")
	embModelID, _ := GetValue("memory_embedding_model_id")
	if embProviderID == "" || embModelID == "" {
		s.app.Logger.Warn("[memory] embedding model not configured, skip re-embed")
		return
	}

	mainDB := sqlite.DB()
	if mainDB == nil {
		return
	}

	type providerRow struct {
		Type        string `bun:"type"`
		APIKey      string `bun:"api_key"`
		APIEndpoint string `bun:"api_endpoint"`
		ExtraConfig string `bun:"extra_config"`
	}
	var prov providerRow
	if err := mainDB.NewSelect().Table("providers").
		Column("type", "api_key", "api_endpoint", "extra_config").
		Where("provider_id = ?", embProviderID).
		Scan(ctx, &prov); err != nil {
		s.app.Logger.Error("[memory] get embedding provider failed", "error", err)
		return
	}

	embedder, err := embedding.NewEmbedder(ctx, &embedding.ProviderConfig{
		ProviderType: prov.Type,
		APIKey:       prov.APIKey,
		APIEndpoint:  prov.APIEndpoint,
		ModelID:      embModelID,
		Dimension:    dimension,
		ExtraConfig:  prov.ExtraConfig,
	})
	if err != nil {
		s.app.Logger.Error("[memory] create embedder failed", "error", err)
		return
	}

	// Re-embed thematic facts
	var facts []memory.ThematicFact
	if err := memDB.NewSelect().Model(&facts).Scan(ctx); err != nil {
		s.app.Logger.Error("[memory] query thematic facts failed", "error", err)
	} else {
		for _, f := range facts {
			text := f.Topic + ": " + f.Content
			vecs, embErr := embedder.EmbedStrings(ctx, []string{text})
			if embErr != nil {
				s.app.Logger.Warn("[memory] embed thematic fact failed", "id", f.ID, "error", embErr)
				continue
			}
			if len(vecs) > 0 {
				vecJSON, _ := json.Marshal(vecs[0])
				if _, err := memDB.ExecContext(ctx, `INSERT INTO thematic_facts_vec(id, embedding) VALUES (?, ?)`, f.ID, string(vecJSON)); err != nil {
					s.app.Logger.Warn("[memory] insert thematic vec failed", "id", f.ID, "error", err)
				}
			}
		}
		s.app.Logger.Info("[memory] thematic facts re-embedded", "count", len(facts))
	}

	// Re-embed event streams
	var events []memory.EventStream
	if err := memDB.NewSelect().Model(&events).Scan(ctx); err != nil {
		s.app.Logger.Error("[memory] query event streams failed", "error", err)
	} else {
		for _, e := range events {
			vecs, embErr := embedder.EmbedStrings(ctx, []string{e.Content})
			if embErr != nil {
				s.app.Logger.Warn("[memory] embed event stream failed", "id", e.ID, "error", embErr)
				continue
			}
			if len(vecs) > 0 {
				vecJSON, _ := json.Marshal(vecs[0])
				if _, err := memDB.ExecContext(ctx, `INSERT INTO event_streams_vec(id, embedding) VALUES (?, ?)`, e.ID, string(vecJSON)); err != nil {
					s.app.Logger.Warn("[memory] insert event vec failed", "id", e.ID, "error", err)
				}
			}
		}
		s.app.Logger.Info("[memory] event streams re-embedded", "count", len(events))
	}

	s.app.Logger.Info("[memory] vector rebuild complete")
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

	// 1) Rebuild vec0 table to match new dimension
	// Best-effort: create a tmp table first, then swap to reduce downtime.
	tmpName := fmt.Sprintf("doc_vec_tmp_%d", time.Now().UnixNano())
	oldName := fmt.Sprintf("doc_vec_old_%d", time.Now().UnixNano())

	_, _ = db.ExecContext(ctx, fmt.Sprintf(`DROP TABLE IF EXISTS "%s";`, tmpName))
	_, _ = db.ExecContext(ctx, fmt.Sprintf(`DROP TABLE IF EXISTS "%s";`, oldName))

	_, err := db.ExecContext(ctx, fmt.Sprintf(
		`CREATE VIRTUAL TABLE "%s" USING vec0(id INTEGER PRIMARY KEY, content FLOAT[%d]);`,
		tmpName, dimension,
	))
	if err != nil {
		s.app.Logger.Error("create tmp doc_vec failed", "error", err)
		return
	}

	// Try rename-swap first (keeps old table if swap fails).
	_, errRenameOld := db.ExecContext(ctx, fmt.Sprintf(`ALTER TABLE doc_vec RENAME TO "%s";`, oldName))
	_, errRenameNew := db.ExecContext(ctx, fmt.Sprintf(`ALTER TABLE "%s" RENAME TO doc_vec;`, tmpName))
	if errRenameNew != nil {
		// Rollback: try restoring old table name if we renamed it.
		if errRenameOld == nil {
			_, _ = db.ExecContext(ctx, fmt.Sprintf(`ALTER TABLE "%s" RENAME TO doc_vec;`, oldName))
		}
		_, _ = db.ExecContext(ctx, fmt.Sprintf(`DROP TABLE IF EXISTS "%s";`, tmpName))

		// Fallback to drop+create (as last resort).
		_, _ = db.ExecContext(ctx, `DROP TABLE IF EXISTS doc_vec;`)
		_, err2 := db.ExecContext(ctx, fmt.Sprintf(
			`CREATE VIRTUAL TABLE IF NOT EXISTS doc_vec USING vec0(id INTEGER PRIMARY KEY, content FLOAT[%d]);`,
			dimension,
		))
		if err2 != nil {
			s.app.Logger.Error("fallback rebuild doc_vec failed", "error", err2)
		}
		return
	}
	// Drop old table after swap (ignore errors).
	if errRenameOld == nil {
		_, _ = db.ExecContext(ctx, fmt.Sprintf(`DROP TABLE IF EXISTS "%s";`, oldName))
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
