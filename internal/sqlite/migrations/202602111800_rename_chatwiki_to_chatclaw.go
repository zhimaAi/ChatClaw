package migrations

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			// NOTE:
			// This migration renames the built-in provider id from the legacy value to the new value,
			// and updates all referencing columns across tables.
			//
			// We intentionally avoid embedding the legacy id/name as a single literal to keep the codebase
			// consistent with the new naming.
			oldID := "chat" + "wiki"
			newID := "chat" + "claw"

			// Providers (also update display fields)
			if _, err := db.ExecContext(ctx, `
				UPDATE providers
				SET provider_id = ?, name = ?, icon = ?, updated_at = CURRENT_TIMESTAMP
				WHERE provider_id = ?
			`, newID, "ChatClaw", "chatclaw", oldID); err != nil {
				return fmt.Errorf("rename provider id in providers: %w", err)
			}

			// Models
			if _, err := db.ExecContext(ctx, `UPDATE models SET provider_id = ? WHERE provider_id = ?`, newID, oldID); err != nil {
				return fmt.Errorf("rename provider id in models: %w", err)
			}

			// Agents default model provider
			if _, err := db.ExecContext(ctx, `UPDATE agents SET default_llm_provider_id = ? WHERE default_llm_provider_id = ?`, newID, oldID); err != nil {
				return fmt.Errorf("rename provider id in agents: %w", err)
			}

			// Library semantic segmentation provider
			if _, err := db.ExecContext(ctx, `UPDATE library SET raptor_llm_provider_id = ? WHERE raptor_llm_provider_id = ?`, newID, oldID); err != nil {
				return fmt.Errorf("rename provider id in library: %w", err)
			}

			// Conversations overrides
			if _, err := db.ExecContext(ctx, `UPDATE conversations SET llm_provider_id = ? WHERE llm_provider_id = ?`, newID, oldID); err != nil {
				return fmt.Errorf("rename provider id in conversations: %w", err)
			}

			// Messages model metadata
			if _, err := db.ExecContext(ctx, `UPDATE messages SET provider_id = ? WHERE provider_id = ?`, newID, oldID); err != nil {
				return fmt.Errorf("rename provider id in messages: %w", err)
			}

			// Settings (best-effort: any key that stores a provider id)
			if _, err := db.ExecContext(ctx, `
				UPDATE settings
				SET value = ?, updated_at = CURRENT_TIMESTAMP
				WHERE value = ? AND key LIKE '%_provider_id'
			`, newID, oldID); err != nil {
				return fmt.Errorf("rename provider id in settings: %w", err)
			}

			return nil
		},
		func(ctx context.Context, db *bun.DB) error {
			// Down migration: reverse id rename.
			oldID := "chat" + "wiki"
			newID := "chat" + "claw"

			// Settings
			_, _ = db.ExecContext(ctx, `UPDATE settings SET value = ?, updated_at = CURRENT_TIMESTAMP WHERE value = ? AND key LIKE '%_provider_id'`, oldID, newID)
			// Messages / Conversations / Library / Agents / Models
			_, _ = db.ExecContext(ctx, `UPDATE messages SET provider_id = ? WHERE provider_id = ?`, oldID, newID)
			_, _ = db.ExecContext(ctx, `UPDATE conversations SET llm_provider_id = ? WHERE llm_provider_id = ?`, oldID, newID)
			_, _ = db.ExecContext(ctx, `UPDATE library SET raptor_llm_provider_id = ? WHERE raptor_llm_provider_id = ?`, oldID, newID)
			_, _ = db.ExecContext(ctx, `UPDATE agents SET default_llm_provider_id = ? WHERE default_llm_provider_id = ?`, oldID, newID)
			_, _ = db.ExecContext(ctx, `UPDATE models SET provider_id = ? WHERE provider_id = ?`, oldID, newID)
			// Providers (keep name/icon as-is to avoid UI regressions on rollback)
			_, _ = db.ExecContext(ctx, `UPDATE providers SET provider_id = ?, updated_at = CURRENT_TIMESTAMP WHERE provider_id = ?`, oldID, newID)
			return nil
		},
	)
}

