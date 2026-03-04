package chat

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	einoagent "chatclaw/internal/eino/agent"
	"chatclaw/internal/eino/tools"
	"chatclaw/internal/errs"
	"chatclaw/internal/sqlite"

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
}

// ChatService handles chat operations
type ChatService struct {
	app               *application.App
	toolRegistry      *tools.ToolRegistry
	bgProcessManager  *tools.BgProcessManager
	activeGenerations sync.Map // map[int64]*activeGeneration
}

// NewChatService creates a new ChatService
func NewChatService(app *application.App) *ChatService {
	return &ChatService{
		app:              app,
		toolRegistry:     tools.NewToolRegistry(),
		bgProcessManager: tools.NewBgProcessManager(),
	}
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

// SendMessage sends a message and starts a ReAct generation loop
func (s *ChatService) SendMessage(input SendMessageInput) (*SendMessageResult, error) {
	if input.ConversationID <= 0 {
		return nil, errs.New("error.chat_conversation_id_required")
	}
	content := strings.TrimSpace(input.Content)
	hasImages := len(input.Images) > 0

	// Validate: content or images must be non-empty
	if content == "" && !hasImages {
		return nil, errs.New("error.chat_content_required")
	}

	// Validate images
	if hasImages {
		const maxImages = 4
		const maxImageSize = 2 * 1024 * 1024  // 2MB per image
		const maxTotalSize = 8 * 1024 * 1024  // 8MB total

		if len(input.Images) > maxImages {
			return nil, errs.New("error.chat_too_many_images")
		}

		var totalSize int64
		for _, img := range input.Images {
			// Validate mime type
			if !strings.HasPrefix(img.MimeType, "image/") {
				return nil, errs.New("error.chat_invalid_image_type")
			}

			// Validate base64
			if img.Base64 == "" {
				return nil, errs.New("error.chat_image_base64_required")
			}

			// Validate size
			if img.Size > maxImageSize {
				return nil, errs.New("error.chat_image_too_large")
			}
			totalSize += img.Size
		}

		if totalSize > maxTotalSize {
			return nil, errs.New("error.chat_images_total_too_large")
		}
	}

	// Serialize images to JSON
	imagesJSON := "[]"
	if hasImages {
		b, err := json.Marshal(input.Images)
		if err != nil {
			return nil, errs.Wrap("error.chat_images_serialize_failed", err)
		}
		imagesJSON = string(b)
	}

	s.app.Logger.Info("[chat] SendMessage", "conv", input.ConversationID, "tab", input.TabID, "content_len", len(content), "images_count", len(input.Images))

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

	agentConfig, providerConfig, agentExtras, err := s.getAgentAndProviderConfig(ctx, db, input.ConversationID)
	if err != nil {
		return nil, err
	}

	if agentExtras.ChatMode == "chat" {
		return s.startGeneration(db, input.ConversationID, input.TabID, agentConfig, providerConfig, agentExtras, func(genCtx context.Context, requestID string) {
			s.runChatModeGeneration(genCtx, db, input.ConversationID, input.TabID, requestID, content, imagesJSON, agentConfig, providerConfig, agentExtras)
		})
	}

	return s.startGeneration(db, input.ConversationID, input.TabID, agentConfig, providerConfig, agentExtras, func(genCtx context.Context, requestID string) {
		s.runGeneration(genCtx, db, input.ConversationID, input.TabID, requestID, content, imagesJSON, agentConfig, providerConfig, agentExtras)
	})
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

// tryDeleteGeneration removes the generation from the map only if it is still the active one.
func (s *ChatService) tryDeleteGeneration(conversationID int64, gen *activeGeneration) {
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
