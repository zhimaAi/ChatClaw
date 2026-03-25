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
	QueryRequest(ctx context.Context, method string, params any, out any) error
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

// GetOpenClawLastAssistantReply fetches the last assistant message text from
// the OpenClaw Gateway session. Returns empty string if unavailable.
func (s *ChatService) GetOpenClawLastAssistantReply(conversationID int64) string {
	if s.openclawGateway == nil || !s.openclawGateway.IsReady() {
		return ""
	}

	sessionKey := fmt.Sprintf("conv_%d", conversationID)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var result struct {
		Messages []struct {
			Role    string `json:"role"`
			Content any    `json:"content"`
		} `json:"messages"`
	}
	if err := s.openclawGateway.QueryRequest(ctx, "sessions.get", map[string]any{
		"key":   sessionKey,
		"limit": 10,
	}, &result); err != nil {
		s.app.Logger.Warn("[openclaw-chat] GetOpenClawLastAssistantReply: sessions.get failed",
			"conv", conversationID, "err", err)
		return ""
	}

	s.app.Logger.Info("[openclaw-chat] GetOpenClawLastAssistantReply: sessions.get result",
		"conv", conversationID, "message_count", len(result.Messages))

	// Walk backwards to find the last assistant message with text content
	for i := len(result.Messages) - 1; i >= 0; i-- {
		msg := result.Messages[i]
		s.app.Logger.Debug("[openclaw-chat] GetOpenClawLastAssistantReply: message",
			"conv", conversationID, "index", i, "role", msg.Role)
		if msg.Role == "assistant" {
			if text := extractTextFromContent(msg.Content); text != "" {
				return text
			}
		}
	}
	return ""
}

func extractTextFromContent(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []any:
		var parts []string
		for _, block := range v {
			if bm, ok := block.(map[string]any); ok {
				if t, _ := bm["type"].(string); t == "text" {
					if text, _ := bm["text"].(string); text != "" {
						parts = append(parts, text)
					}
				}
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

// cleanOpenClawUserMessage strips the "Sender (untrusted metadata)" block
// and the "[Day YYYY-MM-DD HH:MM TZ] " timestamp prefix that the Gateway
// automatically prepends to user messages sent via chat.send.
func cleanOpenClawUserMessage(s string) string {
	s = strings.TrimLeft(s, " \t\n")

	// Strip "Sender (untrusted metadata):\n```json\n...\n```\n" block
	if strings.HasPrefix(s, "Sender (untrusted metadata)") {
		// Find the closing ``` and skip past it
		if idx := strings.Index(s, "```\n"); idx != -1 {
			rest := s[idx+4:]
			if idx2 := strings.Index(rest, "```"); idx2 != -1 {
				s = strings.TrimLeft(rest[idx2+3:], " \t\n")
			}
		}
	}

	// Strip "[Day YYYY-MM-DD HH:MM TZ] " timestamp prefix
	if strings.HasPrefix(s, "[") {
		if idx := strings.Index(s, "] "); idx != -1 && idx < 60 {
			s = strings.TrimLeft(s[idx+2:], " \t\n")
		}
	}

	return s
}

// GetOpenClawMessages fetches conversation history from the OpenClaw Gateway
// via the sessions.get WebSocket RPC method.
func (s *ChatService) GetOpenClawMessages(conversationID int64) ([]Message, error) {
	if conversationID <= 0 {
		return nil, errs.New("error.chat_conversation_id_required")
	}
	if s.openclawGateway == nil || !s.openclawGateway.IsReady() {
		return nil, nil
	}

	sessionKey := fmt.Sprintf("conv_%d", conversationID)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var result struct {
		Messages []struct {
			Role    string `json:"role"`
			Content any    `json:"content"`
		} `json:"messages"`
	}
	if err := s.openclawGateway.QueryRequest(ctx, "sessions.get", map[string]any{
		"key":   sessionKey,
		"limit": 200,
	}, &result); err != nil {
		s.app.Logger.Warn("[openclaw-chat] sessions.get failed",
			"conv", conversationID, "err", err)
		return nil, nil
	}

	// OpenClaw transcripts break a single agent turn into multiple
	// assistant+toolResult messages (e.g. thinking→toolCalls→toolResults→
	// thinking→toolCalls→toolResults→final text). We merge consecutive
	// assistant/toolResult messages into one assistant Message and build
	// ordered segments so the frontend can render them interleaved.

	type toolCallEntry struct {
		id   string
		name string
	}

	type segment struct {
		Type        string           `json:"type"`
		Content     string           `json:"content,omitempty"`
		ToolCallIDs []string         `json:"tool_call_ids,omitempty"`
	}

	type assistantGroup struct {
		segments      []segment
		allToolCalls  []map[string]any
		toolResults   []Message
		contentAll    strings.Builder
		thinkingAll   strings.Builder
		// pending tracks toolCall ids from the most recent assistant message,
		// consumed in order by subsequent toolResult messages.
		pending []toolCallEntry
	}

	// appendToolCallSegment adds a tool call id to the last 'tools' segment,
	// or creates a new one if the last segment is not 'tools'.
	appendToolCallSeg := func(g *assistantGroup, id string) {
		if n := len(g.segments); n > 0 && g.segments[n-1].Type == "tools" {
			g.segments[n-1].ToolCallIDs = append(g.segments[n-1].ToolCallIDs, id)
		} else {
			g.segments = append(g.segments, segment{Type: "tools", ToolCallIDs: []string{id}})
		}
	}

	var messages []Message
	var msgIDCounter int64
	now := time.Now()

	var group *assistantGroup

	flushGroup := func() {
		if group == nil {
			return
		}
		msgIDCounter--
		msg := Message{
			ID:              msgIDCounter,
			ConversationID:  conversationID,
			Role:            "assistant",
			Content:         group.contentAll.String(),
			ThinkingContent: group.thinkingAll.String(),
			Status:          StatusSuccess,
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		if len(group.allToolCalls) > 0 {
			tc, _ := json.Marshal(group.allToolCalls)
			msg.ToolCalls = string(tc)
		}
		if len(group.segments) > 0 {
			seg, _ := json.Marshal(group.segments)
			msg.Segments = string(seg)
		}
		messages = append(messages, msg)
		messages = append(messages, group.toolResults...)
		group = nil
	}

	for _, m := range result.Messages {
		if m.Role == "system" {
			continue
		}

		if m.Role == "toolResult" {
			resultContent := extractTextFromContent(m.Content)
			if group != nil && len(group.pending) > 0 {
				tc := group.pending[0]
				group.pending = group.pending[1:]
				msgIDCounter--
				group.toolResults = append(group.toolResults, Message{
					ID:             msgIDCounter,
					ConversationID: conversationID,
					Role:           "tool",
					Content:        resultContent,
					ToolCallID:     tc.id,
					ToolCallName:   tc.name,
					Status:         StatusSuccess,
					CreatedAt:      now,
					UpdatedAt:      now,
				})
			}
			continue
		}

		if m.Role == "assistant" {
			if group == nil {
				group = &assistantGroup{}
			}
			group.pending = nil

			blocks, ok := m.Content.([]any)
			if !ok {
				if s, ok := m.Content.(string); ok && s != "" {
					if group.contentAll.Len() > 0 {
						group.contentAll.WriteString("\n")
					}
					group.contentAll.WriteString(s)
					group.segments = append(group.segments, segment{Type: "content", Content: s})
				}
				continue
			}
			for _, block := range blocks {
				bm, ok := block.(map[string]any)
				if !ok {
					continue
				}
				blockType, _ := bm["type"].(string)
				switch blockType {
				case "text":
					if text, _ := bm["text"].(string); text != "" {
						if group.contentAll.Len() > 0 {
							group.contentAll.WriteString("\n")
						}
						group.contentAll.WriteString(text)
						group.segments = append(group.segments, segment{Type: "content", Content: text})
					}
				case "thinking":
					if t, _ := bm["thinking"].(string); t != "" {
						if group.thinkingAll.Len() > 0 {
							group.thinkingAll.WriteString("\n\n")
						}
						group.thinkingAll.WriteString(t)
						group.segments = append(group.segments, segment{Type: "thinking", Content: t})
					}
				case "toolCall":
					name, _ := bm["name"].(string)
					id, _ := bm["id"].(string)
					argsRaw := bm["arguments"]
					argsJSON, _ := json.Marshal(argsRaw)
					group.allToolCalls = append(group.allToolCalls, map[string]any{
						"id":       id,
						"type":     "function",
						"function": map[string]any{"name": name, "arguments": string(argsJSON)},
					})
					entry := toolCallEntry{id: id, name: name}
					group.pending = append(group.pending, entry)
					appendToolCallSeg(group, id)
				}
			}
			continue
		}

		// User or other role: flush any pending assistant group first.
		flushGroup()

		msgIDCounter--
		contentStr := extractTextFromContent(m.Content)
		if m.Role == "user" {
			contentStr = cleanOpenClawUserMessage(contentStr)
		}

		messages = append(messages, Message{
			ID:             msgIDCounter,
			ConversationID: conversationID,
			Role:           m.Role,
			Content:        contentStr,
			Status:         StatusSuccess,
			CreatedAt:      now,
			UpdatedAt:      now,
		})
	}
	flushGroup()

	return messages, nil
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

// openClawChatRunState tracks the streaming state for a single chat.send invocation.
type openClawChatRunState struct {
	requestID       string
	contentBuilder  strings.Builder
	thinkingBuilder strings.Builder
	finishReason    string
	inputTokens     int
	outputTokens    int
	seq             int32
	// Cumulative content already emitted, used to compute deltas from
	// the cumulative message object in chat events.
	emittedThinking string
	emittedContent  string
	seenToolCalls   map[string]bool
	seenToolResults map[string]bool
}

func (st *openClawChatRunState) nextSeq() int {
	return int(atomic.AddInt32(&st.seq, 1))
}

func (st *openClawChatRunState) chatEvent(conversationID int64, tabID, requestID string, messageID ...int64) ChatEvent {
	var mid int64
	if len(messageID) > 0 {
		mid = messageID[0]
	}
	return ChatEvent{
		ConversationID: conversationID,
		TabID:          tabID,
		RequestID:      requestID,
		Seq:            st.nextSeq(),
		MessageID:      mid,
		Ts:             time.Now().UnixMilli(),
	}
}

// handleOpenClawAgentEvent processes "agent" events from the Gateway.
// These carry streaming deltas for text, tool phases, and lifecycle signals.
func (s *ChatService) handleOpenClawAgentEvent(
	conversationID int64,
	sessionKey string,
	activeRunID *atomic.Value,
	st *openClawChatRunState,
	done chan struct{},
	ce func() ChatEvent,
	emit func(string, any),
	emitError func(string, any),
	payload json.RawMessage,
) {
	var frame struct {
		RunID      string          `json:"runId"`
		SessionKey string          `json:"sessionKey"`
		Stream     string          `json:"stream"`
		Data       json.RawMessage `json:"data"`
	}
	if json.Unmarshal(payload, &frame) != nil {
		return
	}

	// Route by runId
	if rid, _ := activeRunID.Load().(string); rid != "" {
		if frame.RunID != "" && frame.RunID != rid {
			return
		}
	}
	if frame.RunID != "" {
		activeRunID.CompareAndSwap(nil, frame.RunID)
		activeRunID.CompareAndSwap("", frame.RunID)
	}

	switch frame.Stream {
	case "assistant":
		var d struct {
			Delta string `json:"delta"`
		}
		if json.Unmarshal(frame.Data, &d) != nil || d.Delta == "" {
			return
		}
		st.contentBuilder.WriteString(d.Delta)
		st.emittedContent = st.contentBuilder.String()
		s.appendGenerationContent(conversationID, st.requestID, d.Delta)
		emit(EventChatChunk, ChatChunkEvent{
			ChatEvent: ce(),
			Delta:     d.Delta,
		})
		if cb, ok := s.chunkCallbacks.Load(conversationID); ok {
			cb.(ChunkCallback)(st.contentBuilder.String())
		}

	case "thinking":
		var d struct {
			Delta string `json:"delta"`
		}
		if json.Unmarshal(frame.Data, &d) != nil || d.Delta == "" {
			return
		}
		st.thinkingBuilder.WriteString(d.Delta)
		st.emittedThinking = st.thinkingBuilder.String()
		emit(EventChatThinking, ChatThinkingEvent{
			ChatEvent: ce(),
			Delta:     d.Delta,
		})

	case "tool":
		var d struct {
			Phase      string          `json:"phase"`
			Name       string          `json:"name"`
			ToolCallID string          `json:"toolCallId"`
			Args       json.RawMessage `json:"args"`
			Result     json.RawMessage `json:"result"`
			Meta       string          `json:"meta"`
			IsError    bool            `json:"isError"`
		}
		if json.Unmarshal(frame.Data, &d) != nil {
			return
		}
		switch d.Phase {
		case "start":
			argsJSON := ""
			if len(d.Args) > 0 {
				argsJSON = string(d.Args)
			}
			if st.seenToolCalls == nil {
				st.seenToolCalls = make(map[string]bool)
			}
			st.seenToolCalls[d.ToolCallID] = true
			emit(EventChatTool, ChatToolEvent{
				ChatEvent:  ce(),
				Type:       "call",
				ToolCallID: d.ToolCallID,
				ToolName:   d.Name,
				ArgsJSON:   argsJSON,
			})
		case "result":
			resultJSON := ""
			if len(d.Result) > 0 {
				resultJSON = string(d.Result)
			} else if d.Meta != "" {
				resultJSON = fmt.Sprintf(`{"summary":%q}`, d.Meta)
			}
			if st.seenToolResults == nil {
				st.seenToolResults = make(map[string]bool)
			}
			st.seenToolResults[d.ToolCallID] = true
			emit(EventChatTool, ChatToolEvent{
				ChatEvent:  ce(),
				Type:       "result",
				ToolCallID: d.ToolCallID,
				ToolName:   d.Name,
				ResultJSON: resultJSON,
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
		s.app.Logger.Info("[openclaw-chat] agent lifecycle",
			"conv", conversationID, "phase", d.Phase)
		switch d.Phase {
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
}

// handleOpenClawChatEvent processes "chat" events from the Gateway.
// Chat events carry the full cumulative message object with content blocks
// (thinking, text, toolCall, toolResult). We diff against previously emitted
// state and emit the appropriate frontend events.
func (s *ChatService) handleOpenClawChatEvent(
	conversationID int64,
	sessionKey string,
	activeRunID *atomic.Value,
	st *openClawChatRunState,
	done chan struct{},
	ce func() ChatEvent,
	emit func(string, any),
	emitError func(string, any),
	payload json.RawMessage,
) {
	var chatEvt struct {
		State        string `json:"state"`
		SessionKey   string `json:"sessionKey"`
		RunID        string `json:"runId"`
		ErrorMessage string `json:"errorMessage"`
		Message      struct {
			Role       string          `json:"role"`
			Content    json.RawMessage `json:"content"`
			StopReason string          `json:"stopReason"`
		} `json:"message"`
	}
	if json.Unmarshal(payload, &chatEvt) != nil {
		s.app.Logger.Warn("[openclaw-chat] chat event: unmarshal failed", "conv", conversationID)
		return
	}

	s.app.Logger.Debug("[openclaw-chat] chat event parsed",
		"conv", conversationID,
		"state", chatEvt.State,
		"sessionKey", chatEvt.SessionKey,
		"wantSession", sessionKey,
		"runId", chatEvt.RunID,
		"role", chatEvt.Message.Role,
		"contentLen", len(chatEvt.Message.Content),
		"stopReason", chatEvt.Message.StopReason)

	// Filter by session
	if chatEvt.SessionKey != "" && chatEvt.SessionKey != sessionKey &&
		!strings.HasSuffix(chatEvt.SessionKey, ":"+sessionKey) {
		s.app.Logger.Debug("[openclaw-chat] chat event: session mismatch, skipping",
			"conv", conversationID, "got", chatEvt.SessionKey, "want", sessionKey)
		return
	}

	// Capture runId
	if chatEvt.RunID != "" {
		activeRunID.CompareAndSwap(nil, chatEvt.RunID)
		activeRunID.CompareAndSwap("", chatEvt.RunID)
	}

	// Filter by runId
	if rid, _ := activeRunID.Load().(string); rid != "" {
		if chatEvt.RunID != "" && chatEvt.RunID != rid {
			return
		}
	}

	switch chatEvt.State {
	case "error":
		emitError("error.chat_generation_failed", map[string]any{"Error": chatEvt.ErrorMessage})
		select {
		case <-done:
		default:
			close(done)
		}
		return

	case "aborted":
		select {
		case <-done:
		default:
			close(done)
		}
		return

	case "delta", "final":
		// Process message content below

	default:
		return
	}

	// Skip non-assistant messages (e.g., tool_result role)
	if chatEvt.Message.Role != "assistant" && chatEvt.Message.Role != "" {
		return
	}

	if len(chatEvt.Message.Content) > 0 {
		var blocks []struct {
			Type       string          `json:"type"`
			Text       string          `json:"text"`
			Thinking   string          `json:"thinking"`
			ToolCallID string          `json:"toolCallId"`
			Name       string          `json:"name"`
			Args       json.RawMessage `json:"args"`
			Content    json.RawMessage `json:"content"`
		}
		if json.Unmarshal(chatEvt.Message.Content, &blocks) == nil {
			blockTypes := make([]string, len(blocks))
			for i, b := range blocks {
				blockTypes[i] = b.Type
			}
			s.app.Logger.Debug("[openclaw-chat] chat event content blocks",
				"conv", conversationID,
				"blockCount", len(blocks),
				"types", blockTypes)
			s.processOpenClawContentBlocks(conversationID, blocks, st, ce, emit)
		} else {
			s.app.Logger.Warn("[openclaw-chat] chat event: content parse failed",
				"conv", conversationID,
				"rawContent", string(chatEvt.Message.Content[:min(200, len(chatEvt.Message.Content))]))
		}
	}

	// On "final" state, signal completion
	if chatEvt.State == "final" {
		st.finishReason = "stop"
		if chatEvt.Message.StopReason != "" {
			st.finishReason = chatEvt.Message.StopReason
		}
		select {
		case <-done:
		default:
			close(done)
		}
	}
}

// processOpenClawContentBlocks extracts thinking, text, and tool content
// from the cumulative message content blocks, computing deltas against
// what has been previously emitted.
func (s *ChatService) processOpenClawContentBlocks(
	conversationID int64,
	blocks []struct {
		Type       string          `json:"type"`
		Text       string          `json:"text"`
		Thinking   string          `json:"thinking"`
		ToolCallID string          `json:"toolCallId"`
		Name       string          `json:"name"`
		Args       json.RawMessage `json:"args"`
		Content    json.RawMessage `json:"content"`
	},
	st *openClawChatRunState,
	ce func() ChatEvent,
	emit func(string, any),
) {
	// Collect cumulative thinking and text from all blocks
	var allThinking strings.Builder
	var allText strings.Builder
	for _, b := range blocks {
		switch b.Type {
		case "thinking":
			if b.Thinking != "" {
				if allThinking.Len() > 0 {
					allThinking.WriteString("\n\n")
				}
				allThinking.WriteString(b.Thinking)
			}
		case "text":
			allText.WriteString(b.Text)
		case "toolCall":
			// Track seen tool calls and emit new ones
			if b.ToolCallID != "" && !st.seenToolCalls[b.ToolCallID] {
				if st.seenToolCalls == nil {
					st.seenToolCalls = make(map[string]bool)
				}
				st.seenToolCalls[b.ToolCallID] = true
				argsJSON := ""
				if len(b.Args) > 0 {
					argsJSON = string(b.Args)
				}
				emit(EventChatTool, ChatToolEvent{
					ChatEvent:  ce(),
					Type:       "call",
					ToolCallID: b.ToolCallID,
					ToolName:   b.Name,
					ArgsJSON:   argsJSON,
				})
			}
		case "toolResult":
			if b.ToolCallID != "" && !st.seenToolResults[b.ToolCallID] {
				if st.seenToolResults == nil {
					st.seenToolResults = make(map[string]bool)
				}
				st.seenToolResults[b.ToolCallID] = true
				resultJSON := ""
				if len(b.Content) > 0 {
					resultJSON = string(b.Content)
				}
				emit(EventChatTool, ChatToolEvent{
					ChatEvent:  ce(),
					Type:       "result",
					ToolCallID: b.ToolCallID,
					ResultJSON: resultJSON,
				})
			}
		}
	}

	// Emit thinking delta
	newThinking := allThinking.String()
	if newThinking != "" && len(newThinking) > len(st.emittedThinking) {
		delta := newThinking[len(st.emittedThinking):]
		st.emittedThinking = newThinking
		st.thinkingBuilder.Reset()
		st.thinkingBuilder.WriteString(newThinking)
		emit(EventChatThinking, ChatThinkingEvent{
			ChatEvent: ce(),
			Delta:     delta,
		})
	}

	// Emit text delta
	newText := allText.String()
	if newText != "" && len(newText) > len(st.emittedContent) {
		delta := newText[len(st.emittedContent):]
		st.emittedContent = newText
		st.contentBuilder.Reset()
		st.contentBuilder.WriteString(newText)
		s.appendGenerationContent(conversationID, st.requestID, delta)
		emit(EventChatChunk, ChatChunkEvent{
			ChatEvent: ce(),
			Delta:     delta,
		})
		if cb, ok := s.chunkCallbacks.Load(conversationID); ok {
			cb.(ChunkCallback)(newText)
		}
	}
}

// runOpenClawChatRun sends an "agent" RPC (blocking) and processes
// concurrent "agent" and "chat" events for real-time streaming output.
// Agent events provide text/tool deltas; chat events provide thinking blocks.
func (s *ChatService) runOpenClawChatRun(ctx context.Context, conversationID int64, tabID, requestID, userContent string, cfg openClawAgentConfig) {
	st := &openClawChatRunState{requestID: requestID}

	assistantMsgID := -conversationID*1000 - int64(time.Now().UnixMilli()%100000)

	emit := func(eventName string, payload any) {
		s.app.Event.Emit(eventName, payload)
	}

	ce := func() ChatEvent {
		return st.chatEvent(conversationID, tabID, requestID, assistantMsgID)
	}

	emitError := func(errorKey string, errorData any) {
		s.app.Logger.Error("[openclaw-chat] error",
			"conv", conversationID, "tab", tabID, "req", requestID,
			"key", errorKey, "data", errorData)
		emit(EventChatError, ChatErrorEvent{
			ChatEvent: ce(),
			Status:    StatusError,
			ErrorKey:  errorKey,
			ErrorData: errorData,
		})
	}

	emit(EventChatStart, ChatStartEvent{
		ChatEvent: ce(),
		Status:    StatusStreaming,
	})

	sessionKey := fmt.Sprintf("conv_%d", conversationID)
	idempotencyKey := requestID
	listenerKey := fmt.Sprintf("openclaw-chat-%d-%s", conversationID, requestID)

	done := make(chan struct{})
	var activeRunID atomic.Value

	// Listen for all gateway events and route accordingly.
	s.openclawGateway.AddEventListener(listenerKey, func(event string, payload json.RawMessage) {
		s.app.Logger.Info("[openclaw-chat] event received",
			"conv", conversationID, "event", event,
			"payloadLen", len(payload))

		switch event {
		case "chat":
			s.handleOpenClawChatEvent(conversationID, sessionKey, &activeRunID, st, done, ce, emit, emitError, payload)
		case "agent":
			s.handleOpenClawAgentEvent(conversationID, sessionKey, &activeRunID, st, done, ce, emit, emitError, payload)
		case "agent_late_error":
			var errEvt struct {
				Error string `json:"error"`
				RunID string `json:"runId"`
			}
			if json.Unmarshal(payload, &errEvt) != nil {
				return
			}
			// Only handle if runId matches or we haven't captured a runId yet
			if rid, _ := activeRunID.Load().(string); rid != "" && errEvt.RunID != "" && errEvt.RunID != rid {
				return
			}
			s.app.Logger.Error("[openclaw-chat] late error from gateway",
				"conv", conversationID, "runId", errEvt.RunID, "error", errEvt.Error)
			emitError("error.chat_generation_failed", map[string]any{"Error": errEvt.Error})
			select {
			case <-done:
			default:
				close(done)
			}
		}
	})

	defer s.openclawGateway.RemoveEventListener(listenerKey)

	// Use the "agent" RPC (blocking: returns when the run completes).
	// While it blocks, chat/agent events arrive via readLoop for real-time streaming.
	params := map[string]any{
		"message":        userContent,
		"sessionKey":     sessionKey,
		"idempotencyKey": idempotencyKey,
		"agentId":        cfg.OpenClawAgentID,
	}
	if cfg.EnableThinking {
		params["thinking"] = "medium"
	}

	s.app.Logger.Info("[openclaw-chat] sending agent RPC",
		"conv", conversationID,
		"agentId", cfg.OpenClawAgentID,
		"sessionKey", sessionKey,
		"idempotencyKey", idempotencyKey,
		"contentLen", len(userContent))

	var runResult struct {
		RunID string `json:"runId"`
	}
	reqErr := s.openclawGateway.Request(ctx, "agent", params, &runResult)
	if reqErr != nil {
		if ctx.Err() != nil {
			emit(EventChatStopped, ChatStoppedEvent{
				ChatEvent: ce(),
				Status:    StatusCancelled,
			})
			return
		}
		emitError("error.chat_generation_failed", map[string]any{"Error": reqErr.Error()})
		return
	}

	if runResult.RunID != "" {
		activeRunID.Store(runResult.RunID)
	}

	s.app.Logger.Info("[openclaw-chat] agent RPC completed",
		"conv", conversationID, "runId", runResult.RunID)

	// The agent RPC may return before all streaming events are delivered
	// (e.g., the Gateway sends an early ack with runId). Wait for the
	// actual completion signal from lifecycle "end" / chat "final" events
	// so we don't tear down the event listener prematurely.
	select {
	case <-done:
		// Completed via streaming events
	default:
		s.app.Logger.Info("[openclaw-chat] agent RPC returned before completion event, waiting for events",
			"conv", conversationID, "runId", runResult.RunID)
		select {
		case <-done:
		case <-ctx.Done():
			emit(EventChatStopped, ChatStoppedEvent{
				ChatEvent: ce(),
				Status:    StatusCancelled,
			})
			return
		case <-time.After(3 * time.Minute):
			s.app.Logger.Warn("[openclaw-chat] timed out waiting for agent completion events",
				"conv", conversationID, "runId", runResult.RunID)
		}
	}

	if st.finishReason == "" {
		st.finishReason = "stop"
	}

	emit(EventChatComplete, ChatCompleteEvent{
		ChatEvent:    ce(),
		Status:       StatusSuccess,
		FinishReason: st.finishReason,
	})
}
