package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			sql := `
create table if not exists conversations (
	id integer primary key autoincrement,
	created_at datetime not null default current_timestamp,
	updated_at datetime not null default current_timestamp,

	agent_id integer not null,
	name text not null,
	last_message text not null,
	is_deleted boolean not null default false
);
create index if not exists idx_conversations_agent_id on conversations(agent_id, updated_at desc) where is_deleted = 0;
`
			if _, err := db.ExecContext(ctx, sql); err != nil {
				return err
			}
			return nil
		},
		func(ctx context.Context, db *bun.DB) error {
			if _, err := db.ExecContext(ctx, `drop table if exists conversations;`); err != nil {
				return err
			}
			return nil
		},
	)
}
