package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			sql := `
CREATE TABLE IF NOT EXISTS mcp_servers (
	id          TEXT PRIMARY KEY,
	name        TEXT NOT NULL,
	transport   TEXT NOT NULL DEFAULT 'stdio',
	command     TEXT NOT NULL DEFAULT '',
	args        TEXT NOT NULL DEFAULT '[]',
	env         TEXT NOT NULL DEFAULT '{}',
	url         TEXT NOT NULL DEFAULT '',
	headers     TEXT NOT NULL DEFAULT '{}',
	timeout     INTEGER NOT NULL DEFAULT 30,
	enabled     BOOLEAN DEFAULT 1,
	created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
`
			if _, err := db.ExecContext(ctx, sql); err != nil {
				return err
			}
			return nil
		},
		func(ctx context.Context, db *bun.DB) error {
			if _, err := db.ExecContext(ctx, `DROP TABLE IF EXISTS mcp_servers;`); err != nil {
				return err
			}
			return nil
		},
	)
}
