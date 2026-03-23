package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			sql := `
CREATE TABLE IF NOT EXISTS chatwiki_bindings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    server_url TEXT NOT NULL DEFAULT '',
    token TEXT NOT NULL,
    ttl INTEGER NOT NULL DEFAULT 0,
    exp INTEGER NOT NULL DEFAULT 0,
    user_id TEXT NOT NULL,
    user_name TEXT NOT NULL DEFAULT '',
    chatwiki_version TEXT NOT NULL DEFAULT 'dev',
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);
`
			_, err := db.ExecContext(ctx, sql)
			return err
		},
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `DROP TABLE IF EXISTS chatwiki_bindings;`)
			return err
		},
	)
}
