package memory

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"chatclaw/internal/define"
	"chatclaw/internal/services/memory/migrations"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	_ "github.com/mattn/go-sqlite3"
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

func DB() *bun.DB { return db }

func InitDB(app *application.App) error {
	var initErr error
	once.Do(func() { initErr = doInitDB(app) })
	return initErr
}

func doInitDB(app *application.App) error {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(cfgDir, define.AppID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	dbPath = filepath.Join(dir, "memory.db")
	if app != nil {
		app.Logger.Info("memory sqlite path", "path", dbPath)
	}

	// Enable sqlite-vec extension (CGO version requires calling before Open)
	sqlite_vec.Auto()

	sqlDB, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return err
	}

	sqlDB.SetMaxOpenConns(4)
	sqlDB.SetMaxIdleConns(4)
	sqlDB.SetConnMaxLifetime(0)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := sqlDB.PingContext(ctx); err != nil {
		sqlDB.Close()
		return err
	}

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

func CloseDB() error {
	if db == nil {
		return nil
	}
	err := db.Close()
	db = nil
	return err
}

// RebuildVectorTables rebuilds the vector virtual tables with the specified dimension.
func RebuildVectorTables(ctx context.Context, dimension int) error {
	if db == nil {
		return fmt.Errorf("memory db not initialized")
	}

	// thematic_facts_vec
	_, err := db.ExecContext(ctx, `DROP TABLE IF EXISTS thematic_facts_vec;`)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, fmt.Sprintf(`
CREATE VIRTUAL TABLE thematic_facts_vec USING vec0(
    id INTEGER PRIMARY KEY,
    embedding FLOAT[%d]
);`, dimension))
	if err != nil {
		return err
	}

	// event_streams_vec
	_, err = db.ExecContext(ctx, `DROP TABLE IF EXISTS event_streams_vec;`)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, fmt.Sprintf(`
CREATE VIRTUAL TABLE event_streams_vec USING vec0(
    id INTEGER PRIMARY KEY,
    embedding FLOAT[%d]
);`, dimension))
	if err != nil {
		return err
	}

	return nil
}
