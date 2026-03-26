package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			if _, err := db.ExecContext(ctx, `ALTER TABLE channels ADD COLUMN openclaw_scope INTEGER NOT NULL DEFAULT 0`); err != nil {
				return err
			}
			// Best-effort backfill: mark OpenClaw-only channels when agent_id matches openclaw_agents
			// but not agents (avoids false positives when the same numeric id exists in both tables).
			if _, err := db.ExecContext(ctx, `
UPDATE channels SET openclaw_scope = 1
WHERE agent_id > 0
  AND EXISTS (SELECT 1 FROM openclaw_agents oa WHERE oa.id = channels.agent_id)
  AND NOT EXISTS (SELECT 1 FROM agents a WHERE a.id = channels.agent_id)
`); err != nil {
				return err
			}
			return nil
		},
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `ALTER TABLE channels DROP COLUMN openclaw_scope`)
			return err
		},
	)
}
