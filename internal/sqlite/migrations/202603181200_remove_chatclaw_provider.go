package migrations

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			return removeChatClawProviderData(ctx, db)
		},
		func(ctx context.Context, db *bun.DB) error {
			_ = ctx
			_ = db
			return nil
		},
	)
}

func removeChatClawProviderData(ctx context.Context, db *bun.DB) error {
	const providerID = "chatclaw"

	if _, err := db.ExecContext(ctx, `DELETE FROM models WHERE provider_id = ?`, providerID); err != nil {
		return fmt.Errorf("delete chatclaw models: %w", err)
	}

	if err := clearChatClawProviderReferences(ctx, db, providerID); err != nil {
		return err
	}

	if _, err := db.ExecContext(ctx, `DELETE FROM providers WHERE provider_id = ?`, providerID); err != nil {
		return fmt.Errorf("delete chatclaw provider: %w", err)
	}

	return nil
}

func clearChatClawProviderReferences(ctx context.Context, db *bun.DB, providerID string) error {
	type tableUpdate struct {
		table   string
		column  string
		updates []string
	}

	updates := []tableUpdate{
		{
			table:  "agents",
			column: "default_llm_provider_id",
			updates: []string{
				`UPDATE agents SET default_llm_provider_id = '', default_llm_model_id = '' WHERE default_llm_provider_id = ?`,
			},
		},
		{
			table:  "library",
			column: "raptor_llm_provider_id",
			updates: []string{
				`UPDATE library SET raptor_llm_provider_id = '', raptor_llm_model_id = '' WHERE raptor_llm_provider_id = ?`,
			},
		},
		{
			table:  "conversations",
			column: "llm_provider_id",
			updates: []string{
				`UPDATE conversations SET llm_provider_id = '', llm_model_id = '' WHERE llm_provider_id = ?`,
			},
		},
		{
			table:  "messages",
			column: "provider_id",
			updates: []string{
				`UPDATE messages SET provider_id = '', model_id = '' WHERE provider_id = ?`,
			},
		},
	}

	for _, update := range updates {
		exists, err := hasColumn(ctx, db, update.table, update.column)
		if err != nil {
			return fmt.Errorf("check %s.%s: %w", update.table, update.column, err)
		}
		if !exists {
			continue
		}
		for _, statement := range update.updates {
			if _, err := db.ExecContext(ctx, statement, providerID); err != nil {
				return fmt.Errorf("clear %s references: %w", update.table, err)
			}
		}
	}

	settingsTableExists, err := hasTable(ctx, db, "settings")
	if err != nil {
		return fmt.Errorf("check settings table: %w", err)
	}
	if settingsTableExists {
		if _, err := db.ExecContext(ctx, `
			UPDATE settings
			SET value = '', updated_at = CURRENT_TIMESTAMP
			WHERE value = ? AND key LIKE '%_provider_id'
		`, providerID); err != nil {
			return fmt.Errorf("clear settings provider references: %w", err)
		}
		if _, err := db.ExecContext(ctx, `
			UPDATE settings
			SET value = '', updated_at = CURRENT_TIMESTAMP
			WHERE key IN ('embedding_model_id', 'memory_extract_model_id', 'memory_embedding_model_id')
			  AND EXISTS (
			    SELECT 1 FROM settings AS sp
			    WHERE sp.key = CASE settings.key
			      WHEN 'embedding_model_id' THEN 'embedding_provider_id'
			      WHEN 'memory_extract_model_id' THEN 'memory_extract_provider_id'
			      WHEN 'memory_embedding_model_id' THEN 'memory_embedding_provider_id'
			    END
			    AND sp.value = ''
			  )
		`); err != nil {
			return fmt.Errorf("clear settings model references: %w", err)
		}
	}

	return nil
}

func hasTable(ctx context.Context, db *bun.DB, tableName string) (bool, error) {
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(1) FROM sqlite_master WHERE type = 'table' AND name = ?`, tableName).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}
