package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `
				CREATE TABLE IF NOT EXISTS assistant_mcps (
					id          TEXT PRIMARY KEY,
					name        TEXT NOT NULL DEFAULT '',
					description TEXT NOT NULL DEFAULT '',
					enabled     INTEGER NOT NULL DEFAULT 1,
					port        INTEGER NOT NULL DEFAULT 0,
					token       TEXT NOT NULL DEFAULT '',
					tools       TEXT NOT NULL DEFAULT '[]',
					created_at  TEXT NOT NULL DEFAULT '',
					updated_at  TEXT NOT NULL DEFAULT ''
				);
			`)
			return err
		},
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `DROP TABLE IF EXISTS assistant_mcps;`)
			return err
		},
	)
}
