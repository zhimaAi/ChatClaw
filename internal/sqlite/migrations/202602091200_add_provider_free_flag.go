package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

// 202602091200_add_provider_free_flag
// Add `is_free` flag to providers table and mark ChatWiki as free provider.
func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			// Add is_free column with default false for existing rows.
			if _, err := db.ExecContext(ctx, `
ALTER TABLE providers
ADD COLUMN is_free boolean NOT NULL DEFAULT 0;
`); err != nil {
				return err
			}

			// Mark ChatWiki provider as free.
			if _, err := db.ExecContext(ctx, `
UPDATE providers
SET is_free = 1
WHERE provider_id = 'chatwiki';
`); err != nil {
				return err
			}

			return nil
		},
		func(ctx context.Context, db *bun.DB) error {
			// Rollback: keep column for backward compatibility, just reset flags.
			_, err := db.ExecContext(ctx, `
UPDATE providers
SET is_free = 0;
`)
			return err
		},
	)
}

