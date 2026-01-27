package db

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"willchat/internal/define"
	"willchat/internal/db/migrations"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
	"github.com/uptrace/bun/migrate"
	"github.com/wailsapp/wails/v3/pkg/application"
)

var (
	mu sync.Mutex

	sqlReadDB  *sql.DB
	sqlWriteDB *sql.DB
	bunReadDB  *bun.DB
	bunWriteDB *bun.DB

	dbPath string
)

type sqliteConfig struct {
	BusyTimeoutMs int
	ForeignKeys   bool
}

func defaultSQLiteConfig() sqliteConfig {
	return sqliteConfig{
		BusyTimeoutMs: 5000,
		ForeignKeys:   true,
	}
}

const (
	// 读连接
	defaultMaxReadConns = 4
	
	// 写连接
	defaultMaxWriteConns = 1
)

func Path() string {
	mu.Lock()
	defer mu.Unlock()
	return dbPath
}

// WriteDB 用于写入/事务
func WriteDB() *bun.DB {
	mu.Lock()
	defer mu.Unlock()
	return bunWriteDB
}

// ReadDB 用于只读查询
func ReadDB() *bun.DB {
	mu.Lock()
	defer mu.Unlock()
	return bunReadDB
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

	return filepath.Join(dir, define.DefaultDBFileName), nil
}

func openSQLite(path string) (*sql.DB, error) {
	return sql.Open(sqliteshim.ShimName, path)
}

func configureSQLitePool(sqldb *sql.DB, maxOpenConns int) {
	if maxOpenConns <= 0 {
		maxOpenConns = 1
	}
	sqldb.SetMaxOpenConns(maxOpenConns)
	sqldb.SetMaxIdleConns(maxOpenConns)
	sqldb.SetConnMaxLifetime(0)
}

type sqliteExecer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func applySQLitePragmas(ctx context.Context, execer sqliteExecer, cfg sqliteConfig) error {
	if cfg.BusyTimeoutMs > 0 {
		_, _ = execer.ExecContext(ctx, `PRAGMA busy_timeout = `+strconv.Itoa(cfg.BusyTimeoutMs)+`;`)
	}

	if cfg.ForeignKeys {
		if _, err := execer.ExecContext(ctx, `PRAGMA foreign_keys = ON;`); err != nil {
			return err
		}
	}

	return nil
}

func readJournalMode(ctx context.Context, sqldb *sql.DB) string {
	journalMode := ""
	_ = sqldb.QueryRowContext(ctx, `PRAGMA journal_mode;`).Scan(&journalMode)
	return journalMode
}

func warmUpSQLitePool(ctx context.Context, sqldb *sql.DB, cfg sqliteConfig, connections int) error {
	// 预热：显式取出连接并设置 PRAGMA，再归还到池里。
	// 这样后续并发读更容易直接复用“已初始化的连接”，避免第一次使用时才设置导致的抖动。
	for i := 0; i < connections; i++ {
		conn, err := sqldb.Conn(ctx)
		if err != nil {
			return err
		}
		if err := applySQLitePragmas(ctx, conn, cfg); err != nil {
			_ = conn.Close()
			return err
		}
		_ = conn.Close()
	}
	return nil
}

func runMigrations(ctx context.Context, db *bun.DB) (*migrate.MigrationGroup, error) {
	migrator := migrate.NewMigrator(db, migrations.Migrations)
	if err := migrator.Init(ctx); err != nil {
		return nil, err
	}
	return migrator.Migrate(ctx)
}

// Init 打开数据库并执行迁移
// 该方法可重复调用（幂等）。
func Init(app *application.App) error {
	mu.Lock()
	defer mu.Unlock()

	if bunWriteDB != nil {
		return nil
	}

	path, err := resolveDBPath()
	if err != nil {
		return err
	}
	dbPath = path
	if app != nil {
		app.Logger.Info("db path", "path", dbPath)
	}

	writeSQL, err := openSQLite(dbPath)
	if err != nil {
		return err
	}
	configureSQLitePool(writeSQL, defaultMaxWriteConns)

	readSQL, err := openSQLite(dbPath)
	if err != nil {
		_ = writeSQL.Close()
		return err
	}
	configureSQLitePool(readSQL, defaultMaxReadConns)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 基础连通性检查
	if err := writeSQL.PingContext(ctx); err != nil {
		_ = writeSQL.Close()
		_ = readSQL.Close()
		return err
	}
	if err := readSQL.PingContext(ctx); err != nil {
		_ = writeSQL.Close()
		_ = readSQL.Close()
		return err
	}

	cfg := defaultSQLiteConfig()

	// 连接级 PRAGMA：需要对每条连接生效（读写池都要设置）。
	if err := applySQLitePragmas(ctx, writeSQL, cfg); err != nil {
		_ = writeSQL.Close()
		_ = readSQL.Close()
		return err
	}
	if err := applySQLitePragmas(ctx, readSQL, cfg); err != nil {
		_ = writeSQL.Close()
		_ = readSQL.Close()
		return err
	}

	// 预热连接池：让每条连接都初始化好“连接级 PRAGMA”，避免并发读时用到“裸连接”。
	if err := warmUpSQLitePool(ctx, writeSQL, cfg, defaultMaxWriteConns); err != nil {
		_ = writeSQL.Close()
		_ = readSQL.Close()
		return err
	}
	if err := warmUpSQLitePool(ctx, readSQL, cfg, defaultMaxReadConns); err != nil {
		_ = writeSQL.Close()
		_ = readSQL.Close()
		return err
	}

	writeBun := bun.NewDB(writeSQL, sqlitedialect.New())
	readBun := bun.NewDB(readSQL, sqlitedialect.New())

	// 迁移只走写库
	group, err := runMigrations(ctx, writeBun)
	if err != nil {
		_ = writeBun.Close()
		_ = readBun.Close()
		return err
	}

	// journal_mode/synchronous 属于数据库策略，已在迁移中设置；
	// 这里在迁移之后读取一次用于日志校验。
	journalMode := readJournalMode(ctx, writeSQL)

	sqlWriteDB = writeSQL
	sqlReadDB = readSQL
	bunWriteDB = writeBun
	bunReadDB = readBun

	if app != nil {
		if journalMode != "" {
			if journalMode == "wal" || journalMode == "WAL" {
				app.Logger.Info("sqlite journal_mode", "mode", journalMode)
			} else {
				app.Logger.Warn("sqlite journal_mode not wal", "mode", journalMode)
			}
		}
		if group != nil && !group.IsZero() {
			app.Logger.Info("db migrated", "path", dbPath, "group", group.String())
		} else {
			app.Logger.Debug("db migration up-to-date", "path", dbPath)
		}
	}

	return nil
}

func Close(app *application.App) error {
	mu.Lock()
	defer mu.Unlock()

	if bunWriteDB == nil {
		return nil
	}
	errWrite := bunWriteDB.Close()
	errRead := error(nil)
	if bunReadDB != nil {
		errRead = bunReadDB.Close()
	}

	sqlWriteDB = nil
	sqlReadDB = nil
	bunWriteDB = nil
	bunReadDB = nil

	if app != nil {
		if errWrite != nil && !errors.Is(errWrite, sql.ErrConnDone) {
			app.Logger.Warn("db close failed (write)", "error", errWrite)
		}
		if errRead != nil && !errors.Is(errRead, sql.ErrConnDone) {
			app.Logger.Warn("db close failed (read)", "error", errRead)
		}
	}

	if errWrite != nil {
		return errWrite
	}
	return errRead
}
