package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			sql := `
-- Create library_folders table
CREATE TABLE IF NOT EXISTS library_folders (
	id integer primary key autoincrement,
	created_at datetime not null default current_timestamp,
	updated_at datetime not null default current_timestamp,
	
	library_id integer not null,
	name text not null,
	sort_order integer not null default 0,
	
	foreign key(library_id) references library(id) on delete cascade
);

-- Create indexes for library_folders
CREATE INDEX IF NOT EXISTS idx_folders_library_sort ON library_folders(library_id, sort_order);
CREATE UNIQUE INDEX IF NOT EXISTS idx_folders_library_name ON library_folders(library_id, name);

-- Add folder_id column to documents table
ALTER TABLE documents ADD COLUMN folder_id integer;

-- Create index for folder_id
CREATE INDEX IF NOT EXISTS idx_docs_folder_id ON documents(folder_id);

-- Add foreign key constraint for folder_id (ON DELETE SET NULL)
-- Note: SQLite doesn't support ALTER TABLE ADD CONSTRAINT, so we'll handle this in application logic
`
			if _, err := db.ExecContext(ctx, sql); err != nil {
				return err
			}
			return nil
		},
		func(ctx context.Context, db *bun.DB) error {
			sql := `
DROP INDEX IF EXISTS idx_docs_folder_id;
ALTER TABLE documents DROP COLUMN folder_id;
DROP INDEX IF EXISTS idx_folders_library_name;
DROP INDEX IF EXISTS idx_folders_library_sort;
DROP TABLE IF EXISTS library_folders;
`
			if _, err := db.ExecContext(ctx, sql); err != nil {
				return err
			}
			return nil
		},
	)
}
