package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `alter table channels add column last_sender_id text not null default ''`)
			return err
		},
		func(ctx context.Context, db *bun.DB) error {
			return nil
		},
	)
}
