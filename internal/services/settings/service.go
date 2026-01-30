package settings

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"willchat/internal/errs"
	"willchat/internal/sqlite"

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
	db := sqlite.DB()
	if db == nil {
		return nil, errs.New("error.sqlite_not_initialized")
	}
	return db, nil
}

func (s *SettingsService) List(category Category) ([]Setting, error) {
	db, err := s.db()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	cat := strings.TrimSpace(string(category))

	models := make([]settingModel, 0)
	q := db.NewSelect().
		Model(&models).
		OrderExpr("category ASC, key ASC")
	if cat != "" {
		q = q.Where("category = ?", cat)
	}
	if err := q.Scan(ctx); err != nil {
		return nil, errs.Wrap("error.setting_read_failed", err)
	}

	out := make([]Setting, 0, len(models))
	for i := range models {
		out = append(out, models[i].toDTO())
	}
	return out, nil
}

func (s *SettingsService) Get(key string) (*Setting, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, errs.New("error.setting_key_required")
	}

	db, err := s.db()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var m settingModel
	err = db.NewSelect().Model(&m).Where("key = ?", key).Limit(1).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.Newf("error.setting_not_found", map[string]any{"Key": key})
		}
		return nil, errs.Wrap("error.setting_read_failed", err)
	}
	it := m.toDTO()
	return &it, nil
}

func (s *SettingsService) SetValue(key string, value string) (*Setting, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, errs.New("error.setting_key_required")
	}

	db, err := s.db()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// 只更新 value 字段，不改变其他元数据
	result, err := db.NewUpdate().
		Model((*settingModel)(nil)).
		Set("value = ?", value).
		Set("updated_at = ?", time.Now().UTC()).
		Where("key = ?", key).
		Exec(ctx)
	if err != nil {
		return nil, errs.Wrap("error.setting_write_failed", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return nil, errs.Newf("error.setting_not_found", map[string]any{"Key": key})
	}

	return s.Get(key)
}
