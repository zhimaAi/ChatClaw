package migrations

import (
	"context"
	"time"

	"willchat/internal/define"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			// Add chatwiki as default pinned provider for existing users (INSERT OR IGNORE if already exists from init)
			now := time.Now().UTC().Format("2006-01-02 15:04:05")
			sql := `
INSERT OR IGNORE INTO providers (provider_id, name, type, icon, is_builtin, enabled, sort_order, api_endpoint, api_key, extra_config, created_at, updated_at)
VALUES ('chatwiki', 'ChatWiki', 'openai', 'chatwiki', 1, 1, 0, ?, '', '{}', ?, ?);
`
			if _, err := db.ExecContext(ctx, sql, define.ChatWikiAPIEndpoint, now, now); err != nil {
				return err
			}
			// Ensure chatwiki is enabled by default (for both new insert and existing)
			_, err := db.ExecContext(ctx, `UPDATE providers SET enabled = 1 WHERE provider_id = 'chatwiki'`)
			return err
		},
		func(ctx context.Context, db *bun.DB) error {
			// Rollback: remove chatwiki provider
			_, err := db.ExecContext(ctx, `DELETE FROM providers WHERE provider_id = 'chatwiki'`)
			return err
		},
	)
}
