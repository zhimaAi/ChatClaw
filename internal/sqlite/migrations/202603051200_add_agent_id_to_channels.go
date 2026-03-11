package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `ALTER TABLE channels ADD COLUMN agent_id INTEGER NOT NULL DEFAULT 0`)
			return err
		},
		func(ctx context.Context, db *bun.DB) error {
			return nil
		},
	)
}
