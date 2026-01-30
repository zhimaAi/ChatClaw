package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			sql := `
CREATE TABLE IF NOT EXISTS settings (
    key TEXT PRIMARY KEY NOT NULL,
    value TEXT,
    type TEXT DEFAULT 'string',
    category TEXT DEFAULT 'general',
    description TEXT,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);

INSERT OR IGNORE INTO settings (key, value, type, category, description, created_at, updated_at) VALUES
  ('language', 'zh-CN', 'string', 'general', 'The language of the application', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
INSERT OR IGNORE INTO settings (key, value, type, category, description, created_at, updated_at) VALUES
  ('theme', 'light', 'string', 'general', 'The theme of the application', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
`
			if _, err := db.ExecContext(ctx, sql); err != nil {
				return err
			}
			return nil
		},
		func(ctx context.Context, db *bun.DB) error {
			sql := `
DROP TABLE IF EXISTS settings;
`
			if _, err := db.ExecContext(ctx, sql); err != nil {
				return err
			}
			return nil
		},
	)
}
