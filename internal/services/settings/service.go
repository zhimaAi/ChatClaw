package settings

import (
	"context"
	"database/sql"
	"sort"
	"strings"
	"time"

	"willchat/internal/errs"

	"github.com/uptrace/bun"
	"github.com/wailsapp/wails/v3/pkg/application"
)

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
