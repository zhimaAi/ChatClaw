package chat

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"time"

	einoagent "chatclaw/internal/eino/agent"
	"chatclaw/internal/eino/tools"
	"chatclaw/internal/errs"
	"chatclaw/internal/sqlite"

	"github.com/cloudwego/eino/adk"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// activeGeneration tracks an active generation
type activeGeneration struct {
	cancel    context.CancelFunc
	requestID string
	tabID     string
	done      chan struct{}

	// mu protects the Interrupt/Resume fields below, which are written by
	// the generation goroutine and read by SendMessage on the main goroutine.
	mu           sync.Mutex
	runner       *adk.Runner
	checkpointID string
	interrupted  bool
	agentCleanup func() // deferred agent cleanup, held during interrupt
}

// ChatService handles chat operations
type ChatService struct {
	app               *application.App
	toolRegistry      *tools.ToolRegistry
	bgProcessManager  *tools.BgProcessManager
	checkpointStore   adk.CheckPointStore
	activeGenerations sync.Map // map[int64]*activeGeneration
}

// NewChatService creates a new ChatService
func NewChatService(app *application.App) *ChatService {
	return &ChatService{
		app:              app,
		toolRegistry:     tools.NewToolRegistry(),
		bgProcessManager: tools.NewBgProcessManager(),
		checkpointStore:  newInMemoryCheckPointStore(),
	}
}

// inMemoryCheckPointStore is a simple in-memory implementation of adk.CheckPointStore.
type inMemoryCheckPointStore struct {
	mu sync.RWMutex
	m  map[string][]byte
}

func newInMemoryCheckPointStore() *inMemoryCheckPointStore {
	return &inMemoryCheckPointStore{m: make(map[string][]byte)}
}

func (s *inMemoryCheckPointStore) Get(_ context.Context, id string) ([]byte, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.m[id]
	return v, ok, nil
}

func (s *inMemoryCheckPointStore) Set(_ context.Context, id string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[id] = data
	return nil
}

// Shutdown cleans up all resources held by the ChatService, including
// killing any background processes started by execute_background.
func (s *ChatService) Shutdown() {
	s.bgProcessManager.Cleanup()
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

// SendMessage sends a message and starts a ReAct generation loop.
// If the conversation is in an interrupted state (waiting for user confirmation),
// the message is treated as a resume response instead of starting a new generation.
func (s *ChatService) SendMessage(input SendMessageInput) (*SendMessageResult, error) {
	if input.ConversationID <= 0 {
		return nil, errs.New("error.chat_conversation_id_required")
	}
	content := strings.TrimSpace(input.Content)
	if content == "" {
		return nil, errs.New("error.chat_content_required")
	}

	s.app.Logger.Info("[chat] SendMessage", "conv", input.ConversationID, "tab", input.TabID, "content_len", len(content))

	if existing, ok := s.activeGenerations.Load(input.ConversationID); ok {
		gen := existing.(*activeGeneration)
		gen.mu.Lock()
		isInterrupted := gen.interrupted
		gen.mu.Unlock()
		if isInterrupted {
			return s.handleResumeMessage(input.ConversationID, gen, content)
		}
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

	agentConfig, providerConfig, agentExtras, err := s.getAgentAndProviderConfig(ctx, db, input.ConversationID)
	if err != nil {
		return nil, err
	}

	if agentExtras.ChatMode == "chat" {
		return s.startGeneration(db, input.ConversationID, input.TabID, agentConfig, providerConfig, agentExtras, func(genCtx context.Context, requestID string) {
			s.runChatModeGeneration(genCtx, db, input.ConversationID, input.TabID, requestID, content, agentConfig, providerConfig, agentExtras)
		})
	}

	return s.startGeneration(db, input.ConversationID, input.TabID, agentConfig, providerConfig, agentExtras, func(genCtx context.Context, requestID string) {
		s.runGeneration(genCtx, db, input.ConversationID, input.TabID, requestID, content, agentConfig, providerConfig, agentExtras)
	})
}

// handleResumeMessage processes user confirmation/rejection for an interrupted generation.
func (s *ChatService) handleResumeMessage(conversationID int64, gen *activeGeneration, content string) (*SendMessageResult, error) {
	db, err := s.db()
	if err != nil {
		return nil, err
	}

	userMsg := &messageModel{
		ConversationID: conversationID,
		Role:           RoleUser,
		Content:        content,
		Status:         StatusSuccess,
		ToolCalls:      "[]",
	}
	dbCtx, dbCancel := context.WithTimeout(context.Background(), 5*time.Second)
	if _, insertErr := db.NewInsert().Model(userMsg).Exec(dbCtx); insertErr != nil {
		dbCancel()
		s.app.Logger.Error("[chat] failed to save resume user message", "conv", conversationID, "error", insertErr)
		return nil, errs.Wrap("error.chat_message_save_failed", insertErr)
	}
	dbCancel()

	approved := isApproval(content)

	gen.mu.Lock()
	gen.interrupted = false
	gen.mu.Unlock()

	go func() {
		s.resumeGeneration(gen, conversationID, approved)
	}()

	return &SendMessageResult{RequestID: gen.requestID, MessageID: userMsg.ID}, nil
}

// isApproval checks whether the user message indicates approval.
func isApproval(content string) bool {
	lower := strings.ToLower(strings.TrimSpace(content))
	approvals := []string{"确认", "confirm", "yes", "y", "ok", "approve", "是", "好", "继续", "执行"}
	for _, a := range approvals {
		if lower == a {
			return true
		}
	}
	return false
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

	if existing, ok := s.activeGenerations.Load(input.ConversationID); ok {
		oldGen := existing.(*activeGeneration)
		oldGen.cancel()
		s.activeGenerations.Delete(input.ConversationID)
		select {
		case <-oldGen.done:
		case <-time.After(3 * time.Second):
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

	if err := s.deleteMessagesAfter(ctx, db, input.ConversationID, input.MessageID, false); err != nil {
		return nil, err
	}

	if _, err := db.NewUpdate().
		Model((*messageModel)(nil)).
		Set("content = ?", content).
		Where("id = ?", input.MessageID).
		Exec(ctx); err != nil {
		return nil, errs.Wrap("error.chat_message_update_failed", err)
	}

	agentConfig, providerConfig, agentExtras, err := s.getAgentAndProviderConfig(ctx, db, input.ConversationID)
	if err != nil {
		return nil, err
	}

	var result *SendMessageResult
	if agentExtras.ChatMode == "chat" {
		result, err = s.startGeneration(db, input.ConversationID, input.TabID, agentConfig, providerConfig, agentExtras, func(genCtx context.Context, requestID string) {
			s.runChatModeWithExistingHistory(genCtx, db, input.ConversationID, input.TabID, requestID, agentConfig, providerConfig, agentExtras)
		})
	} else {
		result, err = s.startGeneration(db, input.ConversationID, input.TabID, agentConfig, providerConfig, agentExtras, func(genCtx context.Context, requestID string) {
			s.runGenerationWithExistingHistory(genCtx, db, input.ConversationID, input.TabID, requestID, agentConfig, providerConfig, agentExtras)
		})
	}
	if err != nil {
		return nil, err
	}
	result.MessageID = input.MessageID
	return result, nil
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
	return nil
}

// startGeneration creates a new generation context and launches the goroutine.
func (s *ChatService) startGeneration(db *bun.DB, conversationID int64, tabID string, _ einoagent.Config, _ einoagent.ProviderConfig, _ AgentExtras, runFn func(ctx context.Context, requestID string)) (*SendMessageResult, error) {
	requestID := uuid.New().String()
	genCtx, cancel := context.WithCancel(context.Background())

	gen := &activeGeneration{
		cancel:    cancel,
		requestID: requestID,
		tabID:     tabID,
		done:      make(chan struct{}),
	}
	s.activeGenerations.Store(conversationID, gen)

	go func() {
		defer close(gen.done)
		defer s.tryDeleteGeneration(conversationID, gen)
		runFn(genCtx, requestID)
	}()

	return &SendMessageResult{
		RequestID: requestID,
		MessageID: 0,
	}, nil
}

// tryDeleteGeneration removes the generation from the map only if it is still
// the active one and not in an interrupted state (waiting for user confirmation).
func (s *ChatService) tryDeleteGeneration(conversationID int64, gen *activeGeneration) {
	gen.mu.Lock()
	isInterrupted := gen.interrupted
	gen.mu.Unlock()
	if isInterrupted {
		return
	}
	if cur, ok := s.activeGenerations.Load(conversationID); ok && cur == gen {
		s.activeGenerations.Delete(conversationID)
	}
}

// deleteMessagesAfter deletes all messages after the given message ID.
// archive parameter is reserved for future use.
func (s *ChatService) deleteMessagesAfter(ctx context.Context, db *bun.DB, conversationID, messageID int64, archive bool) error {
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
