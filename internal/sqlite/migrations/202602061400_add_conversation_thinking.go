package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			sql := `
alter table conversations add column enable_thinking boolean not null default false;
`
			if _, err := db.ExecContext(ctx, sql); err != nil {
				return err
			}
			return nil
		},
		func(ctx context.Context, db *bun.DB) error {
			// SQLite does not support dropping columns directly, would need to recreate table
			// For now, we just leave the column in the rollback case
			return nil
		},
	)
}
