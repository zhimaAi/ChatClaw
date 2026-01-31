package sqlite

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"sync"
	"time"

	"willchat/internal/define"
	"willchat/internal/sqlite/migrations"

	_ "github.com/asg017/sqlite-vec-go-bindings/ncruces"
	_ "github.com/ncruces/go-sqlite3/driver"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/migrate"
	"github.com/wailsapp/wails/v3/pkg/application"
)

var (
	once   sync.Once
	db     *bun.DB
	dbPath string
)

func Path() string { return dbPath }
func DB() *bun.DB  { return db }

func Init(app *application.App) error {
	var initErr error
	once.Do(func() { initErr = doInit(app) })
	return initErr
}

func doInit(app *application.App) error {
	path, err := resolveDBPath()
	if err != nil {
		return err
	}

	dbPath = path
	if app != nil {
		app.Logger.Info("sqlite path", "path", dbPath)
	}

	sqlDB, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return err
	}

	// SQLite WAL 模式下读写可并发，但写必须串行，设置少量连接即可
	sqlDB.SetMaxOpenConns(4)
	sqlDB.SetMaxIdleConns(4)
	sqlDB.SetConnMaxLifetime(0)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := sqlDB.PingContext(ctx); err != nil {
		sqlDB.Close()
		return err
	}

	// 连接级 PRAGMA
	if _, err := sqlDB.ExecContext(ctx, `PRAGMA busy_timeout = 5000;`); err != nil {
		sqlDB.Close()
		return err
	}
	if _, err := sqlDB.ExecContext(ctx, `PRAGMA foreign_keys = ON;`); err != nil {
		sqlDB.Close()
		return err
	}

	// 验证 sqlite-vec 扩展已加载
	var vecVersion string
	if err := sqlDB.QueryRowContext(ctx, `SELECT vec_version()`).Scan(&vecVersion); err != nil {
		sqlDB.Close()
		return err
	}
	if app != nil {
		app.Logger.Info("sqlite-vec loaded", "version", vecVersion)
	}

	bunDB := bun.NewDB(sqlDB, sqlitedialect.New())

	// 运行迁移
	migrator := migrate.NewMigrator(bunDB, migrations.Migrations)
	if err := migrator.Init(ctx); err != nil {
		bunDB.Close()
		return err
	}
	group, err := migrator.Migrate(ctx)
	if err != nil {
		bunDB.Close()
		return err
	}

	db = bunDB

	if app != nil && group != nil && !group.IsZero() {
		app.Logger.Info("sqlite migrated", "group", group.String())
	}

	return nil
}

func Close(app *application.App) error {
	if db == nil {
		return nil
	}
	err := db.Close()
	db = nil
	if err != nil && app != nil {
		app.Logger.Warn("sqlite close failed", "error", err)
	}
	return err
}

func resolveDBPath() (string, error) {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(cfgDir, define.AppID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, define.DefaultSQLiteFileName), nil
}
