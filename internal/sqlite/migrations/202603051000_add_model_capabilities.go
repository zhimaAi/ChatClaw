package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			// 为 models 表添加 capabilities 字段
			_, err := db.ExecContext(ctx, `
				alter table models add column capabilities text not null default '["text"]';
			`)
			return err
		},
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `
				alter table models drop column capabilities;
			`)
			return err
		},
	)
}
