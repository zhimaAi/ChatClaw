package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			if _, err := db.ExecContext(ctx, `ALTER TABLE agents ADD COLUMN mcp_server_enabled_ids TEXT NOT NULL DEFAULT '[]'`); err != nil {
				return err
			}
			if _, err := db.ExecContext(ctx, `UPDATE agents SET mcp_server_enabled_ids = mcp_server_ids WHERE mcp_server_ids != '' AND mcp_server_ids != '[]'`); err != nil {
				return err
			}
			return nil
		},
		func(ctx context.Context, db *bun.DB) error {
			return nil
		},
	)
}
