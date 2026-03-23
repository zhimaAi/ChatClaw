package chat

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"chatclaw/internal/errs"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// OpenClawGatewayInfo provides the connection details for the local OpenClaw Gateway.
type OpenClawGatewayInfo interface {
	GatewayURL() string
	GatewayToken() string
	IsReady() bool
}

// SetOpenClawGateway injects the OpenClaw gateway info.
func (s *ChatService) SetOpenClawGateway(gw OpenClawGatewayInfo) {
	s.openclawGateway = gw
}

// SendOpenClawMessage sends a message via the OpenClaw OpenResponses API.
func (s *ChatService) SendOpenClawMessage(input SendMessageInput) (*SendMessageResult, error) {
	if input.ConversationID <= 0 {
		return nil, errs.New("error.chat_conversation_id_required")
	}
	content := strings.TrimSpace(input.Content)
	hasAttachments := len(input.Images) > 0
	if content == "" && !hasAttachments {
		return nil, errs.New("error.chat_content_required")
	}

	if s.openclawGateway == nil || !s.openclawGateway.IsReady() {
		return nil, errs.New("error.openclaw_gateway_not_ready")
	}

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
	agentConfig, err := s.getOpenClawAgentConfig(ctx, db, input.ConversationID)
	if err != nil {
		return nil, err
	}

	imagesJSON := "[]"
	if hasAttachments {
		b, err := json.Marshal(input.Images)
		if err != nil {
			return nil, errs.Wrap("error.chat_images_serialize_failed", err)
		}
		imagesJSON = string(b)
	}

	s.app.Logger.Info("[openclaw-chat] SendOpenClawMessage", "conv", input.ConversationID, "tab", input.TabID, "content_len", len(content), "attachments_count", len(input.Images))

	requestID := uuid.New().String()
	genCtx, cancel := context.WithCancel(context.Background())

	gen := &activeGeneration{
		cancel:    cancel,
		requestID: requestID,
		tabID:     input.TabID,
		done:      make(chan struct{}),
	}
	s.activeGenerations.Store(input.ConversationID, gen)

	go func() {
		defer close(gen.done)
		defer s.tryDeleteGeneration(input.ConversationID, gen)
		s.runOpenClawGeneration(genCtx, db, input.ConversationID, input.TabID, requestID, content, imagesJSON, agentConfig)
	}()

	return &SendMessageResult{RequestID: requestID}, nil
}

// openClawAgentConfig holds the config needed for an OpenClaw API call.
type openClawAgentConfig struct {
	OpenClawAgentID string
	EnableThinking  bool
}

// getOpenClawAgentConfig reads the openclaw_agents table to build the API request config.
func (s *ChatService) getOpenClawAgentConfig(ctx context.Context, db *bun.DB, conversationID int64) (openClawAgentConfig, error) {
	type conversationRow struct {
		AgentID        int64  `bun:"agent_id"`
		EnableThinking bool   `bun:"enable_thinking"`
	}
	var conv conversationRow
	if err := db.NewSelect().
		Table("conversations").
		Column("agent_id", "enable_thinking").
		Where("id = ?", conversationID).
		Scan(ctx, &conv); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return openClawAgentConfig{}, errs.New("error.chat_conversation_not_found")
		}
		return openClawAgentConfig{}, errs.Wrap("error.chat_conversation_read_failed", err)
	}

	type agentRow struct {
		OpenClawAgentID string `bun:"openclaw_agent_id"`
	}
	var agent agentRow
	if err := db.NewSelect().
		Table("openclaw_agents").
		Column("openclaw_agent_id").
		Where("id = ?", conv.AgentID).
		Scan(ctx, &agent); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return openClawAgentConfig{}, errs.New("error.chat_agent_not_found")
		}
		return openClawAgentConfig{}, errs.Wrap("error.chat_agent_read_failed", err)
	}

	return openClawAgentConfig{
		OpenClawAgentID: agent.OpenClawAgentID,
		EnableThinking:  conv.EnableThinking,
	}, nil
}

// runOpenClawGeneration calls the OpenClaw OpenResponses API via SSE and
// translates the events into chat:* Wails events.
func (s *ChatService) runOpenClawGeneration(ctx context.Context, db *bun.DB, conversationID int64, tabID, requestID, userContent, imagesJSON string, cfg openClawAgentConfig) {
	gc := &generationContext{
		service:        s,
		db:             db,
		conversationID: conversationID,
		tabID:          tabID,
		requestID:      requestID,
	}

	if imagesJSON == "" {
		imagesJSON = "[]"
	}
	userMsg := &messageModel{
		ConversationID: conversationID,
		Role:           RoleUser,
		Content:        userContent,
		Status:         StatusSuccess,
		ToolCalls:      "[]",
		ImagesJSON:     imagesJSON,
	}
	dbCtx, dbCancel := context.WithTimeout(context.Background(), 5*time.Second)
	if _, err := db.NewInsert().Model(userMsg).Exec(dbCtx); err != nil {
		dbCancel()
		gc.emitError("error.chat_message_save_failed", nil)
		return
	}
	dbCancel()

	gc.emit(EventChatUserMessage, ChatUserMessageEvent{
		ChatEvent:  gc.chatEvent(userMsg.ID),
		Content:    userContent,
		ImagesJSON: imagesJSON,
	})

	assistantMsg := &messageModel{
		ConversationID: conversationID,
		Role:           RoleAssistant,
		Content:        "",
		Status:         StatusStreaming,
		ToolCalls:      "[]",
		ImagesJSON:     "[]",
	}
	dbCtx2, dbCancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	if _, err := db.NewInsert().Model(assistantMsg).Exec(dbCtx2); err != nil {
		dbCancel2()
		gc.emitError("error.chat_message_save_failed", nil)
		return
	}
	dbCancel2()

	gc.emit(EventChatStart, ChatStartEvent{
		ChatEvent: gc.chatEvent(assistantMsg.ID),
		Status:    StatusStreaming,
	})

	// Build conversation history for OpenClaw input
	inputItems := s.buildOpenClawInput(ctx, db, conversationID, userContent, imagesJSON)

	// Build the request body
	requestBody := s.buildOpenClawRequestBody(cfg, inputItems, conversationID)

	bodyJSON, err := json.Marshal(requestBody)
	if err != nil {
		gc.emitError("error.chat_generation_failed", map[string]any{"Error": err.Error()})
		s.updateMessageStatus(db, assistantMsg.ID, StatusError, err.Error(), "")
		return
	}

	gatewayURL := s.openclawGateway.GatewayURL()
	gatewayToken := s.openclawGateway.GatewayToken()

	req, err := http.NewRequestWithContext(ctx, "POST", gatewayURL+"/v1/responses", bytes.NewReader(bodyJSON))
	if err != nil {
		gc.emitError("error.chat_generation_failed", map[string]any{"Error": err.Error()})
		s.updateMessageStatus(db, assistantMsg.ID, StatusError, err.Error(), "")
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+gatewayToken)
	req.Header.Set("Accept", "text/event-stream")

	client := &http.Client{Timeout: 0}
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			s.updateMessageFinal(db, assistantMsg.ID, "", "", "[]", "[]", StatusCancelled, "", "cancelled", 0, 0)
			gc.emit(EventChatStopped, ChatStoppedEvent{
				ChatEvent: gc.chatEvent(assistantMsg.ID),
				Status:    StatusCancelled,
			})
			return
		}
		gc.emitError("error.chat_generation_failed", map[string]any{"Error": err.Error()})
		s.updateMessageStatus(db, assistantMsg.ID, StatusError, err.Error(), "")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		errMsg := fmt.Sprintf("OpenClaw API error (HTTP %d): %s", resp.StatusCode, string(bodyBytes))
		gc.emitError("error.chat_generation_failed", map[string]any{"Error": errMsg})
		s.updateMessageStatus(db, assistantMsg.ID, StatusError, errMsg, "")
		return
	}

	s.consumeOpenClawSSE(ctx, gc, assistantMsg, resp.Body)
}

// openClawSSEState tracks state during SSE event processing.
type openClawSSEState struct {
	contentBuilder  strings.Builder
	thinkingBuilder strings.Builder
	segments        []segment
	lastSegmentType string
	toolCalls       []openClawToolCall
	finishReason    string
	inputTokens     int
	outputTokens    int
}

type openClawToolCall struct {
	ID        string `json:"call_id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

func (ss *openClawSSEState) addContent(delta string) {
	if delta == "" {
		return
	}
	ss.contentBuilder.WriteString(delta)
	if ss.lastSegmentType == "content" && len(ss.segments) > 0 {
		ss.segments[len(ss.segments)-1].Content += delta
	} else {
		ss.segments = append(ss.segments, segment{Type: "content", Content: delta})
		ss.lastSegmentType = "content"
	}
}

func (ss *openClawSSEState) addThinking(delta string) {
	if delta == "" {
		return
	}
	ss.thinkingBuilder.WriteString(delta)
	if ss.lastSegmentType == "thinking" && len(ss.segments) > 0 {
		ss.segments[len(ss.segments)-1].Content += delta
	} else {
		ss.segments = append(ss.segments, segment{Type: "thinking", Content: delta})
		ss.lastSegmentType = "thinking"
	}
}

func (ss *openClawSSEState) segmentsStr() string {
	if len(ss.segments) > 0 {
		if b, err := json.Marshal(ss.segments); err == nil {
			return string(b)
		}
	}
	return "[]"
}

func (ss *openClawSSEState) toolCallsStr() string {
	if len(ss.toolCalls) > 0 {
		type tc struct {
			ID       string `json:"id"`
			Function struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			} `json:"function"`
		}
		calls := make([]tc, len(ss.toolCalls))
		for i, t := range ss.toolCalls {
			calls[i].ID = t.ID
			calls[i].Function.Name = t.Name
			calls[i].Function.Arguments = t.Arguments
		}
		if b, err := json.Marshal(calls); err == nil {
			return string(b)
		}
	}
	return "[]"
}

// consumeOpenClawSSE reads SSE events from the OpenClaw response and
// translates them into the existing chat:* Wails events.
func (s *ChatService) consumeOpenClawSSE(ctx context.Context, gc *generationContext, assistantMsg *messageModel, body io.Reader) {
	ss := &openClawSSEState{}
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 256*1024), 1024*1024)

	var eventType string
	var dataLines []string

	for scanner.Scan() {
		if ctx.Err() != nil {
			s.updateMessageFinal(gc.db, assistantMsg.ID, ss.contentBuilder.String(), ss.thinkingBuilder.String(), ss.toolCallsStr(), ss.segmentsStr(), StatusCancelled, "", "cancelled", ss.inputTokens, ss.outputTokens)
			gc.emit(EventChatStopped, ChatStoppedEvent{
				ChatEvent: gc.chatEvent(assistantMsg.ID),
				Status:    StatusCancelled,
			})
			return
		}

		line := scanner.Text()

		if line == "" {
			// Empty line = event boundary
			if len(dataLines) > 0 {
				data := strings.Join(dataLines, "\n")
				if data == "[DONE]" {
					break
				}
				s.processOpenClawEvent(gc, ss, assistantMsg, eventType, data)
			}
			eventType = ""
			dataLines = nil
			continue
		}

		if strings.HasPrefix(line, "event: ") {
			eventType = strings.TrimPrefix(line, "event: ")
		} else if strings.HasPrefix(line, "data: ") {
			dataLines = append(dataLines, strings.TrimPrefix(line, "data: "))
		} else if line == "data:" {
			dataLines = append(dataLines, "")
		}
	}

	if ctx.Err() != nil {
		s.updateMessageFinal(gc.db, assistantMsg.ID, ss.contentBuilder.String(), ss.thinkingBuilder.String(), ss.toolCallsStr(), ss.segmentsStr(), StatusCancelled, "", "cancelled", ss.inputTokens, ss.outputTokens)
		gc.emit(EventChatStopped, ChatStoppedEvent{
			ChatEvent: gc.chatEvent(assistantMsg.ID),
			Status:    StatusCancelled,
		})
		return
	}

	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		s.app.Logger.Error("[openclaw-chat] SSE scanner error", "conv", gc.conversationID, "error", err)
		gc.emitError("error.chat_stream_failed", map[string]any{"Error": err.Error()})
		s.updateMessageFinal(gc.db, assistantMsg.ID, ss.contentBuilder.String(), ss.thinkingBuilder.String(), ss.toolCallsStr(), ss.segmentsStr(), StatusError, err.Error(), "", ss.inputTokens, ss.outputTokens)
		return
	}

	s.updateMessageFinal(gc.db, assistantMsg.ID, ss.contentBuilder.String(), ss.thinkingBuilder.String(), ss.toolCallsStr(), ss.segmentsStr(), StatusSuccess, "", ss.finishReason, ss.inputTokens, ss.outputTokens)
	gc.emit(EventChatComplete, ChatCompleteEvent{
		ChatEvent:    gc.chatEvent(assistantMsg.ID),
		Status:       StatusSuccess,
		FinishReason: ss.finishReason,
	})
}

// processOpenClawEvent handles a single SSE event from the OpenClaw API.
func (s *ChatService) processOpenClawEvent(gc *generationContext, ss *openClawSSEState, assistantMsg *messageModel, eventType, data string) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(data), &raw); err != nil {
		s.app.Logger.Warn("[openclaw-chat] failed to parse SSE data", "event", eventType, "error", err)
		return
	}

	switch eventType {
	case "response.output_text.delta":
		delta, _ := raw["delta"].(string)
		if delta != "" {
			ss.addContent(delta)
			s.appendGenerationContent(gc.conversationID, gc.requestID, delta)
			gc.emit(EventChatChunk, ChatChunkEvent{
				ChatEvent: gc.chatEvent(assistantMsg.ID),
				Delta:     delta,
			})
			if cb, ok := s.chunkCallbacks.Load(gc.conversationID); ok {
				cb.(ChunkCallback)(ss.contentBuilder.String())
			}
		}

	case "response.reasoning_summary_text.delta", "response.reasoning_text.delta":
		delta, _ := raw["delta"].(string)
		if delta != "" {
			ss.addThinking(delta)
			gc.emit(EventChatThinking, ChatThinkingEvent{
				ChatEvent: gc.chatEvent(assistantMsg.ID),
				Delta:     delta,
			})
		}

	case "response.output_item.added":
		item, ok := raw["item"].(map[string]any)
		if !ok {
			return
		}
		itemType, _ := item["type"].(string)
		if itemType == "function_call" {
			name, _ := item["name"].(string)
			callID, _ := item["call_id"].(string)
			if name != "" && callID != "" {
				ss.toolCalls = append(ss.toolCalls, openClawToolCall{
					ID:   callID,
					Name: name,
				})
				// Add tool call segment
				if ss.lastSegmentType != "tools" || len(ss.segments) == 0 {
					ss.segments = append(ss.segments, segment{Type: "tools", ToolCallIDs: []string{callID}})
					ss.lastSegmentType = "tools"
				} else {
					ss.segments[len(ss.segments)-1].ToolCallIDs = append(ss.segments[len(ss.segments)-1].ToolCallIDs, callID)
				}
				gc.emit(EventChatTool, ChatToolEvent{
					ChatEvent:  gc.chatEvent(assistantMsg.ID),
					Type:       "call",
					ToolCallID: callID,
					ToolName:   name,
				})
			}
		}

	case "response.function_call_arguments.delta":
		delta, _ := raw["delta"].(string)
		itemID, _ := raw["item_id"].(string)
		if delta != "" {
			for i := range ss.toolCalls {
				if ss.toolCalls[i].ID == itemID || (len(ss.toolCalls) > 0 && i == len(ss.toolCalls)-1) {
					ss.toolCalls[i].Arguments += delta
					break
				}
			}
		}

	case "response.function_call_arguments.done":
		name, _ := raw["name"].(string)
		args, _ := raw["arguments"].(string)
		itemID, _ := raw["item_id"].(string)
		for i := range ss.toolCalls {
			if ss.toolCalls[i].ID == itemID || ss.toolCalls[i].Name == name {
				ss.toolCalls[i].Arguments = args
				break
			}
		}
		gc.emit(EventChatTool, ChatToolEvent{
			ChatEvent:  gc.chatEvent(assistantMsg.ID),
			Type:       "call",
			ToolCallID: itemID,
			ToolName:   name,
			ArgsJSON:   args,
		})

	case "response.completed":
		resp, ok := raw["response"].(map[string]any)
		if ok {
			if usage, ok := resp["usage"].(map[string]any); ok {
				if v, ok := usage["input_tokens"].(float64); ok {
					ss.inputTokens = int(v)
				}
				if v, ok := usage["output_tokens"].(float64); ok {
					ss.outputTokens = int(v)
				}
			}
			ss.finishReason = "stop"
		}

	case "response.failed":
		resp, ok := raw["response"].(map[string]any)
		if ok {
			if errObj, ok := resp["error"].(map[string]any); ok {
				errMsg, _ := errObj["message"].(string)
				gc.emitError("error.chat_generation_failed", map[string]any{"Error": errMsg})
				s.updateMessageFinal(gc.db, assistantMsg.ID, ss.contentBuilder.String(), ss.thinkingBuilder.String(), ss.toolCallsStr(), ss.segmentsStr(), StatusError, errMsg, "", ss.inputTokens, ss.outputTokens)
			}
		}

	case "response.incomplete":
		ss.finishReason = "length"
	}
}

// EditAndResendOpenClaw edits a message and resends via the OpenClaw API.
func (s *ChatService) EditAndResendOpenClaw(input EditAndResendInput) (*SendMessageResult, error) {
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

	if s.openclawGateway == nil || !s.openclawGateway.IsReady() {
		return nil, errs.New("error.openclaw_gateway_not_ready")
	}

	if existing, ok := s.activeGenerations.Load(input.ConversationID); ok {
		oldGen := existing.(*activeGeneration)
		oldGen.cancel()
		s.activeGenerations.Delete(input.ConversationID)
		select {
		case <-oldGen.done:
		case <-time.After(3 * time.Second):
			return nil, errs.New("error.chat_previous_generation_not_finished")
		}
	}

	db, err := s.db()
	if err != nil {
		return nil, err
	}

	ctx := context.Background()

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

	imagesJSON := msg.ImagesJSON
	if len(input.Images) > 0 {
		b, jsonErr := json.Marshal(input.Images)
		if jsonErr != nil {
			return nil, errs.Wrap("error.chat_images_serialize_failed", jsonErr)
		}
		imagesJSON = string(b)
		if _, updateErr := db.NewUpdate().Model((*messageModel)(nil)).
			Where("id = ?", input.MessageID).
			Set("content = ?, images_json = ?", content, imagesJSON).
			Exec(ctx); updateErr != nil {
			return nil, errs.Wrap("error.chat_message_update_failed", updateErr)
		}
	} else {
		if _, updateErr := db.NewUpdate().Model((*messageModel)(nil)).
			Where("id = ?", input.MessageID).
			Set("content = ?", content).
			Exec(ctx); updateErr != nil {
			return nil, errs.Wrap("error.chat_message_update_failed", updateErr)
		}
	}

	agentConfig, cfgErr := s.getOpenClawAgentConfig(ctx, db, input.ConversationID)
	if cfgErr != nil {
		return nil, cfgErr
	}

	requestID := uuid.New().String()
	genCtx, cancel := context.WithCancel(context.Background())

	gen := &activeGeneration{
		cancel:    cancel,
		requestID: requestID,
		tabID:     input.TabID,
		done:      make(chan struct{}),
	}
	s.activeGenerations.Store(input.ConversationID, gen)

	go func() {
		defer close(gen.done)
		defer s.tryDeleteGeneration(input.ConversationID, gen)
		s.runOpenClawGenerationFromHistory(genCtx, db, input.ConversationID, input.TabID, requestID, agentConfig)
	}()

	return &SendMessageResult{RequestID: requestID, MessageID: input.MessageID}, nil
}

// runOpenClawGenerationFromHistory runs OpenClaw generation using messages already in DB.
func (s *ChatService) runOpenClawGenerationFromHistory(ctx context.Context, db *bun.DB, conversationID int64, tabID, requestID string, cfg openClawAgentConfig) {
	gc := &generationContext{
		service:        s,
		db:             db,
		conversationID: conversationID,
		tabID:          tabID,
		requestID:      requestID,
	}

	assistantMsg := &messageModel{
		ConversationID: conversationID,
		Role:           RoleAssistant,
		Content:        "",
		Status:         StatusStreaming,
		ToolCalls:      "[]",
		ImagesJSON:     "[]",
	}
	dbCtx, dbCancel := context.WithTimeout(context.Background(), 5*time.Second)
	if _, err := db.NewInsert().Model(assistantMsg).Exec(dbCtx); err != nil {
		dbCancel()
		gc.emitError("error.chat_message_save_failed", nil)
		return
	}
	dbCancel()

	gc.emit(EventChatStart, ChatStartEvent{
		ChatEvent: gc.chatEvent(assistantMsg.ID),
		Status:    StatusStreaming,
	})

	inputItems := s.buildOpenClawInput(ctx, db, conversationID, "", "")
	requestBody := s.buildOpenClawRequestBody(cfg, inputItems, conversationID)

	bodyJSON, err := json.Marshal(requestBody)
	if err != nil {
		gc.emitError("error.chat_generation_failed", map[string]any{"Error": err.Error()})
		s.updateMessageStatus(db, assistantMsg.ID, StatusError, err.Error(), "")
		return
	}

	gatewayURL := s.openclawGateway.GatewayURL()
	gatewayToken := s.openclawGateway.GatewayToken()

	req, err := http.NewRequestWithContext(ctx, "POST", gatewayURL+"/v1/responses", bytes.NewReader(bodyJSON))
	if err != nil {
		gc.emitError("error.chat_generation_failed", map[string]any{"Error": err.Error()})
		s.updateMessageStatus(db, assistantMsg.ID, StatusError, err.Error(), "")
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+gatewayToken)
	req.Header.Set("Accept", "text/event-stream")

	client := &http.Client{Timeout: 0}
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			s.updateMessageFinal(db, assistantMsg.ID, "", "", "[]", "[]", StatusCancelled, "", "cancelled", 0, 0)
			gc.emit(EventChatStopped, ChatStoppedEvent{
				ChatEvent: gc.chatEvent(assistantMsg.ID),
				Status:    StatusCancelled,
			})
			return
		}
		gc.emitError("error.chat_generation_failed", map[string]any{"Error": err.Error()})
		s.updateMessageStatus(db, assistantMsg.ID, StatusError, err.Error(), "")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		errMsg := fmt.Sprintf("OpenClaw API error (HTTP %d): %s", resp.StatusCode, string(bodyBytes))
		gc.emitError("error.chat_generation_failed", map[string]any{"Error": errMsg})
		s.updateMessageStatus(db, assistantMsg.ID, StatusError, errMsg, "")
		return
	}

	s.consumeOpenClawSSE(ctx, gc, assistantMsg, resp.Body)
}

// buildOpenClawRequestBody creates the JSON request body for the OpenClaw OpenResponses API.
func (s *ChatService) buildOpenClawRequestBody(cfg openClawAgentConfig, inputItems []any, conversationID int64) map[string]any {
	body := map[string]any{
		"model":  "openclaw:" + cfg.OpenClawAgentID,
		"stream": true,
		"input":  inputItems,
		"user":   fmt.Sprintf("conv_%d", conversationID),
	}
	if cfg.EnableThinking {
		body["reasoning"] = map[string]any{
			"effort": "high",
		}
	}
	return body
}

// buildOpenClawInput constructs the input array for the OpenClaw API request.
// It loads conversation history and formats it as OpenClaw items.
func (s *ChatService) buildOpenClawInput(ctx context.Context, db *bun.DB, conversationID int64, latestContent, latestImagesJSON string) []any {
	var models []messageModel
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_ = db.NewSelect().
		Model(&models).
		Where("conversation_id = ?", conversationID).
		Where("status IN (?)", bun.In([]string{StatusSuccess, StatusCancelled})).
		OrderExpr("created_at ASC, id ASC").
		Scan(dbCtx)

	items := make([]any, 0, len(models)+1)

	for _, m := range models {
		switch m.Role {
		case RoleUser:
			content := buildOpenClawUserContent(m.Content, m.ImagesJSON)
			items = append(items, map[string]any{
				"type":    "message",
				"role":    "user",
				"content": content,
			})
		case RoleAssistant:
			if strings.TrimSpace(m.Content) != "" {
				items = append(items, map[string]any{
					"type":    "message",
					"role":    "assistant",
					"content": m.Content,
				})
			}
		}
	}

	return items
}

// buildOpenClawUserContent builds the content for a user message.
// If there are images, it returns an array of content parts; otherwise a plain string.
func buildOpenClawUserContent(text, imagesJSON string) any {
	if imagesJSON == "" || imagesJSON == "[]" {
		return text
	}

	var images []ImagePayload
	if err := json.Unmarshal([]byte(imagesJSON), &images); err != nil || len(images) == 0 {
		return text
	}

	var parts []any
	if strings.TrimSpace(text) != "" {
		parts = append(parts, map[string]any{
			"type": "input_text",
			"text": text,
		})
	}

	for _, img := range images {
		if img.Kind == "file" {
			if img.Base64 != "" && img.MimeType != "" {
				parts = append(parts, map[string]any{
					"type": "input_file",
					"source": map[string]any{
						"type":       "base64",
						"media_type": img.MimeType,
						"data":       img.Base64,
						"filename":   img.OriginalName,
					},
				})
			}
		} else {
			if img.Base64 != "" && img.MimeType != "" {
				dataURL := fmt.Sprintf("data:%s;base64,%s", img.MimeType, img.Base64)
				parts = append(parts, map[string]any{
					"type": "input_image",
					"source": map[string]any{
						"type": "url",
						"url":  dataURL,
					},
				})
			}
		}
	}

	if len(parts) == 0 {
		return text
	}
	return parts
}
