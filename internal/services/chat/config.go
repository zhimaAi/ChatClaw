package chat

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	einoagent "chatclaw/internal/eino/agent"
	"chatclaw/internal/errs"
	"chatclaw/internal/services/toolchain"

	"github.com/uptrace/bun"
)

// AgentExtras contains additional agent configuration not in einoagent.Config
type AgentExtras struct {
	AgentID        int64
	LibraryIDs     []int64
	MatchThreshold float64
	MemoryEnabled  bool
	ChatMode       string // "chat" or "task"
}

// getAgentAndProviderConfig gets the agent and provider configuration for a conversation
func (s *ChatService) getAgentAndProviderConfig(ctx context.Context, db *bun.DB, conversationID int64) (einoagent.Config, einoagent.ProviderConfig, AgentExtras, error) {
	type conversationRow struct {
		AgentID        int64  `bun:"agent_id"`
		LLMProviderID  string `bun:"llm_provider_id"`
		LLMModelID     string `bun:"llm_model_id"`
		LibraryIDs     string `bun:"library_ids"`
		EnableThinking bool   `bun:"enable_thinking"`
		ChatMode       string `bun:"chat_mode"`
	}
	var conv conversationRow
	if err := db.NewSelect().
		Table("conversations").
		Column("agent_id", "llm_provider_id", "llm_model_id", "library_ids", "enable_thinking", "chat_mode").
		Where("id = ?", conversationID).
		Scan(ctx, &conv); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return einoagent.Config{}, einoagent.ProviderConfig{}, AgentExtras{}, errs.New("error.chat_conversation_not_found")
		}
		return einoagent.Config{}, einoagent.ProviderConfig{}, AgentExtras{}, errs.Wrap("error.chat_conversation_read_failed", err)
	}

	var convLibraryIDs []int64
	if conv.LibraryIDs != "" && conv.LibraryIDs != "[]" {
		if err := json.Unmarshal([]byte(conv.LibraryIDs), &convLibraryIDs); err != nil {
			s.app.Logger.Warn("[chat] failed to parse library_ids", "conv", conversationID, "error", err)
			convLibraryIDs = []int64{}
		}
	}

	type agentRow struct {
		Name                    string  `bun:"name"`
		Prompt                  string  `bun:"prompt"`
		DefaultLLMProviderID    string  `bun:"default_llm_provider_id"`
		DefaultLLMModelID       string  `bun:"default_llm_model_id"`
		LLMTemperature          float64 `bun:"llm_temperature"`
		LLMTopP                 float64 `bun:"llm_top_p"`
		LLMMaxTokens            int     `bun:"llm_max_tokens"`
		EnableLLMTemperature    bool    `bun:"enable_llm_temperature"`
		EnableLLMTopP           bool    `bun:"enable_llm_top_p"`
		EnableLLMMaxTokens      bool    `bun:"enable_llm_max_tokens"`
		LLMMaxContextCount      int     `bun:"llm_max_context_count"`
		RetrievalTopK           int     `bun:"retrieval_top_k"`
		RetrievalMatchThreshold float64 `bun:"retrieval_match_threshold"`
		SandboxMode             string  `bun:"sandbox_mode"`
		SandboxNetwork          bool    `bun:"sandbox_network"`
		WorkDir                 string  `bun:"work_dir"`
	}
	var agent agentRow
	if err := db.NewSelect().
		Table("agents").
		Column("name", "prompt", "default_llm_provider_id", "default_llm_model_id",
			"llm_temperature", "llm_top_p", "llm_max_tokens",
			"enable_llm_temperature", "enable_llm_top_p", "enable_llm_max_tokens",
			"llm_max_context_count", "retrieval_top_k", "retrieval_match_threshold",
			"sandbox_mode", "sandbox_network", "work_dir").
		Where("id = ?", conv.AgentID).
		Scan(ctx, &agent); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return einoagent.Config{}, einoagent.ProviderConfig{}, AgentExtras{}, errs.New("error.chat_agent_not_found")
		}
		return einoagent.Config{}, einoagent.ProviderConfig{}, AgentExtras{}, errs.Wrap("error.chat_agent_read_failed", err)
	}

	providerID := conv.LLMProviderID
	modelID := conv.LLMModelID
	if providerID == "" {
		providerID = agent.DefaultLLMProviderID
	}
	if modelID == "" {
		modelID = agent.DefaultLLMModelID
	}

	if providerID == "" || modelID == "" {
		return einoagent.Config{}, einoagent.ProviderConfig{}, AgentExtras{}, errs.New("error.chat_model_not_configured")
	}

	type providerRow struct {
		Type        string `bun:"type"`
		APIKey      string `bun:"api_key"`
		APIEndpoint string `bun:"api_endpoint"`
		ExtraConfig string `bun:"extra_config"`
		Enabled     bool   `bun:"enabled"`
	}
	var provider providerRow
	if err := db.NewSelect().
		Table("providers").
		Column("type", "api_key", "api_endpoint", "extra_config", "enabled").
		Where("provider_id = ?", providerID).
		Scan(ctx, &provider); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return einoagent.Config{}, einoagent.ProviderConfig{}, AgentExtras{}, errs.Newf("error.chat_provider_not_found", map[string]any{"ProviderID": providerID})
		}
		return einoagent.Config{}, einoagent.ProviderConfig{}, AgentExtras{}, errs.Wrap("error.chat_provider_read_failed", err)
	}

	if !provider.Enabled {
		return einoagent.Config{}, einoagent.ProviderConfig{}, AgentExtras{}, errs.New("error.chat_provider_not_enabled")
	}

	instruction := fmt.Sprintf("# System Instruction\n\n%s", strings.TrimSpace(agent.Prompt))

	var memoryEnabledStr string
	_ = db.NewSelect().Table("settings").Column("value").Where("key = ?", "memory_enabled").Scan(ctx, &memoryEnabledStr)
	memoryEnabled := memoryEnabledStr == "true"

	agentConfig := einoagent.Config{
		Name:            agent.Name,
		Instruction:     instruction,
		ModelID:         modelID,
		Temperature:     &agent.LLMTemperature,
		TopP:            &agent.LLMTopP,
		MaxTokens:       &agent.LLMMaxTokens,
		EnableTemp:      agent.EnableLLMTemperature,
		EnableTopP:      agent.EnableLLMTopP,
		EnableMaxTokens: agent.EnableLLMMaxTokens,
		ContextCount:    agent.LLMMaxContextCount,
		RetrievalTopK:   agent.RetrievalTopK,
		EnableThinking:  conv.EnableThinking,
		SandboxMode:     agent.SandboxMode,
		SandboxNetwork:  agent.SandboxNetwork,
		WorkDir:         agent.WorkDir,
		AgentID:         conv.AgentID,
		ConversationID:  conversationID,
		ToolchainBinDir: toolchain.BinDirIfReady(),
	}

	providerConfig := einoagent.ProviderConfig{
		ProviderID:  providerID,
		Type:        provider.Type,
		APIKey:      provider.APIKey,
		APIEndpoint: provider.APIEndpoint,
		ExtraConfig: provider.ExtraConfig,
	}

	if len(convLibraryIDs) > 0 {
		s.app.Logger.Info("[chat] using library_ids", "library_ids", convLibraryIDs)
	}

	chatMode := conv.ChatMode
	if chatMode == "" {
		chatMode = "task"
	} else if chatMode != "chat" && chatMode != "task" {
		s.app.Logger.Warn("[chat] invalid chat_mode found in conversation, fallback to task", "conv", conversationID, "chat_mode", chatMode)
		chatMode = "task"
	}

	extras := AgentExtras{
		AgentID:        conv.AgentID,
		LibraryIDs:     convLibraryIDs,
		MatchThreshold: agent.RetrievalMatchThreshold,
		MemoryEnabled:  memoryEnabled,
		ChatMode:       chatMode,
	}

	return agentConfig, providerConfig, extras, nil
}
