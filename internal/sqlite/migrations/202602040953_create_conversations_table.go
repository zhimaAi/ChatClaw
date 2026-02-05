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
	is_pinned boolean not null default false
);
-- 索引设计：按 agent_id 查询，置顶优先，然后按更新时间倒序
create index if not exists idx_conversations_agent_id on conversations(agent_id, is_pinned desc, updated_at desc);
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
