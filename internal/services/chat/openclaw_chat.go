package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"chatclaw/internal/errs"

	"github.com/google/uuid"
)

// OpenClawGatewayInfo provides the connection details and RPC access for the local OpenClaw Gateway.
type OpenClawGatewayInfo interface {
	GatewayURL() string
	GatewayToken() string
	IsReady() bool
	Request(ctx context.Context, method string, params any, out any) error
	AddEventListener(key string, fn func(event string, payload json.RawMessage))
	RemoveEventListener(key string)
}

// SetOpenClawGateway injects the OpenClaw gateway info.
func (s *ChatService) SetOpenClawGateway(gw OpenClawGatewayInfo) {
	s.openclawGateway = gw
}

// openClawAgentConfig holds the config needed for an OpenClaw chat.run call.
type openClawAgentConfig struct {
	OpenClawAgentID string
	EnableThinking  bool
}

// getOpenClawAgentConfig reads the openclaw_agents table to build the chat.run config.
func (s *ChatService) getOpenClawAgentConfig(conversationID int64) (openClawAgentConfig, error) {
	db, err := s.db()
	if err != nil {
		return openClawAgentConfig{}, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	type conversationRow struct {
		AgentID        int64 `bun:"agent_id"`
		EnableThinking bool  `bun:"enable_thinking"`
	}
	var conv conversationRow
	if err := db.NewSelect().
		Table("conversations").
		Column("agent_id", "enable_thinking").
		Where("id = ?", conversationID).
		Scan(ctx, &conv); err != nil {
		return openClawAgentConfig{}, errs.New("error.chat_conversation_not_found")
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
		return openClawAgentConfig{}, errs.New("error.chat_agent_not_found")
	}

	return openClawAgentConfig{
		OpenClawAgentID: agent.OpenClawAgentID,
		EnableThinking:  conv.EnableThinking,
	}, nil
}

// SendOpenClawMessage sends a message via the OpenClaw WebSocket chat.run API.
// Messages are NOT stored in the local database; OpenClaw manages session history.
func (s *ChatService) SendOpenClawMessage(input SendMessageInput) (*SendMessageResult, error) {
	if input.ConversationID <= 0 {
		return nil, errs.New("error.chat_conversation_id_required")
	}
	content := strings.TrimSpace(input.Content)
	if content == "" && len(input.Images) == 0 {
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

	agentConfig, err := s.getOpenClawAgentConfig(input.ConversationID)
	if err != nil {
		return nil, err
	}

	s.app.Logger.Info("[openclaw-chat] SendOpenClawMessage",
		"conv", input.ConversationID, "tab", input.TabID,
		"content_len", len(content), "attachments", len(input.Images))

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
		s.runOpenClawChatRun(genCtx, input.ConversationID, input.TabID, requestID, content, agentConfig)
	}()

	return &SendMessageResult{RequestID: requestID}, nil
}

// EditAndResendOpenClaw handles edit-and-resend for OpenClaw conversations.
// Since messages are not stored in the local database, we simply send a new message.
func (s *ChatService) EditAndResendOpenClaw(input EditAndResendInput) (*SendMessageResult, error) {
	if input.ConversationID <= 0 {
		return nil, errs.New("error.chat_conversation_id_required")
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

	agentConfig, err := s.getOpenClawAgentConfig(input.ConversationID)
	if err != nil {
		return nil, err
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
		s.runOpenClawChatRun(genCtx, input.ConversationID, input.TabID, requestID, content, agentConfig)
	}()

	return &SendMessageResult{RequestID: requestID, MessageID: input.MessageID}, nil
}

// openClawChatRunState tracks the streaming state for a single chat.run invocation.
type openClawChatRunState struct {
	contentBuilder  strings.Builder
	thinkingBuilder strings.Builder
	finishReason    string
	inputTokens     int
	outputTokens    int
	seq             int32
}

func (st *openClawChatRunState) nextSeq() int {
	return int(atomic.AddInt32(&st.seq, 1))
}

func (st *openClawChatRunState) chatEvent(conversationID int64, tabID, requestID string) ChatEvent {
	return ChatEvent{
		ConversationID: conversationID,
		TabID:          tabID,
		RequestID:      requestID,
		Seq:            st.nextSeq(),
		Ts:             time.Now().UnixMilli(),
	}
}

// runOpenClawChatRun executes a chat.run WebSocket RPC and translates
// gateway events into chat:* Wails events for the frontend.
func (s *ChatService) runOpenClawChatRun(ctx context.Context, conversationID int64, tabID, requestID, userContent string, cfg openClawAgentConfig) {
	st := &openClawChatRunState{}

	emit := func(eventName string, payload any) {
		s.app.Event.Emit(eventName, payload)
	}

	emitError := func(errorKey string, errorData any) {
		s.app.Logger.Error("[openclaw-chat] error",
			"conv", conversationID, "tab", tabID, "req", requestID,
			"key", errorKey, "data", errorData)
		emit(EventChatError, ChatErrorEvent{
			ChatEvent: st.chatEvent(conversationID, tabID, requestID),
			Status:    StatusError,
			ErrorKey:  errorKey,
			ErrorData: errorData,
		})
	}

	// Emit user message event so frontend can display it immediately
	emit(EventChatUserMessage, ChatUserMessageEvent{
		ChatEvent:  st.chatEvent(conversationID, tabID, requestID),
		Content:    userContent,
		ImagesJSON: "[]",
	})

	// Emit start event
	emit(EventChatStart, ChatStartEvent{
		ChatEvent: st.chatEvent(conversationID, tabID, requestID),
		Status:    StatusStreaming,
	})

	sessionKey := fmt.Sprintf("conv_%d", conversationID)
	idempotencyKey := requestID
	listenerKey := fmt.Sprintf("openclaw-chat-%d-%s", conversationID, requestID)

	done := make(chan struct{})

	// The actual runId is returned by the server; we'll capture it from
	// the first event or the RPC response and use sessionKey for initial routing.
	var activeRunID atomic.Value

	// All streaming events arrive as event="agent" with payload fields for routing.
	s.openclawGateway.AddEventListener(listenerKey, func(event string, payload json.RawMessage) {
		if event != "agent" {
			return
		}

		var frame struct {
			RunID      string          `json:"runId"`
			SessionKey string          `json:"sessionKey"`
			Stream     string          `json:"stream"`
			Data       json.RawMessage `json:"data"`
		}
		if json.Unmarshal(payload, &frame) != nil {
			return
		}

		// Route by runId if known, otherwise by sessionKey
		if rid, _ := activeRunID.Load().(string); rid != "" {
			if frame.RunID != rid {
				return
			}
		} else if frame.SessionKey != sessionKey {
			return
		}

		// Capture runId from the first matching event
		if frame.RunID != "" {
			activeRunID.CompareAndSwap(nil, frame.RunID)
			activeRunID.CompareAndSwap("", frame.RunID)
		}

		switch frame.Stream {
		case "assistant":
			var d struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}
			if json.Unmarshal(frame.Data, &d) != nil {
				return
			}
			if d.Text != "" {
				// The server sends cumulative text, not deltas.
				// Extract the actual delta by comparing with what we already have.
				prev := st.contentBuilder.String()
				if len(d.Text) > len(prev) && strings.HasPrefix(d.Text, prev) {
					delta := d.Text[len(prev):]
					st.contentBuilder.Reset()
					st.contentBuilder.WriteString(d.Text)
					s.appendGenerationContent(conversationID, requestID, delta)
					emit(EventChatChunk, ChatChunkEvent{
						ChatEvent: st.chatEvent(conversationID, tabID, requestID),
						Delta:     delta,
					})
					if cb, ok := s.chunkCallbacks.Load(conversationID); ok {
						cb.(ChunkCallback)(d.Text)
					}
				} else if d.Text != prev {
					// Fallback: full replacement (shouldn't happen normally)
					delta := d.Text
					if len(prev) > 0 {
						delta = d.Text[len(prev):]
					}
					st.contentBuilder.Reset()
					st.contentBuilder.WriteString(d.Text)
					if delta != "" {
						s.appendGenerationContent(conversationID, requestID, delta)
						emit(EventChatChunk, ChatChunkEvent{
							ChatEvent: st.chatEvent(conversationID, tabID, requestID),
							Delta:     delta,
						})
					}
					if cb, ok := s.chunkCallbacks.Load(conversationID); ok {
						cb.(ChunkCallback)(d.Text)
					}
				}
			}

		case "tool":
			var d struct {
				Phase      string          `json:"phase"`
				Name       string          `json:"name"`
				ToolCallID string          `json:"toolCallId"`
				Input      json.RawMessage `json:"input"`
				Result     json.RawMessage `json:"result"`
				Error      string          `json:"error"`
			}
			if json.Unmarshal(frame.Data, &d) != nil {
				return
			}
			switch d.Phase {
			case "start":
				argsJSON := ""
				if len(d.Input) > 0 {
					argsJSON = string(d.Input)
				}
				emit(EventChatTool, ChatToolEvent{
					ChatEvent:  st.chatEvent(conversationID, tabID, requestID),
					Type:       "call",
					ToolCallID: d.ToolCallID,
					ToolName:   d.Name,
					ArgsJSON:   argsJSON,
				})
			case "result":
				resultJSON := ""
				if len(d.Result) > 0 {
					resultJSON = string(d.Result)
				}
				emit(EventChatTool, ChatToolEvent{
					ChatEvent:  st.chatEvent(conversationID, tabID, requestID),
					Type:       "result",
					ToolCallID: d.ToolCallID,
					ToolName:   d.Name,
					ResultJSON: resultJSON,
				})
			case "error":
				emit(EventChatTool, ChatToolEvent{
					ChatEvent:  st.chatEvent(conversationID, tabID, requestID),
					Type:       "result",
					ToolCallID: d.ToolCallID,
					ToolName:   d.Name,
					ResultJSON: fmt.Sprintf(`{"error":%q}`, d.Error),
				})
			}

		case "lifecycle":
			var d struct {
				Phase string `json:"phase"`
				Error string `json:"error"`
			}
			if json.Unmarshal(frame.Data, &d) != nil {
				return
			}
			switch d.Phase {
			case "thinking":
				emit(EventChatThinking, ChatThinkingEvent{
					ChatEvent: st.chatEvent(conversationID, tabID, requestID),
					Delta:     "...",
				})
			case "end":
				st.finishReason = "stop"
				select {
				case <-done:
				default:
					close(done)
				}
			case "error":
				emitError("error.chat_generation_failed", map[string]any{"Error": d.Error})
				select {
				case <-done:
				default:
					close(done)
				}
			}
		}
	})

	defer s.openclawGateway.RemoveEventListener(listenerKey)

	params := map[string]any{
		"message":        userContent,
		"sessionKey":     sessionKey,
		"idempotencyKey": idempotencyKey,
		"agentId":        cfg.OpenClawAgentID,
	}
	if cfg.EnableThinking {
		params["thinking"] = "medium"
	}

	var runResult struct {
		RunID string `json:"runId"`
	}
	reqErr := s.openclawGateway.Request(ctx, "agent", params, &runResult)
	if reqErr != nil {
		if ctx.Err() != nil {
			emit(EventChatStopped, ChatStoppedEvent{
				ChatEvent: st.chatEvent(conversationID, tabID, requestID),
				Status:    StatusCancelled,
			})
			return
		}
		emitError("error.chat_generation_failed", map[string]any{"Error": reqErr.Error()})
		return
	}

	// Store the server-assigned runId for precise event routing
	if runResult.RunID != "" {
		activeRunID.Store(runResult.RunID)
	}

	s.app.Logger.Info("[openclaw-chat] agent RPC accepted",
		"conv", conversationID, "runId", runResult.RunID)

	// Wait for lifecycle "end" event or context cancellation
	select {
	case <-done:
	case <-ctx.Done():
		// Attempt to abort the run
		rid, _ := activeRunID.Load().(string)
		if rid != "" {
			abortCtx, abortCancel := context.WithTimeout(context.Background(), 3*time.Second)
			_ = s.openclawGateway.Request(abortCtx, "chat.abort", map[string]any{"runId": rid}, nil)
			abortCancel()
		}

		emit(EventChatStopped, ChatStoppedEvent{
			ChatEvent: st.chatEvent(conversationID, tabID, requestID),
			Status:    StatusCancelled,
		})
		return
	}

	emit(EventChatComplete, ChatCompleteEvent{
		ChatEvent:    st.chatEvent(conversationID, tabID, requestID),
		Status:       StatusSuccess,
		FinishReason: st.finishReason,
	})
}
