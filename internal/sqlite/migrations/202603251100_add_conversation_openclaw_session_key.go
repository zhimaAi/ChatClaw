package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			sql := `
ALTER TABLE conversations ADD COLUMN openclaw_session_key TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_conversations_openclaw_session_key ON conversations(openclaw_session_key);
`
			if _, err := db.ExecContext(ctx, sql); err != nil {
				return err
			}
			return nil
		},
		func(ctx context.Context, db *bun.DB) error {
			sql := `
DROP INDEX IF EXISTS idx_conversations_openclaw_session_key;
ALTER TABLE conversations DROP COLUMN openclaw_session_key;
`
			if _, err := db.ExecContext(ctx, sql); err != nil {
				return err
			}
			return nil
		},
	)
}
