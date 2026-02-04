package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			sql := `
create table if not exists library (
	id integer primary key autoincrement,
	created_at datetime not null default current_timestamp,
	updated_at datetime not null default current_timestamp,
	
	name varchar(100) not null,
	
	semantic_segment_provider_id varchar(64) not null default '',
	semantic_segment_model_id varchar(128) not null default '',
	
	top_k integer not null default 20,
	chunk_size integer not null default 1024,
	chunk_overlap integer not null default 100,
	match_threshold float not null default 0.5,
	sort_order integer not null default 0
);
`
			if _, err := db.ExecContext(ctx, sql); err != nil {
				return err
			}
			return nil
		},
		func(ctx context.Context, db *bun.DB) error {
			if _, err := db.ExecContext(ctx, `drop table if exists library`); err != nil {
				return err
			}
			return nil
		},
	)
}
