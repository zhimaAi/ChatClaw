package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			if _, err := db.ExecContext(ctx, `PRAGMA journal_mode = WAL;`); err != nil {
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

