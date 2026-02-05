package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			sql := `
-- Add segments column to messages table for storing interleaved content/tool-call order
-- Format: JSON array of {type: "content"|"tools", content?: string, tool_call_ids?: string[]}
ALTER TABLE messages ADD COLUMN segments TEXT NOT NULL DEFAULT '[]';
`
			if _, err := db.ExecContext(ctx, sql); err != nil {
				return err
			}
			return nil
		},
		func(ctx context.Context, db *bun.DB) error {
			// SQLite doesn't support DROP COLUMN directly, but we can leave it for rollback
			// In production, you'd need to recreate the table without the column
			return nil
		},
	)
}
