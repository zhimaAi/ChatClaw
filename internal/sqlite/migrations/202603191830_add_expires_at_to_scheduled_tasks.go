package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `
alter table scheduled_tasks add column expires_at datetime;
`)
			return err
		},
		func(ctx context.Context, db *bun.DB) error {
			return nil
		},
	)
}
