package settings

import (
	"context"
	"database/sql"
	"sort"
	"strings"
	"sync"
	"time"

	"willchat/internal/errs"
	"willchat/internal/sqlite"

	"github.com/uptrace/bun"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// settingsCache 存储 settings key/value 的内存缓存（以及少量用于 List 过滤的元数据）
// 约束：写入必须同时写 DB + 更新缓存；读取只走缓存。
type settingsCache struct {
	mu sync.RWMutex

	values     map[string]string
	categories map[string]Category

	loaded bool
}

var globalSettingsCache = &settingsCache{
	values:     make(map[string]string),
	categories: make(map[string]Category),
}

// InitCache 启动时一次性加载 settings 到内存缓存。
// 必须在 sqlite.Init(app) 之后、app.Run() 之前调用。
func InitCache(app *application.App) error {
	db := sqlite.DB()
	if db == nil {
		return errs.New("error.sqlite_not_initialized")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	type row struct {
		Key      string         `bun:"key"`
		Value    sql.NullString `bun:"value"`
		Category string         `bun:"category"`
	}

	rows := make([]row, 0, 64)
	if err := db.NewSelect().
		Table("settings").
		Column("key", "value", "category").
		OrderExpr("category ASC, key ASC").
		Scan(ctx, &rows); err != nil {
		return errs.Wrap("error.setting_read_failed", err)
	}

	globalSettingsCache.mu.Lock()
	defer globalSettingsCache.mu.Unlock()

	// 重建（避免残留旧 key）
	globalSettingsCache.values = make(map[string]string, len(rows))
	globalSettingsCache.categories = make(map[string]Category, len(rows))
	for _, r := range rows {
		k := strings.TrimSpace(r.Key)
		if k == "" {
			continue
		}
		if r.Value.Valid {
			globalSettingsCache.values[k] = r.Value.String
		} else {
			globalSettingsCache.values[k] = ""
		}
		globalSettingsCache.categories[k] = Category(strings.TrimSpace(r.Category))
	}
	globalSettingsCache.loaded = true

	if app != nil {
		app.Logger.Info("settings cache loaded", "count", len(globalSettingsCache.values))
	}
	return nil
}

func cacheLoaded() bool {
	globalSettingsCache.mu.RLock()
	defer globalSettingsCache.mu.RUnlock()
	return globalSettingsCache.loaded
}

func getCachedValue(key string) (string, bool) {
	globalSettingsCache.mu.RLock()
	defer globalSettingsCache.mu.RUnlock()
	v, ok := globalSettingsCache.values[key]
	return v, ok
}

func getCachedCategory(key string) (Category, bool) {
	globalSettingsCache.mu.RLock()
	defer globalSettingsCache.mu.RUnlock()
	c, ok := globalSettingsCache.categories[key]
	return c, ok
}

func setCachedValue(key, value string) {
	globalSettingsCache.mu.Lock()
	defer globalSettingsCache.mu.Unlock()
	globalSettingsCache.values[key] = value
}

func listCachedKeys(category Category) []string {
	globalSettingsCache.mu.RLock()
	defer globalSettingsCache.mu.RUnlock()

	keys := make([]string, 0, len(globalSettingsCache.values))
	for k := range globalSettingsCache.values {
		if category == "" || globalSettingsCache.categories[k] == category {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

// GetValue 读取缓存中的值
func GetValue(key string) (string, bool) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", false
	}
	return getCachedValue(key)
}

// GetBool 从缓存中读取布尔值；缺失/非法时返回 defaultValue
func GetBool(key string, defaultValue bool) bool {
	v, ok := GetValue(key)
	if !ok {
		return defaultValue
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "1", "yes", "y", "on":
		return true
	case "false", "0", "no", "n", "off":
		return false
	default:
		return defaultValue
	}
}

// dbForWrite is a small helper for write paths.
func dbForWrite() (*bun.DB, error) {
	db := sqlite.DB()
	if db == nil {
		return nil, errs.New("error.sqlite_not_initialized")
	}
	return db, nil
}

