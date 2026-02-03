package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			sql := `
INSERT OR IGNORE INTO settings (key, value, type, category, description, created_at, updated_at) VALUES
  ('embedding_provider_id', 'openai', 'string', 'general', '全局嵌入供应商', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
INSERT OR IGNORE INTO settings (key, value, type, category, description, created_at, updated_at) VALUES
  ('embedding_model_id', 'text-embedding-3-small', 'string', 'general', '全局嵌入模型', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
INSERT OR IGNORE INTO settings (key, value, type, category, description, created_at, updated_at) VALUES
  ('embedding_dimension', '1536', 'string', 'general', '全局嵌入向量维度', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
`
			_, err := db.ExecContext(ctx, sql)
			return err
		},
		func(ctx context.Context, db *bun.DB) error {
			sql := `
DELETE FROM settings WHERE key IN ('embedding_provider_id','embedding_model_id','embedding_dimension');
`
			_, err := db.ExecContext(ctx, sql)
			return err
		},
	)
}
