package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			sql := `
-- Add parent_id column to library_folders table for nested folders
ALTER TABLE library_folders ADD COLUMN parent_id integer;

-- Create index for parent_id
CREATE INDEX IF NOT EXISTS idx_folders_parent_id ON library_folders(parent_id);

-- Drop the unique index on (library_id, name) since folders with same name can exist in different parent folders
DROP INDEX IF EXISTS idx_folders_library_name;

-- Create new unique index on (library_id, parent_id, name) to allow same name in different folders
CREATE UNIQUE INDEX IF NOT EXISTS idx_folders_library_parent_name ON library_folders(library_id, COALESCE(parent_id, -1), name);
`
			if _, err := db.ExecContext(ctx, sql); err != nil {
				return err
			}
			return nil
		},
		func(ctx context.Context, db *bun.DB) error {
			sql := `
DROP INDEX IF EXISTS idx_folders_library_parent_name;
CREATE UNIQUE INDEX IF EXISTS idx_folders_library_name ON library_folders(library_id, name);
DROP INDEX IF EXISTS idx_folders_parent_id;
ALTER TABLE library_folders DROP COLUMN parent_id;
`
			if _, err := db.ExecContext(ctx, sql); err != nil {
				return err
			}
			return nil
		},
	)
}
