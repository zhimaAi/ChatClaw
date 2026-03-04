package migrations

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			// Update Qwen provider type from "openai" to "qwen" to use the dedicated Qwen chat model implementation
			if _, err := db.ExecContext(ctx, `
				UPDATE providers
				SET type = ?, updated_at = CURRENT_TIMESTAMP
				WHERE provider_id = ? AND type = ?
			`, "qwen", "qwen", "openai"); err != nil {
				return fmt.Errorf("update qwen provider type: %w", err)
			}
			return nil
		},
		func(ctx context.Context, db *bun.DB) error {
			// Rollback: change type back to "openai"
			if _, err := db.ExecContext(ctx, `
				UPDATE providers
				SET type = ?, updated_at = CURRENT_TIMESTAMP
				WHERE provider_id = ?
			`, "openai", "qwen"); err != nil {
				return fmt.Errorf("rollback qwen provider type: %w", err)
			}
			return nil
		},
	)
}
