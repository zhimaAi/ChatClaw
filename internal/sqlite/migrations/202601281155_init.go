package migrations

import (
	"context"
	"runtime"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			// NOTE: On Windows we use github.com/ncruces/go-sqlite3 (wazero/wasm) driver.
			// WAL has been observed to crash with "wasm error: out of bounds memory access" in some environments.
			// Use DELETE mode on Windows for stability.
			journalMode := "WAL"
			if runtime.GOOS == "windows" {
				journalMode = "DELETE"
			}
			if _, err := db.ExecContext(ctx, `PRAGMA journal_mode = `+journalMode+`;`); err != nil {
				return err
			}
			if _, err := db.ExecContext(ctx, `PRAGMA synchronous = NORMAL;`); err != nil {
				return err
			}
			return nil
		},
		func(ctx context.Context, db *bun.DB) error {
			_ = ctx
			_ = db
			return nil
		},
	)
}
