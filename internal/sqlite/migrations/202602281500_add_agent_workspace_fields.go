package migrations

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("get user home dir: %w", err)
			}
			defaultWorkDir := filepath.Join(home, ".chatclaw")
			if err := os.MkdirAll(defaultWorkDir, 0o755); err != nil {
				return fmt.Errorf("create default work dir: %w", err)
			}

			sql := fmt.Sprintf(`
ALTER TABLE agents ADD COLUMN sandbox_mode varchar(16) NOT NULL DEFAULT 'codex';
ALTER TABLE agents ADD COLUMN sandbox_network boolean NOT NULL DEFAULT true;
ALTER TABLE agents ADD COLUMN work_dir text NOT NULL DEFAULT '%s';
`, defaultWorkDir)

			_, err = db.ExecContext(ctx, sql)
			if err != nil {
				return fmt.Errorf("add workspace columns to agents: %w", err)
			}

			// Remove old global workspace settings (if present from earlier migrations)
			_, _ = db.ExecContext(ctx, `
DELETE FROM settings WHERE key IN ('workspace_sandbox_mode','workspace_work_dir','workspace_sandbox_network');
`)
			return nil
		},
		func(ctx context.Context, db *bun.DB) error {
			// SQLite doesn't support DROP COLUMN in all versions; skip rollback
			return nil
		},
	)
}
