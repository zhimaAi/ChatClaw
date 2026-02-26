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
  ('memory_enabled', 'false', 'boolean', 'memory', '是否开启长期记忆', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
INSERT OR IGNORE INTO settings (key, value, type, category, description, created_at, updated_at) VALUES
  ('memory_extract_provider_id', '', 'string', 'memory', '记忆提取大模型供应商', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
INSERT OR IGNORE INTO settings (key, value, type, category, description, created_at, updated_at) VALUES
  ('memory_extract_model_id', '', 'string', 'memory', '记忆提取大模型', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
INSERT OR IGNORE INTO settings (key, value, type, category, description, created_at, updated_at) VALUES
  ('memory_embedding_provider_id', '', 'string', 'memory', '记忆向量模型供应商', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
INSERT OR IGNORE INTO settings (key, value, type, category, description, created_at, updated_at) VALUES
  ('memory_embedding_model_id', '', 'string', 'memory', '记忆向量模型', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
INSERT OR IGNORE INTO settings (key, value, type, category, description, created_at, updated_at) VALUES
  ('memory_embedding_dimension', '1536', 'string', 'memory', '记忆向量维度', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
`
			_, err := db.ExecContext(ctx, sql)
			return err
		},
		func(ctx context.Context, db *bun.DB) error {
			sql := `
DELETE FROM settings WHERE key IN ('memory_enabled','memory_extract_provider_id','memory_extract_model_id','memory_embedding_provider_id','memory_embedding_model_id','memory_embedding_dimension');
`
			_, err := db.ExecContext(ctx, sql)
			return err
		},
	)
}
