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
	
	semantic_segmentation_enabled integer not null default 0,
	raptor_llm_provider_id varchar(64) not null default '',
	raptor_llm_model_id varchar(128) not null default '',
	
	chunk_size integer not null default 512,
	chunk_overlap integer not null default 50,
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
