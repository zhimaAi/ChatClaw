package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `ALTER TABLE mcp_servers ADD COLUMN description TEXT NOT NULL DEFAULT '';`)
			return err
		},
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `ALTER TABLE mcp_servers DROP COLUMN description;`)
			return err
		},
	)
}
