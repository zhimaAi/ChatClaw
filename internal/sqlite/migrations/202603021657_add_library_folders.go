package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			sql := `
CREATE TABLE IF NOT EXISTS library_folders (
	id integer primary key autoincrement,
	created_at datetime not null default current_timestamp,
	updated_at datetime not null default current_timestamp,
	library_id integer not null,
	name text not null,
	sort_order integer not null default 0,
	parent_id integer,
	foreign key(library_id) references library(id) on delete cascade
);

CREATE INDEX IF NOT EXISTS idx_folders_library_sort ON library_folders(library_id, sort_order);
CREATE INDEX IF NOT EXISTS idx_folders_parent_id ON library_folders(parent_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_folders_library_parent_name ON library_folders(library_id, COALESCE(parent_id, -1), name);

ALTER TABLE documents ADD COLUMN folder_id integer;
CREATE INDEX IF NOT EXISTS idx_docs_folder_id ON documents(folder_id);
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

DROP INDEX IF EXISTS idx_folders_library_parent_name;
DROP INDEX IF EXISTS idx_folders_parent_id;
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

