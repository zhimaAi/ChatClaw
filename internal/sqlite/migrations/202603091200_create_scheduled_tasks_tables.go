package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			sql := `
create table if not exists scheduled_tasks (
	id integer primary key autoincrement,
	created_at datetime not null default current_timestamp,
	updated_at datetime not null default current_timestamp,
	deleted_at datetime,
	name text not null,
	prompt text not null,
	agent_id integer not null,
	schedule_type varchar(16) not null,
	schedule_value text not null default '',
	cron_expr varchar(64) not null,
	timezone varchar(64) not null default 'Local',
	enabled boolean not null default true,
	last_run_at datetime,
	next_run_at datetime,
	last_status varchar(16) not null default 'pending',
	last_error text not null default '',
	last_run_id integer
);
create index if not exists idx_scheduled_tasks_enabled_next_run_at on scheduled_tasks(enabled, next_run_at);
create index if not exists idx_scheduled_tasks_deleted_at on scheduled_tasks(deleted_at);

create table if not exists scheduled_task_runs (
	id integer primary key autoincrement,
	created_at datetime not null default current_timestamp,
	updated_at datetime not null default current_timestamp,
	task_id integer not null,
	trigger_type varchar(16) not null,
	status varchar(16) not null,
	started_at datetime not null,
	finished_at datetime,
	duration_ms integer not null default 0,
	error_message text not null default '',
	conversation_id integer,
	user_message_id integer,
	assistant_message_id integer,
	snapshot_task_name text not null,
	snapshot_prompt text not null,
	snapshot_agent_id integer not null
);
create index if not exists idx_scheduled_task_runs_task_id_started_at on scheduled_task_runs(task_id, started_at desc, id desc);
create index if not exists idx_scheduled_task_runs_conversation_id on scheduled_task_runs(conversation_id);
`
			if _, err := db.ExecContext(ctx, sql); err != nil {
				return err
			}
			return nil
		},
		func(ctx context.Context, db *bun.DB) error {
			sql := `
drop index if exists idx_scheduled_task_runs_conversation_id;
drop index if exists idx_scheduled_task_runs_task_id_started_at;
drop table if exists scheduled_task_runs;
drop index if exists idx_scheduled_tasks_deleted_at;
drop index if exists idx_scheduled_tasks_enabled_next_run_at;
drop table if exists scheduled_tasks;
`
			if _, err := db.ExecContext(ctx, sql); err != nil {
				return err
			}
			return nil
		},
	)
}
