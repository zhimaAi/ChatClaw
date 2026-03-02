package migrations

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
			escapedWorkDir := strings.ReplaceAll(defaultWorkDir, "'", "''")

			sql := fmt.Sprintf(`
ALTER TABLE agents ADD COLUMN sandbox_mode varchar(16) NOT NULL DEFAULT 'codex';
ALTER TABLE agents ADD COLUMN sandbox_network boolean NOT NULL DEFAULT true;
ALTER TABLE agents ADD COLUMN work_dir text NOT NULL DEFAULT '%s';
`, escapedWorkDir)

			_, err = db.ExecContext(ctx, sql)
			if err != nil {
				return fmt.Errorf("add workspace columns to agents: %w", err)
			}

			return nil
		},
		func(ctx context.Context, db *bun.DB) error {
			// SQLite doesn't support DROP COLUMN in all versions; skip rollback
			return nil
		},
	)
}
