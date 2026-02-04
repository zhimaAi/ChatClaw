package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			sql := `
create table if not exists agents (
    id integer primary key autoincrement,
	created_at datetime not null default current_timestamp,
    updated_at datetime not null default current_timestamp,
	
    name varchar(100) not null,
    prompt varchar(1000) not null,
	icon text not null default '',
	
	default_llm_provider_id varchar(64) not null,
	default_llm_model_id varchar(128) not null,

	llm_temperature float not null default 0.5,
	llm_top_p float not null default 1.0,
	context_count int not null default 50,
	llm_max_tokens int not null default 1000,
	enable_llm_temperature boolean not null default false,
	enable_llm_top_p boolean not null default false,
	enable_llm_max_tokens boolean not null default false,
	match_threshold float not null default 0.5
);			
`
			if _, err := db.ExecContext(ctx, sql); err != nil {
				return err
			}
			return nil
		},
		func(ctx context.Context, db *bun.DB) error {
			_ = ctx
			_ = db
			return nil
		},
	)
}
