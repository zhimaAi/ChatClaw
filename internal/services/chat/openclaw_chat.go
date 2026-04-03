package chat

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"chatclaw/internal/define"
	"chatclaw/internal/errs"
	"chatclaw/internal/services/channels"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
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
	AgentID         int64
	OpenClawAgentID string
	SessionKey      string
	ProviderID      string
	ModelID         string
	Name            string // Conversation name for OpenClaw session label
	Capabilities    []string
	EnableThinking  bool
	LibraryIDs      []int64
	LibraryNames    map[int64]string
}

type openClawTranscriptMsg struct {
	Role       string   `json:"role"`
	Content    any      `json:"content"`
	ToolUseID  string   `json:"tool_use_id,omitempty"`
	ToolCallID string   `json:"toolCallId,omitempty"`
	ToolName   string   `json:"toolName,omitempty"`
	StopReason string   `json:"stopReason,omitempty"`
	MediaPath  string   `json:"MediaPath,omitempty"`
	MediaPaths []string `json:"MediaPaths,omitempty"`
	MediaType  string   `json:"MediaType,omitempty"`
	MediaTypes []string `json:"MediaTypes,omitempty"`
}

type openClawRawStreamRecord struct {
	Ts          int64  `json:"ts"`
	Event       string `json:"event"`
	RunID       string `json:"runId"`
	SessionID   string `json:"sessionId"`
	EvtType     string `json:"evtType"`
	Delta       string `json:"delta"`
	Content     string `json:"content"`
	RawText     string `json:"rawText"`
	RawThinking string `json:"rawThinking"`
}

const openClawSyntheticResumePrompt = "Continue where you left off. The previous model attempt failed or timed out."

// openClawSessionKey builds the Gateway session key for a conversation.
// The key uses the canonical "agent:<agentId>:<rest>" format so that the
// Gateway correctly associates the session with the specified agent.
// Without this prefix, the Gateway defaults to the "main" agent, which
// causes INVALID_REQUEST errors for any non-default agent.
func openClawSessionKey(agentID string, conversationID int64) string {
	return fmt.Sprintf("agent:%s:conv_%d", agentID, conversationID)
}

func openClawSessionCandidateAgentIDs(openClawAgentID string) []string {
	id := strings.TrimSpace(openClawAgentID)
	if id == "" {
		id = define.OpenClawMainAgentID
	}
	if strings.EqualFold(id, define.OpenClawMainAgentID) {
		return []string{define.OpenClawMainAgentID}
	}
	return []string{id, define.OpenClawMainAgentID}
}

func openClawConversationSourceFromExternalID(externalID string) (channels.ChannelConversationSource, bool) {
	source, ok := channels.ParseChannelConversationExternalID(externalID)
	if !ok || source.ChannelID <= 0 || strings.TrimSpace(source.TargetID) == "" {
		return channels.ChannelConversationSource{}, false
	}
	return source, true
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func normalizeOpenClawMessageText(s string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
}

func stripOpenClawFinalWrapper(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	lower := strings.ToLower(s)
	if !strings.HasPrefix(lower, "<final>") || !strings.HasSuffix(lower, "</final>") {
		return s
	}
	start := strings.Index(lower, "<final>")
	end := strings.LastIndex(lower, "</final>")
	if start == -1 || end == -1 || end <= start+len("<final>") {
		return s
	}
	return strings.TrimSpace(s[start+len("<final>") : end])
}

func isOpenClawSyntheticResumeUserMessage(s string) bool {
	normalized := normalizeOpenClawMessageText(s)
	if normalized == "" {
		return false
	}
	return strings.EqualFold(normalized, normalizeOpenClawMessageText(openClawSyntheticResumePrompt))
}

func sortedKeysFromCounts(counts map[string]int) []string {
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func openClawRawStreamPath() string {
	return strings.TrimSpace(os.Getenv("OPENCLAW_RAW_STREAM_PATH"))
}

func openClawRawStreamCurrentOffset(rawPath string) int64 {
	rawPath = strings.TrimSpace(rawPath)
	if rawPath == "" {
		return -1
	}
	info, err := os.Stat(rawPath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0
		}
		return -1
	}
	return info.Size()
}

func (s *ChatService) logOpenClawRawStreamSummary(conversationID int64, runID string) {
	runID = strings.TrimSpace(runID)
	rawPath := openClawRawStreamPath()
	if runID == "" || rawPath == "" {
		return
	}

	file, err := os.Open(rawPath)
	if err != nil {
		s.app.Logger.Warn("[openclaw-chat] raw stream debug: open failed",
			"conv", conversationID,
			"runId", runID,
			"path", rawPath,
			"error", err)
		return
	}
	defer file.Close()

	const maxBytes = int64(8 << 20)
	if info, statErr := file.Stat(); statErr == nil && info.Size() > maxBytes {
		start := info.Size() - maxBytes
		if _, seekErr := file.Seek(start, io.SeekStart); seekErr == nil {
			if start > 0 {
				discardPartialJSONLLine(file)
			}
		} else {
			_, _ = file.Seek(0, io.SeekStart)
		}
	} else {
		_, _ = file.Seek(0, io.SeekStart)
	}

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	eventCounts := make(map[string]int)
	thinkingEvtTypes := make(map[string]int)
	textEvtTypes := make(map[string]int)
	lastRawThinking := ""
	lastRawText := ""
	matched := 0

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var record openClawRawStreamRecord
		if json.Unmarshal(line, &record) != nil {
			continue
		}
		if strings.TrimSpace(record.RunID) != runID {
			continue
		}
		matched++
		eventCounts[record.Event]++
		switch record.Event {
		case "assistant_thinking_stream":
			thinkingEvtTypes[strings.TrimSpace(record.EvtType)]++
		case "assistant_text_stream":
			textEvtTypes[strings.TrimSpace(record.EvtType)]++
		case "assistant_message_end":
			if strings.TrimSpace(record.RawThinking) != "" {
				lastRawThinking = record.RawThinking
			}
			if strings.TrimSpace(record.RawText) != "" {
				lastRawText = record.RawText
			}
		}
	}

	if err := scanner.Err(); err != nil {
		s.app.Logger.Warn("[openclaw-chat] raw stream debug: scan failed",
			"conv", conversationID,
			"runId", runID,
			"path", rawPath,
			"error", err)
		return
	}

	s.app.Logger.Info("[openclaw-chat] raw stream summary",
		"conv", conversationID,
		"runId", runID,
		"path", rawPath,
		"matchedRecords", matched,
		"events", sortedKeysFromCounts(eventCounts),
		"thinkingEventTypes", sortedKeysFromCounts(thinkingEvtTypes),
		"textEventTypes", sortedKeysFromCounts(textEvtTypes),
		"assistantThinkingStreamCount", eventCounts["assistant_thinking_stream"],
		"assistantTextStreamCount", eventCounts["assistant_text_stream"],
		"assistantMessageEndCount", eventCounts["assistant_message_end"],
		"finalRawThinkingLen", len(lastRawThinking),
		"finalRawTextLen", len(lastRawText),
		"finalRawThinkingPreview", lastRawThinking[:min(120, len(lastRawThinking))])
}

func discardPartialJSONLLine(file *os.File) {
	buf := make([]byte, 1)
	for {
		n, err := file.Read(buf)
		if n > 0 {
			if buf[0] == '\n' {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

func appendOpenClawChannelSessionCandidates(candidates []string, agentIDs []string, platform, scope, targetID string) []string {
	targetID = strings.TrimSpace(targetID)
	scope = strings.TrimSpace(scope)
	if targetID == "" || len(agentIDs) == 0 {
		return candidates
	}

	appendForAgent := func(agentID string, keys ...string) {
		agentID = strings.TrimSpace(agentID)
		if agentID == "" {
			return
		}
		for _, key := range keys {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			candidates = append(candidates, fmt.Sprintf("agent:%s:%s", agentID, key))
		}
	}

	switch strings.TrimSpace(platform) {
	case channels.PlatformWeCom:
		lowerTarget := strings.ToLower(targetID)
		for _, agentID := range agentIDs {
			// Prefer the exact platform-native namespace when we know the chat scope.
			if scope == channels.ChannelConversationScopeGroup || scope == channels.ChannelConversationScopeDM {
				exact := []string{
					fmt.Sprintf("wecom:%s:%s", scope, lowerTarget),
					fmt.Sprintf("wecom:%s:%s", scope, targetID),
				}
				if scope == channels.ChannelConversationScopeDM {
					exact = append([]string{
						fmt.Sprintf("wecom:direct:%s", lowerTarget),
						fmt.Sprintf("wecom:direct:%s", targetID),
					}, exact...)
				}
				appendForAgent(agentID, exact...)
				continue
			}
			// Legacy conversations have no stored scope; keep trying both namespaces.
			appendForAgent(agentID,
				fmt.Sprintf("wecom:group:%s", lowerTarget),
				fmt.Sprintf("wecom:dm:%s", lowerTarget),
				fmt.Sprintf("wecom:direct:%s", lowerTarget),
				fmt.Sprintf("wecom:group:%s", targetID),
				fmt.Sprintf("wecom:dm:%s", targetID),
				fmt.Sprintf("wecom:direct:%s", targetID),
			)
		}
	case channels.PlatformFeishu:
		for _, agentID := range agentIDs {
			switch scope {
			case channels.ChannelConversationScopeDM:
				appendForAgent(agentID,
					fmt.Sprintf("feishu:direct:%s", targetID),
					fmt.Sprintf("feishu:dm:%s", targetID),
					fmt.Sprintf("feishu:chat:%s", targetID),
				)
			case channels.ChannelConversationScopeGroup:
				appendForAgent(agentID,
					fmt.Sprintf("feishu:group:%s", targetID),
					fmt.Sprintf("feishu:chat:%s", targetID),
				)
			default:
				appendForAgent(agentID,
					fmt.Sprintf("feishu:group:%s", targetID),
					fmt.Sprintf("feishu:chat:%s", targetID),
					fmt.Sprintf("feishu:direct:%s", targetID),
					fmt.Sprintf("feishu:dm:%s", targetID),
				)
			}
		}
	case channels.PlatformDingTalk:
		for _, agentID := range agentIDs {
			switch scope {
			case channels.ChannelConversationScopeGroup:
				appendForAgent(agentID,
					fmt.Sprintf("dingtalk:group:%s", targetID),
					fmt.Sprintf("dingtalk-connector:group:%s", targetID),
				)
			case channels.ChannelConversationScopeDM:
				appendForAgent(agentID,
					fmt.Sprintf("dingtalk:dm:%s", targetID),
					fmt.Sprintf("dingtalk:direct:%s", targetID),
					fmt.Sprintf("dingtalk-connector:dm:%s", targetID),
					fmt.Sprintf("dingtalk-connector:direct:%s", targetID),
				)
			default:
				appendForAgent(agentID,
					fmt.Sprintf("dingtalk:group:%s", targetID),
					fmt.Sprintf("dingtalk:dm:%s", targetID),
					fmt.Sprintf("dingtalk-connector:group:%s", targetID),
					fmt.Sprintf("dingtalk-connector:dm:%s", targetID),
				)
			}
		}
	case channels.PlatformWhatsapp:
		for _, agentID := range agentIDs {
			switch scope {
			case channels.ChannelConversationScopeDM:
				appendForAgent(agentID,
					fmt.Sprintf("whatsapp:dm:%s", targetID),
					fmt.Sprintf("whatsapp:direct:%s", targetID),
				)
			case channels.ChannelConversationScopeGroup:
				appendForAgent(agentID,
					fmt.Sprintf("whatsapp:group:%s", targetID),
				)
			default:
				appendForAgent(agentID,
					fmt.Sprintf("whatsapp:group:%s", targetID),
					fmt.Sprintf("whatsapp:dm:%s", targetID),
					fmt.Sprintf("whatsapp:direct:%s", targetID),
				)
			}
		}
	}

	return candidates
}

// resolveOpenClawSessionKeys returns candidate session keys for a conversation.
// Channel-originated OpenClaw conversations may be written into platform-native
// keys (e.g. "agent:main:wecom:group:<id>"), while local runs use "conv_<id>".
func (s *ChatService) resolveOpenClawSessionKeys(conversationID int64, openClawAgentID string) []string {
	agentIDs := openClawSessionCandidateAgentIDs(openClawAgentID)
	id := agentIDs[0]

	candidates := []string{
		fmt.Sprintf("agent:%s:conv_%d", id, conversationID),
	}

	db, err := s.db()
	if err != nil {
		return candidates
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var externalID string
	if err := db.NewSelect().
		Table("conversations").
		Column("external_id").
		Where("id = ?", conversationID).
		Scan(ctx, &externalID); err != nil {
		return candidates
	}

	source, ok := openClawConversationSourceFromExternalID(externalID)
	if !ok {
		return candidates
	}

	var platform string
	if err := db.NewSelect().
		Table("channels").
		Column("platform").
		Where("id = ?", source.ChannelID).
		Scan(ctx, &platform); err != nil {
		return candidates
	}

	candidates = appendOpenClawChannelSessionCandidates(
		candidates,
		agentIDs,
		strings.TrimSpace(platform),
		strings.TrimSpace(source.Scope),
		strings.TrimSpace(source.TargetID),
	)

	return dedupeStrings(candidates)
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
		AgentID            int64  `bun:"agent_id"`
		Name               string `bun:"name"`
		LLMProviderID      string `bun:"llm_provider_id"`
		LLMModelID         string `bun:"llm_model_id"`
		EnableThinking     bool   `bun:"enable_thinking"`
		LibraryIDs         string `bun:"library_ids"`
		OpenClawSessionKey string `bun:"openclaw_session_key"`
	}
	var conv conversationRow
	if err := db.NewSelect().
		Table("conversations").
		Column("agent_id", "name", "llm_provider_id", "llm_model_id", "enable_thinking", "library_ids", "openclaw_session_key").
		Where("id = ?", conversationID).
		Scan(ctx, &conv); err != nil {
		return openClawAgentConfig{}, errs.New("error.chat_conversation_not_found")
	}

	type agentRow struct {
		OpenClawAgentID      string `bun:"openclaw_agent_id"`
		DefaultLLMProviderID string `bun:"default_llm_provider_id"`
		DefaultLLMModelID    string `bun:"default_llm_model_id"`
	}
	var agent agentRow
	if err := db.NewSelect().
		Table("openclaw_agents").
		Column("openclaw_agent_id", "default_llm_provider_id", "default_llm_model_id").
		Where("id = ?", conv.AgentID).
		Scan(ctx, &agent); err != nil {
		return openClawAgentConfig{}, errs.New("error.chat_agent_not_found")
	}

	providerID := strings.TrimSpace(conv.LLMProviderID)
	modelID := strings.TrimSpace(conv.LLMModelID)
	if providerID == "" {
		providerID = strings.TrimSpace(agent.DefaultLLMProviderID)
	}
	if modelID == "" {
		modelID = strings.TrimSpace(agent.DefaultLLMModelID)
	}

	cfg := openClawAgentConfig{
		AgentID:         conv.AgentID,
		Name:            strings.TrimSpace(conv.Name),
		OpenClawAgentID: agent.OpenClawAgentID,
		SessionKey:      strings.TrimSpace(conv.OpenClawSessionKey),
		ProviderID:      providerID,
		ModelID:         modelID,
		EnableThinking:  conv.EnableThinking,
	}
	if providerID != "" && modelID != "" {
		cfg.Capabilities = getModelCapabilities(providerID, modelID)
	}

	if conv.LibraryIDs != "" {
		var ids []int64
		if json.Unmarshal([]byte(conv.LibraryIDs), &ids) == nil && len(ids) > 0 {
			cfg.LibraryIDs = ids

			type libRow struct {
				ID   int64  `bun:"id"`
				Name string `bun:"name"`
			}
			var libs []libRow
			if err := db.NewSelect().
				Table("library").
				Column("id", "name").
				Where("id IN (?)", bun.In(ids)).
				Scan(ctx, &libs); err == nil && len(libs) > 0 {
				cfg.LibraryNames = make(map[int64]string, len(libs))
				for _, lib := range libs {
					cfg.LibraryNames[lib.ID] = lib.Name
				}
			}
		}
	}

	return cfg, nil
}

// resolveOpenClawSessionKey prefers the session key persisted on the conversation record.
// When the stored value is missing, it falls back to the canonical conv_<id> key.
func resolveOpenClawSessionKey(cfg openClawAgentConfig, conversationID int64) string {
	if strings.TrimSpace(cfg.SessionKey) != "" {
		return normalizeOpenClawSessionKeyAgent(strings.TrimSpace(cfg.SessionKey), cfg.OpenClawAgentID, conversationID)
	}
	return openClawSessionKey(cfg.OpenClawAgentID, conversationID)
}

func normalizeOpenClawSessionKeyAgent(sessionKey string, openclawAgentID string, conversationID int64) string {
	sessionKey = strings.TrimSpace(sessionKey)
	openclawAgentID = strings.TrimSpace(openclawAgentID)
	if sessionKey == "" {
		return openClawSessionKey(openclawAgentID, conversationID)
	}
	if openclawAgentID == "" {
		openclawAgentID = define.OpenClawMainAgentID
	}
	if !strings.HasPrefix(sessionKey, "agent:") {
		return openClawSessionKey(openclawAgentID, conversationID)
	}
	parts := strings.SplitN(sessionKey, ":", 3)
	if len(parts) < 3 {
		return openClawSessionKey(openclawAgentID, conversationID)
	}
	return "agent:" + openclawAgentID + ":" + parts[2]
}

func buildOpenClawMessagesFromTranscript(conversationID int64, transcriptMessages []openClawTranscriptMsg) []Message {
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
		Type        string   `json:"type"`
		Content     string   `json:"content,omitempty"`
		ToolCallIDs []string `json:"tool_call_ids,omitempty"`
	}

	type assistantGroup struct {
		segments     []segment
		allToolCalls []map[string]any
		toolResults  []Message
		contentAll   strings.Builder
		thinkingAll  strings.Builder
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

	for _, m := range transcriptMessages {
		if m.Role == "system" {
			continue
		}

		if m.Role == "toolResult" {
			resultContent := extractTextFromContent(m.Content)
			if group == nil {
				group = &assistantGroup{}
			}
			if len(group.pending) > 0 {
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
			} else {
				toolCallID := m.ToolCallID
				if toolCallID == "" {
					toolCallID = m.ToolUseID
				}
				toolName := m.ToolName
				if toolCallID == "" {
					toolCallID = fmt.Sprintf("tool_%d", -msgIDCounter)
				}
				group.allToolCalls = append(group.allToolCalls, map[string]any{
					"id":       toolCallID,
					"type":     "function",
					"function": map[string]any{"name": toolName, "arguments": "{}"},
				})
				appendToolCallSeg(group, toolCallID)
				msgIDCounter--
				group.toolResults = append(group.toolResults, Message{
					ID:             msgIDCounter,
					ConversationID: conversationID,
					Role:           "tool",
					Content:        resultContent,
					ToolCallID:     toolCallID,
					ToolCallName:   toolName,
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
					s = stripOpenClawFinalWrapper(s)
					if s == "" {
						continue
					}
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
						text = stripOpenClawFinalWrapper(text)
						if text == "" {
							continue
						}
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

		flushGroup()

		msgIDCounter--
		contentStr := extractTextFromContent(m.Content)
		attachments := buildOpenClawTranscriptAttachments(
			m.Content,
			contentStr,
			m.MediaPath,
			m.MediaPaths,
			m.MediaType,
			m.MediaTypes,
		)
		if m.Role == "user" {
			contentStr = cleanOpenClawUserMessage(contentStr)
			if len(attachments) == 0 && (contentStr == "" || isOpenClawSyntheticResumeUserMessage(contentStr)) {
				continue
			}
		} else if m.Role == "assistant" {
			contentStr = stripOpenClawFinalWrapper(contentStr)
		}

		msg := Message{
			ID:             msgIDCounter,
			ConversationID: conversationID,
			Role:           m.Role,
			Content:        contentStr,
			Status:         StatusSuccess,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		if len(attachments) > 0 {
			if b, err := json.Marshal(attachments); err == nil {
				msg.ImagesJSON = string(b)
			}
		}
		messages = append(messages, msg)
	}
	flushGroup()

	return messages
}

// buildKnowledgeContextMessage wraps the user's message with a system instruction
// about knowledge-base usage. When libraries are selected it tells the agent
// which IDs to search; when none are selected it tells the agent NOT to use
// the search_knowledge tool.
func buildKnowledgeContextMessage(userContent string, libraryIDs []int64, libraryNames map[int64]string) string {
	if len(libraryIDs) == 0 {
		return userContent + "\n\n<chatclaw_context hidden=\"true\">\n" +
			"No knowledge bases are selected for this conversation. " +
			"Do NOT use the search_knowledge or list_libraries tools.\n" +
			"</chatclaw_context>"
	}

	var sb strings.Builder
	sb.WriteString(userContent)
	sb.WriteString("\n\n<chatclaw_context hidden=\"true\">\n")
	sb.WriteString("The user has selected the following knowledge bases for this conversation. ")
	sb.WriteString("When answering, use the search_knowledge tool with the library_ids parameter ")
	sb.WriteString("set to these IDs to search ONLY the selected knowledge bases:\n")
	for _, id := range libraryIDs {
		name := libraryNames[id]
		if name == "" {
			name = "unknown"
		}
		sb.WriteString(fmt.Sprintf("- library_id: %d, name: %q\n", id, name))
	}
	sb.WriteString("</chatclaw_context>")
	return sb.String()
}

// openClawSessionKeyForAgent builds the Gateway session key for a conversation.
// OpenClaw expects agent-scoped keys like "agent:<agentId>:..." (see session docs).
// A bare "conv_<id>" is interpreted as the default agent ("main"), which breaks
// runs when agentId is not "main". The previous "agentId:conv_<id>" form was
// also rejected by the gateway; the canonical prefix is "agent:".
func openClawSessionKeyForAgent(agentID string, conversationID int64) string {
	id := strings.TrimSpace(agentID)
	if id == "" {
		id = define.OpenClawMainAgentID
	}
	return fmt.Sprintf("agent:%s:conv_%d", id, conversationID)
}

func openClawSessionKeyMatches(got, want string) bool {
	got = strings.ToLower(strings.TrimSpace(got))
	want = strings.ToLower(strings.TrimSpace(want))
	if got == "" || want == "" {
		return true
	}
	if got == want {
		return true
	}
	// Some gateway payloads may include namespaced forms; keep backward compatibility.
	return strings.HasSuffix(got, ":"+want)
}

// GetOpenClawLastAssistantReply fetches the last assistant message text from
// the OpenClaw Gateway session. Returns empty string if unavailable.
func (s *ChatService) GetOpenClawLastAssistantReply(conversationID int64) string {
	if s.openclawGateway == nil || !s.openclawGateway.IsReady() {
		return ""
	}

	cfg, err := s.getOpenClawAgentConfig(conversationID)
	if err != nil {
		return ""
	}
	sessionKeys := dedupeStrings(append(
		[]string{strings.TrimSpace(cfg.SessionKey)},
		s.resolveOpenClawSessionKeys(conversationID, cfg.OpenClawAgentID)...,
	))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	type sessionResult struct {
		Key      string
		Messages []struct {
			Role    string `json:"role"`
			Content any    `json:"content"`
		} `json:"messages"`
	}
	var picked sessionResult
	for _, key := range sessionKeys {
		var current sessionResult
		current.Key = key
		if err := s.openclawGateway.QueryRequest(ctx, "sessions.get", map[string]any{
			"key":   key,
			"limit": 10,
		}, &current); err != nil {
			continue
		}
		if len(current.Messages) == 0 && len(picked.Messages) > 0 {
			continue
		}
		if len(current.Messages) >= len(picked.Messages) {
			picked = current
		}
	}
	if len(picked.Messages) == 0 {
		return ""
	}

	s.app.Logger.Info("[openclaw-chat] GetOpenClawLastAssistantReply: sessions.get result",
		"conv", conversationID, "session_key", picked.Key, "message_count", len(picked.Messages))

	// Walk backwards to find the last assistant message with text content
	for i := len(picked.Messages) - 1; i >= 0; i-- {
		msg := picked.Messages[i]
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

func extractChatClawHiddenBlock(input, tag string) (inner string, cleaned string, found bool) {
	start := strings.Index(input, "<"+tag)
	if start == -1 {
		return "", input, false
	}

	openEndRel := strings.Index(input[start:], ">")
	if openEndRel == -1 {
		return "", input, false
	}
	openEnd := start + openEndRel

	closeTag := "</" + tag + ">"
	closeStartRel := strings.Index(input[openEnd+1:], closeTag)
	if closeStartRel == -1 {
		return "", input, false
	}
	closeStart := openEnd + 1 + closeStartRel
	closeEnd := closeStart + len(closeTag)

	inner = input[openEnd+1 : closeStart]
	cleaned = strings.TrimSpace(input[:start] + input[closeEnd:])
	return inner, cleaned, true
}

func stripChatClawHiddenBlocks(input string, tags ...string) string {
	out := input
	for _, tag := range tags {
		for {
			_, cleaned, found := extractChatClawHiddenBlock(out, tag)
			if !found {
				break
			}
			out = cleaned
		}
	}
	return out
}

func buildOpenClawAttachmentContextMessage(userContent string, attachments []ImagePayload) string {
	type attachmentContext struct {
		Instruction string         `json:"instruction"`
		Files       []ImagePayload `json:"files"`
	}

	var files []ImagePayload
	for _, att := range attachments {
		if att.Kind != "file" || strings.TrimSpace(att.FilePath) == "" {
			continue
		}
		files = append(files, ImagePayload{
			ID:           att.ID,
			Kind:         "file",
			Source:       "local_file",
			MimeType:     att.MimeType,
			FileName:     att.FileName,
			FilePath:     att.FilePath,
			Size:         att.Size,
			OriginalName: att.OriginalName,
		})
	}
	if len(files) == 0 {
		return userContent
	}

	payload, err := json.Marshal(attachmentContext{
		Instruction: "The user attached files that are already saved on local disk. Use the read/search tools with the exact file_path values below when you need to inspect them.",
		Files:       files,
	})
	if err != nil {
		return userContent
	}

	block := "<chatclaw_attachments hidden=\"true\">\n" + string(payload) + "\n</chatclaw_attachments>"
	if strings.TrimSpace(userContent) == "" {
		return block
	}
	return userContent + "\n\n" + block
}

func extractOpenClawAttachmentContext(content string) ([]ImagePayload, string) {
	inner, cleaned, found := extractChatClawHiddenBlock(content, "chatclaw_attachments")
	if !found {
		return nil, content
	}

	start := strings.Index(inner, "{")
	end := strings.LastIndex(inner, "}")
	if start == -1 || end < start {
		return nil, cleaned
	}

	var payload struct {
		Files []ImagePayload `json:"files"`
	}
	if err := json.Unmarshal([]byte(inner[start:end+1]), &payload); err != nil {
		return nil, cleaned
	}
	return payload.Files, cleaned
}

func guessOpenClawAttachmentMime(filePath, fallback string) string {
	mimeType := strings.TrimSpace(strings.ToLower(fallback))
	if mimeType != "" {
		return mimeType
	}
	switch strings.ToLower(filepath.Ext(filePath)) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".bmp":
		return "image/bmp"
	case ".svg":
		return "image/svg+xml"
	case ".pdf":
		return "application/pdf"
	case ".md":
		return "text/markdown"
	case ".csv":
		return "text/csv"
	case ".json":
		return "application/json"
	case ".xml":
		return "application/xml"
	case ".html":
		return "text/html"
	case ".txt", ".log":
		return "text/plain"
	default:
		return ""
	}
}

func loadOpenClawTranscriptImagePayload(filePath, mimeType string) *ImagePayload {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil
	}
	resolvedMime := guessOpenClawAttachmentMime(filePath, mimeType)
	if resolvedMime == "" || !strings.HasPrefix(resolvedMime, "image/") {
		return nil
	}
	encoded := base64.StdEncoding.EncodeToString(data)
	return &ImagePayload{
		Kind:     "image",
		Source:   "local_file",
		MimeType: resolvedMime,
		Base64:   encoded,
		DataURL:  "data:" + resolvedMime + ";base64," + encoded,
		FilePath: filePath,
		FileName: filepath.Base(filePath),
		Size:     int64(len(data)),
	}
}

func firstNonEmptyOpenClawString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func openClawStringValue(v any) string {
	s, _ := v.(string)
	return s
}

func extractOpenClawTranscriptContentAttachments(content any) []ImagePayload {
	blocks, ok := content.([]any)
	if !ok {
		return nil
	}

	var attachments []ImagePayload
	for _, block := range blocks {
		bm, ok := block.(map[string]any)
		if !ok {
			continue
		}

		blockType := strings.ToLower(strings.TrimSpace(openClawStringValue(bm["type"])))
		if blockType != "image" {
			continue
		}

		base64Data := strings.TrimSpace(firstNonEmptyOpenClawString(
			openClawStringValue(bm["data"]),
			openClawStringValue(bm["base64"]),
			openClawStringValue(bm["content"]),
		))
		mimeType := guessOpenClawAttachmentMime(
			firstNonEmptyOpenClawString(
				openClawStringValue(bm["filePath"]),
				openClawStringValue(bm["file_path"]),
				openClawStringValue(bm["path"]),
				openClawStringValue(bm["fileName"]),
				openClawStringValue(bm["file_name"]),
				openClawStringValue(bm["name"]),
			),
			firstNonEmptyOpenClawString(
				openClawStringValue(bm["mimeType"]),
				openClawStringValue(bm["mime_type"]),
				openClawStringValue(bm["mediaType"]),
				openClawStringValue(bm["media_type"]),
			),
		)
		if base64Data == "" || !strings.HasPrefix(strings.ToLower(mimeType), "image/") {
			continue
		}

		fileName := firstNonEmptyOpenClawString(
			openClawStringValue(bm["fileName"]),
			openClawStringValue(bm["file_name"]),
			openClawStringValue(bm["name"]),
		)
		attachments = append(attachments, ImagePayload{
			Kind:         "image",
			Source:       "inline_base64",
			MimeType:     mimeType,
			Base64:       base64Data,
			DataURL:      "data:" + mimeType + ";base64," + base64Data,
			FileName:     fileName,
			OriginalName: fileName,
		})
	}

	return attachments
}

func buildOpenClawTranscriptAttachments(
	contentBlocks any,
	content string,
	mediaPath string,
	mediaPaths []string,
	mediaType string,
	mediaTypes []string,
) []ImagePayload {
	filesFromContext, _ := extractOpenClawAttachmentContext(content)
	var attachments []ImagePayload
	if inlineAttachments := extractOpenClawTranscriptContentAttachments(contentBlocks); len(inlineAttachments) > 0 {
		attachments = append(attachments, inlineAttachments...)
	}
	if len(filesFromContext) > 0 {
		attachments = append(attachments, filesFromContext...)
	}

	paths := make([]string, 0, 1+len(mediaPaths))
	if strings.TrimSpace(mediaPath) != "" {
		paths = append(paths, strings.TrimSpace(mediaPath))
	}
	for _, p := range mediaPaths {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			paths = append(paths, trimmed)
		}
	}

	if len(paths) == 0 {
		return attachments
	}

	var resolvedTypes []string
	if len(mediaTypes) > 0 {
		resolvedTypes = mediaTypes
	} else if strings.TrimSpace(mediaType) != "" {
		resolvedTypes = []string{mediaType}
	}

	for i, p := range paths {
		currentType := ""
		if i < len(resolvedTypes) {
			currentType = resolvedTypes[i]
		} else if len(resolvedTypes) == 1 {
			currentType = resolvedTypes[0]
		}

		if img := loadOpenClawTranscriptImagePayload(p, currentType); img != nil {
			attachments = append(attachments, *img)
			continue
		}

		fileName := filepath.Base(p)
		attachments = append(attachments, ImagePayload{
			Kind:         "file",
			Source:       "local_file",
			MimeType:     guessOpenClawAttachmentMime(p, currentType),
			FilePath:     p,
			FileName:     fileName,
			OriginalName: fileName,
		})
	}

	return attachments
}

// OpenClaw channel plugins (e.g. WeCom) prefix user text with fenced JSON metadata blocks.
var openClawUntrustedMetadataMarkers = []string{
	"Conversation info (untrusted metadata)",
	"Sender (untrusted metadata)",
}

var openClawChannelMessageIDLineRE = regexp.MustCompile(`^\[message_id:\s*[^\]]+\]$`)

// skipOpenClawLeadingCodeFence consumes a markdown ``` fenced block at the start of s.
func skipOpenClawLeadingCodeFence(s string) (rest string, ok bool) {
	s = strings.TrimLeft(s, " \t\n\r")
	if !strings.HasPrefix(s, "```") {
		return "", false
	}
	afterTicks := s[3:]
	nl := strings.Index(afterTicks, "\n")
	if nl < 0 {
		return "", false
	}
	body := afterTicks[nl+1:]
	closeNL := strings.Index(body, "\n```")
	if closeNL >= 0 {
		tail := body[closeNL+len("\n```"):]
		return strings.TrimLeft(tail, " \t\n\r"), true
	}
	closeCRNL := strings.Index(body, "\r\n```")
	if closeCRNL >= 0 {
		tail := body[closeCRNL+len("\r\n```"):]
		return strings.TrimLeft(tail, " \t\n\r"), true
	}
	// Closing fence without a preceding newline (still valid markdown in practice)
	if idx := strings.Index(body, "```"); idx >= 0 {
		tail := strings.TrimLeft(body[idx+3:], " \t\n\r")
		return tail, true
	}
	return "", false
}

func stripOpenClawUntrustedMetadataPrefixes(s string) string {
	s = strings.TrimLeft(s, " \t\n\r")
	for {
		stripped := false
		for _, marker := range openClawUntrustedMetadataMarkers {
			if !strings.HasPrefix(s, marker) {
				continue
			}
			after := strings.TrimLeft(s[len(marker):], " \t")
			if strings.HasPrefix(after, ":") {
				after = strings.TrimLeft(after[1:], " \t\n\r")
			} else {
				after = strings.TrimLeft(after, " \t\n\r")
			}
			rest, ok := skipOpenClawLeadingCodeFence(after)
			if !ok {
				continue
			}
			s = strings.TrimLeft(rest, " \t\n\r")
			stripped = true
			break
		}
		if !stripped {
			break
		}
	}
	return s
}

// CleanOpenClawChannelUserMessage removes OpenClaw-injected channel metadata fences,
// gateway timestamp prefixes, and hidden ChatClaw blocks so UI and previews show plain user text.
func CleanOpenClawChannelUserMessage(s string) string {
	return cleanOpenClawUserMessage(s)
}

// cleanOpenClawUserMessage strips "(untrusted metadata)" fenced blocks from channel plugins,
// the "[Day YYYY-MM-DD HH:MM TZ] " timestamp prefix that the Gateway may prepend, and
// hidden <chatclaw_context> / <chatclaw_attachments> blocks.
func cleanOpenClawUserMessage(s string) string {
	s = stripOpenClawUntrustedMetadataPrefixes(s)

	// Strip "[Day YYYY-MM-DD HH:MM TZ] " timestamp prefix
	if strings.HasPrefix(s, "[") {
		if idx := strings.Index(s, "] "); idx != -1 && idx < 60 {
			s = strings.TrimLeft(s[idx+2:], " \t\n")
		}
	}

	s = stripChatClawHiddenBlocks(s, "chatclaw_context", "chatclaw_attachments")
	return stripOpenClawChannelEnvelopeLines(s)
}

func stripOpenClawChannelEnvelopeLines(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}

	if idx := strings.Index(s, "\n[System:"); idx >= 0 {
		s = strings.TrimSpace(s[:idx])
	} else if strings.HasPrefix(s, "[System:") {
		return ""
	}

	lines := strings.Split(s, "\n")
	start := 0
	for start < len(lines) {
		line := strings.TrimSpace(lines[start])
		if line == "" || openClawChannelMessageIDLineRE.MatchString(line) {
			start++
			continue
		}
		break
	}

	return strings.TrimSpace(strings.Join(lines[start:], "\n"))
}

func (s *ChatService) buildOpenClawRPCAttachments(attachments []ImagePayload) []map[string]any {
	if len(attachments) == 0 {
		return nil
	}

	rpcAttachments := make([]map[string]any, 0, len(attachments))
	for _, att := range attachments {
		if att.Kind == "file" {
			continue
		}

		mimeType := guessOpenClawAttachmentMime(att.FilePath, att.MimeType)
		if !strings.HasPrefix(mimeType, "image/") {
			continue
		}

		base64Data := strings.TrimSpace(att.Base64)
		if base64Data == "" && att.FilePath != "" {
			data, err := os.ReadFile(att.FilePath)
			if err != nil {
				s.app.Logger.Warn("[openclaw-chat] failed to read image attachment",
					"path", att.FilePath, "error", err)
				continue
			}
			base64Data = base64.StdEncoding.EncodeToString(data)
		}
		if base64Data == "" {
			continue
		}

		fileName := strings.TrimSpace(att.FileName)
		if fileName == "" && att.FilePath != "" {
			fileName = filepath.Base(att.FilePath)
		}

		rpcAttachments = append(rpcAttachments, map[string]any{
			"type":     "image",
			"mimeType": mimeType,
			"fileName": fileName,
			"content":  base64Data,
		})
	}

	return rpcAttachments
}

func hasOpenClawImageAttachment(attachments []ImagePayload) bool {
	for _, att := range attachments {
		if att.Kind != "" && att.Kind != "image" {
			continue
		}
		mimeType := strings.ToLower(strings.TrimSpace(att.MimeType))
		if strings.HasPrefix(mimeType, "image/") {
			return true
		}
		if att.FilePath != "" {
			resolved := strings.ToLower(guessOpenClawAttachmentMime(att.FilePath, att.MimeType))
			if strings.HasPrefix(resolved, "image/") {
				return true
			}
		}
	}
	return false
}

// GetOpenClawMessages fetches conversation history from the OpenClaw Gateway
// via the official chat.history RPC method.
func (s *ChatService) GetOpenClawMessages(conversationID int64) ([]Message, error) {
	if conversationID <= 0 {
		return nil, errs.New("error.chat_conversation_id_required")
	}

	cfg, err := s.getOpenClawAgentConfig(conversationID)
	if err != nil {
		return nil, err
	}
	storedRaw := strings.TrimSpace(cfg.SessionKey)
	primary := resolveOpenClawSessionKey(cfg, conversationID)
	candidates := dedupeStrings(append(
		[]string{storedRaw, primary},
		s.resolveOpenClawSessionKeys(conversationID, cfg.OpenClawAgentID)...,
	))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var result struct {
		Messages []openClawTranscriptMsg `json:"messages"`
	}
	chosenSessionKey := ""
	for _, key := range candidates {
		if strings.TrimSpace(key) == "" {
			continue
		}
		var current struct {
			Messages []openClawTranscriptMsg `json:"messages"`
		}
		if err := s.openclawGateway.QueryRequest(ctx, "chat.history", map[string]any{
			"sessionKey": key,
			"limit":      200,
		}, &current); err != nil {
			continue
		}
		if len(current.Messages) == 0 {
			continue
		}
		result = current
		chosenSessionKey = key
		break
	}
	if len(result.Messages) == 0 {
		s.app.Logger.Warn("[openclaw-chat] chat.history empty for all candidate keys",
			"conv", conversationID, "keys", candidates)
		return nil, nil
	}
	s.app.Logger.Info("[openclaw-chat] chat.history picked session key",
		"conv", conversationID, "session_key", chosenSessionKey, "message_count", len(result.Messages))
	messages := buildOpenClawMessagesFromTranscript(conversationID, result.Messages)
	assistantCount := 0
	assistantThinkingCount := 0
	for _, msg := range messages {
		if msg.Role != "assistant" {
			continue
		}
		assistantCount++
		if strings.TrimSpace(msg.ThinkingContent) != "" {
			assistantThinkingCount++
		}
	}
	s.app.Logger.Info("[openclaw-chat] chat.history transcript summary",
		"conv", conversationID,
		"assistantMessages", assistantCount,
		"assistantMessagesWithThinking", assistantThinkingCount)

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

	attachments := input.Images
	if len(attachments) > 0 && hasOpenClawImageAttachment(attachments) &&
		agentConfig.ProviderID != "" && agentConfig.ModelID != "" &&
		!hasCapability(agentConfig.Capabilities, "image") {
		return nil, errs.Newf("error.chat_model_not_support_image", map[string]any{
			"ProviderID": agentConfig.ProviderID,
			"ModelID":    agentConfig.ModelID,
		})
	}
	if len(attachments) > 0 {
		db, dbErr := s.db()
		if dbErr != nil {
			return nil, dbErr
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		updated, saveErr := s.saveImagesToWorkDir(ctx, db, agentConfig.AgentID, input.ConversationID, attachments)
		cancel()
		if saveErr != nil {
			s.app.Logger.Warn("[openclaw-chat] failed to save attachments to workdir, using original payloads",
				"conv", input.ConversationID, "error", saveErr)
		} else {
			attachments = updated
		}
	}

	s.app.Logger.Info("[openclaw-chat] SendOpenClawMessage",
		"conv", input.ConversationID, "tab", input.TabID,
		"content_len", len(content), "attachments", len(attachments))

	requestID := uuid.New().String()
	s.emitOpenClawUserMessage(input.ConversationID, input.TabID, requestID, content, attachments)
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
		s.runOpenClawChatRun(genCtx, input.ConversationID, input.TabID, requestID, content, attachments, agentConfig)
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
	if content == "" && len(input.Images) == 0 {
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

	attachments := input.Images
	if len(attachments) > 0 && hasOpenClawImageAttachment(attachments) &&
		agentConfig.ProviderID != "" && agentConfig.ModelID != "" &&
		!hasCapability(agentConfig.Capabilities, "image") {
		return nil, errs.Newf("error.chat_model_not_support_image", map[string]any{
			"ProviderID": agentConfig.ProviderID,
			"ModelID":    agentConfig.ModelID,
		})
	}
	if len(attachments) > 0 {
		db, dbErr := s.db()
		if dbErr != nil {
			return nil, dbErr
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		updated, saveErr := s.saveImagesToWorkDir(ctx, db, agentConfig.AgentID, input.ConversationID, attachments)
		cancel()
		if saveErr != nil {
			s.app.Logger.Warn("[openclaw-chat] failed to save edit attachments to workdir, using original payloads",
				"conv", input.ConversationID, "error", saveErr)
		} else {
			attachments = updated
		}
	}

	requestID := uuid.New().String()
	s.emitOpenClawUserMessage(input.ConversationID, input.TabID, requestID, content, attachments)
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
		s.runOpenClawChatRun(genCtx, input.ConversationID, input.TabID, requestID, content, attachments, agentConfig)
	}()

	return &SendMessageResult{RequestID: requestID, MessageID: input.MessageID}, nil
}

// emitOpenClawUserMessage mirrors the standard chat pipeline by sending a local
// user-message event before the OpenClaw gateway starts streaming.
func (s *ChatService) emitOpenClawUserMessage(conversationID int64, tabID, requestID, content string, attachments []ImagePayload) {
	imagesJSON := "[]"
	if len(attachments) > 0 {
		if data, err := json.Marshal(attachments); err == nil {
			imagesJSON = string(data)
		}
	}

	// Use a synthetic negative message id so the frontend can render the user
	// bubble immediately without requiring a local DB insert for OpenClaw chats.
	messageID := -conversationID*1000000 - time.Now().UnixMilli()%1000000
	s.app.Event.Emit(EventChatUserMessage, ChatUserMessageEvent{
		ChatEvent: ChatEvent{
			ConversationID: conversationID,
			TabID:          tabID,
			RequestID:      requestID,
			Seq:            0,
			MessageID:      messageID,
			Ts:             time.Now().UnixMilli(),
		},
		Content:    content,
		ImagesJSON: imagesJSON,
	})
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
	emittedThinking  string
	emittedContent   string
	seenToolCalls    map[string]bool
	seenToolResults  map[string]bool
	thinkingMu       sync.Mutex
	rawBridgeMu      sync.RWMutex
	rawThinkingFlush func()
}

func (st *openClawChatRunState) nextSeq() int {
	return int(atomic.AddInt32(&st.seq, 1))
}

func (st *openClawChatRunState) applyThinkingDelta(delta, text string) (resolvedDelta string, next string) {
	st.thinkingMu.Lock()
	defer st.thinkingMu.Unlock()

	current := st.emittedThinking
	resolvedDelta, next = normalizeOpenClawThinkingDelta(current, delta, text)
	if resolvedDelta == "" {
		return "", current
	}

	st.thinkingBuilder.Reset()
	st.thinkingBuilder.WriteString(next)
	st.emittedThinking = next
	return resolvedDelta, next
}

func (st *openClawChatRunState) currentThinkingLen() int {
	st.thinkingMu.Lock()
	defer st.thinkingMu.Unlock()
	return len(st.emittedThinking)
}

func (st *openClawChatRunState) setRawThinkingFlush(fn func()) {
	st.rawBridgeMu.Lock()
	defer st.rawBridgeMu.Unlock()
	st.rawThinkingFlush = fn
}

func (st *openClawChatRunState) flushRawThinking() {
	st.rawBridgeMu.RLock()
	fn := st.rawThinkingFlush
	st.rawBridgeMu.RUnlock()
	if fn != nil {
		fn()
	}
}

func normalizeOpenClawReasoningText(text string) string {
	if text == "" {
		return ""
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.TrimPrefix(text, "Reasoning:\n")
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if len(line) >= 2 && strings.HasPrefix(line, "_") && strings.HasSuffix(line, "_") {
			lines[i] = line[1 : len(line)-1]
		}
	}
	return strings.Join(lines, "\n")
}

// normalizeOpenClawThinkingDelta prefers the cumulative "text" payload when it
// cleanly extends the current emitted thinking. This makes the ChatClaw bridge
// tolerant of OpenClaw builds that incorrectly send delta=text for every
// thinking event after the first one.
func normalizeOpenClawThinkingDelta(current, delta, text string) (resolvedDelta string, next string) {
	if normalizedText := normalizeOpenClawReasoningText(text); normalizedText != "" {
		if strings.HasPrefix(normalizedText, current) {
			suffix := normalizedText[len(current):]
			return suffix, normalizedText
		}
		if current == "" {
			return normalizedText, normalizedText
		}
	}
	if normalizedDelta := normalizeOpenClawReasoningText(delta); normalizedDelta != "" {
		delta = normalizedDelta
	}
	if delta != "" && strings.HasPrefix(delta, current) {
		suffix := delta[len(current):]
		return suffix, current + suffix
	}
	if delta == "" {
		return "", current
	}
	return delta, current + delta
}

func openClawRawThinkingText(record openClawRawStreamRecord) (delta string, text string, ok bool) {
	switch strings.TrimSpace(record.Event) {
	case "assistant_thinking_stream":
		if strings.TrimSpace(record.Delta) == "" && strings.TrimSpace(record.Content) == "" {
			return "", "", false
		}
		return record.Delta, record.Content, true
	case "assistant_message_end":
		if strings.TrimSpace(record.RawThinking) == "" {
			return "", "", false
		}
		return "", record.RawThinking, true
	default:
		return "", "", false
	}
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

func (s *ChatService) bridgeOpenClawRawThinkingRecord(
	conversationID int64,
	record openClawRawStreamRecord,
	st *openClawChatRunState,
	ce func() ChatEvent,
	emit func(string, any),
) bool {
	rawDelta, rawText, ok := openClawRawThinkingText(record)
	if !ok {
		return false
	}

	resolvedDelta, nextThinking := st.applyThinkingDelta(rawDelta, rawText)
	if resolvedDelta == "" {
		return false
	}

	s.app.Logger.Info("[openclaw-chat] raw stream thinking delta bridged",
		"conv", conversationID,
		"runId", strings.TrimSpace(record.RunID),
		"event", strings.TrimSpace(record.Event),
		"evtType", strings.TrimSpace(record.EvtType),
		"deltaLen", len(resolvedDelta),
		"thinkingLen", len(nextThinking),
		"deltaPreview", resolvedDelta[:min(100, len(resolvedDelta))])

	emit(EventChatThinking, ChatThinkingEvent{
		ChatEvent: ce(),
		Delta:     resolvedDelta,
	})
	return true
}

func (s *ChatService) bridgeOpenClawRawThinkingStream(
	ctx context.Context,
	conversationID int64,
	rawPath string,
	startOffset int64,
	activeRunID *atomic.Value,
	st *openClawChatRunState,
	done <-chan struct{},
	ce func() ChatEvent,
	emit func(string, any),
) {
	rawPath = strings.TrimSpace(rawPath)
	if rawPath == "" || startOffset < 0 {
		return
	}

	s.app.Logger.Info("[openclaw-chat] raw thinking bridge enabled",
		"conv", conversationID,
		"path", rawPath,
		"startOffset", startOffset)

	go func() {
		var drainMu sync.Mutex
		offset := startOffset
		pending := make([]openClawRawStreamRecord, 0, 64)

		processRecord := func(record openClawRawStreamRecord) {
			s.bridgeOpenClawRawThinkingRecord(conversationID, record, st, ce, emit)
		}

		flushPending := func(runID string) {
			runID = strings.TrimSpace(runID)
			if runID == "" || len(pending) == 0 {
				return
			}
			for _, record := range pending {
				if strings.TrimSpace(record.RunID) == runID {
					processRecord(record)
				}
			}
			pending = pending[:0]
		}

		drain := func() {
			info, err := os.Stat(rawPath)
			if err != nil {
				return
			}
			if info.Size() < offset {
				offset = info.Size()
			}
			if info.Size() == offset {
				flushPending(strings.TrimSpace(func() string {
					runID, _ := activeRunID.Load().(string)
					return runID
				}()))
				return
			}

			file, err := os.Open(rawPath)
			if err != nil {
				return
			}
			defer file.Close()

			if _, err := file.Seek(offset, io.SeekStart); err != nil {
				return
			}

			reader := bufio.NewReader(file)
			for {
				line, err := reader.ReadBytes('\n')
				if len(line) == 0 && err != nil {
					break
				}
				if len(line) == 0 {
					continue
				}
				if line[len(line)-1] != '\n' {
					break
				}
				offset += int64(len(line))

				trimmed := strings.TrimSpace(string(line))
				if trimmed == "" {
					if err != nil {
						break
					}
					continue
				}

				var record openClawRawStreamRecord
				if json.Unmarshal([]byte(trimmed), &record) != nil {
					if err != nil {
						break
					}
					continue
				}

				runID, _ := activeRunID.Load().(string)
				runID = strings.TrimSpace(runID)
				if runID == "" {
					pending = append(pending, record)
					if len(pending) > 512 {
						pending = pending[len(pending)-512:]
					}
				} else if strings.TrimSpace(record.RunID) == runID {
					processRecord(record)
				}

				if err != nil {
					break
				}
			}

			runID, _ := activeRunID.Load().(string)
			flushPending(runID)
		}

		lockedDrain := func() {
			drainMu.Lock()
			defer drainMu.Unlock()
			drain()
		}

		st.setRawThinkingFlush(lockedDrain)
		defer st.setRawThinkingFlush(nil)

		lockedDrain()
		ticker := time.NewTicker(40 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				lockedDrain()
				return
			case <-done:
				lockedDrain()
				return
			case <-ticker.C:
				lockedDrain()
			}
		}
	}()
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

	// Filter by session first to avoid cross-session stream contamination.
	if !openClawSessionKeyMatches(frame.SessionKey, sessionKey) {
		s.app.Logger.Debug("[openclaw-chat] agent event: session mismatch, skipping",
			"conv", conversationID, "got", frame.SessionKey, "want", sessionKey)
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

	s.app.Logger.Info("[openclaw-chat] agent event",
		"conv", conversationID, "stream", frame.Stream,
		"dataLen", len(frame.Data),
		"rawData", string(frame.Data))

	switch frame.Stream {
	case "assistant":
		var d struct {
			Delta string `json:"delta"`
		}
		if json.Unmarshal(frame.Data, &d) != nil || d.Delta == "" {
			return
		}

		// Raw thinking is bridged from a file-backed stream, so it can lag slightly
		// behind the websocket assistant chunks. Flush pending reasoning first so
		// the UI keeps the final thinking tail ahead of the answer body.
		st.flushRawThinking()

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
			Text  string `json:"text"`
		}
		if json.Unmarshal(frame.Data, &d) != nil {
			return
		}
		resolvedDelta, _ := st.applyThinkingDelta(d.Delta, d.Text)
		if resolvedDelta == "" {
			return
		}
		s.app.Logger.Info("[openclaw-chat] >>> THINKING stream event received",
			"conv", conversationID,
			"deltaLen", len(resolvedDelta),
			"rawDeltaLen", len(d.Delta),
			"rawTextLen", len(d.Text),
			"usedTextPrefixFix", d.Text != "" && resolvedDelta != normalizeOpenClawReasoningText(d.Delta),
			"deltaPreview", resolvedDelta[:min(100, len(resolvedDelta))])
		emit(EventChatThinking, ChatThinkingEvent{
			ChatEvent: ce(),
			Delta:     resolvedDelta,
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
			st.flushRawThinking()
			st.finishReason = "stop"
			select {
			case <-done:
			default:
				close(done)
			}
		case "error":
			st.flushRawThinking()
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

	s.app.Logger.Debug("[openclaw-chat] chat event",
		"conv", conversationID,
		"state", chatEvt.State,
		"role", chatEvt.Message.Role,
		"contentLen", len(chatEvt.Message.Content))

	// Filter by session
	if !openClawSessionKeyMatches(chatEvt.SessionKey, sessionKey) {
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
			thinkingBlocks := 0
			textBlocks := 0
			toolCallBlocks := 0
			toolResultBlocks := 0
			thinkingLen := 0
			textLen := 0
			for _, b := range blocks {
				switch b.Type {
				case "thinking":
					thinkingBlocks++
					thinkingLen += len(b.Thinking)
				case "text":
					textBlocks++
					textLen += len(b.Text)
				case "toolCall":
					toolCallBlocks++
				case "toolResult":
					toolResultBlocks++
				}
			}
			s.app.Logger.Info("[openclaw-chat] chat event content blocks",
				"conv", conversationID,
				"state", chatEvt.State,
				"blockCount", len(blocks),
				"types", blockTypes,
				"thinkingBlocks", thinkingBlocks,
				"thinkingLen", thinkingLen,
				"textBlocks", textBlocks,
				"textLen", textLen,
				"toolCallBlocks", toolCallBlocks,
				"toolResultBlocks", toolResultBlocks)
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
	if newThinking != "" {
		s.app.Logger.Info("[openclaw-chat] >>> THINKING from chat event content blocks",
			"conv", conversationID,
			"newThinkingLen", len(newThinking),
			"emittedThinkingLen", st.currentThinkingLen(),
			"thinkingPreview", newThinking[:min(100, len(newThinking))])
	}
	if delta, _ := st.applyThinkingDelta("", newThinking); delta != "" {
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
func (s *ChatService) runOpenClawChatRun(ctx context.Context, conversationID int64, tabID, requestID, userContent string, attachments []ImagePayload, cfg openClawAgentConfig) {
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

	sessionKey := resolveOpenClawSessionKey(cfg, conversationID)
	idempotencyKey := requestID
	listenerKey := fmt.Sprintf("openclaw-chat-%d-%s", conversationID, requestID)
	rawPath := openClawRawStreamPath()
	rawStartOffset := openClawRawStreamCurrentOffset(rawPath)

	s.app.Logger.Info("[openclaw-chat] runOpenClawChatRun config",
		"conv", conversationID,
		"sessionKey", sessionKey,
		"agentId", cfg.OpenClawAgentID,
		"provider", cfg.ProviderID,
		"model", cfg.ModelID,
		"caps", cfg.Capabilities,
		"enableThinking", cfg.EnableThinking)

	// OpenClaw session-store mutations may wait on a file lock for up to ~10s.
	// Use the dedicated query connection and a slightly longer timeout so we do
	// not self-cancel before the gateway's normal lock window expires.
	patchCtx, cancel := context.WithTimeout(ctx, 15*time.Second)

	// Build session patch params: label (conversation name) and model for OpenClaw console display.
	patchParams := map[string]any{
		"key":            sessionKey,
		"reasoningLevel": "stream",
	}
	// Sync conversation name to OpenClaw session label for console UI.
	if cfg.Name != "" {
		patchParams["label"] = cfg.Name
	}
	// Sync model config to OpenClaw session to ensure correct model is used.
	// Format: provider/model (e.g., "openai/gpt-4o").
	if cfg.ProviderID != "" && cfg.ModelID != "" {
		patchParams["model"] = cfg.ProviderID + "/" + cfg.ModelID
	}

	var patchResp struct {
		Key   string `json:"key"`
		Entry *struct {
			ReasoningLevel string `json:"reasoningLevel"`
			SessionID      string `json:"sessionId"`
		} `json:"entry"`
	}
	if err := s.openclawGateway.QueryRequest(patchCtx, "sessions.patch", patchParams, &patchResp); err != nil {
		s.app.Logger.Warn("[openclaw-chat] failed to patch session with label and model",
			"conv", conversationID,
			"sessionKey", sessionKey,
			"label", cfg.Name,
			"model", cfg.ProviderID+"/"+cfg.ModelID,
			"enableThinking", cfg.EnableThinking,
			"error", err)
	} else {
		s.app.Logger.Info("[openclaw-chat] session patched with label and model",
			"conv", conversationID,
			"sessionKey", sessionKey,
			"resolvedKey", strings.TrimSpace(patchResp.Key),
			"label", cfg.Name,
			"model", cfg.ProviderID+"/"+cfg.ModelID,
			"resolvedReasoningLevel", strings.TrimSpace(func() string {
				if patchResp.Entry == nil {
					return ""
				}
				return patchResp.Entry.ReasoningLevel
			}()),
			"resolvedSessionID", strings.TrimSpace(func() string {
				if patchResp.Entry == nil {
					return ""
				}
				return patchResp.Entry.SessionID
			}()),
			"enableThinking", cfg.EnableThinking)
	}
	cancel()

	done := make(chan struct{})
	var activeRunID atomic.Value

	s.bridgeOpenClawRawThinkingStream(ctx, conversationID, rawPath, rawStartOffset, &activeRunID, st, done, ce, emit)

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

	// Inject knowledge-base context: tell the agent which libraries to use,
	// or explicitly instruct it not to use knowledge search when none are selected.
	messageToSend := buildKnowledgeContextMessage(
		buildOpenClawAttachmentContextMessage(userContent, attachments),
		cfg.LibraryIDs,
		cfg.LibraryNames,
	)
	rpcAttachments := s.buildOpenClawRPCAttachments(attachments)

	// Use the "agent" RPC (blocking: returns when the run completes).
	// While it blocks, chat/agent events arrive via readLoop for real-time streaming.
	params := map[string]any{
		"message":        messageToSend,
		"sessionKey":     sessionKey,
		"idempotencyKey": idempotencyKey,
		"agentId":        cfg.OpenClawAgentID,
	}
	if cfg.ProviderID != "" {
		params["provider"] = cfg.ProviderID
	}
	if cfg.ModelID != "" {
		params["model"] = cfg.ModelID
	}
	if len(rpcAttachments) > 0 {
		params["attachments"] = rpcAttachments
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

	s.app.Logger.Info("[openclaw-chat] agent RPC returned",
		"conv", conversationID, "runId", runResult.RunID)

	// Wait for the run to finish via lifecycle "end" event or context cancel.
	// The agent RPC may return before all streaming events are delivered
	// (e.g., the Gateway sends an early ack with runId). Wait for the
	// actual completion signal from lifecycle "end" / chat "final" events
	// so we don't tear down the event listener prematurely.
	select {
	case <-done:
		s.app.Logger.Info("[openclaw-chat] run finished via lifecycle event",
			"conv", conversationID)
	case <-ctx.Done():
		rid, _ := activeRunID.Load().(string)
		if rid != "" {
			abortCtx, abortCancel := context.WithTimeout(context.Background(), 3*time.Second)
			_ = s.openclawGateway.Request(abortCtx, "chat.abort", map[string]any{
				"sessionKey": sessionKey,
			}, nil)
			abortCancel()
		}
		emit(EventChatStopped, ChatStoppedEvent{
			ChatEvent: ce(),
			Status:    StatusCancelled,
		})
		return
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
	rid, _ := activeRunID.Load().(string)
	if rid == "" {
		rid = runResult.RunID
	}
	s.logOpenClawRawStreamSummary(conversationID, rid)
	emit(EventChatComplete, ChatCompleteEvent{
		ChatEvent:    ce(),
		Status:       StatusSuccess,
		FinishReason: st.finishReason,
	})
}
