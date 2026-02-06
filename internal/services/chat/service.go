package chat

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	einoagent "willchat/internal/eino/agent"
	einoembed "willchat/internal/eino/embedding"
	"willchat/internal/eino/processor"
	"willchat/internal/eino/tools"
	"willchat/internal/errs"
	"willchat/internal/services/retrieval"
	"willchat/internal/sqlite"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
	"github.com/wailsapp/wails/v3/pkg/application"
)

var (
	// Enable LLM request/response previews (may contain user content).
	debugLLM = os.Getenv("WILLCHAT_DEBUG_LLM") == "1"
)

func truncateRunes(s string, max int) string {
	if max <= 0 || s == "" {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "...(truncated)"
}

func summarizeMessagesForLog(messages []*schema.Message, maxMsgs int, maxContent int) string {
	if len(messages) == 0 {
		return ""
	}
	start := 0
	if maxMsgs > 0 && len(messages) > maxMsgs {
		start = len(messages) - maxMsgs
	}
	var b strings.Builder
	for i := start; i < len(messages); i++ {
		m := messages[i]
		b.WriteString("[")
		b.WriteString(string(m.Role))
		b.WriteString("] ")
		b.WriteString(truncateRunes(m.Content, maxContent))
		if m.ToolCallID != "" {
			b.WriteString(" tool_call_id=")
			b.WriteString(m.ToolCallID)
		}
		if m.Name != "" {
			b.WriteString(" name=")
			b.WriteString(m.Name)
		}
		if i != len(messages)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// activeGeneration tracks an active generation
type activeGeneration struct {
	cancel    context.CancelFunc
	requestID string
	tabID     string
	done      chan struct{} // closed when the generation goroutine finishes
}

// ChatService handles chat operations
type ChatService struct {
	app               *application.App
	toolRegistry      *tools.ToolRegistry
	activeGenerations sync.Map // map[int64]*activeGeneration
}

// NewChatService creates a new ChatService
func NewChatService(app *application.App) *ChatService {
	return &ChatService{
		app:          app,
		toolRegistry: tools.NewToolRegistry(),
	}
}

func (s *ChatService) db() (*bun.DB, error) {
	db := sqlite.DB()
	if db == nil {
		return nil, errs.New("error.sqlite_not_initialized")
	}
	return db, nil
}

// GetMessages returns all messages for a conversation
func (s *ChatService) GetMessages(conversationID int64) ([]Message, error) {
	if conversationID <= 0 {
		return nil, errs.New("error.chat_conversation_id_required")
	}

	db, err := s.db()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var models []messageModel
	if err := db.NewSelect().
		Model(&models).
		Where("conversation_id = ?", conversationID).
		OrderExpr("created_at ASC, id ASC").
		Scan(ctx); err != nil {
		return nil, errs.Wrap("error.chat_messages_failed", err)
	}

	messages := make([]Message, len(models))
	for i := range models {
		messages[i] = models[i].toDTO()
	}
	return messages, nil
}

// SendMessage sends a message and starts a ReAct generation loop
func (s *ChatService) SendMessage(input SendMessageInput) (*SendMessageResult, error) {
	if input.ConversationID <= 0 {
		return nil, errs.New("error.chat_conversation_id_required")
	}
	content := strings.TrimSpace(input.Content)
	if content == "" {
		return nil, errs.New("error.chat_content_required")
	}

	s.app.Logger.Info("[chat] SendMessage", "conv", input.ConversationID, "tab", input.TabID, "content_len", len(content))

	// Check if there's already an active generation for this conversation
	if existing, ok := s.activeGenerations.Load(input.ConversationID); ok {
		gen := existing.(*activeGeneration)
		if gen.tabID != input.TabID {
			return nil, errs.New("error.chat_generation_in_progress_other_tab")
		}
		return nil, errs.New("error.chat_generation_in_progress")
	}

	db, err := s.db()
	if err != nil {
		return nil, err
	}

	ctx := context.Background()

	// Get conversation and agent info
	agentConfig, providerConfig, agentExtras, err := s.getAgentAndProviderConfig(ctx, db, input.ConversationID)
	if err != nil {
		return nil, err
	}

	// Generate request ID
	requestID := uuid.New().String()

	// Create cancellable context for generation
	genCtx, cancel := context.WithCancel(ctx)

	// Register active generation
	gen := &activeGeneration{
		cancel:    cancel,
		requestID: requestID,
		tabID:     input.TabID,
		done:      make(chan struct{}),
	}
	s.activeGenerations.Store(input.ConversationID, gen)

	// Start generation in goroutine
	go func() {
		defer close(gen.done)
		defer s.tryDeleteGeneration(input.ConversationID, gen)
		s.runGeneration(genCtx, db, input.ConversationID, input.TabID, requestID, content, agentConfig, providerConfig, agentExtras)
	}()

	return &SendMessageResult{
		RequestID: requestID,
		MessageID: 0, // Will be sent via event
	}, nil
}

// EditAndResend edits a message and resends
func (s *ChatService) EditAndResend(input EditAndResendInput) (*SendMessageResult, error) {
	if input.ConversationID <= 0 {
		return nil, errs.New("error.chat_conversation_id_required")
	}
	if input.MessageID <= 0 {
		return nil, errs.New("error.chat_message_id_required")
	}
	content := strings.TrimSpace(input.NewContent)
	if content == "" {
		return nil, errs.New("error.chat_content_required")
	}

	s.app.Logger.Info("[chat] EditAndResend", "conv", input.ConversationID, "tab", input.TabID, "msg", input.MessageID, "content_len", len(content))

	// Stop any existing generation and wait for it to finish
	if existing, ok := s.activeGenerations.Load(input.ConversationID); ok {
		oldGen := existing.(*activeGeneration)
		oldGen.cancel()
		s.activeGenerations.Delete(input.ConversationID)
		// Wait for old goroutine to finish (with timeout to avoid deadlock)
		select {
		case <-oldGen.done:
			// Old generation finished cleanly
		case <-time.After(3 * time.Second):
			// Timeout: old goroutine may still be running and could write stale data.
			// Refuse to start a new generation to avoid data races.
			s.app.Logger.Error("[chat] old generation did not finish within timeout, refusing to start new generation", "conv", input.ConversationID)
			return nil, errs.New("error.chat_previous_generation_not_finished")
		}
	}

	db, err := s.db()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Verify the message exists and belongs to this conversation
	var msg messageModel
	if err := db.NewSelect().
		Model(&msg).
		Where("id = ?", input.MessageID).
		Where("conversation_id = ?", input.ConversationID).
		Where("role = ?", RoleUser).
		Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.New("error.chat_message_not_found")
		}
		return nil, errs.Wrap("error.chat_message_read_failed", err)
	}

	// Delete all messages after this one
	if err := s.deleteMessagesAfter(ctx, db, input.ConversationID, input.MessageID, false); err != nil {
		return nil, err
	}

	// Update the message content
	if _, err := db.NewUpdate().
		Model((*messageModel)(nil)).
		Set("content = ?", content).
		Where("id = ?", input.MessageID).
		Exec(ctx); err != nil {
		return nil, errs.Wrap("error.chat_message_update_failed", err)
	}

	// Get agent and provider config
	agentConfig, providerConfig, agentExtras, err := s.getAgentAndProviderConfig(ctx, db, input.ConversationID)
	if err != nil {
		return nil, err
	}

	// Generate request ID
	requestID := uuid.New().String()

	// Create cancellable context for generation
	genCtx, genCancel := context.WithCancel(context.Background())

	// Register active generation
	gen := &activeGeneration{
		cancel:    genCancel,
		requestID: requestID,
		tabID:     input.TabID,
		done:      make(chan struct{}),
	}
	s.activeGenerations.Store(input.ConversationID, gen)

	// Start generation in goroutine (don't insert user message, it's already there)
	go func() {
		defer close(gen.done)
		defer s.tryDeleteGeneration(input.ConversationID, gen)
		s.runGenerationWithExistingHistory(genCtx, db, input.ConversationID, input.TabID, requestID, agentConfig, providerConfig, agentExtras)
	}()

	return &SendMessageResult{
		RequestID: requestID,
		MessageID: input.MessageID,
	}, nil
}

// StopGeneration stops the current generation for a conversation
func (s *ChatService) StopGeneration(conversationID int64) error {
	if conversationID <= 0 {
		return errs.New("error.chat_conversation_id_required")
	}

	existing, ok := s.activeGenerations.Load(conversationID)
	if !ok {
		return errs.New("error.chat_no_active_generation")
	}

	gen := existing.(*activeGeneration)
	gen.cancel()
	// Note: cleanup will happen in runGeneration when context is cancelled
	return nil
}

// deleteMessagesAfter deletes all messages after the given message ID
// archive parameter is reserved for future use
func (s *ChatService) deleteMessagesAfter(ctx context.Context, db *bun.DB, conversationID, messageID int64, archive bool) error {
	// TODO: If archive is true, move messages to an archive table instead of deleting
	_, err := db.NewDelete().
		Model((*messageModel)(nil)).
		Where("conversation_id = ?", conversationID).
		Where("id > ?", messageID).
		Exec(ctx)
	if err != nil {
		return errs.Wrap("error.chat_messages_delete_failed", err)
	}
	return nil
}

// AgentExtras contains additional agent configuration not in einoagent.Config
type AgentExtras struct {
	LibraryIDs     []int64
	MatchThreshold float64
}

// getAgentAndProviderConfig gets the agent and provider configuration for a conversation
func (s *ChatService) getAgentAndProviderConfig(ctx context.Context, db *bun.DB, conversationID int64) (einoagent.Config, einoagent.ProviderConfig, AgentExtras, error) {
	// Get conversation
	type conversationRow struct {
		AgentID        int64  `bun:"agent_id"`
		LLMProviderID  string `bun:"llm_provider_id"`
		LLMModelID     string `bun:"llm_model_id"`
		LibraryIDs     string `bun:"library_ids"`
		EnableThinking bool   `bun:"enable_thinking"`
	}
	var conv conversationRow
	if err := db.NewSelect().
		Table("conversations").
		Column("agent_id", "llm_provider_id", "llm_model_id", "library_ids", "enable_thinking").
		Where("id = ?", conversationID).
		Scan(ctx, &conv); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return einoagent.Config{}, einoagent.ProviderConfig{}, AgentExtras{}, errs.New("error.chat_conversation_not_found")
		}
		return einoagent.Config{}, einoagent.ProviderConfig{}, AgentExtras{}, errs.Wrap("error.chat_conversation_read_failed", err)
	}

	// Parse conversation-level library_ids JSON array
	var convLibraryIDs []int64
	if conv.LibraryIDs != "" && conv.LibraryIDs != "[]" {
		if err := json.Unmarshal([]byte(conv.LibraryIDs), &convLibraryIDs); err != nil {
			s.app.Logger.Warn("[chat] failed to parse library_ids", "conv", conversationID, "error", err)
			// Continue with empty library IDs on parse error
			convLibraryIDs = []int64{}
		}
	}

	// Get agent
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
	}
	var agent agentRow
	if err := db.NewSelect().
		Table("agents").
		Column("name", "prompt", "default_llm_provider_id", "default_llm_model_id",
			"llm_temperature", "llm_top_p", "llm_max_tokens",
			"enable_llm_temperature", "enable_llm_top_p", "enable_llm_max_tokens",
			"llm_max_context_count", "retrieval_top_k", "retrieval_match_threshold").
		Where("id = ?", conv.AgentID).
		Scan(ctx, &agent); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return einoagent.Config{}, einoagent.ProviderConfig{}, AgentExtras{}, errs.New("error.chat_agent_not_found")
		}
		return einoagent.Config{}, einoagent.ProviderConfig{}, AgentExtras{}, errs.Wrap("error.chat_agent_read_failed", err)
	}

	// Determine which provider/model to use (conversation overrides agent default)
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

	// Get provider
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

	agentConfig := einoagent.Config{
		Name:            agent.Name,
		Instruction:     agent.Prompt,
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
	}

	providerConfig := einoagent.ProviderConfig{
		ProviderID:  providerID,
		Type:        provider.Type,
		APIKey:      provider.APIKey,
		APIEndpoint: provider.APIEndpoint,
		ExtraConfig: provider.ExtraConfig,
	}

	// Use conversation-level library_ids for retrieval
	if len(convLibraryIDs) > 0 {
		s.app.Logger.Info("[chat] using library_ids", "library_ids", convLibraryIDs)
	}

	extras := AgentExtras{
		LibraryIDs:     convLibraryIDs,
		MatchThreshold: agent.RetrievalMatchThreshold,
	}

	return agentConfig, providerConfig, extras, nil
}

// tryDeleteGeneration removes the generation from the map only if it is still the active one.
// This prevents a finishing old goroutine from deleting a newer generation's entry.
func (s *ChatService) tryDeleteGeneration(conversationID int64, gen *activeGeneration) {
	if cur, ok := s.activeGenerations.Load(conversationID); ok && cur == gen {
		s.activeGenerations.Delete(conversationID)
	}
}

// runGeneration runs the generation loop
func (s *ChatService) runGeneration(ctx context.Context, db *bun.DB, conversationID int64, tabID, requestID, userContent string, agentConfig einoagent.Config, providerConfig einoagent.ProviderConfig, agentExtras AgentExtras) {

	var seq int32 = 0
	nextSeq := func() int {
		return int(atomic.AddInt32(&seq, 1))
	}

	emit := func(eventName string, payload any) {
		s.app.Event.Emit(eventName, payload)
	}

	emitError := func(errorKey string, errorData any) {
		s.app.Logger.Error("[chat] error", "conv", conversationID, "tab", tabID, "req", requestID, "key", errorKey, "data", errorData)
		emit(EventChatError, ChatErrorEvent{
			ChatEvent: ChatEvent{
				ConversationID: conversationID,
				TabID:          tabID,
				RequestID:      requestID,
				Seq:            nextSeq(),
				Ts:             time.Now().UnixMilli(),
			},
			Status:    StatusError,
			ErrorKey:  errorKey,
			ErrorData: errorData,
		})
	}

	// Insert user message
	userMsg := &messageModel{
		ConversationID: conversationID,
		Role:           RoleUser,
		Content:        userContent,
		Status:         StatusSuccess,
		ToolCalls:      "[]",
	}

	dbCtx, dbCancel := context.WithTimeout(context.Background(), 5*time.Second)
	if _, err := db.NewInsert().Model(userMsg).Exec(dbCtx); err != nil {
		dbCancel()
		emitError("error.chat_message_save_failed", nil)
		return
	}
	dbCancel()

	// Run generation with existing history
	s.runGenerationWithExistingHistory(ctx, db, conversationID, tabID, requestID, agentConfig, providerConfig, agentExtras)
}

// runGenerationWithExistingHistory runs the generation loop with existing message history
func (s *ChatService) runGenerationWithExistingHistory(ctx context.Context, db *bun.DB, conversationID int64, tabID, requestID string, agentConfig einoagent.Config, providerConfig einoagent.ProviderConfig, agentExtras AgentExtras) {

	var seq int32 = 0
	nextSeq := func() int {
		return int(atomic.AddInt32(&seq, 1))
	}

	emit := func(eventName string, payload any) {
		s.app.Event.Emit(eventName, payload)
	}

	emitError := func(errorKey string, errorData any) {
		s.app.Logger.Error("[chat] error", "conv", conversationID, "tab", tabID, "req", requestID, "key", errorKey, "data", errorData)
		emit(EventChatError, ChatErrorEvent{
			ChatEvent: ChatEvent{
				ConversationID: conversationID,
				TabID:          tabID,
				RequestID:      requestID,
				Seq:            nextSeq(),
				Ts:             time.Now().UnixMilli(),
			},
			Status:    StatusError,
			ErrorKey:  errorKey,
			ErrorData: errorData,
		})
	}

	// Create assistant message placeholder
	assistantMsg := &messageModel{
		ConversationID: conversationID,
		Role:           RoleAssistant,
		Content:        "",
		ProviderID:     providerConfig.ProviderID,
		ModelID:        agentConfig.ModelID,
		Status:         StatusStreaming,
		ToolCalls:      "[]",
	}

	dbCtx, dbCancel := context.WithTimeout(context.Background(), 5*time.Second)
	if _, err := db.NewInsert().Model(assistantMsg).Exec(dbCtx); err != nil {
		dbCancel()
		emitError("error.chat_message_save_failed", nil)
		return
	}
	dbCancel()

	// Emit start event
	emit(EventChatStart, ChatStartEvent{
		ChatEvent: ChatEvent{
			ConversationID: conversationID,
			TabID:          tabID,
			RequestID:      requestID,
			Seq:            nextSeq(),
			MessageID:      assistantMsg.ID,
			Ts:             time.Now().UnixMilli(),
		},
		Status: StatusStreaming,
	})

	// Load existing messages for context
	messages, err := s.loadMessagesForContext(ctx, db, conversationID, agentConfig.ContextCount)
	if err != nil {
		emitError("error.chat_messages_failed", nil)
		s.updateMessageStatus(db, assistantMsg.ID, StatusError, "Failed to load messages", "")
		return
	}

	// LLM request log (summary)
	s.app.Logger.Info("[llm] start", "conv", conversationID, "tab", tabID, "req", requestID,
		"provider_id", providerConfig.ProviderID, "provider_type", providerConfig.Type,
		"model", agentConfig.ModelID, "endpoint", providerConfig.APIEndpoint, "messages", len(messages))
	if debugLLM {
		s.app.Logger.Debug("[llm] context", "conv", conversationID, "req", requestID, "context", summarizeMessagesForLog(messages, 12, 160))
	}

	// Create extra tools (e.g., LibraryRetrieverTool if agent has associated libraries)
	var extraTools []tool.BaseTool
	if len(agentExtras.LibraryIDs) > 0 {
		retrieverTool, toolErr := s.createLibraryRetrieverTool(ctx, db, agentExtras.LibraryIDs, agentConfig.RetrievalTopK, agentExtras.MatchThreshold)
		if toolErr != nil {
			s.app.Logger.Warn("[chat] failed to create library retriever tool", "error", toolErr)
			// Continue without the retriever tool
		} else if retrieverTool != nil {
			extraTools = append(extraTools, retrieverTool)
			s.app.Logger.Info("[chat] library retriever tool created", "libraries", len(agentExtras.LibraryIDs), "topK", agentConfig.RetrievalTopK, "threshold", agentExtras.MatchThreshold)
		}
	}

	// Create agent
	agentConfig.Provider = providerConfig
	agent, err := einoagent.NewChatModelAgent(ctx, agentConfig, s.toolRegistry, extraTools)
	if err != nil {
		emitError("error.chat_agent_create_failed", map[string]any{"Error": err.Error()})
		s.updateMessageStatus(db, assistantMsg.ID, StatusError, err.Error(), "")
		return
	}

	// Create runner
	runner := adk.NewRunner(ctx, adk.RunnerConfig{
		Agent:           agent,
		EnableStreaming: true,
	})

	// Run generation
	var contentBuilder strings.Builder
	var thinkingBuilder strings.Builder
	var toolCallsJSON []byte
	var finishReason string
	var inputTokens, outputTokens int

	// Track segments for interleaved thinking/content/tool-call display
	type segment struct {
		Type        string   `json:"type"`                    // "thinking", "content" or "tools"
		Content     string   `json:"content,omitempty"`       // for type="content" or "thinking"
		ToolCallIDs []string `json:"tool_call_ids,omitempty"` // for type="tools"
	}
	var segments []segment
	var lastSegmentType string                             // "thinking", "content", or "tools"
	var lastSegmentToolCallIDs map[string]bool // to track which tool calls are in the last segment

	// Helper to add thinking to segments
	addThinkingToSegments := func(thinking string) {
		if thinking == "" {
			return
		}
		if lastSegmentType == "thinking" && len(segments) > 0 {
			// Append to last thinking segment
			segments[len(segments)-1].Content += thinking
		} else {
			// Start new thinking segment
			segments = append(segments, segment{Type: "thinking", Content: thinking})
			lastSegmentType = "thinking"
			lastSegmentToolCallIDs = nil
		}
	}

	// Helper to add content to segments
	addContentToSegments := func(content string) {
		if content == "" {
			return
		}
		if lastSegmentType == "content" && len(segments) > 0 {
			// Append to last content segment
			segments[len(segments)-1].Content += content
		} else {
			// Start new content segment
			segments = append(segments, segment{Type: "content", Content: content})
			lastSegmentType = "content"
			lastSegmentToolCallIDs = nil
		}
	}

	// Helper to add tool call to segments
	addToolCallToSegments := func(toolCallID string) {
		if toolCallID == "" {
			return
		}
		if lastSegmentType != "tools" || len(segments) == 0 {
			// Start new tools segment
			segments = append(segments, segment{Type: "tools", ToolCallIDs: []string{toolCallID}})
			lastSegmentType = "tools"
			lastSegmentToolCallIDs = map[string]bool{toolCallID: true}
		} else if !lastSegmentToolCallIDs[toolCallID] {
			// Add to existing tools segment (if not already there)
			segments[len(segments)-1].ToolCallIDs = append(segments[len(segments)-1].ToolCallIDs, toolCallID)
			lastSegmentToolCallIDs[toolCallID] = true
		}
	}

	// Track tool call deltas from streaming providers.
	// Some OpenAI-compatible providers stream tool_calls in multiple chunks where
	// function.name / function.arguments / id might be temporarily empty.
	type toolCallState struct {
		id   string
		name string
		args string
	}
	toolStatesByKey := make(map[string]*toolCallState) // key can be id or idx fallback
	toolOrder := make([]string, 0)

	updateArgs := func(oldArgs, newArgs string) string {
		if newArgs == "" {
			return oldArgs
		}
		if oldArgs == "" {
			return newArgs
		}
		// If provider sends snapshots, newArgs should start with oldArgs.
		if strings.HasPrefix(newArgs, oldArgs) {
			return newArgs
		}
		// If provider sends an older snapshot, ignore.
		if strings.HasPrefix(oldArgs, newArgs) {
			return oldArgs
		}
		// Otherwise treat as delta chunk.
		return oldArgs + newArgs
	}

	buildToolCallsForDB := func() []schema.ToolCall {
		out := make([]schema.ToolCall, 0, len(toolOrder))
		seen := make(map[string]struct{})
		for _, key := range toolOrder {
			st := toolStatesByKey[key]
			if st == nil || st.id == "" || st.name == "" {
				continue
			}
			if _, ok := seen[st.id]; ok {
				continue
			}
			seen[st.id] = struct{}{}
			args := st.args
			if !json.Valid([]byte(args)) {
				// Keep history valid for providers that strictly require JSON arguments.
				args = "{}"
			}
			out = append(out, schema.ToolCall{
				ID: st.id,
				Function: schema.FunctionCall{
					Name:      st.name,
					Arguments: args,
				},
			})
		}
		return out
	}

	// indexKeyMap tracks the canonical key used for each streaming index so that
	// subsequent delta chunks (which may lack an ID) merge into the correct state.
	indexKeyMap := make(map[int]string)

	updateToolStates := func(toolCalls []schema.ToolCall) {
		for i, tc := range toolCalls {
			// Determine the index for this chunk (prefer explicit Index, fallback to slice position).
			idx := i
			if tc.Index != nil {
				idx = *tc.Index
			}

			// Determine the lookup key: prefer ID, then check if we already have a
			// canonical key for this index (from a prior chunk that carried the ID),
			// and finally fall back to "idx_N".
			key := tc.ID
			if key == "" {
				if existing, ok := indexKeyMap[idx]; ok {
					key = existing
				} else {
					key = fmt.Sprintf("idx_%d", idx)
				}
			}

			st, ok := toolStatesByKey[key]
			if !ok {
				st = &toolCallState{}
				toolStatesByKey[key] = st
				toolOrder = append(toolOrder, key)
			}
			if tc.ID != "" {
				st.id = tc.ID
				// Register this key as the canonical key for the index so that
				// future delta chunks without an ID can find it.
				indexKeyMap[idx] = key
			}
			if tc.Function.Name != "" {
				st.name = tc.Function.Name
			}
			st.args = updateArgs(st.args, tc.Function.Arguments)
		}

		// Keep toolCallsJSON updated for DB persistence.
		if calls := buildToolCallsForDB(); len(calls) > 0 {
			toolCallsJSON, _ = json.Marshal(calls)
		}
	}

	iter := runner.Run(ctx, messages)
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}

		// Check for cancellation
		if ctx.Err() != nil {
			// Save partial content
			segmentsJSON, _ := json.Marshal(segments)
			s.updateMessageFinal(db, assistantMsg.ID, contentBuilder.String(), thinkingBuilder.String(), string(toolCallsJSON), string(segmentsJSON), StatusCancelled, "", "cancelled", inputTokens, outputTokens)
			emit(EventChatStopped, ChatStoppedEvent{
				ChatEvent: ChatEvent{
					ConversationID: conversationID,
					TabID:          tabID,
					RequestID:      requestID,
					Seq:            nextSeq(),
					MessageID:      assistantMsg.ID,
					Ts:             time.Now().UnixMilli(),
				},
				Status: StatusCancelled,
			})
			return
		}

		if event.Err != nil {
			errMsg := event.Err.Error()
			errorKey := "error.chat_generation_failed"
			if strings.Contains(errMsg, "exceeds max iterations") {
				errorKey = "error.max_iterations_exceeded"
			}
			s.app.Logger.Error("[chat] generation failed", "conv", conversationID, "tab", tabID, "req", requestID, "error", event.Err)
			emitError(errorKey, map[string]any{"Error": errMsg})
			// Save partial content accumulated before the error
			toolCallsStr := "[]"
			if len(toolCallsJSON) > 0 {
				toolCallsStr = string(toolCallsJSON)
			}
			segmentsStr := "[]"
			if len(segments) > 0 {
				if segBytes, err := json.Marshal(segments); err == nil {
					segmentsStr = string(segBytes)
				}
			}
			s.updateMessageFinal(db, assistantMsg.ID, contentBuilder.String(), thinkingBuilder.String(), toolCallsStr, segmentsStr, StatusError, errMsg, "", inputTokens, outputTokens)
			return
		}

		if event.Output != nil && event.Output.MessageOutput != nil {
			msgOutput := event.Output.MessageOutput

			if msgOutput.IsStreaming && msgOutput.MessageStream != nil {
				// Process streaming
				for {
					msg, err := msgOutput.MessageStream.Recv()
					if err == io.EOF {
						break
					}
					if err != nil {
						if ctx.Err() != nil {
							// Context cancelled
							break
						}
						s.app.Logger.Error("[chat] stream recv failed", "conv", conversationID, "tab", tabID, "req", requestID, "error", err)
						emitError("error.chat_stream_failed", map[string]any{"Error": err.Error()})
						break
					}

					// Handle thinking content (ReasoningContent)
					if msg.ReasoningContent != "" {
						thinkingBuilder.WriteString(msg.ReasoningContent)
						addThinkingToSegments(msg.ReasoningContent)
						emit(EventChatThinking, ChatThinkingEvent{
							ChatEvent: ChatEvent{
								ConversationID: conversationID,
								TabID:          tabID,
								RequestID:      requestID,
								Seq:            nextSeq(),
								MessageID:      assistantMsg.ID,
								Ts:             time.Now().UnixMilli(),
							},
							Delta: msg.ReasoningContent,
						})
					}

					// Handle content
					if msg.Content != "" {
						contentBuilder.WriteString(msg.Content)
						addContentToSegments(msg.Content)
						emit(EventChatChunk, ChatChunkEvent{
							ChatEvent: ChatEvent{
								ConversationID: conversationID,
								TabID:          tabID,
								RequestID:      requestID,
								Seq:            nextSeq(),
								MessageID:      assistantMsg.ID,
								Ts:             time.Now().UnixMilli(),
							},
							Delta: msg.Content,
						})
					}

					// Handle tool calls
					if len(msg.ToolCalls) > 0 {
						updateToolStates(msg.ToolCalls)
						// Emit tool call updates for chunks that carry an ID or can be
						// resolved to a known tool call via indexKeyMap.
						for i, tc := range msg.ToolCalls {
							idx := i
							if tc.Index != nil {
								idx = *tc.Index
							}

							// Resolve the tool call ID: use explicit ID, or look up via index.
							resolvedID := tc.ID
							if resolvedID == "" {
								if canonicalKey, ok := indexKeyMap[idx]; ok {
									if st := toolStatesByKey[canonicalKey]; st != nil {
										resolvedID = st.id
									}
								}
							}
							if resolvedID == "" {
								continue
							}

							// Look up accumulated state for this tool call.
							var toolName, args string
							for _, key := range toolOrder {
								st := toolStatesByKey[key]
								if st != nil && st.id == resolvedID {
									toolName = st.name
									args = st.args
									break
								}
							}
							if toolName == "" {
								continue
							}
							// Add tool call to segments
							addToolCallToSegments(resolvedID)
							if args != "" && !json.Valid([]byte(args)) {
								s.app.Logger.Warn("[chat] tool arguments not valid JSON", "conv", conversationID, "tab", tabID, "req", requestID, "tool", toolName, "call_id", resolvedID, "args", args)
							}
							s.app.Logger.Info("[llm] tool_call", "conv", conversationID, "tab", tabID, "req", requestID, "tool", toolName, "call_id", resolvedID, "args", truncateRunes(args, 300))
							emit(EventChatTool, ChatToolEvent{
								ChatEvent: ChatEvent{
									ConversationID: conversationID,
									TabID:          tabID,
									RequestID:      requestID,
									Seq:            nextSeq(),
									MessageID:      assistantMsg.ID,
									Ts:             time.Now().UnixMilli(),
								},
								Type:       "call",
								ToolCallID: resolvedID,
								ToolName:   toolName,
								ArgsJSON:   args,
							})
						}
					}

					// Handle response meta
					if msg.ResponseMeta != nil {
						if msg.ResponseMeta.FinishReason != "" {
							finishReason = msg.ResponseMeta.FinishReason
						}
						if msg.ResponseMeta.Usage != nil {
							inputTokens = int(msg.ResponseMeta.Usage.PromptTokens)
							outputTokens = int(msg.ResponseMeta.Usage.CompletionTokens)
						}
					}
				}
			} else if msgOutput.Message != nil {
				// Non-streaming message
				msg := msgOutput.Message

				// Some providers output final tool_calls in a non-streaming message; capture them.
				if len(msg.ToolCalls) > 0 {
					updateToolStates(msg.ToolCalls)
				}

				if msg.Role == schema.Tool {
					// Tool result
					toolName := msg.Name
					if toolName == "" && msg.ToolCallID != "" {
						for _, key := range toolOrder {
							st := toolStatesByKey[key]
							if st != nil && st.id == msg.ToolCallID && st.name != "" {
								toolName = st.name
								break
							}
						}
					}
					s.app.Logger.Info("[llm] tool_result", "conv", conversationID, "tab", tabID, "req", requestID, "tool", toolName, "call_id", msg.ToolCallID, "result_len", len(msg.Content))
					emit(EventChatTool, ChatToolEvent{
						ChatEvent: ChatEvent{
							ConversationID: conversationID,
							TabID:          tabID,
							RequestID:      requestID,
							Seq:            nextSeq(),
							MessageID:      assistantMsg.ID,
							Ts:             time.Now().UnixMilli(),
						},
						Type:       "result",
						ToolCallID: msg.ToolCallID,
						ToolName:   toolName,
						ResultJSON: msg.Content,
					})

					// Save tool message to DB
					toolMsg := &messageModel{
						ConversationID: conversationID,
						Role:           RoleTool,
						Content:        msg.Content,
						Status:         StatusSuccess,
						ToolCallID:     msg.ToolCallID,
						ToolCallName:   toolName,
						ToolCalls:      "[]",
					}
					dbCtx, dbCancel := context.WithTimeout(context.Background(), 5*time.Second)
					if _, err := db.NewInsert().Model(toolMsg).Exec(dbCtx); err != nil {
						s.app.Logger.Warn("[chat] failed to save tool message", "conv", conversationID, "tool", toolName, "call_id", msg.ToolCallID, "error", err)
					}
					dbCancel()
				} else if msg.Content != "" {
					contentBuilder.WriteString(msg.Content)
					addContentToSegments(msg.Content)
					emit(EventChatChunk, ChatChunkEvent{
						ChatEvent: ChatEvent{
							ConversationID: conversationID,
							TabID:          tabID,
							RequestID:      requestID,
							Seq:            nextSeq(),
							MessageID:      assistantMsg.ID,
							Ts:             time.Now().UnixMilli(),
						},
						Delta: msg.Content,
					})
				}

				// Handle response meta
				if msg.ResponseMeta != nil {
					if msg.ResponseMeta.FinishReason != "" {
						finishReason = msg.ResponseMeta.FinishReason
					}
					if msg.ResponseMeta.Usage != nil {
						inputTokens = int(msg.ResponseMeta.Usage.PromptTokens)
						outputTokens = int(msg.ResponseMeta.Usage.CompletionTokens)
					}
				}
			}
		}
	}

	// Check final cancellation
	if ctx.Err() != nil {
		segmentsJSONFinal, _ := json.Marshal(segments)
		s.updateMessageFinal(db, assistantMsg.ID, contentBuilder.String(), thinkingBuilder.String(), string(toolCallsJSON), string(segmentsJSONFinal), StatusCancelled, "", "cancelled", inputTokens, outputTokens)
		s.app.Logger.Info("[llm] complete", "conv", conversationID, "tab", tabID, "req", requestID,
			"status", StatusCancelled, "finish", "cancelled", "input_tokens", inputTokens,
			"output_tokens", outputTokens, "content_len", len(contentBuilder.String()), "thinking_len", len(thinkingBuilder.String()))
		emit(EventChatStopped, ChatStoppedEvent{
			ChatEvent: ChatEvent{
				ConversationID: conversationID,
				TabID:          tabID,
				RequestID:      requestID,
				Seq:            nextSeq(),
				MessageID:      assistantMsg.ID,
				Ts:             time.Now().UnixMilli(),
			},
			Status: StatusCancelled,
		})
		return
	}

	// Update final message
	toolCallsStr := "[]"
	if len(toolCallsJSON) > 0 {
		toolCallsStr = string(toolCallsJSON)
	}
	segmentsStr := "[]"
	if len(segments) > 0 {
		if segBytes, err := json.Marshal(segments); err == nil {
			segmentsStr = string(segBytes)
		}
	}
	s.updateMessageFinal(db, assistantMsg.ID, contentBuilder.String(), thinkingBuilder.String(), toolCallsStr, segmentsStr, StatusSuccess, "", finishReason, inputTokens, outputTokens)

	// LLM completion log
	s.app.Logger.Info("[llm] complete", "conv", conversationID, "tab", tabID, "req", requestID,
		"status", StatusSuccess, "finish", finishReason, "input_tokens", inputTokens,
		"output_tokens", outputTokens, "content_len", len(contentBuilder.String()),
		"thinking_len", len(thinkingBuilder.String()), "tool_calls_len", len(toolCallsStr))
	if debugLLM {
		s.app.Logger.Debug("[llm] output", "conv", conversationID, "req", requestID, "output", truncateRunes(contentBuilder.String(), 800))
	}

	// Emit complete event
	emit(EventChatComplete, ChatCompleteEvent{
		ChatEvent: ChatEvent{
			ConversationID: conversationID,
			TabID:          tabID,
			RequestID:      requestID,
			Seq:            nextSeq(),
			MessageID:      assistantMsg.ID,
			Ts:             time.Now().UnixMilli(),
		},
		Status:       StatusSuccess,
		FinishReason: finishReason,
	})
}

// loadMessagesForContext loads messages for agent context
// contextCount: maximum number of messages to include (0 or >=200 means unlimited)
func (s *ChatService) loadMessagesForContext(ctx context.Context, db *bun.DB, conversationID int64, contextCount int) ([]*schema.Message, error) {
	var models []messageModel

	// Determine if we need to limit context
	needLimit := contextCount > 0 && contextCount < 200

	q := db.NewSelect().
		Model(&models).
		Where("conversation_id = ?", conversationID).
		Where("status IN (?)", bun.In([]string{StatusSuccess, StatusCancelled}))

	if needLimit {
		// Get the latest N messages: order DESC with limit, then reverse in memory
		q = q.OrderExpr("created_at DESC, id DESC").Limit(contextCount)
	} else {
		// No limit: get all messages in chronological order
		q = q.OrderExpr("created_at ASC, id ASC")
	}

	if err := q.Scan(ctx); err != nil {
		return nil, err
	}

	// If we limited and ordered DESC, reverse to get chronological order
	if needLimit {
		for i, j := 0, len(models)-1; i < j; i, j = i+1, j-1 {
			models[i], models[j] = models[j], models[i]
		}
	}

	// Build tool name map for repairing assistant tool_calls entries.
	toolNameByCallID := make(map[string]string)
	for _, m := range models {
		if m.Role == RoleTool && m.ToolCallID != "" && m.ToolCallName != "" {
			if _, ok := toolNameByCallID[m.ToolCallID]; !ok {
				toolNameByCallID[m.ToolCallID] = m.ToolCallName
			}
		}
	}

	messages := make([]*schema.Message, 0, len(models))
	for _, m := range models {
		var role schema.RoleType
		switch m.Role {
		case RoleUser:
			role = schema.User
		case RoleAssistant:
			role = schema.Assistant
		case RoleSystem:
			role = schema.System
		case RoleTool:
			role = schema.Tool
		default:
			continue
		}

		msg := &schema.Message{
			Role:    role,
			Content: m.Content,
		}

		if m.Role == RoleTool {
			msg.ToolCallID = m.ToolCallID
			msg.Name = m.ToolCallName
		}

		if m.Role == RoleAssistant && m.ToolCalls != "" && m.ToolCalls != "[]" {
			var toolCalls []schema.ToolCall
			if err := json.Unmarshal([]byte(m.ToolCalls), &toolCalls); err == nil {
				// Repair tool calls for strict OpenAI-compatible providers:
				// - tool call id must be present if a tool message references it
				// - function.name should not be empty
				// - function.arguments must be a valid JSON object string (at least "{}")
				repaired := make([]schema.ToolCall, 0, len(toolCalls))
				for _, tc := range toolCalls {
					if tc.ID == "" {
						continue
					}
					if tc.Function.Name == "" {
						if name, ok := toolNameByCallID[tc.ID]; ok {
							tc.Function.Name = name
						}
					}
					if tc.Function.Arguments == "" || !json.Valid([]byte(tc.Function.Arguments)) {
						tc.Function.Arguments = "{}"
					}
					if tc.Function.Name == "" {
						continue
					}
					repaired = append(repaired, tc)
				}
				if len(repaired) > 0 {
					msg.ToolCalls = repaired
				}
			}
		}

		messages = append(messages, msg)
	}

	return messages, nil
}

// updateMessageStatus updates the message status
func (s *ChatService) updateMessageStatus(db *bun.DB, messageID int64, status, errorMsg, finishReason string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := db.NewUpdate().
		Model((*messageModel)(nil)).
		Set("status = ?", status).
		Set("error = ?", errorMsg).
		Set("finish_reason = ?", finishReason).
		Where("id = ?", messageID).
		Exec(ctx); err != nil {
		s.app.Logger.Error("update message status failed", "messageID", messageID, "error", err)
	}
}

// updateMessageFinal updates the final message content
func (s *ChatService) updateMessageFinal(db *bun.DB, messageID int64, content, thinking, toolCalls, segmentsJSON, status, errorMsg, finishReason string, inputTokens, outputTokens int) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := db.NewUpdate().
		Model((*messageModel)(nil)).
		Set("content = ?", content).
		Set("thinking_content = ?", thinking).
		Set("tool_calls = ?", toolCalls).
		Set("segments = ?", segmentsJSON).
		Set("status = ?", status).
		Set("error = ?", errorMsg).
		Set("finish_reason = ?", finishReason).
		Set("input_tokens = ?", inputTokens).
		Set("output_tokens = ?", outputTokens).
		Where("id = ?", messageID).
		Exec(ctx); err != nil {
		s.app.Logger.Error("update message final failed", "messageID", messageID, "error", err)
	}
}

// createLibraryRetrieverTool creates a LibraryRetrieverTool for the given library IDs
func (s *ChatService) createLibraryRetrieverTool(ctx context.Context, db *bun.DB, libraryIDs []int64, topK int, matchThreshold float64) (tool.BaseTool, error) {
	if len(libraryIDs) == 0 {
		return nil, nil
	}

	// Get embedding config for creating embedder
	embeddingConfig, err := processor.GetEmbeddingConfig(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("get embedding config: %w", err)
	}

	// Create embedder for vector search
	embedder, err := einoembed.NewEmbedder(ctx, &einoembed.ProviderConfig{
		ProviderType: embeddingConfig.ProviderType,
		APIKey:       embeddingConfig.APIKey,
		APIEndpoint:  embeddingConfig.APIEndpoint,
		ModelID:      embeddingConfig.ModelID,
		Dimension:    embeddingConfig.Dimension,
		ExtraConfig:  embeddingConfig.ExtraConfig,
	})
	if err != nil {
		return nil, fmt.Errorf("create embedder: %w", err)
	}
	if embedder == nil {
		return nil, fmt.Errorf("embedder is nil after creation")
	}

	// Create retrieval service
	retrievalService := retrieval.NewService(db, embedder)

	// Set default topK if not specified
	if topK <= 0 {
		topK = 10
	}

	// Create the library retriever tool
	retrieverTool, err := tools.NewLibraryRetrieverTool(ctx, &tools.LibraryRetrieverConfig{
		LibraryIDs:     libraryIDs,
		TopK:           topK,
		MatchThreshold: matchThreshold,
		Retriever:      retrievalService,
	})
	if err != nil {
		return nil, fmt.Errorf("create library retriever tool: %w", err)
	}

	return retrieverTool, nil
}
