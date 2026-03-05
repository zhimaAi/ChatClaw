package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			sql := `
ALTER TABLE conversations ADD COLUMN external_id TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_conversations_external_id ON conversations(agent_id, external_id);
`
			if _, err := db.ExecContext(ctx, sql); err != nil {
				return err
			}
			return nil
		},
		func(ctx context.Context, db *bun.DB) error {
			sql := `
DROP INDEX IF EXISTS idx_conversations_external_id;
ALTER TABLE conversations DROP COLUMN external_id;
`
			if _, err := db.ExecContext(ctx, sql); err != nil {
				return err
			}
			return nil
		},
	)
}
