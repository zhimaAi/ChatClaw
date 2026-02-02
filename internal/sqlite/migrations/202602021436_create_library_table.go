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
	
	name varchar(128) not null,
	
	embedding_provider_id varchar(64) not null,
	embedding_model_id varchar(128) not null,
	embedding_dimension integer not null default 1536,
	rerank_provider_id varchar(64) not null,
	rerank_model_id varchar(128) not null,
	
	top_k integer not null default 10,
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
