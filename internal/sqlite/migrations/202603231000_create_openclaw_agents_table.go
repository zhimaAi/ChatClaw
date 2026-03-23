package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			sql := `
create table if not exists openclaw_agents (
    id integer primary key autoincrement,
	created_at datetime not null default current_timestamp,
    updated_at datetime not null default current_timestamp,

    name varchar(100) not null,
    openclaw_agent_id varchar(128) not null unique,
    prompt varchar(1000) not null,
	icon text not null default '',

	default_llm_provider_id varchar(64) not null default '',
	default_llm_model_id varchar(128) not null default '',

	llm_temperature float not null default 0.5,
	llm_top_p float not null default 1.0,
	llm_max_context_count int not null default 50,
	llm_max_tokens int not null default 1000,
	enable_llm_temperature boolean not null default false,
	enable_llm_top_p boolean not null default false,
	enable_llm_max_tokens boolean not null default false,
	retrieval_match_threshold float not null default 0.5,
	retrieval_top_k int not null default 20,

	sandbox_mode varchar(20) not null default 'codex',
	sandbox_network boolean not null default true,
	work_dir text not null default '',

	mcp_enabled boolean not null default false,
	mcp_server_ids text not null default '[]',
	mcp_server_enabled_ids text not null default '[]'
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
