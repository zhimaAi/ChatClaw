package chat

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"chatclaw/internal/errs"

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
	ProviderID      string
	ModelID         string
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

// openClawSessionKey builds the Gateway session key for a conversation.
// The key uses the canonical "agent:<agentId>:<rest>" format so that the
// Gateway correctly associates the session with the specified agent.
// Without this prefix, the Gateway defaults to the "main" agent, which
// causes INVALID_REQUEST errors for any non-default agent.
func openClawSessionKey(agentID string, conversationID int64) string {
	return fmt.Sprintf("agent:%s:conv_%d", agentID, conversationID)
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
		AgentID        int64  `bun:"agent_id"`
		LLMProviderID  string `bun:"llm_provider_id"`
		LLMModelID     string `bun:"llm_model_id"`
		EnableThinking bool   `bun:"enable_thinking"`
		LibraryIDs     string `bun:"library_ids"`
	}
	var conv conversationRow
	if err := db.NewSelect().
		Table("conversations").
		Column("agent_id", "llm_provider_id", "llm_model_id", "enable_thinking", "library_ids").
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
		OpenClawAgentID: agent.OpenClawAgentID,
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

// cleanOpenClawUserMessage strips the "Sender (untrusted metadata)" block,
// the "[Day YYYY-MM-DD HH:MM TZ] " timestamp prefix that the Gateway
// automatically prepends to user messages sent via chat.send, and
// the <chatclaw_context> block injected by buildKnowledgeContextMessage.
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

	return stripChatClawHiddenBlocks(s, "chatclaw_context", "chatclaw_attachments")
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
		return nil, nil
	}

	sessionKey := openClawSessionKey(cfg.OpenClawAgentID, conversationID)
	if s.openclawGateway == nil || !s.openclawGateway.IsReady() {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var result struct {
		Messages []openClawTranscriptMsg `json:"messages"`
	}
	if err := s.openclawGateway.QueryRequest(ctx, "chat.history", map[string]any{
		"sessionKey": sessionKey,
		"limit":      200,
	}, &result); err != nil {
		s.app.Logger.Warn("[openclaw-chat] chat.history failed",
			"conv", conversationID, "err", err)
		return nil, nil
	}

	return buildOpenClawMessagesFromTranscript(conversationID, result.Messages), nil
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

// openClawChatRunState tracks the streaming state for a single chat.send invocation.
type openClawChatRunState struct {
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

		st.contentBuilder.WriteString(d.Delta)
		st.emittedContent = st.contentBuilder.String()
		s.appendGenerationContent(conversationID, "", d.Delta)
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
		s.app.Logger.Info("[openclaw-chat] >>> THINKING stream event received",
			"conv", conversationID, "deltaLen", len(d.Delta),
			"deltaPreview", d.Delta[:min(100, len(d.Delta))])
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

	s.app.Logger.Debug("[openclaw-chat] chat event",
		"conv", conversationID,
		"state", chatEvt.State,
		"role", chatEvt.Message.Role,
		"contentLen", len(chatEvt.Message.Content))

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
	if newThinking != "" {
		s.app.Logger.Info("[openclaw-chat] >>> THINKING from chat event content blocks",
			"conv", conversationID,
			"newThinkingLen", len(newThinking),
			"emittedThinkingLen", len(st.emittedThinking),
			"thinkingPreview", newThinking[:min(100, len(newThinking))])
	}
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
		s.appendGenerationContent(conversationID, "", delta)
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
	st := &openClawChatRunState{}

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

	sessionKey := openClawSessionKey(cfg.OpenClawAgentID, conversationID)
	idempotencyKey := requestID
	listenerKey := fmt.Sprintf("openclaw-chat-%d-%s", conversationID, requestID)

	s.app.Logger.Info("[openclaw-chat] runOpenClawChatRun config",
		"conv", conversationID,
		"sessionKey", sessionKey,
		"agentId", cfg.OpenClawAgentID,
		"provider", cfg.ProviderID,
		"model", cfg.ModelID,
		"caps", cfg.Capabilities,
		"enableThinking", cfg.EnableThinking)

	done := make(chan struct{})
	var activeRunID atomic.Value

	// Listen for all gateway events and route accordingly.
	s.openclawGateway.AddEventListener(listenerKey, func(event string, payload json.RawMessage) {
		s.app.Logger.Debug("[openclaw-chat] event received",
			"conv", conversationID, "event", event,
			"payloadLen", len(payload))

		if event == "chat" {
			s.handleOpenClawChatEvent(conversationID, sessionKey, &activeRunID, st, done, ce, emit, emitError, payload)
			return
		}
		// Handle "agent" events for streaming text, tools, and lifecycle.
		if event == "agent" {
			s.handleOpenClawAgentEvent(conversationID, sessionKey, &activeRunID, st, done, ce, emit, emitError, payload)
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
