package migrations

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"chatclaw/internal/define"
	"chatclaw/internal/services/i18n"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			return seedDefaultData(ctx, db)
		},
		func(ctx context.Context, db *bun.DB) error {
			// Down: remove seeded data only if it still matches the default names.
			locale := i18n.GetLocale()
			data := seedDataForLocale(locale)
			_, _ = db.ExecContext(ctx, `DELETE FROM agents WHERE name = ?`, data.AgentName)
			_, _ = db.ExecContext(ctx, `DELETE FROM library WHERE name = ?`, data.LibraryName)
			return nil
		},
	)
}

// defaultSeedContent holds localised default content.
type defaultSeedContent struct {
	LibraryName string
	AgentName   string
	AgentPrompt string
}

func seedDataForLocale(locale string) defaultSeedContent {
	if locale == "zh-CN" {
		return defaultSeedContent{
			LibraryName: "默认知识库",
			AgentName:   define.DefaultAgentNameForLocale(locale),
			AgentPrompt: define.DefaultAgentPromptForLocale(locale),
		}
	}
	// en-US and all other locales
	return defaultSeedContent{
		LibraryName: "Default Library",
		AgentName:   define.DefaultAgentNameForLocale(locale),
		AgentPrompt: define.DefaultAgentPromptForLocale(locale),
	}
}

func seedDefaultData(ctx context.Context, db *bun.DB) error {
	locale := i18n.GetLocale()
	data := seedDataForLocale(locale)
	now := time.Now().UTC().Format("2006-01-02 15:04:05")

	// --- Seed default library (only if the table is empty) ---
	var libCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(1) FROM library`).Scan(&libCount); err != nil {
		return fmt.Errorf("seed: count library: %w", err)
	}

	var libraryID int64
	if libCount == 0 {
		res, err := db.ExecContext(ctx, `
			INSERT INTO library (created_at, updated_at, name, semantic_segmentation_enabled,
				raptor_llm_provider_id, raptor_llm_model_id, chunk_size, chunk_overlap, sort_order)
			VALUES (?, ?, ?, 0, '', '', 1024, 100, 1)
		`, now, now, data.LibraryName)
		if err != nil {
			return fmt.Errorf("seed: insert library: %w", err)
		}
		libraryID, _ = res.LastInsertId()
	}

	// --- Seed default agent (only if the table is empty) ---
	var agentCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(1) FROM agents`).Scan(&agentCount); err != nil {
		return fmt.Errorf("seed: count agents: %w", err)
	}

	if agentCount == 0 {
		// Build library_ids JSON array
		libraryIDs := "[]"
		if libraryID > 0 {
			b, _ := json.Marshal([]int64{libraryID})
			libraryIDs = string(b)
		}

		// Check if agents table has library_ids column (added in a later migration)
		var colCount int
		hasLibraryIDs := false
		err := db.QueryRowContext(ctx,
			`SELECT COUNT(1) FROM pragma_table_info('agents') WHERE name = 'library_ids'`,
		).Scan(&colCount)
		if err == nil && colCount > 0 {
			hasLibraryIDs = true
		}

		if hasLibraryIDs {
			_, err = db.ExecContext(ctx, `
				INSERT INTO agents (created_at, updated_at, name, openclaw_agent_id, prompt, icon,
					default_llm_provider_id, default_llm_model_id,
					llm_temperature, llm_top_p, llm_max_context_count, llm_max_tokens,
					enable_llm_temperature, enable_llm_top_p, enable_llm_max_tokens,
					retrieval_match_threshold, retrieval_top_k, library_ids)
				VALUES (?, ?, ?, ?, ?, '',
					'', '',
					0.5, 1.0, 50, 1000,
					0, 0, 0,
					0.5, 20, ?)
			`, now, now, data.AgentName, define.OpenClawMainAgentID, data.AgentPrompt, libraryIDs)
		} else {
			_, err = db.ExecContext(ctx, `
				INSERT INTO agents (created_at, updated_at, name, openclaw_agent_id, prompt, icon,
					default_llm_provider_id, default_llm_model_id,
					llm_temperature, llm_top_p, llm_max_context_count, llm_max_tokens,
					enable_llm_temperature, enable_llm_top_p, enable_llm_max_tokens,
					retrieval_match_threshold, retrieval_top_k)
				VALUES (?, ?, ?, ?, ?, '',
					'', '',
					0.5, 1.0, 50, 1000,
					0, 0, 0,
					0.5, 20)
			`, now, now, data.AgentName, define.OpenClawMainAgentID, data.AgentPrompt)
		}
		if err != nil && err != sql.ErrNoRows {
			return fmt.Errorf("seed: insert agent: %w", err)
		}
	}

	return nil
}
