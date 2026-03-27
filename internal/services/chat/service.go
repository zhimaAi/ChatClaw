package chat

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"chatclaw/internal/define"
	einoagent "chatclaw/internal/eino/agent"
	"chatclaw/internal/eino/tools"
	"chatclaw/internal/errs"
	"chatclaw/internal/services/channels"
	"chatclaw/internal/services/chatwiki"
	"chatclaw/internal/sqlite"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// chatWikiBindingGetter is a narrow interface so other packages (e.g. chat) can depend on
// ChatWiki binding access without referencing *chatwiki.ChatWikiService in signatures.
// Referencing the concrete service type in exported APIs causes Wails bindings to register
// ChatWikiService as a model type and duplicate the TS identifier with the service module.
type chatWikiBindingGetter interface {
	GetBinding() (*chatwiki.Binding, error)
}

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
	streamText   string
}

// ChunkCallback is called each time a new content chunk is appended during streaming.
// accumulated is the full assistant content generated so far (not just the delta).
type ChunkCallback func(accumulated string)

// ChatService handles chat operations
type ChatService struct {
	app                *application.App
	toolRegistry       *tools.ToolRegistry
	bgProcessManager   *tools.BgProcessManager
	checkpointStore    adk.CheckPointStore
	chatWikiService    chatWikiBindingGetter
	extraToolFactories []func() ([]tool.BaseTool, error)
	activeGenerations  sync.Map // map[int64]*activeGeneration
	gateway            *channels.Gateway
	chunkCallbacks     sync.Map // map[int64]ChunkCallback — per-conversation streaming sinks
	openclawGateway    OpenClawGatewayInfo
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

func (s *ChatService) RegisterExtraToolFactory(factory func() ([]tool.BaseTool, error)) {
	if factory == nil {
		return
	}
	s.extraToolFactories = append(s.extraToolFactories, factory)
}

// SetChatWikiService injects ChatWiki service so chat features can reuse
// binding/token logic via unified entry points.
func (s *ChatService) SetChatWikiService(svc chatWikiBindingGetter) {
	s.chatWikiService = svc
}

// inMemoryCheckPointStore is a simple in-memory implementation of adk.CheckPointStore.
type inMemoryCheckPointStore struct {
	mu sync.RWMutex
	m  map[string][]byte
}

func newInMemoryCheckPointStore() *inMemoryCheckPointStore {
	return &inMemoryCheckPointStore{m: make(map[string][]byte)}
}

// idHash generates a short hash for IDs
func idHash(id int64) string {
	h := sha256.Sum256([]byte("chatclaw:" + strconv.FormatInt(id, 10)))
	return hex.EncodeToString(h[:6])
}

// resolveWorkDir resolves the workspace directory for a given agent and conversation
func (s *ChatService) resolveWorkDir(ctx context.Context, db *bun.DB, agentID, conversationID int64) (string, error) {
	type agentRow struct {
		WorkDir string `bun:"work_dir"`
	}
	var agent agentRow
	err := db.NewSelect().
		Table("agents").
		Column("work_dir").
		Where("id = ?", agentID).
		Scan(ctx, &agent)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", errs.Wrap("error.chat_agent_read_failed", err)
	}

	workDir := agent.WorkDir
	if errors.Is(err, sql.ErrNoRows) {
		type openClawAgentRow struct {
			WorkDir         string `bun:"work_dir"`
			OpenClawAgentID string `bun:"openclaw_agent_id"`
		}
		var ocAgent openClawAgentRow
		if ocErr := db.NewSelect().
			Table("openclaw_agents").
			Column("work_dir", "openclaw_agent_id").
			Where("id = ?", agentID).
			Scan(ctx, &ocAgent); ocErr != nil {
			return "", errs.Wrap("error.chat_agent_read_failed", ocErr)
		}

		workDir = strings.TrimSpace(ocAgent.WorkDir)
		if workDir == "" {
			if ocRoot, ocRootErr := define.OpenClawDataRootDir(); ocRootErr == nil {
				workDir = ocRoot
			}
		}
		if ocAgent.OpenClawAgentID != "" {
			workDir = filepath.Join(workDir, "workspace-"+ocAgent.OpenClawAgentID)
		}
	}
	if workDir == "" {
		// Use default work dir
		workDir = defaultWorkDir()
	}

	dir := filepath.Join(workDir, "sessions", idHash(agentID))
	if conversationID > 0 {
		dir = filepath.Join(dir, idHash(conversationID))
	}
	return dir, nil
}

// defaultWorkDir returns the default working directory for agents
func defaultWorkDir() string {
	dir, err := define.AppDataDir()
	if err != nil {
		return ""
	}
	return dir
}

// saveImagesToWorkDir saves images and files to the conversation's work directory and returns updated payloads.
func (s *ChatService) saveImagesToWorkDir(ctx context.Context, db *bun.DB, agentID, conversationID int64, images []ImagePayload) ([]ImagePayload, error) {
	if len(images) == 0 {
		return images, nil
	}

	workDir, err := s.resolveWorkDir(ctx, db, agentID, conversationID)
	if err != nil {
		return nil, errs.Wrap("error.chat_resolve_workdir_failed", err)
	}

	imagesDir := filepath.Join(workDir, "images")
	filesDir := filepath.Join(workDir, "files")
	if err := os.MkdirAll(imagesDir, 0755); err != nil {
		return nil, errs.Wrap("error.chat_create_images_dir_failed", err)
	}
	if err := os.MkdirAll(filesDir, 0755); err != nil {
		return nil, errs.Wrap("error.chat_create_files_dir_failed", err)
	}

	updatedImages := make([]ImagePayload, len(images))
	for i, img := range images {
		data, err := base64.StdEncoding.DecodeString(img.Base64)
		if err != nil {
			s.app.Logger.Warn("[chat] failed to decode attachment base64", "kind", img.Kind, "error", err)
			updatedImages[i] = img
			continue
		}

		if img.Kind == "file" {
			// Save file attachment to files/ subdirectory, preserving original name
			originalName := img.OriginalName
			if originalName == "" {
				originalName = img.FileName
			}
			if originalName == "" {
				originalName = "unnamed"
			}
			filename := fmt.Sprintf("file_%d_%d_%s", conversationID, time.Now().UnixNano(), sanitizeFileName(originalName))
			savePath := filepath.Join(filesDir, filename)

			if err := os.WriteFile(savePath, data, 0644); err != nil {
				s.app.Logger.Warn("[chat] failed to write file", "error", err)
				updatedImages[i] = img
				continue
			}

			updatedImages[i] = ImagePayload{
				ID:           img.ID,
				Kind:         "file",
				Source:       "local_file",
				MimeType:     img.MimeType,
				FilePath:     savePath,
				FileName:     filename,
				OriginalName: originalName,
				Size:         int64(len(data)),
			}
			s.app.Logger.Info("[chat] file saved to workdir", "path", savePath, "original", originalName)
		} else {
			// Save image to images/ subdirectory
			ext := ".png"
			if idx := strings.LastIndex(img.MimeType, "/"); idx >= 0 {
				switch img.MimeType[idx+1:] {
				case "jpeg", "jpg":
					ext = ".jpg"
				case "gif":
					ext = ".gif"
				case "webp":
					ext = ".webp"
				case "svg+xml":
					ext = ".svg"
				case "bmp":
					ext = ".bmp"
				}
			}
			filename := fmt.Sprintf("image_%d_%d%s", conversationID, time.Now().UnixNano(), ext)
			savePath := filepath.Join(imagesDir, filename)

			if err := os.WriteFile(savePath, data, 0644); err != nil {
				s.app.Logger.Warn("[chat] failed to write image file", "error", err)
				updatedImages[i] = img
				continue
			}

			updatedImages[i] = ImagePayload{
				ID:       img.ID,
				Kind:     "image",
				Source:   "local_file",
				MimeType: img.MimeType,
				Base64:   img.Base64,
				FilePath: savePath,
				FileName: filename,
				Size:     int64(len(data)),
			}
			s.app.Logger.Info("[chat] image saved to workdir", "path", savePath)
		}
	}

	return updatedImages, nil
}

// sanitizeFileName removes or replaces characters that are unsafe in file paths.
func sanitizeFileName(name string) string {
	replacer := strings.NewReplacer(
		"/", "_", "\\", "_", ":", "_", "*", "_",
		"?", "_", "\"", "_", "<", "_", ">", "_", "|", "_",
	)
	result := replacer.Replace(name)
	if len(result) > 200 {
		result = result[:200]
	}
	return result
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

// SetGateway sets the channel gateway reference. Called after bootstrap since
// the Gateway is created after the ChatService.
func (s *ChatService) SetGateway(gw *channels.Gateway) {
	s.gateway = gw
}

// RegisterChunkCallback registers a callback that is invoked on every content
// chunk emitted during streaming for the given conversation.
// The callback receives the full accumulated content (not just the delta).
// Only one callback per conversation is supported; registering again overwrites.
func (s *ChatService) RegisterChunkCallback(conversationID int64, cb ChunkCallback) {
	s.chunkCallbacks.Store(conversationID, cb)
}

// UnregisterChunkCallback removes the streaming chunk callback for a conversation.
func (s *ChatService) UnregisterChunkCallback(conversationID int64) {
	s.chunkCallbacks.Delete(conversationID)
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
	hasAttachments := len(input.Images) > 0

	// Validate: content or attachments must be non-empty
	if content == "" && !hasAttachments {
		return nil, errs.New("error.chat_content_required")
	}

	// Validate attachments (images + files)
	if hasAttachments {
		const maxImages = 4
		const maxImageSize int64 = 2 * 1024 * 1024  // 2MB per image
		const maxImageTotal int64 = 8 * 1024 * 1024 // 8MB total images
		const maxFiles = 4
		const maxFileSize int64 = 20 * 1024 * 1024 // 20MB per file

		var imageCount, fileCount int
		var imageTotalSize int64

		allowedFileMIME := map[string]bool{
			"application/pdf":    true,
			"application/msword": true,
			"application/vnd.openxmlformats-officedocument.wordprocessingml.document": true,
			"application/vnd.ms-excel": true,
			"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         true,
			"application/vnd.ms-powerpoint":                                             true,
			"application/vnd.openxmlformats-officedocument.presentationml.presentation": true,
			"text/plain":               true,
			"text/csv":                 true,
			"text/markdown":            true,
			"text/html":                true,
			"text/xml":                 true,
			"application/json":         true,
			"application/xml":          true,
			"application/rtf":          true,
			"application/octet-stream": true, // fallback for .log etc.
		}

		for _, att := range input.Images {
			if att.Base64 == "" {
				return nil, errs.New("error.chat_image_base64_required")
			}

			if att.Kind == "file" {
				fileCount++
				if fileCount > maxFiles {
					return nil, errs.New("error.chat_too_many_files")
				}
				if att.Size > maxFileSize {
					return nil, errs.New("error.chat_file_too_large")
				}
				if !allowedFileMIME[att.MimeType] && !strings.HasPrefix(att.MimeType, "text/") {
					return nil, errs.New("error.chat_invalid_file_type")
				}
			} else {
				// Default: treat as image
				imageCount++
				if imageCount > maxImages {
					return nil, errs.New("error.chat_too_many_images")
				}
				if !strings.HasPrefix(att.MimeType, "image/") {
					return nil, errs.New("error.chat_invalid_image_type")
				}
				if att.Size > maxImageSize {
					return nil, errs.New("error.chat_image_too_large")
				}
				imageTotalSize += att.Size
			}
		}

		if imageTotalSize > maxImageTotal {
			return nil, errs.New("error.chat_images_total_too_large")
		}
	}

	// Serialize attachments to JSON
	imagesJSON := "[]"
	if hasAttachments {
		b, err := json.Marshal(input.Images)
		if err != nil {
			return nil, errs.Wrap("error.chat_images_serialize_failed", err)
		}
		imagesJSON = string(b)
	}

	s.app.Logger.Info("[chat] SendMessage", "conv", input.ConversationID, "tab", input.TabID, "content_len", len(content), "attachments_count", len(input.Images))

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

	// Save attachments (images + files) to work directory and update payloads
	if hasAttachments && len(input.Images) > 0 {
		updatedImages, saveErr := s.saveImagesToWorkDir(ctx, db, agentConfig.AgentID, input.ConversationID, input.Images)
		if saveErr != nil {
			s.app.Logger.Warn("[chat] failed to save images to workdir, using original", "error", saveErr)
			// Continue with original images if save fails
		} else {
			// Update imagesJSON with saved image paths
			b, err := json.Marshal(updatedImages)
			if err != nil {
				s.app.Logger.Warn("[chat] failed to serialize updated images", "error", err)
			} else {
				imagesJSON = string(b)
			}
		}
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

	// Get agent config first (needed for saving images to workdir)
	agentConfig, providerConfig, agentExtras, err := s.getAgentAndProviderConfig(ctx, db, input.ConversationID)
	if err != nil {
		return nil, err
	}

	// Update message content and images
	// If new images are provided, update them; otherwise keep existing images
	updateQuery := db.NewUpdate().Model((*messageModel)(nil)).Where("id = ?", input.MessageID)
	imagesJSON := msg.ImagesJSON // keep existing by default
	if len(input.Images) > 0 {
		// Save new images to work directory
		updatedImages, saveErr := s.saveImagesToWorkDir(ctx, db, agentConfig.AgentID, input.ConversationID, input.Images)
		if saveErr != nil {
			s.app.Logger.Warn("[chat] failed to save images to workdir, using original", "error", saveErr)
			// Use original input images if save fails
			b, err := json.Marshal(input.Images)
			if err != nil {
				return nil, errs.Wrap("error.chat_images_serialize_failed", err)
			}
			imagesJSON = string(b)
		} else {
			// Use updated images with file paths
			b, err := json.Marshal(updatedImages)
			if err != nil {
				return nil, errs.Wrap("error.chat_images_serialize_failed", err)
			}
			imagesJSON = string(b)
		}
		updateQuery = updateQuery.Set("content = ?, images_json = ?", content, imagesJSON)
	} else {
		updateQuery = updateQuery.Set("content = ?", content)
	}
	if _, err := updateQuery.Exec(ctx); err != nil {
		return nil, errs.Wrap("error.chat_message_update_failed", err)
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

// WaitForGeneration waits until the active generation for a conversation is finished.
func (s *ChatService) WaitForGeneration(conversationID int64, requestID string) error {
	existing, ok := s.activeGenerations.Load(conversationID)
	if !ok {
		return nil
	}

	gen := existing.(*activeGeneration)
	if gen.requestID != requestID {
		return nil
	}

	<-gen.done
	return nil
}

// GetGenerationContent returns the currently accumulated assistant content for an active generation.
func (s *ChatService) GetGenerationContent(conversationID int64, requestID string) (string, bool) {
	existing, ok := s.activeGenerations.Load(conversationID)
	if !ok {
		return "", false
	}

	gen := existing.(*activeGeneration)
	if gen.requestID != requestID {
		return "", false
	}

	gen.mu.Lock()
	defer gen.mu.Unlock()
	return gen.streamText, true
}

func (s *ChatService) appendGenerationContent(conversationID int64, requestID string, delta string) {
	if delta == "" {
		return
	}

	existing, ok := s.activeGenerations.Load(conversationID)
	if !ok {
		return
	}

	gen := existing.(*activeGeneration)
	if gen.requestID != requestID {
		return
	}

	gen.mu.Lock()
	gen.streamText += delta
	gen.mu.Unlock()
}

// HasActiveGeneration reports whether the conversation still has a live generation.
// Cron history uses this to avoid marking manual runs as completed too early.
func (s *ChatService) HasActiveGeneration(conversationID int64) bool {
	if conversationID <= 0 {
		return false
	}
	_, ok := s.activeGenerations.Load(conversationID)
	return ok
}

// startGeneration creates a new generation context and launches the goroutine.
func (s *ChatService) startGeneration(db *bun.DB, conversationID int64, tabID string, agentConfig einoagent.Config, providerConfig einoagent.ProviderConfig, agentExtras AgentExtras, runFn func(ctx context.Context, requestID string)) (*SendMessageResult, error) {
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
