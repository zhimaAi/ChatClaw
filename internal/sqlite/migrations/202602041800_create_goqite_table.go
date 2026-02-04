package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			// goqite schema for SQLite
			// https://github.com/maragudk/goqite
			sql := `
CREATE TABLE IF NOT EXISTS goqite (
  id TEXT PRIMARY KEY DEFAULT ('m_' || lower(hex(randomblob(16)))),
  created TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ')),
  updated TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ')),
  queue TEXT NOT NULL,
  body BLOB NOT NULL,
  timeout TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ')),
  received INTEGER NOT NULL DEFAULT 0
) STRICT;

CREATE TRIGGER IF NOT EXISTS goqite_updated_timestamp AFTER UPDATE ON goqite BEGIN
  UPDATE goqite SET updated = strftime('%Y-%m-%dT%H:%M:%fZ') WHERE id = old.id;
END;

CREATE INDEX IF NOT EXISTS goqite_queue_created_idx ON goqite (queue, created);
`
			if _, err := db.ExecContext(ctx, sql); err != nil {
				return err
			}
			return nil
		},
		func(ctx context.Context, db *bun.DB) error {
			sql := `
DROP TRIGGER IF EXISTS goqite_updated_timestamp;
DROP INDEX IF EXISTS goqite_queue_created_idx;
DROP TABLE IF EXISTS goqite;
`
			if _, err := db.ExecContext(ctx, sql); err != nil {
				return err
			}
			return nil
		},
	)
}
