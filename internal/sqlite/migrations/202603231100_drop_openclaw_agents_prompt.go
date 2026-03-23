package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `ALTER TABLE openclaw_agents DROP COLUMN prompt`)
			return err
		},
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `ALTER TABLE openclaw_agents ADD COLUMN prompt varchar(1000) not null default ''`)
			return err
		},
	)
}
