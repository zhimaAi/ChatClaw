package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			sql := `
CREATE TABLE IF NOT EXISTS installed_skills (
	slug         TEXT PRIMARY KEY,
	version      TEXT NOT NULL DEFAULT '',
	source       TEXT NOT NULL,
	enabled      BOOLEAN DEFAULT 1,
	installed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
`
			if _, err := db.ExecContext(ctx, sql); err != nil {
				return err
			}
			return nil
		},
		func(ctx context.Context, db *bun.DB) error {
			if _, err := db.ExecContext(ctx, `DROP TABLE IF EXISTS installed_skills;`); err != nil {
				return err
			}
			return nil
		},
	)
}
