package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			sql := `
-- Add library_ids field to conversations table for conversation-level knowledge base selection
ALTER TABLE conversations ADD COLUMN library_ids TEXT NOT NULL DEFAULT '[]';
`
			if _, err := db.ExecContext(ctx, sql); err != nil {
				return err
			}
			return nil
		},
		func(ctx context.Context, db *bun.DB) error {
			// SQLite doesn't support DROP COLUMN directly, but we can leave it as is
			// since the down migration is rarely used in production
			return nil
		},
	)
}
