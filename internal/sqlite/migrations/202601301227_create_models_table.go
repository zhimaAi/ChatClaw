package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			sql := `
create table if not exists models (
    id integer primary key autoincrement,
	created_at datetime not null default current_timestamp,
	updated_at datetime not null default current_timestamp,

	supplier varchar(32) not null,
	type varchar(16) not null default 'openai',
	enabled boolean not null default true,
	icon varchar(1024) not null default '',
	api_endpoint varchar(1024) not null,
    api_key varchar(1024) not null,
    llm_define_list text not null default '[]',
	embedding_define_list text not null default '[]'
);
`
			if _, err := db.ExecContext(ctx, sql); err != nil {
				return err
			}
			return nil
		},
		func(ctx context.Context, db *bun.DB) error {
			if _, err := db.ExecContext(ctx, `drop table if exists models`); err != nil {
				return err
			}
			return nil
		},
	)
}
