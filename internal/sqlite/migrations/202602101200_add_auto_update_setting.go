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
('auto_update', 'true', 'boolean', 'general', 'Whether to automatically check and install updates', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
`
			if _, err := db.ExecContext(ctx, sql); err != nil {
				return err
			}
			return nil
		},
		func(ctx context.Context, db *bun.DB) error {
			if _, err := db.ExecContext(ctx, `
DELETE FROM settings WHERE key IN ('auto_update');
`); err != nil {
				return err
			}
			return nil
		},
	)
}
