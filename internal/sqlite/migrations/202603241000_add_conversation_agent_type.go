package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `
ALTER TABLE conversations ADD COLUMN agent_type TEXT NOT NULL DEFAULT 'eino';
`)
			return err
		},
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `
ALTER TABLE conversations DROP COLUMN agent_type;
`)
			return err
		},
	)
}
