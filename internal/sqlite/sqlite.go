package sqlite

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"sync"
	"time"

	"willclaw/internal/define"
	"willclaw/internal/sqlite/migrations"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	_ "github.com/mattn/go-sqlite3"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/migrate"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// DateTimeFormat 统一的时间格式（无时区）
const DateTimeFormat = "2006-01-02 15:04:05"

var (
	once   sync.Once
	db     *bun.DB
	dbPath string
)

func Path() string { return dbPath }
func DB() *bun.DB  { return db }

// NowUTC 返回当前 UTC 时间字符串，格式为 "2006-01-02 15:04:05"
func NowUTC() string {
	return time.Now().UTC().Format(DateTimeFormat)
}

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

	// Enable sqlite-vec extension (CGO version requires calling before Open)
	sqlite_vec.Auto()

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

	bunDB := bun.NewDB(sqlDB, sqlitedialect.New())

	// 运行迁移
	migrator := migrate.NewMigrator(bunDB, migrations.Migrations)
	if err := migrator.Init(ctx); err != nil {
		bunDB.Close()
		return err
	}
	if _, err := migrator.Migrate(ctx); err != nil {
		bunDB.Close()
		return err
	}

	db = bunDB
	return nil
}

func Close() error {
	if db == nil {
		return nil
	}
	err := db.Close()
	db = nil
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
