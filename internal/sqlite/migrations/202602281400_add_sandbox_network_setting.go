package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			sql := `
INSERT OR IGNORE INTO settings (key, value, type, category, description, created_at, updated_at) VALUES
  ('workspace_sandbox_network', 'false', 'string', 'workspace', 'Allow network access in sandbox mode', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
`
			_, err := db.ExecContext(ctx, sql)
			return err
		},
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `DELETE FROM settings WHERE key = 'workspace_sandbox_network';`)
			return err
		},
	)
}
