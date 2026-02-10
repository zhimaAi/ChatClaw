package migrations

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"willclaw/internal/services/i18n"

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
			AgentName:   "默认助手",
			AgentPrompt: "你扮演一名智能问答机器人，具备专业的产品知识和出色的沟通能力\n" +
				"你的回答应该使用自然的对话方式，简单直接地回答，不要解释你的答案；\n" +
				"- 如果用户的问题比较模糊，你应该引导用户明确的提出他的问题，不要贸然回复用户。\n" +
				"- 如果关联了知识库，所有回答都需要来自你的知识库，没有关联知识库也要从正确的方向回答\n" +
				"- 你要注意在知识库资料中，可能包含不相关的知识点，你需要认真分析用户的问题，选择最相关的知识点作为回答",
		}
	}
	// en-US and all other locales
	return defaultSeedContent{
		LibraryName: "Default Library",
		AgentName:   "Default Assistant",
		AgentPrompt: "You are an intelligent Q&A assistant with professional product knowledge and excellent communication skills.\n" +
			"Your answers should be natural and conversational, simple and direct — do not explain your reasoning;\n" +
			"- If the user's question is vague, guide them to clarify before answering.\n" +
			"- If a knowledge base is linked, all answers must come from that knowledge base; if not linked, answer from the correct direction.\n" +
			"- Note that the knowledge base may contain unrelated information — carefully analyse the user's question and select the most relevant knowledge to answer.",
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
				INSERT INTO agents (created_at, updated_at, name, prompt, icon,
					default_llm_provider_id, default_llm_model_id,
					llm_temperature, llm_top_p, llm_max_context_count, llm_max_tokens,
					enable_llm_temperature, enable_llm_top_p, enable_llm_max_tokens,
					retrieval_match_threshold, retrieval_top_k, library_ids)
				VALUES (?, ?, ?, ?, '',
					'', '',
					0.5, 1.0, 50, 1000,
					0, 0, 0,
					0.5, 20, ?)
			`, now, now, data.AgentName, data.AgentPrompt, libraryIDs)
		} else {
			_, err = db.ExecContext(ctx, `
				INSERT INTO agents (created_at, updated_at, name, prompt, icon,
					default_llm_provider_id, default_llm_model_id,
					llm_temperature, llm_top_p, llm_max_context_count, llm_max_tokens,
					enable_llm_temperature, enable_llm_top_p, enable_llm_max_tokens,
					retrieval_match_threshold, retrieval_top_k)
				VALUES (?, ?, ?, ?, '',
					'', '',
					0.5, 1.0, 50, 1000,
					0, 0, 0,
					0.5, 20)
			`, now, now, data.AgentName, data.AgentPrompt)
		}
		if err != nil && err != sql.ErrNoRows {
			return fmt.Errorf("seed: insert agent: %w", err)
		}
	}

	return nil
}
