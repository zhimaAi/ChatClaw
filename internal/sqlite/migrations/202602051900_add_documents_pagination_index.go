package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

// 为文档列表的 cursor 分页优化索引：
// WHERE library_id = ? AND id < ? ORDER BY id DESC LIMIT 100
func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_docs_library_id_id ON documents(library_id, id);`)
			return err
		},
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `DROP INDEX IF EXISTS idx_docs_library_id_id;`)
			return err
		},
	)
}

