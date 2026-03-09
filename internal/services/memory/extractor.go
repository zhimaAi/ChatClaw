package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"

	"chatclaw/internal/eino/chatmodel"
	"chatclaw/internal/eino/embedding"
	"chatclaw/internal/fts/tokenizer"
	i18n "chatclaw/internal/services/i18n"
	"chatclaw/internal/sqlite"

	einoembedding "github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/uptrace/bun"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// Debounce thresholds
const (
	minUserContentRunes = 4
	minUserContentBytes = 8
)

// Common meaningless phrases to skip (Chinese + English)
var trivialPhrases = map[string]bool{
	"好": true, "嗯": true, "嗯嗯": true, "继续": true, "好的": true,
	"行": true, "可以": true, "了解": true, "知道了": true, "收到": true,
	"谢谢": true, "感谢": true, "明白": true, "ok": true, "okay": true,
	"yes": true, "no": true, "sure": true, "thanks": true, "got it": true,
	"go on": true, "continue": true, "next": true, "done": true,
}

type extractionMessageRow struct {
	Role    string `bun:"role"`
	Content string `bun:"content"`
}

// RunMemoryExtraction runs the asynchronous memory extraction process.
// It should be called in a new goroutine after a chat generation completes.
func RunMemoryExtraction(ctx context.Context, app *application.App, conversationID int64) {
	if err := runMemoryExtraction(ctx, app, conversationID); err != nil {
		app.Logger.Error("[memory] extraction failed", "conv", conversationID, "error", err)
	}
}

func runMemoryExtraction(ctx context.Context, app *application.App, conversationID int64) error {
	mainDB := sqlite.DB()
	if mainDB == nil {
		return fmt.Errorf("main db not initialized")
	}
	memDB := DB()
	if memDB == nil {
		return fmt.Errorf("memory db not initialized")
	}

	// 1. Check if memory is enabled
	var memoryEnabledStr string
	if err := mainDB.NewSelect().Table("settings").Column("value").Where("key = ?", "memory_enabled").Scan(ctx, &memoryEnabledStr); err != nil {
		return err
	}
	if memoryEnabledStr != "true" {
		return nil
	}

	// 2. Get extraction model config
	type settingRow struct {
		Key   string
		Value sql.NullString
	}
	var settings []settingRow
	if err := mainDB.NewSelect().Table("settings").Column("key", "value").
		Where("key IN (?)", bun.In([]string{
			"memory_extract_provider_id",
			"memory_extract_model_id",
			"memory_embedding_provider_id",
			"memory_embedding_model_id",
			"memory_embedding_dimension",
		})).Scan(ctx, &settings); err != nil {
		return err
	}

	configMap := make(map[string]string)
	for _, s := range settings {
		if s.Value.Valid {
			configMap[s.Key] = s.Value.String
		}
	}

	extractProviderID := configMap["memory_extract_provider_id"]
	extractModelID := configMap["memory_extract_model_id"]
	if extractProviderID == "" || extractModelID == "" {
		app.Logger.Warn("[memory] extraction skipped: extraction model not configured")
		return nil
	}

	// 3. Get provider details
	type providerRow struct {
		Type        string `bun:"type"`
		APIKey      string `bun:"api_key"`
		APIEndpoint string `bun:"api_endpoint"`
		ExtraConfig string `bun:"extra_config"`
	}
	var extractProvider providerRow
	if err := mainDB.NewSelect().Table("providers").Column("type", "api_key", "api_endpoint", "extra_config").
		Where("provider_id = ?", extractProviderID).Scan(ctx, &extractProvider); err != nil {
		return fmt.Errorf("get extract provider: %w", err)
	}

	// 4. Get conversation and agent
	var conv struct {
		AgentID int64 `bun:"agent_id"`
	}
	if err := mainDB.NewSelect().Table("conversations").Column("agent_id").Where("id = ?", conversationID).Scan(ctx, &conv); err != nil {
		return fmt.Errorf("get conversation: %w", err)
	}

	// 5. Only extract from the latest message pair: assistant (newest) + user (previous)
	var messages []extractionMessageRow
	if err := mainDB.NewSelect().Table("messages").Column("role", "content").
		Where("conversation_id = ?", conversationID).
		Where("status = ?", "success").
		Where("role IN (?)", bun.In([]string{"user", "assistant"})).
		OrderExpr("id DESC").Limit(2).Scan(ctx, &messages); err != nil {
		return fmt.Errorf("get messages: %w", err)
	}
	if len(messages) < 2 {
		app.Logger.Info("[memory] extraction skipped: not enough messages")
		return nil
	}
	if messages[0].Role != "assistant" || messages[1].Role != "user" {
		app.Logger.Info("[memory] extraction skipped: latest pair is not assistant-user")
		return nil
	}
	assistantContent := messages[0].Content

	// 6. Debounce: skip trivial or meaningless messages
	userContent := strings.TrimSpace(messages[1].Content)
	if shouldSkipExtraction(userContent) {
		app.Logger.Info("[memory] extraction skipped: trivial content")
		return nil
	}

	// 7. Load existing memories for this agent to provide context to the LLM,
	// so it can avoid duplicate extraction and make better merge decisions.
	existingContext := buildExistingMemoryContext(ctx, memDB, conv.AgentID)

	// 8. Call extraction LLM
	chatModel, err := chatmodel.NewChatModel(ctx, &chatmodel.ProviderConfig{
		ProviderType:    extractProvider.Type,
		APIKey:          extractProvider.APIKey,
		APIEndpoint:     extractProvider.APIEndpoint,
		ModelID:         extractModelID,
		ExtraConfig:     extractProvider.ExtraConfig,
		Timeout:         60 * time.Second,
		DisableThinking: true,
	})
	if err != nil {
		return fmt.Errorf("create chat model: %w", err)
	}

	prompt := buildExtractionPrompt(existingContext)

	sysMsg := &schema.Message{
		Role:    schema.System,
		Content: fmt.Sprintf(prompt, userContent, assistantContent),
	}

	resp, err := chatModel.Generate(ctx, []*schema.Message{sysMsg}, model.WithTemperature(0.1))
	if err != nil {
		return fmt.Errorf("generate memory extraction: %w", err)
	}

	content := resp.Content
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	if content == "{}" || content == "" {
		app.Logger.Info("[memory] no new memory extracted")
		return nil
	}

	var result struct {
		EventStream       string `json:"event_stream"`
		CoreProfileUpdate string `json:"core_profile_update"`
		ThematicFacts     []struct {
			Topic   string `json:"topic"`
			Content string `json:"content"`
		} `json:"thematic_facts"`
	}

	if err := json.Unmarshal([]byte(content), &result); err != nil {
		app.Logger.Warn("[memory] failed to parse extraction result", "error", err, "content", content)
		return nil
	}

	// 9. Save to DB and generate embeddings if needed
	var embedder einoembedding.Embedder
	embedProviderID := configMap["memory_embedding_provider_id"]
	embedModelID := configMap["memory_embedding_model_id"]

	if embedProviderID != "" && embedModelID != "" {
		var embedProvider providerRow
		if err := mainDB.NewSelect().Table("providers").Column("type", "api_key", "api_endpoint", "extra_config").
			Where("provider_id = ?", embedProviderID).Scan(ctx, &embedProvider); err == nil {
			dim := 0
			if d, e := strconv.Atoi(configMap["memory_embedding_dimension"]); e == nil && d > 0 {
				dim = d
			}
			embedder, _ = embedding.NewEmbedder(ctx, &embedding.ProviderConfig{
				ProviderType: embedProvider.Type,
				APIKey:       embedProvider.APIKey,
				APIEndpoint:  embedProvider.APIEndpoint,
				ModelID:      embedModelID,
				Dimension:    dim,
				ExtraConfig:  embedProvider.ExtraConfig,
			})
		}
	}

	tx, err := memDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Update Core Profile (upsert)
	if result.CoreProfileUpdate != "" {
		var cp CoreProfile
		err := tx.NewSelect().Model(&cp).Where("agent_id = ?", conv.AgentID).Limit(1).Scan(ctx)
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		if err == sql.ErrNoRows {
			cp = CoreProfile{
				AgentID: conv.AgentID,
				Content: result.CoreProfileUpdate,
			}
			if _, err := tx.NewInsert().Model(&cp).Exec(ctx); err != nil {
				return err
			}
		} else {
			cp.Content = result.CoreProfileUpdate
			cp.UpdatedAt = time.Now().UTC()
			if _, err := tx.NewUpdate().Model(&cp).WherePK().Exec(ctx); err != nil {
				return err
			}
		}
		app.Logger.Info("[memory] core profile updated")
	}

	// Insert Event Stream
	if result.EventStream != "" {
		es := EventStream{
			AgentID: conv.AgentID,
			Date:    time.Now().Format("2006-01-02"),
			Content: result.EventStream,
		}
		_, err := tx.NewInsert().Model(&es).Returning("id").Exec(ctx)
		if err != nil {
			return err
		}

		esTokens := tokenizer.TokenizeContent(es.Content)
		_, _ = tx.ExecContext(ctx,
			`INSERT INTO event_streams_fts(rowid, tokens, agent_id) VALUES (?, ?, ?)`,
			es.ID, esTokens, conv.AgentID)

		if embedder != nil {
			vecs, err := embedder.EmbedStrings(ctx, []string{es.Content})
			if err == nil && len(vecs) > 0 {
				vecJSON, _ := json.Marshal(vecs[0])
				_, _ = tx.ExecContext(ctx, `INSERT INTO event_streams_vec(id, embedding) VALUES (?, ?)`, es.ID, string(vecJSON))
			}
		}
		app.Logger.Info("[memory] event stream added")
	}

	// Upsert Thematic Facts: merge into existing topics instead of always inserting
	for _, tf := range result.ThematicFacts {
		if tf.Topic == "" || tf.Content == "" {
			continue
		}

		// Try to find existing fact with same topic for this agent
		var existing ThematicFact
		err := tx.NewSelect().Model(&existing).
			Where("agent_id = ? AND topic = ?", conv.AgentID, tf.Topic).
			Limit(1).Scan(ctx)

		if err == nil {
			// Capture old tokens before updating content (needed for contentless FTS delete)
			oldTokens := tokenizer.TokenizeContent(existing.Topic + " " + existing.Content)

			// Topic exists: update content (LLM already produced a merged summary)
			existing.Content = tf.Content
			existing.UpdatedAt = time.Now().UTC()
			if _, err := tx.NewUpdate().Model(&existing).WherePK().Exec(ctx); err != nil {
				return err
			}

			// Update FTS5: delete old entry then insert new (contentless delete command)
			_, _ = tx.ExecContext(ctx,
				`INSERT INTO thematic_facts_fts(thematic_facts_fts, rowid, tokens, agent_id) VALUES('delete', ?, ?, ?)`,
				existing.ID, oldTokens, conv.AgentID)
			tfTokens := tokenizer.TokenizeContent(tf.Topic + " " + tf.Content)
			_, _ = tx.ExecContext(ctx,
				`INSERT INTO thematic_facts_fts(rowid, tokens, agent_id) VALUES (?, ?, ?)`,
				existing.ID, tfTokens, conv.AgentID)

			// Update vector
			if embedder != nil {
				vecs, embErr := embedder.EmbedStrings(ctx, []string{tf.Topic + ": " + tf.Content})
				if embErr == nil && len(vecs) > 0 {
					vecJSON, _ := json.Marshal(vecs[0])
					_, _ = tx.ExecContext(ctx, `DELETE FROM thematic_facts_vec WHERE id = ?`, existing.ID)
					_, _ = tx.ExecContext(ctx, `INSERT INTO thematic_facts_vec(id, embedding) VALUES (?, ?)`, existing.ID, string(vecJSON))
				}
			}
			app.Logger.Info("[memory] thematic fact updated", "topic", tf.Topic)
		} else if errors.Is(err, sql.ErrNoRows) {
			// New topic: insert
			fact := ThematicFact{
				AgentID: conv.AgentID,
				Topic:   tf.Topic,
				Content: tf.Content,
			}
			_, err := tx.NewInsert().Model(&fact).Returning("id").Exec(ctx)
			if err != nil {
				return err
			}

			tfTokens := tokenizer.TokenizeContent(fact.Topic + " " + fact.Content)
			_, _ = tx.ExecContext(ctx,
				`INSERT INTO thematic_facts_fts(rowid, tokens, agent_id) VALUES (?, ?, ?)`,
				fact.ID, tfTokens, conv.AgentID)

			if embedder != nil {
				vecs, embErr := embedder.EmbedStrings(ctx, []string{fact.Topic + ": " + fact.Content})
				if embErr == nil && len(vecs) > 0 {
					vecJSON, _ := json.Marshal(vecs[0])
					_, _ = tx.ExecContext(ctx, `INSERT INTO thematic_facts_vec(id, embedding) VALUES (?, ?)`, fact.ID, string(vecJSON))
				}
			}
			app.Logger.Info("[memory] thematic fact added", "topic", tf.Topic)
		} else {
			return err
		}
	}

	return tx.Commit()
}

// shouldSkipExtraction returns true if the user message is too short,
// purely punctuation/emoji, or matches a known trivial phrase.
func shouldSkipExtraction(content string) bool {
	if content == "" {
		return true
	}

	lower := strings.ToLower(content)
	if trivialPhrases[lower] {
		return true
	}

	runes := []rune(content)
	if len(runes) < minUserContentRunes {
		return true
	}
	if len(content) < minUserContentBytes {
		return true
	}

	// Skip if content is purely punctuation, emoji, or whitespace
	meaningful := 0
	for _, r := range runes {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			meaningful++
		}
	}
	return meaningful < 2
}

// buildExistingMemoryContext loads current memory state and formats it as context
// for the extraction LLM, so it can avoid duplicates and make better merge decisions.
func buildExistingMemoryContext(ctx context.Context, memDB *bun.DB, agentID int64) string {
	isZh := i18n.GetLocale() == i18n.LocaleZhCN

	var sb strings.Builder

	var cp CoreProfile
	if err := memDB.NewSelect().Model(&cp).Where("agent_id = ?", agentID).Limit(1).Scan(ctx); err == nil && cp.Content != "" {
		if isZh {
			sb.WriteString("## 已有核心档案\n")
		} else {
			sb.WriteString("## Existing Core Profile\n")
		}
		sb.WriteString(cp.Content)
		sb.WriteString("\n\n")
	}

	var facts []ThematicFact
	if err := memDB.NewSelect().Model(&facts).Where("agent_id = ?", agentID).
		OrderExpr("updated_at DESC").Limit(20).Scan(ctx); err == nil && len(facts) > 0 {
		if isZh {
			sb.WriteString("## 已有主题事实\n")
		} else {
			sb.WriteString("## Existing Thematic Facts\n")
		}
		for _, f := range facts {
			sb.WriteString(fmt.Sprintf("- [%s] %s\n", f.Topic, f.Content))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func buildExtractionPrompt(existingContext string) string {
	isZh := i18n.GetLocale() == i18n.LocaleZhCN

	var sb strings.Builder
	if isZh {
		sb.WriteString(`你是一个记忆提取助手。分析以下用户与助手之间的对话。
提取关于用户的任何新的长期事实性信息（例如，偏好、习惯、事实、限制条件）。

重要规则：
- 不要提取下方记忆上下文中已经存在的信息。
- 对于 thematic_facts：如果主题已存在，请提供更新/合并后的内容版本（合并新旧信息）。使用完全相同的主题名称。
- 对于 core_profile_update：仅在用户基本身份信息发生变化时提供（姓名、语言、核心限制条件）。与现有档案合并。
- 如果没有值得记住的新事实信息，返回空 JSON 对象 {}。
- 不要提取助手的观点，只提取用户透露的事实信息。
- 所有输出内容必须使用中文。
`)
	} else {
		sb.WriteString(`You are a memory extraction assistant. Analyze the following conversation between a User and an Assistant.
Extract any NEW long-term factual information about the User (e.g., preferences, habits, facts, constraints).

IMPORTANT RULES:
- Do NOT extract information that already exists in the memory context below.
- For thematic_facts: if the topic already exists, provide an UPDATED/MERGED version of the content (combining old + new info). Use the EXACT same topic name.
- For core_profile_update: only provide if fundamental user identity info changed (name, language, core constraints). Merge with existing profile.
- If there is no new factual information worth remembering, return an empty JSON object {}.
- Do NOT extract the assistant's opinions, only factual information revealed by the user.
`)
	}

	if existingContext != "" {
		if isZh {
			sb.WriteString("\n# 当前记忆上下文\n")
		} else {
			sb.WriteString("\n# Current Memory Context\n")
		}
		sb.WriteString(existingContext)
	}

	if isZh {
		sb.WriteString(`
仅以如下格式的 JSON 对象回复（所有文本值必须使用中文）：
{
  "event_stream": "本轮对话中揭示的事实事件或信息的简要总结。如无新内容则为空字符串。",
  "core_profile_update": "如需更新则提供更新后的核心档案文本。否则为空字符串。",
  "thematic_facts": [
    {
      "topic": "主题概括（如果是更新已有主题，使用完全相同的主题名称）",
      "content": "该主题的完整更新事实（如适用则与现有内容合并）。"
    }
  ]
}

对话：
用户：%s
助手：%s`)
	} else {
		sb.WriteString(`
Respond ONLY with a JSON object in the following format:
{
  "event_stream": "A concise summary of the factual event or information revealed in this turn. Empty string if nothing new.",
  "core_profile_update": "Updated core profile text if needed. Empty string otherwise.",
  "thematic_facts": [
    {
      "topic": "The general topic (use EXISTING topic name if updating)",
      "content": "The COMPLETE updated fact for this topic (merged with existing if applicable)."
    }
  ]
}

Conversation:
User: %s
Assistant: %s`)
	}

	return sb.String()
}
