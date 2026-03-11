package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	einoagent "chatclaw/internal/eino/agent"
	"chatclaw/internal/eino/tools"
	feishutools "chatclaw/internal/eino/tools/im/feishu"
	wecomtools "chatclaw/internal/eino/tools/im/wecom"
	"chatclaw/internal/define"
	"chatclaw/internal/services/memory"
	"chatclaw/internal/services/skills"
	"chatclaw/internal/sqlite"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/uptrace/bun"
)

// generationContext bundles per-generation state shared across helper methods.
type generationContext struct {
	service        *ChatService
	db             *bun.DB
	conversationID int64
	tabID          string
	requestID      string
	agentConfig    einoagent.Config
	providerConfig einoagent.ProviderConfig
	agentExtras    AgentExtras

	seq int32
}

func (g *generationContext) nextSeq() int {
	return int(atomic.AddInt32(&g.seq, 1))
}

func (g *generationContext) emit(eventName string, payload any) {
	g.service.app.Event.Emit(eventName, payload)
}

func (g *generationContext) emitError(errorKey string, errorData any) {
	g.service.app.Logger.Error("[chat] error", "conv", g.conversationID, "tab", g.tabID, "req", g.requestID, "key", errorKey, "data", errorData)
	g.emit(EventChatError, ChatErrorEvent{
		ChatEvent: ChatEvent{
			ConversationID: g.conversationID,
			TabID:          g.tabID,
			RequestID:      g.requestID,
			Seq:            g.nextSeq(),
			Ts:             time.Now().UnixMilli(),
		},
		Status:    StatusError,
		ErrorKey:  errorKey,
		ErrorData: errorData,
	})
}

func (g *generationContext) chatEvent(messageID int64) ChatEvent {
	return ChatEvent{
		ConversationID: g.conversationID,
		TabID:          g.tabID,
		RequestID:      g.requestID,
		Seq:            g.nextSeq(),
		MessageID:      messageID,
		Ts:             time.Now().UnixMilli(),
	}
}

// runGeneration inserts the user message then delegates to runGenerationCore.
func (s *ChatService) runGeneration(ctx context.Context, db *bun.DB, conversationID int64, tabID, requestID, userContent, imagesJSON string, agentConfig einoagent.Config, providerConfig einoagent.ProviderConfig, agentExtras AgentExtras) {
	gc := &generationContext{
		service:        s,
		db:             db,
		conversationID: conversationID,
		tabID:          tabID,
		requestID:      requestID,
		agentConfig:    agentConfig,
		providerConfig: providerConfig,
		agentExtras:    agentExtras,
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

	s.runGenerationCore(ctx, gc)
}

// runGenerationWithExistingHistory runs generation using messages already in DB.
func (s *ChatService) runGenerationWithExistingHistory(ctx context.Context, db *bun.DB, conversationID int64, tabID, requestID string, agentConfig einoagent.Config, providerConfig einoagent.ProviderConfig, agentExtras AgentExtras) {
	gc := &generationContext{
		service:        s,
		db:             db,
		conversationID: conversationID,
		tabID:          tabID,
		requestID:      requestID,
		agentConfig:    agentConfig,
		providerConfig: providerConfig,
		agentExtras:    agentExtras,
	}
	s.runGenerationCore(ctx, gc)
}

// runGenerationCore is the unified generation loop used by both entry points.
func (s *ChatService) runGenerationCore(ctx context.Context, gc *generationContext) {
	db := gc.db
	conversationID := gc.conversationID
	agentConfig := gc.agentConfig
	providerConfig := gc.providerConfig
	agentExtras := gc.agentExtras

	assistantMsg := &messageModel{
		ConversationID: conversationID,
		Role:           RoleAssistant,
		Content:        "",
		ProviderID:     providerConfig.ProviderID,
		ModelID:        agentConfig.ModelID,
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

	// Agent mode loads all messages — context window management is handled
	// by the Summarization middleware which compresses history automatically.
	messages, err := s.loadMessagesForContext(ctx, db, conversationID, 0, providerConfig.ProviderID, agentConfig.ModelID)
	if err != nil {
		gc.emitError("error.chat_messages_failed", nil)
		s.updateMessageStatus(db, assistantMsg.ID, StatusError, "Failed to load messages", "")
		return
	}

	// Build extra tools and handlers
	extraTools, extraHandlers := s.buildExtras(ctx, gc)

	agentConfig.Provider = providerConfig
	agentResult, err := einoagent.NewChatModelAgent(ctx, agentConfig, s.toolRegistry, s.bgProcessManager, extraTools, extraHandlers, s.app.Logger, len(messages))
	if err != nil {
		gc.emitError("error.chat_agent_create_failed", map[string]any{"Error": err.Error()})
		s.updateMessageStatus(db, assistantMsg.ID, StatusError, err.Error(), "")
		return
	}

	runner := adk.NewRunner(ctx, adk.RunnerConfig{
		Agent:           agentResult.Agent,
		EnableStreaming:  true,
		CheckPointStore: s.checkpointStore,
	})

	checkpointID := fmt.Sprintf("conv_%d_%s", conversationID, gc.requestID)
	result := s.processStream(ctx, gc, runner, assistantMsg, messages, checkpointID)

	if result.interrupted {
		if existing, ok := s.activeGenerations.Load(conversationID); ok {
			ag := existing.(*activeGeneration)
			ag.mu.Lock()
			ag.runner = runner
			ag.checkpointID = checkpointID
			ag.interrupted = true
			ag.agentCleanup = agentResult.Cleanup
			ag.mu.Unlock()
		}
		return
	}

	agentResult.Cleanup()

	if agentExtras.MemoryEnabled {
		go func() {
			time.Sleep(1 * time.Second)
			memory.RunMemoryExtraction(context.Background(), s.app, conversationID)
		}()
	}
}

// resumeGeneration continues a previously interrupted generation.
func (s *ChatService) resumeGeneration(gen *activeGeneration, conversationID int64, approved bool) {
	db, err := s.db()
	if err != nil {
		s.app.Logger.Error("[chat] resume: failed to get db", "conv", conversationID, "error", err)
		s.cleanupGeneration(gen, conversationID)
		return
	}

	gc := &generationContext{
		service:        s,
		db:             db,
		conversationID: conversationID,
		tabID:          gen.tabID,
		requestID:      gen.requestID,
	}

	assistantMsg := &messageModel{
		ConversationID: conversationID,
		Role:           RoleAssistant,
		Content:        "",
		Status:         StatusStreaming,
		ToolCalls:      "[]",
	}
	dbCtx, dbCancel := context.WithTimeout(context.Background(), 5*time.Second)
	if _, insertErr := db.NewInsert().Model(assistantMsg).Exec(dbCtx); insertErr != nil {
		dbCancel()
		gc.emitError("error.chat_message_save_failed", nil)
		s.cleanupGeneration(gen, conversationID)
		return
	}
	dbCancel()

	gc.emit(EventChatStart, ChatStartEvent{
		ChatEvent: gc.chatEvent(assistantMsg.ID),
		Status:    StatusStreaming,
	})

	// Use a cancellable context derived from gen.cancel so that
	// StopGeneration can abort the resumed run.
	ctx, cancel := context.WithCancel(context.Background())
	oldCancel := gen.cancel
	gen.cancel = func() {
		cancel()
		if oldCancel != nil {
			oldCancel()
		}
	}

	iter, resumeErr := gen.runner.Resume(ctx, gen.checkpointID,
		adk.WithToolOptions([]tool.Option{einoagent.WithInterruptApproval(approved)}))
	if resumeErr != nil {
		s.app.Logger.Error("[chat] resume failed", "conv", conversationID, "error", resumeErr)
		gc.emitError("error.chat_generation_failed", map[string]any{"Error": resumeErr.Error()})
		s.updateMessageStatus(db, assistantMsg.ID, StatusError, resumeErr.Error(), "")
		s.cleanupGeneration(gen, conversationID)
		return
	}

	result := s.processResumeStream(ctx, gc, assistantMsg, iter)

	if result.interrupted {
		gen.mu.Lock()
		gen.interrupted = true
		gen.mu.Unlock()
		return
	}

	s.cleanupGeneration(gen, conversationID)
}

// cleanupGeneration releases agent resources and removes the generation from the map.
func (s *ChatService) cleanupGeneration(gen *activeGeneration, conversationID int64) {
	gen.mu.Lock()
	cleanup := gen.agentCleanup
	gen.agentCleanup = nil
	gen.mu.Unlock()

	if cleanup != nil {
		cleanup()
	}
	s.activeGenerations.Delete(conversationID)
}

// buildExtras creates extra tools and handlers based on agent configuration.
func (s *ChatService) buildExtras(ctx context.Context, gc *generationContext) ([]tool.BaseTool, []adk.ChatModelAgentMiddleware) {
	agentConfig := &gc.agentConfig
	agentExtras := gc.agentExtras
	var extraTools []tool.BaseTool
	var extraHandlers []adk.ChatModelAgentMiddleware

	if len(agentExtras.LibraryIDs) > 0 {
		retrieverTool, toolErr := s.createLibraryRetrieverTool(ctx, gc.db, agentExtras.LibraryIDs, agentConfig.RetrievalTopK, agentExtras.MatchThreshold)
		if toolErr != nil {
			s.app.Logger.Warn("[chat] failed to create library retriever tool", "error", toolErr)
		} else if retrieverTool != nil {
			extraTools = append(extraTools, retrieverTool)
			s.app.Logger.Info("[chat] library retriever tool created", "libraries", len(agentExtras.LibraryIDs), "topK", agentConfig.RetrievalTopK, "threshold", agentExtras.MatchThreshold)
		}
	}

	if agentExtras.MemoryEnabled {
		memoryTool, toolErr := tools.NewMemoryRetrieverTool(ctx, &tools.MemoryRetrieverConfig{
			AgentID:        agentExtras.AgentID,
			TopK:           agentConfig.RetrievalTopK,
			MatchThreshold: agentExtras.MatchThreshold,
		})
		if toolErr != nil {
			s.app.Logger.Warn("[chat] failed to create memory retriever tool", "error", toolErr)
		} else if memoryTool != nil {
			extraTools = append(extraTools, memoryTool)
			s.app.Logger.Info("[chat] memory retriever tool created", "agent_id", agentExtras.AgentID)
		}
	}

	if agentExtras.MemoryEnabled {
		cpCtx, cpCancel := context.WithTimeout(ctx, 2*time.Second)
		coreProfile, _ := memory.GetCoreProfileContent(cpCtx, agentExtras.AgentID)
		cpCancel()
		if coreProfile != "" {
			extraHandlers = append(extraHandlers, einoagent.NewInstructionHandler(
				"\n\n# User Core Profile\nThe following core profile contains long-term facts about the user and this conversation's context. Always respect and utilize this information when formulating your response:\n"+coreProfile,
			))
		}
	}

	if agentConfig.SkillsEnabled {
		skillsSvc := skills.NewSkillsService(s.app)
		skillTools, toolErr := tools.NewSkillManagementTools(&tools.SkillManagementConfig{
			SkillsService: skillsSvc,
		})
		if toolErr != nil {
			s.app.Logger.Warn("[chat] failed to create skill management tools", "error", toolErr)
		} else {
			extraTools = append(extraTools, skillTools...)
			s.app.Logger.Info("[chat] skill management tools added", "count", len(skillTools))
		}
	}

	if s.gateway != nil {
		chID, tgtID, hasChannelSource := s.resolveChannelSource(ctx, gc.db, gc.conversationID)

		feishuCfg := &feishutools.FeishuSenderConfig{Gateway: s.gateway}
		if hasChannelSource {
			feishuCfg.DefaultChannelID = chID
			feishuCfg.DefaultTargetID = tgtID
		}
		feishuTool, toolErr := feishutools.NewFeishuSenderTool(feishuCfg)
		if toolErr != nil {
			s.app.Logger.Warn("[chat] failed to create feishu_sender tool", "error", toolErr)
		} else {
			extraTools = append(extraTools, feishuTool)
			s.app.Logger.Info("[chat] feishu_sender tool added", "default_channel", feishuCfg.DefaultChannelID, "default_target", feishuCfg.DefaultTargetID)
		}

		wecomCfg := &wecomtools.WeComSenderConfig{Gateway: s.gateway}
		if hasChannelSource {
			wecomCfg.DefaultChannelID = chID
			wecomCfg.DefaultTargetID = tgtID
		}
		wecomTool, toolErr := wecomtools.NewWeComSenderTool(wecomCfg)
		if toolErr != nil {
			s.app.Logger.Warn("[chat] failed to create wecom_sender tool", "error", toolErr)
		} else {
			extraTools = append(extraTools, wecomTool)
			s.app.Logger.Info("[chat] wecom_sender tool added", "default_channel", wecomCfg.DefaultChannelID, "default_target", wecomCfg.DefaultTargetID)
		}
	}

	return extraTools, extraHandlers
}

// resolveChannelSource parses the conversation's external_id (format "ch:{channelID}:{targetID}")
// to extract the source channel_id and target_id for auto-filling feishu_sender defaults.
func (s *ChatService) resolveChannelSource(ctx context.Context, db *bun.DB, conversationID int64) (channelID int64, targetID string, ok bool) {
	var externalID string
	err := db.NewSelect().
		Table("conversations").
		Column("external_id").
		Where("id = ?", conversationID).
		Scan(ctx, &externalID)
	if err != nil || externalID == "" {
		return 0, "", false
	}

	// Format: "ch:{channelID}:{targetID}"
	if !strings.HasPrefix(externalID, "ch:") {
		return 0, "", false
	}
	parts := strings.SplitN(externalID, ":", 3)
	if len(parts) != 3 {
		return 0, "", false
	}
	chID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || chID <= 0 {
		return 0, "", false
	}
	tgt := strings.TrimSpace(parts[2])
	if tgt == "" {
		return 0, "", false
	}
	return chID, tgt, true
}

// --- streaming / event processing ---

type segment struct {
	Type           string          `json:"type"`
	Content        string          `json:"content,omitempty"`
	ToolCallIDs    []string        `json:"tool_call_ids,omitempty"`
	RetrievalItems []RetrievalItem `json:"retrieval_items,omitempty"`
}

type streamState struct {
	gc              *generationContext
	assistantMsg    *messageModel
	contentBuilder  strings.Builder
	thinkingBuilder strings.Builder
	toolCallsJSON   []byte
	finishReason    string
	inputTokens     int
	outputTokens    int

	segments               []segment
	lastSegmentType        string
	lastSegmentToolCallIDs map[string]bool

	// Tool call delta tracking
	toolStatesByKey map[string]*toolCallState
	toolOrder       []string
	indexKeyMap     map[int]string

	// Current RunPath from the AgentEvent being processed
	currentRunPath []string

	// Maps sub-agent name → active parent tool_call_id (for routing streaming events)
	activeSubAgentToolCall map[string]string
}

type toolCallState struct {
	id   string
	name string
	args string
}

func newStreamState(gc *generationContext, assistantMsg *messageModel) *streamState {
	return &streamState{
		gc:                     gc,
		assistantMsg:           assistantMsg,
		toolStatesByKey:        make(map[string]*toolCallState),
		toolOrder:              make([]string, 0),
		indexKeyMap:            make(map[int]string),
		activeSubAgentToolCall: make(map[string]string),
	}
}

// parentToolCallID returns the tool_call_id of the parent sub-agent invocation
// for the current run_path, or empty string if not inside a sub-agent.
func (ss *streamState) parentToolCallID() string {
	if len(ss.currentRunPath) < 2 {
		return ""
	}
	agentName := ss.currentRunPath[1]
	return ss.activeSubAgentToolCall[agentName]
}

func runPathToStrings(runPath []adk.RunStep) []string {
	if len(runPath) == 0 {
		return nil
	}
	out := make([]string, len(runPath))
	for i, step := range runPath {
		out[i] = step.String()
	}
	return out
}

func (ss *streamState) addThinkingToSegments(thinking string) {
	if thinking == "" {
		return
	}
	if ss.lastSegmentType == "thinking" && len(ss.segments) > 0 {
		ss.segments[len(ss.segments)-1].Content += thinking
	} else {
		ss.segments = append(ss.segments, segment{Type: "thinking", Content: thinking})
		ss.lastSegmentType = "thinking"
		ss.lastSegmentToolCallIDs = nil
	}
}

func (ss *streamState) addContentToSegments(content string) {
	if content == "" {
		return
	}
	if ss.lastSegmentType == "content" && len(ss.segments) > 0 {
		ss.segments[len(ss.segments)-1].Content += content
	} else {
		ss.segments = append(ss.segments, segment{Type: "content", Content: content})
		ss.lastSegmentType = "content"
		ss.lastSegmentToolCallIDs = nil
	}
}

func (ss *streamState) addToolCallToSegments(toolCallID string) {
	if toolCallID == "" {
		return
	}
	if ss.lastSegmentType != "tools" || len(ss.segments) == 0 {
		ss.segments = append(ss.segments, segment{Type: "tools", ToolCallIDs: []string{toolCallID}})
		ss.lastSegmentType = "tools"
		ss.lastSegmentToolCallIDs = map[string]bool{toolCallID: true}
	} else if !ss.lastSegmentToolCallIDs[toolCallID] {
		ss.segments[len(ss.segments)-1].ToolCallIDs = append(ss.segments[len(ss.segments)-1].ToolCallIDs, toolCallID)
		ss.lastSegmentToolCallIDs[toolCallID] = true
	}
}

func (ss *streamState) addRetrievalToSegments(items []RetrievalItem) {
	if len(items) == 0 {
		return
	}
	ss.segments = append(ss.segments, segment{Type: "retrieval", RetrievalItems: items})
	ss.lastSegmentType = "retrieval"
	ss.lastSegmentToolCallIDs = nil
}

func updateArgs(oldArgs, newArgs string) string {
	if newArgs == "" {
		return oldArgs
	}
	if oldArgs == "" {
		return newArgs
	}
	if strings.HasPrefix(newArgs, oldArgs) {
		return newArgs
	}
	if strings.HasPrefix(oldArgs, newArgs) {
		return oldArgs
	}
	return oldArgs + newArgs
}

func (ss *streamState) buildToolCallsForDB() []schema.ToolCall {
	out := make([]schema.ToolCall, 0, len(ss.toolOrder))
	seen := make(map[string]struct{})
	for _, key := range ss.toolOrder {
		st := ss.toolStatesByKey[key]
		if st == nil || st.id == "" || st.name == "" {
			continue
		}
		if _, ok := seen[st.id]; ok {
			continue
		}
		if isHiddenTool(st.name) {
			continue
		}
		seen[st.id] = struct{}{}
		args := st.args
		if !json.Valid([]byte(args)) {
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

func (ss *streamState) updateToolStates(toolCalls []schema.ToolCall) {
	for i, tc := range toolCalls {
		idx := i
		if tc.Index != nil {
			idx = *tc.Index
		}

		key := tc.ID
		if key == "" {
			if existing, ok := ss.indexKeyMap[idx]; ok {
				key = existing
			} else {
				key = fmt.Sprintf("idx_%d", idx)
			}
		}

		st, ok := ss.toolStatesByKey[key]
		if !ok {
			st = &toolCallState{}
			ss.toolStatesByKey[key] = st
			ss.toolOrder = append(ss.toolOrder, key)
		}
		if tc.ID != "" {
			st.id = tc.ID
			ss.indexKeyMap[idx] = key
		}
		if tc.Function.Name != "" {
			st.name = tc.Function.Name
		}
		st.args = updateArgs(st.args, tc.Function.Arguments)
	}

	if calls := ss.buildToolCallsForDB(); len(calls) > 0 {
		ss.toolCallsJSON, _ = json.Marshal(calls)
	}
}

func (ss *streamState) toolCallsStr() string {
	if len(ss.toolCallsJSON) > 0 {
		return string(ss.toolCallsJSON)
	}
	return "[]"
}

func (ss *streamState) segmentsStr() string {
	if len(ss.segments) > 0 {
		if b, err := json.Marshal(ss.segments); err == nil {
			return string(b)
		}
	}
	return "[]"
}

type processStreamResult struct {
	interrupted bool
}

// processStream runs the ADK runner and processes all streaming events.
func (s *ChatService) processStream(ctx context.Context, gc *generationContext, runner *adk.Runner, assistantMsg *messageModel, messages []*schema.Message, checkpointID string) processStreamResult {
	ss := newStreamState(gc, assistantMsg)

	iter := runner.Run(ctx, messages, adk.WithCheckPointID(checkpointID))
	return s.consumeEventIter(ctx, gc, ss, assistantMsg, iter)
}

// processResumeStream processes events from a resumed runner.
func (s *ChatService) processResumeStream(ctx context.Context, gc *generationContext, assistantMsg *messageModel, iter *adk.AsyncIterator[*adk.AgentEvent]) processStreamResult {
	ss := newStreamState(gc, assistantMsg)
	return s.consumeEventIter(ctx, gc, ss, assistantMsg, iter)
}

// consumeEventIter is the shared event loop for both initial runs and resumed runs.
func (s *ChatService) consumeEventIter(ctx context.Context, gc *generationContext, ss *streamState, assistantMsg *messageModel, iter *adk.AsyncIterator[*adk.AgentEvent]) processStreamResult {
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}

		if ctx.Err() != nil {
			s.updateMessageFinal(gc.db, assistantMsg.ID, ss.contentBuilder.String(), ss.thinkingBuilder.String(), ss.toolCallsStr(), ss.segmentsStr(), StatusCancelled, "", "cancelled", ss.inputTokens, ss.outputTokens)
			gc.emit(EventChatStopped, ChatStoppedEvent{
				ChatEvent: gc.chatEvent(assistantMsg.ID),
				Status:    StatusCancelled,
			})
			return processStreamResult{}
		}

		if event.Err != nil {
			errMsg := event.Err.Error()
			errorKey := "error.chat_generation_failed"
			if strings.Contains(errMsg, "exceeds max iterations") {
				errorKey = "error.max_iterations_exceeded"
			}
			s.app.Logger.Error("[chat] generation failed", "conv", gc.conversationID, "tab", gc.tabID, "req", gc.requestID, "error", event.Err)
			gc.emitError(errorKey, map[string]any{"Error": errMsg})
			s.updateMessageFinal(gc.db, assistantMsg.ID, ss.contentBuilder.String(), ss.thinkingBuilder.String(), ss.toolCallsStr(), ss.segmentsStr(), StatusError, errMsg, "", ss.inputTokens, ss.outputTokens)
			return processStreamResult{}
		}

		if event.Action != nil && event.Action.Interrupted != nil {
			return s.handleInterrupt(ctx, gc, ss, assistantMsg, event)
		}

		if event.Output != nil && event.Output.MessageOutput != nil {
			ss.currentRunPath = runPathToStrings(event.RunPath)
			msgOutput := event.Output.MessageOutput

			if msgOutput.IsStreaming && msgOutput.MessageStream != nil {
				s.processStreamingOutput(ctx, gc, ss, msgOutput)
			} else if msgOutput.Message != nil {
				s.processNonStreamingOutput(gc, ss, msgOutput.Message)
			}
		}
	}

	if ctx.Err() != nil {
		s.updateMessageFinal(gc.db, assistantMsg.ID, ss.contentBuilder.String(), ss.thinkingBuilder.String(), ss.toolCallsStr(), ss.segmentsStr(), StatusCancelled, "", "cancelled", ss.inputTokens, ss.outputTokens)
		gc.emit(EventChatStopped, ChatStoppedEvent{
			ChatEvent: gc.chatEvent(assistantMsg.ID),
			Status:    StatusCancelled,
		})
		return processStreamResult{}
	}

	s.updateMessageFinal(gc.db, assistantMsg.ID, ss.contentBuilder.String(), ss.thinkingBuilder.String(), ss.toolCallsStr(), ss.segmentsStr(), StatusSuccess, "", ss.finishReason, ss.inputTokens, ss.outputTokens)

	gc.emit(EventChatComplete, ChatCompleteEvent{
		ChatEvent:    gc.chatEvent(assistantMsg.ID),
		Status:       StatusSuccess,
		FinishReason: ss.finishReason,
	})
	return processStreamResult{}
}

// handleInterrupt processes an Interrupted event by saving a confirmation
// message and pausing until the user replies.
func (s *ChatService) handleInterrupt(_ context.Context, gc *generationContext, ss *streamState, assistantMsg *messageModel, event *adk.AgentEvent) processStreamResult {
	promptText := einoagent.DefaultInterruptPrompt()
	if cmdInfo := extractInterruptCommand(event); cmdInfo != nil {
		promptText = einoagent.FormatInterruptPrompt(cmdInfo)
	}

	existingContent := ss.contentBuilder.String()
	separator := ""
	if existingContent != "" {
		separator = "\n\n"
	}

	ss.addContentToSegments(promptText)

	s.updateMessageFinal(gc.db, assistantMsg.ID, existingContent+separator+promptText, ss.thinkingBuilder.String(), ss.toolCallsStr(), ss.segmentsStr(), StatusInterrupted, "", "interrupted", ss.inputTokens, ss.outputTokens)

	gc.emit(EventChatChunk, ChatChunkEvent{
		ChatEvent: gc.chatEvent(assistantMsg.ID),
		Delta:     promptText,
	})
	gc.emit(EventChatComplete, ChatCompleteEvent{
		ChatEvent:    gc.chatEvent(assistantMsg.ID),
		Status:       StatusInterrupted,
		FinishReason: "interrupted",
	})

	s.app.Logger.Info("[chat] generation interrupted, waiting for user confirmation", "conv", gc.conversationID)

	return processStreamResult{interrupted: true}
}

// extractInterruptCommand walks the interrupt event data to find the
// InterruptInfo payload (command) from the ToolsNode rerun extra map.
func extractInterruptCommand(event *adk.AgentEvent) *einoagent.InterruptInfo {
	if event.Action == nil || event.Action.Interrupted == nil {
		return nil
	}

	cmInfo, ok := event.Action.Interrupted.Data.(*adk.ChatModelAgentInterruptInfo)
	if !ok || cmInfo == nil || cmInfo.Info == nil {
		return nil
	}

	toolExtra, ok := cmInfo.Info.RerunNodesExtra["ToolNode"]
	if !ok {
		return nil
	}

	rerunExtra, ok := toolExtra.(*compose.ToolsInterruptAndRerunExtra)
	if !ok || rerunExtra == nil {
		return nil
	}

	for _, extra := range rerunExtra.RerunExtraMap {
		if info, ok := extra.(*einoagent.InterruptInfo); ok && info.Command != "" {
			return info
		}
	}
	return nil
}

func (s *ChatService) processStreamingOutput(ctx context.Context, gc *generationContext, ss *streamState, msgOutput *adk.MessageVariant) {
	for {
		msg, err := msgOutput.MessageStream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			s.app.Logger.Error("[chat] stream recv failed", "conv", gc.conversationID, "tab", gc.tabID, "req", gc.requestID, "error", err)
			gc.emitError("error.chat_stream_failed", map[string]any{"Error": err.Error()})
			break
		}

		if msg.ReasoningContent != "" {
			ss.thinkingBuilder.WriteString(msg.ReasoningContent)
			ss.addThinkingToSegments(msg.ReasoningContent)
			gc.emit(EventChatThinking, ChatThinkingEvent{
				ChatEvent:        gc.chatEvent(ss.assistantMsg.ID),
				Delta:            msg.ReasoningContent,
				RunPath:          ss.currentRunPath,
				ParentToolCallID: ss.parentToolCallID(),
			})
		}

		if msg.Content != "" {
			ss.contentBuilder.WriteString(msg.Content)
			ss.addContentToSegments(msg.Content)
			gc.emit(EventChatChunk, ChatChunkEvent{
				ChatEvent:        gc.chatEvent(ss.assistantMsg.ID),
				Delta:            msg.Content,
				RunPath:          ss.currentRunPath,
				ParentToolCallID: ss.parentToolCallID(),
			})
		}

		if len(msg.ToolCalls) > 0 {
			ss.updateToolStates(msg.ToolCalls)
			s.emitToolCallChunks(gc, ss, msg.ToolCalls)
		}

		if msg.ResponseMeta != nil {
			if msg.ResponseMeta.FinishReason != "" {
				ss.finishReason = msg.ResponseMeta.FinishReason
			}
			if msg.ResponseMeta.Usage != nil {
				ss.inputTokens += int(msg.ResponseMeta.Usage.PromptTokens)
				ss.outputTokens += int(msg.ResponseMeta.Usage.CompletionTokens)
			}
		}
	}
}

// isHiddenTool returns true for tools whose call/result events should not be
// sent to the frontend or persisted as tool messages.
func isHiddenTool(name string) bool {
	return name == "confirm_execution"
}

func (s *ChatService) processNonStreamingOutput(gc *generationContext, ss *streamState, msg *schema.Message) {
	if len(msg.ToolCalls) > 0 {
		ss.updateToolStates(msg.ToolCalls)
	}

	if msg.Role == schema.Tool {
		toolName := msg.ToolName
		if toolName == "" {
			toolName = msg.Name
		}
		if toolName == "" && msg.ToolCallID != "" {
			for _, key := range ss.toolOrder {
				st := ss.toolStatesByKey[key]
				if st != nil && st.id == msg.ToolCallID && st.name != "" {
					toolName = st.name
					break
				}
			}
		}

		if isHiddenTool(toolName) {
			return
		}

		// Clear sub-agent tracking when the lead agent receives a tool result
		if len(ss.currentRunPath) <= 1 {
			delete(ss.activeSubAgentToolCall, toolName)
		}

		gc.emit(EventChatTool, ChatToolEvent{
			ChatEvent:        gc.chatEvent(ss.assistantMsg.ID),
			Type:             "result",
			ToolCallID:       msg.ToolCallID,
			ToolName:         toolName,
			ResultJSON:       msg.Content,
			RunPath:          ss.currentRunPath,
			ParentToolCallID: ss.parentToolCallID(),
		})

		toolMsg := &messageModel{
			ConversationID: gc.conversationID,
			Role:           RoleTool,
			Content:        msg.Content,
			Status:         StatusSuccess,
			ToolCallID:     msg.ToolCallID,
			ToolCallName:   toolName,
			ToolCalls:      "[]",
		}
		dbCtx, dbCancel := context.WithTimeout(context.Background(), 5*time.Second)
		if _, err := gc.db.NewInsert().Model(toolMsg).Exec(dbCtx); err != nil {
			s.app.Logger.Warn("[chat] failed to save tool message", "conv", gc.conversationID, "tool", toolName, "call_id", msg.ToolCallID, "error", err)
		}
		dbCancel()
	} else if msg.Content != "" {
		ss.contentBuilder.WriteString(msg.Content)
		ss.addContentToSegments(msg.Content)
		gc.emit(EventChatChunk, ChatChunkEvent{
			ChatEvent:        gc.chatEvent(ss.assistantMsg.ID),
			Delta:            msg.Content,
			RunPath:          ss.currentRunPath,
			ParentToolCallID: ss.parentToolCallID(),
		})
	}

	if msg.ResponseMeta != nil {
		if msg.ResponseMeta.FinishReason != "" {
			ss.finishReason = msg.ResponseMeta.FinishReason
		}
		if msg.ResponseMeta.Usage != nil {
			ss.inputTokens += int(msg.ResponseMeta.Usage.PromptTokens)
			ss.outputTokens += int(msg.ResponseMeta.Usage.CompletionTokens)
		}
	}
}

func (s *ChatService) emitToolCallChunks(gc *generationContext, ss *streamState, toolCalls []schema.ToolCall) {
	for i, tc := range toolCalls {
		idx := i
		if tc.Index != nil {
			idx = *tc.Index
		}

		resolvedID := tc.ID
		if resolvedID == "" {
			if canonicalKey, ok := ss.indexKeyMap[idx]; ok {
				if st := ss.toolStatesByKey[canonicalKey]; st != nil {
					resolvedID = st.id
				}
			}
		}
		if resolvedID == "" {
			continue
		}

		var toolName, args string
		for _, key := range ss.toolOrder {
			st := ss.toolStatesByKey[key]
			if st != nil && st.id == resolvedID {
				toolName = st.name
				args = st.args
				break
			}
		}
		if toolName == "" {
			continue
		}

		if isHiddenTool(toolName) {
			continue
		}

		ss.addToolCallToSegments(resolvedID)
		if args != "" && !json.Valid([]byte(args)) {
			s.app.Logger.Warn("[chat] tool arguments not valid JSON", "conv", gc.conversationID, "tab", gc.tabID, "req", gc.requestID, "tool", toolName, "call_id", resolvedID, "args", args)
		}

		// Track sub-agent tool calls from the lead agent layer so child
		// streaming events can be routed to the correct parent tool_call_id.
		if len(ss.currentRunPath) <= 1 {
			ss.activeSubAgentToolCall[toolName] = resolvedID
		}

		gc.emit(EventChatTool, ChatToolEvent{
			ChatEvent:        gc.chatEvent(ss.assistantMsg.ID),
			Type:             "call",
			ToolCallID:       resolvedID,
			ToolName:         toolName,
			ArgsJSON:         args,
			RunPath:          ss.currentRunPath,
			ParentToolCallID: ss.parentToolCallID(),
		})
	}
}

// hasCapability checks if capabilities include a specific type (e.g., "image")
func hasCapability(capabilities []string, capability string) bool {
	if len(capabilities) == 0 {
		return false
	}
	capabilityLower := strings.ToLower(capability)
	for _, c := range capabilities {
		if strings.ToLower(c) == capabilityLower {
			return true
		}
	}
	return false
}

// getModelCapabilities retrieves model capabilities from database or builtin config
func getModelCapabilities(providerID, modelID string) []string {
	// Try to get from builtin config first
	capabilities := define.GetBuiltinModelCapabilities(providerID, modelID)
	if len(capabilities) > 0 {
		return capabilities
	}

	// Try to get from database
	db := sqlite.DB()
	if db == nil {
		return []string{"text"}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var m struct {
		Capabilities string `bun:"capabilities"`
	}
	err := db.NewSelect().
		Model(&m).
		Table("models").
		Where("provider_id = ?", providerID).
		Where("model_id = ?", modelID).
		Limit(1).
		Scan(ctx)
	if err == nil && m.Capabilities != "" {
		var caps []string
		if json.Unmarshal([]byte(m.Capabilities), &caps); len(caps) > 0 {
			return caps
		}
	}

	// Return default text-only capability
	return []string{"text"}
}

// supportsMultimodal checks if a model supports multimodal (vision) capabilities.
// It first checks the model's Capabilities config, then falls back to legacy detection.
func supportsMultimodal(providerID, modelID string) bool {
	// Check Capabilities config first
	capabilities := getModelCapabilities(providerID, modelID)
	if hasCapability(capabilities, "image") {
		return true
	}
	// If capabilities is set but does NOT include "image", model does not support vision
	if len(capabilities) > 0 && capabilities[0] != "text" {
		return false
	}

	// Fallback to legacy detection for backward compatibility
	modelIDLower := strings.ToLower(modelID)
	providerIDLower := strings.ToLower(providerID)

	// OpenAI models with vision support
	if providerIDLower == "openai" || providerIDLower == "azure" {
		if strings.Contains(modelIDLower, "gpt-4") && !strings.Contains(modelIDLower, "gpt-4o-mini") {
			return true
		}
		if strings.Contains(modelIDLower, "gpt-5") {
			return true
		}
		if strings.Contains(modelIDLower, "vision") {
			return true
		}
	}

	// Anthropic Claude models support vision
	if providerIDLower == "anthropic" {
		if strings.Contains(modelIDLower, "claude") {
			return true
		}
	}

	// Google Gemini models support vision
	if providerIDLower == "google" || providerIDLower == "gemini" {
		return true
	}

	// 通义千问 VL (Vision-Language) models support vision
	if providerIDLower == "qwen" {
		if strings.Contains(modelIDLower, "vl") || strings.Contains(modelIDLower, "vision") {
			return true
		}
		// Note: qwen-plus, qwen-max, qwen-flash, qwen-long are text-only models
		// Only qwen-vl-* models support vision
	}

	// DeepSeek models with vision
	if providerIDLower == "deepseek" {
		if strings.Contains(modelIDLower, "vision") {
			return true
		}
	}

	// Default: assume text-only
	return false
}

// loadMessagesForContext loads messages for agent/chat context.
// contextCount: maximum number of messages to include (0 or >=200 means unlimited).
// providerID and modelID are used to check if the model supports multimodal capabilities.
//
// Tool-call repair (dangling tool calls without responses) is handled by the
// PatchToolCalls middleware at the agent level, so this function only performs
// basic deserialization.
func (s *ChatService) loadMessagesForContext(ctx context.Context, db *bun.DB, conversationID int64, contextCount int, providerID, modelID string) ([]*schema.Message, error) {
	var models []messageModel

	// Check if the model supports multimodal capabilities
	supportsMultimodal := supportsMultimodal(providerID, modelID)

	needLimit := contextCount > 0 && contextCount < 200

	q := db.NewSelect().
		Model(&models).
		Where("conversation_id = ?", conversationID).
		Where("status IN (?)", bun.In([]string{StatusSuccess, StatusCancelled}))

	if needLimit {
		q = q.OrderExpr("created_at DESC, id DESC").Limit(contextCount)
	} else {
		q = q.OrderExpr("created_at ASC, id ASC")
	}

	if err := q.Scan(ctx); err != nil {
		return nil, err
	}

	if needLimit {
		for i, j := 0, len(models)-1; i < j; i, j = i+1, j-1 {
			models[i], models[j] = models[j], models[i]
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
			Role: role,
		}

		// Handle user messages with images (multimodal)
		if m.Role == RoleUser {
			var images []ImagePayload
			if m.ImagesJSON != "" && m.ImagesJSON != "[]" {
				if err := json.Unmarshal([]byte(m.ImagesJSON), &images); err != nil {
					s.app.Logger.Warn("[chat] failed to parse images_json", "msg_id", m.ID, "error", err)
				}
			}

			// Filter out images if model doesn't support multimodal
			if !supportsMultimodal && len(images) > 0 {
				s.app.Logger.Info("[chat] filtering out images - model does not support multimodal", "msg_id", m.ID, "provider", providerID, "model", modelID, "image_count", len(images))
				images = nil
			}

			hasText := strings.TrimSpace(m.Content) != ""
			if !hasText && len(images) == 0 {
				// Skip empty messages
				continue
			}

			// Log whether images are being passed to the model
			if len(images) > 0 {
				s.app.Logger.Info("[chat] passing images to model", "msg_id", m.ID, "image_count", len(images))
			} else {
				s.app.Logger.Info("[chat] no images to pass to model", "msg_id", m.ID)
			}

			// If there are images, use multi-content form
			if len(images) > 0 {
				var parts []schema.MessageInputPart
				if hasText {
					parts = append(parts, schema.MessageInputPart{
						Type: schema.ChatMessagePartTypeText,
						Text: m.Content,
					})
				}

				for _, img := range images {
					if img.Source != "inline_base64" || img.Base64 == "" || img.MimeType == "" {
						continue
					}
					// Use Base64Data and MIMEType instead of URL for data URLs (recommended by Eino docs)
					base64Data := img.Base64
					parts = append(parts, schema.MessageInputPart{
						Type: schema.ChatMessagePartTypeImageURL,
						Image: &schema.MessageInputImage{
							MessagePartCommon: schema.MessagePartCommon{
								Base64Data: &base64Data,
								MIMEType:   img.MimeType,
							},
						},
					})
				}

				if len(parts) > 0 {
					msg.UserInputMultiContent = parts
				} else {
					// Fallback to text-only if no valid parts
					msg.Content = m.Content
				}
			} else {
				// No images, use simple content
				msg.Content = m.Content
			}
		} else {
			// Non-user messages: use simple content
			msg.Content = m.Content
		}

		if m.Role == RoleTool {
			msg.ToolCallID = m.ToolCallID
			msg.Name = m.ToolCallName
		}

		if m.Role == RoleAssistant && m.ToolCalls != "" && m.ToolCalls != "[]" {
			var toolCalls []schema.ToolCall
			if err := json.Unmarshal([]byte(m.ToolCalls), &toolCalls); err == nil {
				msg.ToolCalls = toolCalls
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
		s.app.Logger.Error("[chat] update message status failed", "messageID", messageID, "error", err)
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
		s.app.Logger.Error("[chat] update message final failed", "messageID", messageID, "error", err)
	}
}
