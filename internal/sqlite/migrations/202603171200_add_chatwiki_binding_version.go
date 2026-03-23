package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			var colCount int
			if err := db.QueryRowContext(ctx,
				`SELECT COUNT(1) FROM pragma_table_info('chatwiki_bindings') WHERE name = 'chatwiki_version'`,
			).Scan(&colCount); err != nil {
				return err
			}
			if colCount > 0 {
				return nil
			}

			_, err := db.ExecContext(ctx, `
ALTER TABLE chatwiki_bindings
ADD COLUMN chatwiki_version TEXT NOT NULL DEFAULT 'dev';
`)
			return err
		},
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS chatwiki_bindings__rollback (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    server_url TEXT NOT NULL DEFAULT '',
    token TEXT NOT NULL,
    ttl INTEGER NOT NULL DEFAULT 0,
    exp INTEGER NOT NULL DEFAULT 0,
    user_id TEXT NOT NULL,
    user_name TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);
INSERT INTO chatwiki_bindings__rollback (
    id, server_url, token, ttl, exp, user_id, user_name, created_at, updated_at
)
SELECT
    id, server_url, token, ttl, exp, user_id, user_name, created_at, updated_at
FROM chatwiki_bindings;
DROP TABLE chatwiki_bindings;
ALTER TABLE chatwiki_bindings__rollback RENAME TO chatwiki_bindings;
`)
			return err
		},
	)
}
