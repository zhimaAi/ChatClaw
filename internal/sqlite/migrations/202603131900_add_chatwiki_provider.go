package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		return SyncBuiltinProvidersAndModels(ctx, db)
	}, func(ctx context.Context, db *bun.DB) error {
		return nil
	})
}
