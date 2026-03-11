package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			sql := `
ALTER TABLE agents ADD COLUMN mcp_enabled BOOLEAN NOT NULL DEFAULT 1;
ALTER TABLE agents ADD COLUMN mcp_server_ids TEXT NOT NULL DEFAULT '[]';
`
			if _, err := db.ExecContext(ctx, sql); err != nil {
				return err
			}
			return nil
		},
		func(ctx context.Context, db *bun.DB) error {
			// SQLite doesn't support DROP COLUMN in all versions; skip rollback
			return nil
		},
	)
}
