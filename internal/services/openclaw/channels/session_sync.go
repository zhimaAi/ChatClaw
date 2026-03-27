package openclawchannels

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"chatclaw/internal/define"
	"chatclaw/internal/errs"
	"chatclaw/internal/services/channels"
	"chatclaw/internal/services/chat"
	"chatclaw/internal/services/conversations"
	"chatclaw/internal/sqlite"
)

type openClawSessionStoreEntry struct {
	UpdatedAt   int64  `json:"updatedAt"`
	ChatType    string `json:"chatType"`
	SessionFile string `json:"sessionFile"`
	Origin      struct {
		Label string `json:"label"`
	} `json:"origin"`
}

type openClawSessionHistoryLine struct {
	Type    string `json:"type"`
	Message struct {
		Role    string `json:"role"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"message"`
}

type syncedChannelTarget struct {
	Platform string
	Channel  channels.Channel
}

// SyncAgentConversations mirrors plugin-managed OpenClaw channel sessions into ChatClaw conversations.
func (s *OpenClawChannelService) SyncAgentConversations(agentID int64) error {
	if agentID <= 0 {
		return errs.New("error.agent_id_required")
	}
	if s.convSvc == nil {
		return nil
	}

	agent, err := s.agentsSvc.GetAgent(agentID)
	if err != nil {
		return err
	}

	channelTargets, err := s.listSyncTargets(agentID)
	if err != nil {
		return err
	}
	if len(channelTargets) == 0 {
		return nil
	}

	store, err := s.loadAgentSessionStore(strings.TrimSpace(agent.OpenClawAgentID))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return errs.Wrap("error.channel_list_failed", err)
	}

	for sessionKey, entry := range store {
		source, ok := parseOpenClawPluginSessionKey(strings.TrimSpace(agent.OpenClawAgentID), sessionKey)
		if !ok {
			continue
		}
		target, ok := channelTargets[source.Platform]
		if !ok {
			continue
		}
		if err := s.syncSessionConversation(agentID, target.Channel, source.Scope, source.TargetID, entry); err != nil {
			return errs.Wrap("error.channel_list_failed", err)
		}
	}
	return nil
}

func (s *OpenClawChannelService) listSyncTargets(agentID int64) (map[string]syncedChannelTarget, error) {
	db, err := s.db()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var models []channelModel
	if err := db.NewSelect().
		Model(&models).
		Where(openClawChannelVisibilitySQL).
		Where("ch.agent_id = ?", agentID).
		Where("ch.enabled = ?", true).
		OrderExpr("ch.id DESC").
		Scan(ctx); err != nil {
		return nil, err
	}

	out := make(map[string]syncedChannelTarget)
	for i := range models {
		ch := models[i].toDTO()
		platform := strings.TrimSpace(ch.Platform)
		if platform == "" {
			continue
		}
		if _, exists := out[platform]; exists {
			continue
		}
		out[platform] = syncedChannelTarget{
			Platform: platform,
			Channel:  ch,
		}
	}
	return out, nil
}

func (s *OpenClawChannelService) loadAgentSessionStore(openclawAgentID string) (map[string]openClawSessionStoreEntry, error) {
	root, err := define.OpenClawDataRootDir()
	if err != nil {
		return nil, err
	}
	storePath := filepath.Join(root, "agents", openclawAgentID, "sessions", "sessions.json")
	raw, err := os.ReadFile(storePath)
	if err != nil {
		return nil, err
	}

	store := make(map[string]openClawSessionStoreEntry)
	if len(raw) == 0 {
		return store, nil
	}
	if err := json.Unmarshal(raw, &store); err != nil {
		return nil, fmt.Errorf("parse session store: %w", err)
	}
	return store, nil
}

func (s *OpenClawChannelService) syncSessionConversation(
	agentID int64,
	ch channels.Channel,
	scope string,
	targetID string,
	entry openClawSessionStoreEntry,
) error {
	externalID := channels.BuildChannelConversationExternalID(ch.ID, scope, targetID)
	if externalID == "" {
		return nil
	}

	name := buildSyncedConversationName(entry, scope, targetID)
	convID, err := s.convSvc.FindOrCreateByExternalID(agentID, externalID, name, conversations.AgentTypeOpenClaw)
	if err != nil {
		return err
	}

	lastMessage := strings.TrimSpace(extractLastSessionMessage(entry.SessionFile))
	updateAt := sqlite.NowUTC()
	if entry.UpdatedAt > 0 {
		updateAt = time.UnixMilli(entry.UpdatedAt).UTC().Format(sqlite.DateTimeFormat)
	}

	db, err := s.db()
	if err != nil {
		return err
	}
	q := db.NewUpdate().
		Table("conversations").
		Set("updated_at = ?", updateAt).
		Where("id = ?", convID)
	if name != "" {
		q = q.Set("name = ?", name)
	}
	if lastMessage != "" {
		q = q.Set("last_message = ?", lastMessage)
	}
	if _, err := q.Exec(context.Background()); err != nil {
		return fmt.Errorf("update synced conversation: %w", err)
	}
	return nil
}

type openClawPluginSessionSource struct {
	Platform string
	Scope    string
	TargetID string
}

func parseOpenClawPluginSessionKey(agentID string, sessionKey string) (openClawPluginSessionSource, bool) {
	sessionKey = strings.TrimSpace(sessionKey)
	prefix := "agent:" + strings.TrimSpace(agentID) + ":"
	if !strings.HasPrefix(sessionKey, prefix) {
		return openClawPluginSessionSource{}, false
	}

	parts := strings.Split(sessionKey, ":")
	if len(parts) < 5 {
		return openClawPluginSessionSource{}, false
	}
	platform := strings.TrimSpace(parts[2])
	scope := strings.TrimSpace(parts[3])
	targetID := strings.TrimSpace(strings.Join(parts[4:], ":"))
	if platform == "" || targetID == "" {
		return openClawPluginSessionSource{}, false
	}

	switch platform {
	case channels.PlatformWeCom:
		scope = normalizePluginConversationScope(scope)
	default:
		return openClawPluginSessionSource{}, false
	}
	if scope == "" {
		return openClawPluginSessionSource{}, false
	}

	return openClawPluginSessionSource{
		Platform: platform,
		Scope:    scope,
		TargetID: targetID,
	}, true
}

func normalizePluginConversationScope(scope string) string {
	switch strings.TrimSpace(strings.ToLower(scope)) {
	case "dm", "direct":
		return channels.ChannelConversationScopeDM
	case "group":
		return channels.ChannelConversationScopeGroup
	default:
		return ""
	}
}

func buildSyncedConversationName(entry openClawSessionStoreEntry, scope string, targetID string) string {
	name := strings.TrimSpace(entry.Origin.Label)
	if name != "" {
		return name
	}
	if scope == channels.ChannelConversationScopeGroup {
		if t := extractFirstSessionUserMessageText(entry.SessionFile); t != "" {
			return channels.ConversationTitleFromFirstMessage(t)
		}
		return "group:" + channels.NormalizeChannelConversationTargetID(targetID)
	}
	return "user:" + channels.NormalizeChannelConversationTargetID(targetID)
}

func extractLastSessionMessage(sessionFile string) string {
	sessionFile = strings.TrimSpace(sessionFile)
	if sessionFile == "" {
		return ""
	}

	file, err := os.Open(sessionFile)
	if err != nil {
		return ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	last := ""
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var item openClawSessionHistoryLine
		if err := json.Unmarshal([]byte(line), &item); err != nil {
			continue
		}
		if item.Type != "message" {
			continue
		}
		switch strings.TrimSpace(item.Message.Role) {
		case "user", "assistant":
		default:
			continue
		}
		text := extractSessionLineText(item)
		if text != "" {
			last = text
		}
	}

	return last
}

func extractFirstSessionUserMessageText(sessionFile string) string {
	sessionFile = strings.TrimSpace(sessionFile)
	if sessionFile == "" {
		return ""
	}

	file, err := os.Open(sessionFile)
	if err != nil {
		return ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var item openClawSessionHistoryLine
		if err := json.Unmarshal([]byte(line), &item); err != nil {
			continue
		}
		if item.Type != "message" {
			continue
		}
		if strings.TrimSpace(item.Message.Role) != "user" {
			continue
		}
		text := extractSessionLineText(item)
		if text != "" {
			return text
		}
	}

	return ""
}

func extractSessionLineText(item openClawSessionHistoryLine) string {
	for i := len(item.Message.Content) - 1; i >= 0; i-- {
		part := item.Message.Content[i]
		if strings.TrimSpace(part.Type) != "text" {
			continue
		}
		text := strings.TrimSpace(part.Text)
		if text == "" {
			continue
		}
		text = chat.CleanOpenClawChannelUserMessage(text)
		text = stripWeComSpeakerPrefix(text)
		if text != "" {
			return text
		}
	}
	return ""
}

func stripWeComSpeakerPrefix(text string) string {
	text = strings.TrimSpace(text)
	if text == "" || !strings.HasPrefix(text, "[") {
		return text
	}
	closeIdx := strings.Index(text, "]")
	if closeIdx < 0 || closeIdx+1 >= len(text) {
		return text
	}
	rest := strings.TrimSpace(text[closeIdx+1:])
	if idx := strings.Index(rest, "："); idx >= 0 && idx+len("：") < len(rest) {
		return strings.TrimSpace(rest[idx+len("："):])
	}
	return text
}
