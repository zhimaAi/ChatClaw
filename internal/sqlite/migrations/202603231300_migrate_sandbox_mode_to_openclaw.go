package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			// Migrate old eino sandbox_mode values to OpenClaw equivalents:
			//   codex  -> all   (sandbox everything)
			//   native -> off   (no sandbox)
			stmts := []string{
				`UPDATE openclaw_agents SET sandbox_mode = 'all' WHERE sandbox_mode = 'codex'`,
				`UPDATE openclaw_agents SET sandbox_mode = 'off' WHERE sandbox_mode = 'native'`,
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
				`UPDATE openclaw_agents SET sandbox_mode = 'codex' WHERE sandbox_mode = 'all'`,
				`UPDATE openclaw_agents SET sandbox_mode = 'native' WHERE sandbox_mode = 'off'`,
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
