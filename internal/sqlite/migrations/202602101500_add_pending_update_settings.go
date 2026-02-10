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
('pending_update_version', '', 'string', 'general', 'Version of a just-applied update (cleared after first read)', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
('pending_update_notes', '', 'string', 'general', 'Release notes of a just-applied update (cleared after first read)', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
`
			if _, err := db.ExecContext(ctx, sql); err != nil {
				return err
			}
			return nil
		},
		func(ctx context.Context, db *bun.DB) error {
			if _, err := db.ExecContext(ctx, `
DELETE FROM settings WHERE key IN ('pending_update_version', 'pending_update_notes');
`); err != nil {
				return err
			}
			return nil
		},
	)
}
