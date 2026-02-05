package chat

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"willchat/internal/errs"
	"willchat/internal/services/tools"
	"willchat/internal/sqlite"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// activeGeneration tracks an active generation
type activeGeneration struct {
	cancel    context.CancelFunc
	requestID string
	tabID     string
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

	log.Printf("[chat] SendMessage conv=%d tab=%s content_len=%d", input.ConversationID, input.TabID, len(content))

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
	agentConfig, providerConfig, err := s.getAgentAndProviderConfig(ctx, db, input.ConversationID)
	if err != nil {
		return nil, err
	}

	// Generate request ID
	requestID := uuid.New().String()

	// Create cancellable context for generation
	genCtx, cancel := context.WithCancel(ctx)

	// Register active generation
	s.activeGenerations.Store(input.ConversationID, &activeGeneration{
		cancel:    cancel,
		requestID: requestID,
		tabID:     input.TabID,
	})

	// Start generation in goroutine
	go s.runGeneration(genCtx, db, input.ConversationID, input.TabID, requestID, content, agentConfig, providerConfig)

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

	log.Printf("[chat] EditAndResend conv=%d tab=%s msg=%d content_len=%d", input.ConversationID, input.TabID, input.MessageID, len(content))

	// Stop any existing generation
	if existing, ok := s.activeGenerations.Load(input.ConversationID); ok {
		gen := existing.(*activeGeneration)
		gen.cancel()
		s.activeGenerations.Delete(input.ConversationID)
		// Wait a bit for cleanup
		time.Sleep(100 * time.Millisecond)
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
	agentConfig, providerConfig, err := s.getAgentAndProviderConfig(ctx, db, input.ConversationID)
	if err != nil {
		return nil, err
	}

	// Generate request ID
	requestID := uuid.New().String()

	// Create cancellable context for generation
	genCtx, genCancel := context.WithCancel(context.Background())

	// Register active generation
	s.activeGenerations.Store(input.ConversationID, &activeGeneration{
		cancel:    genCancel,
		requestID: requestID,
		tabID:     input.TabID,
	})

	// Start generation in goroutine (don't insert user message, it's already there)
	go s.runGenerationWithExistingHistory(genCtx, db, input.ConversationID, input.TabID, requestID, agentConfig, providerConfig)

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

// getAgentAndProviderConfig gets the agent and provider configuration for a conversation
func (s *ChatService) getAgentAndProviderConfig(ctx context.Context, db *bun.DB, conversationID int64) (AgentConfig, ProviderConfig, error) {
	// Get conversation
	type conversationRow struct {
		AgentID       int64  `bun:"agent_id"`
		LLMProviderID string `bun:"llm_provider_id"`
		LLMModelID    string `bun:"llm_model_id"`
	}
	var conv conversationRow
	if err := db.NewSelect().
		Table("conversations").
		Column("agent_id", "llm_provider_id", "llm_model_id").
		Where("id = ?", conversationID).
		Scan(ctx, &conv); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return AgentConfig{}, ProviderConfig{}, errs.New("error.chat_conversation_not_found")
		}
		return AgentConfig{}, ProviderConfig{}, errs.Wrap("error.chat_conversation_read_failed", err)
	}

	// Get agent
	type agentRow struct {
		Name                 string  `bun:"name"`
		Prompt               string  `bun:"prompt"`
		DefaultLLMProviderID string  `bun:"default_llm_provider_id"`
		DefaultLLMModelID    string  `bun:"default_llm_model_id"`
		LLMTemperature       float64 `bun:"llm_temperature"`
		LLMTopP              float64 `bun:"llm_top_p"`
		LLMMaxTokens         int     `bun:"llm_max_tokens"`
		EnableLLMTemperature bool    `bun:"enable_llm_temperature"`
		EnableLLMTopP        bool    `bun:"enable_llm_top_p"`
		EnableLLMMaxTokens   bool    `bun:"enable_llm_max_tokens"`
	}
	var agent agentRow
	if err := db.NewSelect().
		Table("agents").
		Column("name", "prompt", "default_llm_provider_id", "default_llm_model_id",
			"llm_temperature", "llm_top_p", "llm_max_tokens",
			"enable_llm_temperature", "enable_llm_top_p", "enable_llm_max_tokens").
		Where("id = ?", conv.AgentID).
		Scan(ctx, &agent); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return AgentConfig{}, ProviderConfig{}, errs.New("error.chat_agent_not_found")
		}
		return AgentConfig{}, ProviderConfig{}, errs.Wrap("error.chat_agent_read_failed", err)
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
		return AgentConfig{}, ProviderConfig{}, errs.New("error.chat_model_not_configured")
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
			return AgentConfig{}, ProviderConfig{}, errs.Newf("error.chat_provider_not_found", map[string]any{"ProviderID": providerID})
		}
		return AgentConfig{}, ProviderConfig{}, errs.Wrap("error.chat_provider_read_failed", err)
	}

	if !provider.Enabled {
		return AgentConfig{}, ProviderConfig{}, errs.New("error.chat_provider_not_enabled")
	}

	agentConfig := AgentConfig{
		Name:        agent.Name,
		Instruction: agent.Prompt,
		ModelID:     modelID,
		Temperature: &agent.LLMTemperature,
		TopP:        &agent.LLMTopP,
		MaxTokens:   &agent.LLMMaxTokens,
		EnableTemp:  agent.EnableLLMTemperature,
		EnableTopP:  agent.EnableLLMTopP,
		EnableMaxTokens: agent.EnableLLMMaxTokens,
	}

	providerConfig := ProviderConfig{
		ProviderID:  providerID,
		Type:        provider.Type,
		APIKey:      provider.APIKey,
		APIEndpoint: provider.APIEndpoint,
		ExtraConfig: provider.ExtraConfig,
	}

	return agentConfig, providerConfig, nil
}

// runGeneration runs the generation loop
func (s *ChatService) runGeneration(ctx context.Context, db *bun.DB, conversationID int64, tabID, requestID, userContent string, agentConfig AgentConfig, providerConfig ProviderConfig) {
	defer s.activeGenerations.Delete(conversationID)

	var seq int32 = 0
	nextSeq := func() int {
		return int(atomic.AddInt32(&seq, 1))
	}

	emit := func(eventName string, payload any) {
		// Debug: print every emitted event (avoid logging full content; log sizes and IDs only)
		switch v := payload.(type) {
		case ChatStartEvent:
			log.Printf("[chat] emit=%s conv=%d tab=%s req=%s seq=%d mid=%d", eventName, v.ConversationID, v.TabID, v.RequestID, v.Seq, v.MessageID)
		case ChatChunkEvent:
			log.Printf("[chat] emit=%s conv=%d tab=%s req=%s seq=%d mid=%d delta_len=%d", eventName, v.ConversationID, v.TabID, v.RequestID, v.Seq, v.MessageID, len(v.Delta))
		case ChatThinkingEvent:
			log.Printf("[chat] emit=%s conv=%d tab=%s req=%s seq=%d mid=%d delta_len=%d", eventName, v.ConversationID, v.TabID, v.RequestID, v.Seq, v.MessageID, len(v.Delta))
		case ChatToolEvent:
			argsPreview := v.ArgsJSON
			if len(argsPreview) > 500 {
				argsPreview = argsPreview[:500] + "...(truncated)"
			}
			resPreview := v.ResultJSON
			if len(resPreview) > 500 {
				resPreview = resPreview[:500] + "...(truncated)"
			}
			log.Printf("[chat] emit=%s conv=%d tab=%s req=%s seq=%d mid=%d type=%s tool=%s call_id=%s args_len=%d result_len=%d args=%q result=%q",
				eventName, v.ConversationID, v.TabID, v.RequestID, v.Seq, v.MessageID, v.Type, v.ToolName, v.ToolCallID, len(v.ArgsJSON), len(v.ResultJSON), argsPreview, resPreview)
		case ChatCompleteEvent:
			log.Printf("[chat] emit=%s conv=%d tab=%s req=%s seq=%d mid=%d finish=%s", eventName, v.ConversationID, v.TabID, v.RequestID, v.Seq, v.MessageID, v.FinishReason)
		case ChatStoppedEvent:
			log.Printf("[chat] emit=%s conv=%d tab=%s req=%s seq=%d mid=%d status=%s", eventName, v.ConversationID, v.TabID, v.RequestID, v.Seq, v.MessageID, v.Status)
		case ChatErrorEvent:
			log.Printf("[chat] emit=%s conv=%d tab=%s req=%s seq=%d mid=%d key=%s data=%v", eventName, v.ConversationID, v.TabID, v.RequestID, v.Seq, v.MessageID, v.ErrorKey, v.ErrorData)
		default:
			log.Printf("[chat] emit=%s conv=%d tab=%s req=%s payload=%T", eventName, conversationID, tabID, requestID, payload)
		}
		s.app.Event.Emit(eventName, payload)
	}

	emitError := func(errorKey string, errorData any) {
		log.Printf("[chat] error conv=%d tab=%s req=%s key=%s data=%v", conversationID, tabID, requestID, errorKey, errorData)
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
	s.runGenerationWithExistingHistory(ctx, db, conversationID, tabID, requestID, agentConfig, providerConfig)
}

// runGenerationWithExistingHistory runs the generation loop with existing message history
func (s *ChatService) runGenerationWithExistingHistory(ctx context.Context, db *bun.DB, conversationID int64, tabID, requestID string, agentConfig AgentConfig, providerConfig ProviderConfig) {
	defer s.activeGenerations.Delete(conversationID)

	var seq int32 = 0
	nextSeq := func() int {
		return int(atomic.AddInt32(&seq, 1))
	}

	emit := func(eventName string, payload any) {
		// Debug: print every emitted event (avoid logging full content; log sizes and IDs only)
		switch v := payload.(type) {
		case ChatStartEvent:
			log.Printf("[chat] emit=%s conv=%d tab=%s req=%s seq=%d mid=%d", eventName, v.ConversationID, v.TabID, v.RequestID, v.Seq, v.MessageID)
		case ChatChunkEvent:
			log.Printf("[chat] emit=%s conv=%d tab=%s req=%s seq=%d mid=%d delta_len=%d", eventName, v.ConversationID, v.TabID, v.RequestID, v.Seq, v.MessageID, len(v.Delta))
		case ChatThinkingEvent:
			log.Printf("[chat] emit=%s conv=%d tab=%s req=%s seq=%d mid=%d delta_len=%d", eventName, v.ConversationID, v.TabID, v.RequestID, v.Seq, v.MessageID, len(v.Delta))
		case ChatToolEvent:
			argsPreview := v.ArgsJSON
			if len(argsPreview) > 500 {
				argsPreview = argsPreview[:500] + "...(truncated)"
			}
			resPreview := v.ResultJSON
			if len(resPreview) > 500 {
				resPreview = resPreview[:500] + "...(truncated)"
			}
			log.Printf("[chat] emit=%s conv=%d tab=%s req=%s seq=%d mid=%d type=%s tool=%s call_id=%s args_len=%d result_len=%d args=%q result=%q",
				eventName, v.ConversationID, v.TabID, v.RequestID, v.Seq, v.MessageID, v.Type, v.ToolName, v.ToolCallID, len(v.ArgsJSON), len(v.ResultJSON), argsPreview, resPreview)
		case ChatCompleteEvent:
			log.Printf("[chat] emit=%s conv=%d tab=%s req=%s seq=%d mid=%d finish=%s", eventName, v.ConversationID, v.TabID, v.RequestID, v.Seq, v.MessageID, v.FinishReason)
		case ChatStoppedEvent:
			log.Printf("[chat] emit=%s conv=%d tab=%s req=%s seq=%d mid=%d status=%s", eventName, v.ConversationID, v.TabID, v.RequestID, v.Seq, v.MessageID, v.Status)
		case ChatErrorEvent:
			log.Printf("[chat] emit=%s conv=%d tab=%s req=%s seq=%d mid=%d key=%s data=%v", eventName, v.ConversationID, v.TabID, v.RequestID, v.Seq, v.MessageID, v.ErrorKey, v.ErrorData)
		default:
			log.Printf("[chat] emit=%s conv=%d tab=%s req=%s payload=%T", eventName, conversationID, tabID, requestID, payload)
		}
		s.app.Event.Emit(eventName, payload)
	}

	emitError := func(errorKey string, errorData any) {
		log.Printf("[chat] error conv=%d tab=%s req=%s key=%s data=%v", conversationID, tabID, requestID, errorKey, errorData)
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
	messages, err := s.loadMessagesForContext(ctx, db, conversationID)
	if err != nil {
		emitError("error.chat_messages_failed", nil)
		s.updateMessageStatus(db, assistantMsg.ID, StatusError, "Failed to load messages", "")
		return
	}

	// Create agent
	agentConfig.Provider = providerConfig
	agent, err := createChatModelAgent(ctx, agentConfig, s.toolRegistry)
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

	updateToolStates := func(toolCalls []schema.ToolCall) {
		for i, tc := range toolCalls {
			key := tc.ID
			if key == "" {
				key = fmt.Sprintf("idx_%d", i)
			}
			st, ok := toolStatesByKey[key]
			if !ok {
				st = &toolCallState{}
				toolStatesByKey[key] = st
				toolOrder = append(toolOrder, key)
			}
			if tc.ID != "" {
				st.id = tc.ID
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
			s.updateMessageFinal(db, assistantMsg.ID, contentBuilder.String(), thinkingBuilder.String(), string(toolCallsJSON), StatusCancelled, "", "cancelled", inputTokens, outputTokens)
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
			log.Printf("[chat] generation failed conv=%d tab=%s req=%s err=%v", conversationID, tabID, requestID, event.Err)
			emitError("error.chat_generation_failed", map[string]any{"Error": event.Err.Error()})
			s.updateMessageStatus(db, assistantMsg.ID, StatusError, event.Err.Error(), "")
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
						log.Printf("[chat] stream recv failed conv=%d tab=%s req=%s err=%v", conversationID, tabID, requestID, err)
						emitError("error.chat_stream_failed", map[string]any{"Error": err.Error()})
						break
					}

					// Handle content
					if msg.Content != "" {
						contentBuilder.WriteString(msg.Content)
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
						// Emit tool call updates only when we have a real call_id.
						for _, tc := range msg.ToolCalls {
							if tc.ID == "" {
								continue
							}
							toolName := tc.Function.Name
							if toolName == "" {
								// Try to find name from accumulated state.
								for _, key := range toolOrder {
									st := toolStatesByKey[key]
									if st != nil && st.id == tc.ID && st.name != "" {
										toolName = st.name
										break
									}
								}
							}
							args := tc.Function.Arguments
							// Use accumulated args if available (more complete than current chunk).
							for _, key := range toolOrder {
								st := toolStatesByKey[key]
								if st != nil && st.id == tc.ID {
									args = st.args
									break
								}
							}
							if toolName == "" {
								continue
							}
							if args != "" && !json.Valid([]byte(args)) {
								log.Printf("[chat] WARNING tool arguments not valid JSON conv=%d tab=%s req=%s tool=%s call_id=%s args=%q",
									conversationID, tabID, requestID, toolName, tc.ID, args)
							}
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
								ToolCallID: tc.ID,
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
					db.NewInsert().Model(toolMsg).Exec(dbCtx)
					dbCancel()
				} else if msg.Content != "" {
					contentBuilder.WriteString(msg.Content)
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
		s.updateMessageFinal(db, assistantMsg.ID, contentBuilder.String(), thinkingBuilder.String(), string(toolCallsJSON), StatusCancelled, "", "cancelled", inputTokens, outputTokens)
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
	s.updateMessageFinal(db, assistantMsg.ID, contentBuilder.String(), thinkingBuilder.String(), toolCallsStr, StatusSuccess, "", finishReason, inputTokens, outputTokens)

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
func (s *ChatService) loadMessagesForContext(ctx context.Context, db *bun.DB, conversationID int64) ([]*schema.Message, error) {
	var models []messageModel
	if err := db.NewSelect().
		Model(&models).
		Where("conversation_id = ?", conversationID).
		Where("status IN (?)", bun.In([]string{StatusSuccess, StatusCancelled})).
		OrderExpr("created_at ASC, id ASC").
		Scan(ctx); err != nil {
		return nil, err
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

	db.NewUpdate().
		Model((*messageModel)(nil)).
		Set("status = ?", status).
		Set("error = ?", errorMsg).
		Set("finish_reason = ?", finishReason).
		Where("id = ?", messageID).
		Exec(ctx)
}

// updateMessageFinal updates the final message content
func (s *ChatService) updateMessageFinal(db *bun.DB, messageID int64, content, thinking, toolCalls, status, errorMsg, finishReason string, inputTokens, outputTokens int) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	db.NewUpdate().
		Model((*messageModel)(nil)).
		Set("content = ?", content).
		Set("thinking_content = ?", thinking).
		Set("tool_calls = ?", toolCalls).
		Set("status = ?", status).
		Set("error = ?", errorMsg).
		Set("finish_reason = ?", finishReason).
		Set("input_tokens = ?", inputTokens).
		Set("output_tokens = ?", outputTokens).
		Where("id = ?", messageID).
		Exec(ctx)
}
