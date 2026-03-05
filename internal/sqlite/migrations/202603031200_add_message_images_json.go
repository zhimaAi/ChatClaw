package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			sql := `
-- Add images_json field to messages table for storing image attachments as JSON
ALTER TABLE messages ADD COLUMN images_json TEXT NOT NULL DEFAULT '[]';
`
			if _, err := db.ExecContext(ctx, sql); err != nil {
				return err
			}
			return nil
		},
		func(ctx context.Context, db *bun.DB) error {
			// SQLite doesn't support DROP COLUMN directly.
			// For rollback, we keep the column but note that it's deprecated.
			// In practice, rollback is rarely needed for additive migrations.
			return nil
		},
	)
}
