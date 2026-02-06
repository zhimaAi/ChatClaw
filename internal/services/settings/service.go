package settings

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"willchat/internal/errs"
	"willchat/internal/services/document"
	"willchat/internal/sqlite"
	"willchat/internal/taskmanager"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// reembedMu protects against concurrent embedding config updates
var reembedMu sync.Mutex

// Category 设置分类
type Category string

const (
	CategoryGeneral Category = "general" // 常规设置
	CategorySnap    Category = "snap"    // 吸附设置
	CategoryTools   Category = "tools"   // 功能工具
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
	return CategoryGeneral
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
		if s.app != nil {
			s.app.Logger.Error("create tmp doc_vec failed", "error", err)
		}
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
		if err2 != nil && s.app != nil {
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
		if s.app != nil {
			s.app.Logger.Error("query documents failed", "error", err)
		}
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
			if s.app != nil {
				s.app.Logger.Error("update document for reembed failed", "docID", r.ID, "error", err)
			}
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
