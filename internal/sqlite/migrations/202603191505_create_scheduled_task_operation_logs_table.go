package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			sql := `
create table if not exists scheduled_task_operation_logs (
	id integer primary key autoincrement,
	created_at datetime not null default current_timestamp,
	task_id integer not null,
	task_name_snapshot text not null default '',
	operation_type text not null,
	operation_source text not null,
	changed_fields_json text not null default '[]',
	task_snapshot_json text not null default '{}'
);
create index if not exists idx_scheduled_task_operation_logs_task_id_created_at on scheduled_task_operation_logs(task_id, created_at desc, id desc);
create index if not exists idx_scheduled_task_operation_logs_created_at on scheduled_task_operation_logs(created_at desc, id desc);
`
			if _, err := db.ExecContext(ctx, sql); err != nil {
				return err
			}
			return nil
		},
		func(ctx context.Context, db *bun.DB) error {
			sql := `
drop index if exists idx_scheduled_task_operation_logs_created_at;
drop index if exists idx_scheduled_task_operation_logs_task_id_created_at;
drop table if exists scheduled_task_operation_logs;
`
			if _, err := db.ExecContext(ctx, sql); err != nil {
				return err
			}
			return nil
		},
	)
}
