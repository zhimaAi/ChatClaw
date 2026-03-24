package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			stmts := []string{
				// OpenClaw identity fields
				`ALTER TABLE openclaw_agents ADD COLUMN identity_emoji text not null default ''`,
				`ALTER TABLE openclaw_agents ADD COLUMN identity_theme text not null default ''`,

				// OpenClaw group chat
				`ALTER TABLE openclaw_agents ADD COLUMN group_chat_mention_patterns text not null default '[]'`,

				// OpenClaw tools config
				`ALTER TABLE openclaw_agents ADD COLUMN tools_profile text not null default ''`,
				`ALTER TABLE openclaw_agents ADD COLUMN tools_allow text not null default '[]'`,
				`ALTER TABLE openclaw_agents ADD COLUMN tools_deny text not null default '[]'`,

				// OpenClaw heartbeat
				`ALTER TABLE openclaw_agents ADD COLUMN heartbeat_every text not null default ''`,

				// OpenClaw model params (per-agent overrides synced to Gateway)
				`ALTER TABLE openclaw_agents ADD COLUMN params_temperature text not null default ''`,
				`ALTER TABLE openclaw_agents ADD COLUMN params_max_tokens text not null default ''`,
			}
			for _, s := range stmts {
				if _, err := db.ExecContext(ctx, s); err != nil {
					return err
				}
			}
			return nil
		},
		func(ctx context.Context, db *bun.DB) error {
			stmts := []string{
				`ALTER TABLE openclaw_agents DROP COLUMN identity_emoji`,
				`ALTER TABLE openclaw_agents DROP COLUMN identity_theme`,
				`ALTER TABLE openclaw_agents DROP COLUMN group_chat_mention_patterns`,
				`ALTER TABLE openclaw_agents DROP COLUMN tools_profile`,
				`ALTER TABLE openclaw_agents DROP COLUMN tools_allow`,
				`ALTER TABLE openclaw_agents DROP COLUMN tools_deny`,
				`ALTER TABLE openclaw_agents DROP COLUMN heartbeat_every`,
				`ALTER TABLE openclaw_agents DROP COLUMN params_temperature`,
				`ALTER TABLE openclaw_agents DROP COLUMN params_max_tokens`,
			}
			for _, s := range stmts {
				if _, err := db.ExecContext(ctx, s); err != nil {
					return err
				}
			}
			return nil
		},
	)
}
