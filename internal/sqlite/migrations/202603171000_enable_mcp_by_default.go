package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `
UPDATE settings SET value = 'true', updated_at = CURRENT_TIMESTAMP
WHERE key = 'mcp_enabled';
`)
			return err
		},
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `
UPDATE settings SET value = 'false', updated_at = CURRENT_TIMESTAMP
WHERE key = 'mcp_enabled';
`)
			return err
		},
	)
}
