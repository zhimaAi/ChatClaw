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

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	"github.com/uptrace/bun"
)

type openClawSessionStoreEntry struct {
	UpdatedAt   int64  `json:"updatedAt"`
	ChatType    string `json:"chatType"`
	SessionFile string `json:"sessionFile"`
	DeliveryCtx struct {
		AccountID string `json:"accountId"`
	} `json:"deliveryContext"`
	Origin struct {
		Label     string `json:"label"`
		AccountID string `json:"accountId"`
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
	Platform  string
	AccountID string
	Channel   channels.Channel
}

type syncedSessionCandidate struct {
	source        openClawPluginSessionSource
	entry         openClawSessionStoreEntry
	rawSessionKey string
}

func (s *OpenClawChannelService) updateChannelLastReplyTargetBySessionKey(localAgentID int64, sessionKey string) error {
	sessionKey = strings.TrimSpace(sessionKey)
	if localAgentID <= 0 || sessionKey == "" {
		return nil
	}

	_, platform, ok := parseOpenClawPluginSessionKeyPrefix(sessionKey)
	if !ok || strings.TrimSpace(platform) == "" {
		return nil
	}
	// 统一平台别名，确保 dingtalk-connector / qqbot 也能命中本地渠道和 scope 规则。
	// Normalize platform aliases so dingtalk-connector / qqbot can match local channels and scope rules.
	platform = canonicalOpenClawLastReplyTargetPlatform(platform)

	parts := strings.Split(sessionKey, ":")
	if len(parts) < 5 {
		return nil
	}

	rawScope := strings.TrimSpace(parts[3])
	targetID := strings.TrimSpace(strings.Join(parts[4:], ":"))
	if targetID == "" {
		return nil
	}

	scope := normalizePluginConversationScope(platform, rawScope, targetID, "")
	if scope == "" {
		return nil
	}

	db, err := s.db()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var rows []channelModel
	if err := db.NewSelect().
		Model(&rows).
		Where(openClawChannelVisibilitySQL).
		Where("ch.agent_id = ?", localAgentID).
		Where("ch.enabled = ?", true).
		Where("LOWER(TRIM(ch.platform)) = LOWER(TRIM(?))", platform).
		OrderExpr("ch.id DESC").
		Scan(ctx); err != nil {
		return err
	}

	for i := range rows {
		if err := s.updateSyncedChannelLastReplyTarget(rows[i].ID, scope, targetID); err != nil {
			return err
		}
	}
	return nil
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

	selected := make(map[string]syncedSessionCandidate)

	for _, sourceAgentID := range collectSyncSourceAgentIDs(strings.TrimSpace(agent.OpenClawAgentID)) {
		store, err := s.loadAgentSessionStore(sourceAgentID)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return errs.Wrap("error.channel_list_failed", err)
		}

		for sessionKey, entry := range store {
			source, ok := parseOpenClawPluginSessionKey(sourceAgentID, sessionKey, entry)
			if !ok {
				continue
			}
			target, ok := channelTargets[buildSyncedChannelTargetKey(source.Platform, source.AccountID)]
			if !ok {
				continue
			}

			candidateKey := buildSyncedSessionCandidateKey(target.Channel.ID, source.Scope, source.TargetID)
			next := syncedSessionCandidate{source: source, entry: entry, rawSessionKey: sessionKey}
			current, exists := selected[candidateKey]
			if !exists || shouldReplaceSyncedSessionCandidate(current, next) {
				selected[candidateKey] = next
			}
		}
	}

	for _, candidate := range selected {
		target := channelTargets[buildSyncedChannelTargetKey(candidate.source.Platform, candidate.source.AccountID)]
		if err := s.syncSessionConversation(agentID, target.Channel, candidate.source.Scope, candidate.source.TargetID, candidate.entry, candidate.rawSessionKey); err != nil {
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
		accountID := openClawSyncAccountID(ch)
		key := buildSyncedChannelTargetKey(platform, accountID)
		if _, exists := out[key]; exists {
			continue
		}
		out[key] = syncedChannelTarget{
			Platform:  platform,
			AccountID: accountID,
			Channel:   ch,
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

// openClawSyncAccountID matches OpenClaw session store deliveryContext.accountId for plugin channels.
func openClawSyncAccountID(ch channels.Channel) string {
	switch strings.TrimSpace(ch.Platform) {
	case channels.PlatformDingTalk:
		return openClawChannelAccountKey(ch.ID, ch.ExtraConfig)
	case channels.PlatformWechat:
		if id := extractWechatAccountID(ch.ExtraConfig); id != "" {
			return id
		}
		return openClawWechatAccountID(ch.ID)
	default:
		return openClawManagedAccountID(ch.Platform, ch.ID, ch.ExtraConfig)
	}
}

// openClawQQSessionSyncLegacyExternalIDs lists older ChatClaw QQ conversation keys so session
// sync merges with rows created before QQ used scoped external_ids (ch:{id}:dm:{target}).
func openClawQQSessionSyncLegacyExternalIDs(platform string, channelID int64, scope, targetID string) []string {
	if strings.TrimSpace(platform) != channels.PlatformQQ {
		return nil
	}
	targetID = channels.NormalizeChannelConversationTargetID(targetID)
	if channelID <= 0 || targetID == "" {
		return nil
	}
	var out []string
	add := func(id string) {
		id = channels.CanonicalChannelConversationExternalID(strings.TrimSpace(id))
		if id == "" {
			return
		}
		for _, x := range out {
			if x == id {
				return
			}
		}
		out = append(out, id)
	}
	add(channels.BuildChannelConversationExternalID(channelID, "", targetID))
	switch scope {
	case channels.ChannelConversationScopeDM:
		add(channels.BuildChannelConversationExternalID(channelID, "", "user:"+targetID))
	case channels.ChannelConversationScopeGroup:
		add(channels.BuildChannelConversationExternalID(channelID, "", "group:"+targetID))
	}
	return out
}

func (s *OpenClawChannelService) syncSessionConversation(
	agentID int64,
	ch channels.Channel,
	scope string,
	targetID string,
	entry openClawSessionStoreEntry,
	openClawSessionKey string,
) error {
	externalID := channels.BuildChannelConversationExternalID(ch.ID, scope, targetID)
	if externalID == "" {
		return nil
	}

	name := s.buildSyncedConversationName(ch, entry, scope, targetID)
	legacy := openClawQQSessionSyncLegacyExternalIDs(ch.Platform, ch.ID, scope, targetID)
	convID, err := s.convSvc.FindOrCreateByExternalID(agentID, externalID, name, conversations.AgentTypeOpenClaw, legacy...)
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
	currentName, _ := loadConversationName(context.Background(), db, convID)
	q := db.NewUpdate().
		Table("conversations").
		Set("updated_at = ?", updateAt).
		Where("id = ?", convID)
	var allowNameUpdate bool
	if strings.TrimSpace(ch.Platform) == channels.PlatformWechat {
		allowNameUpdate = shouldUpdateWechatSyncedConversationName(currentName, name, scope, targetID)
	} else {
		allowNameUpdate = shouldUpdateSyncedConversationName(currentName, name, scope, targetID)
	}
	if allowNameUpdate {
		q = q.Set("name = ?", name)
	}
	if lastMessage != "" {
		q = q.Set("last_message = ?", lastMessage)
	}
	if sk := strings.TrimSpace(openClawSessionKey); sk != "" {
		q = q.Set("openclaw_session_key = ?", sk)
	}
	if _, err := q.Exec(context.Background()); err != nil {
		return fmt.Errorf("update synced conversation: %w", err)
	}
	return nil
}

func (s *OpenClawChannelService) updateSyncedChannelLastReplyTarget(channelID int64, scope string, targetID string) error {
	db, err := s.db()
	if err != nil {
		if s.app != nil {
			s.app.Logger.Warn("openclaw sync: db unavailable for channel last reply target",
				"channel_id", channelID,
				"scope", scope,
				"target_id", targetID,
				"error", err,
			)
		}
		return err
	}

	chatID, senderID := syncedChannelReplyTarget(scope, targetID)
	if chatID == "" && senderID == "" {
		if s.app != nil {
			s.app.Logger.Info("openclaw sync: empty channel last reply target resolved",
				"channel_id", channelID,
				"scope", scope,
				"target_id", targetID,
			)
		}
		return nil
	}
	if s.app != nil {
		s.app.Logger.Info("openclaw sync: resolved channel last reply target",
			"channel_id", channelID,
			"scope", scope,
			"target_id", targetID,
			"chat_id", chatID,
			"sender_id", senderID,
		)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return channels.UpdateChannelLastReplyTarget(ctx, db, channelID, chatID, senderID)
}

func syncedChannelReplyTarget(scope string, targetID string) (chatID string, senderID string) {
	targetID = strings.TrimSpace(targetID)
	if targetID == "" {
		return "", ""
	}

	switch strings.TrimSpace(scope) {
	case channels.ChannelConversationScopeGroup:
		return targetID, ""
	default:
		return "", targetID
	}
}

type openClawPluginSessionSource struct {
	Platform  string
	AccountID string
	RawScope  string
	Scope     string
	TargetID  string
}

func parseOpenClawPluginSessionKey(agentID string, sessionKey string, entry openClawSessionStoreEntry) (openClawPluginSessionSource, bool) {
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
	switch strings.ToLower(platform) {
	case "dingtalk-connector":
		platform = channels.PlatformDingTalk
	case "openclaw-weixin", "weixin":
		platform = channels.PlatformWechat
	case openClawQQChannelID:
		platform = channels.PlatformQQ
	}
	scope := strings.TrimSpace(parts[3])
	targetID := strings.TrimSpace(strings.Join(parts[4:], ":"))
	if platform == "" || targetID == "" {
		return openClawPluginSessionSource{}, false
	}

	scope = normalizePluginConversationScope(platform, scope, targetID, entry.ChatType)
	if scope == "" {
		return openClawPluginSessionSource{}, false
	}

	return openClawPluginSessionSource{
		Platform:  platform,
		AccountID: extractOpenClawSessionAccountID(platform, entry),
		RawScope:  strings.TrimSpace(parts[3]),
		Scope:     scope,
		TargetID:  targetID,
	}, true
}

func collectSyncSourceAgentIDs(openclawAgentID string) []string {
	openclawAgentID = strings.TrimSpace(openclawAgentID)
	if openclawAgentID == "" {
		return nil
	}
	if openclawAgentID == "main" {
		return []string{openclawAgentID}
	}
	return []string{openclawAgentID, "main"}
}

func buildSyncedChannelTargetKey(platform string, accountID string) string {
	return strings.TrimSpace(platform) + ":" + strings.TrimSpace(accountID)
}

func buildSyncedSessionCandidateKey(channelID int64, scope string, targetID string) string {
	return fmt.Sprintf("%d:%s:%s", channelID, strings.TrimSpace(scope), channels.NormalizeChannelConversationTargetID(targetID))
}

func shouldReplaceSyncedSessionCandidate(current, next syncedSessionCandidate) bool {
	if next.entry.UpdatedAt != current.entry.UpdatedAt {
		return next.entry.UpdatedAt > current.entry.UpdatedAt
	}
	return syncedSessionScopePriority(next.source.RawScope) > syncedSessionScopePriority(current.source.RawScope)
}

func syncedSessionScopePriority(scope string) int {
	switch strings.TrimSpace(strings.ToLower(scope)) {
	case "group", "direct", "dm", "p2p":
		return 2
	case "chat":
		return 1
	default:
		return 0
	}
}

func extractOpenClawSessionAccountID(platform string, entry openClawSessionStoreEntry) string {
	accountID := strings.TrimSpace(entry.DeliveryCtx.AccountID)
	if accountID == "" {
		accountID = strings.TrimSpace(entry.Origin.AccountID)
	}
	if accountID != "" {
		return accountID
	}
	if strings.TrimSpace(platform) == channels.PlatformFeishu {
		return ""
	}
	return "default"
}

func normalizePluginConversationScope(platform string, scope string, targetID string, chatType string) string {
	switch strings.TrimSpace(platform) {
	case channels.PlatformWeCom:
		return normalizeWeComPluginConversationScope(scope)
	case channels.PlatformFeishu:
		return normalizeFeishuPluginConversationScope(scope, targetID, chatType)
	case channels.PlatformDingTalk:
		return normalizeDingTalkPluginConversationScope(scope, targetID, chatType)
	case channels.PlatformWechat:
		// Weixin OpenClaw plugin uses the same scope/chatType conventions as DingTalk.
		return normalizeDingTalkPluginConversationScope(scope, targetID, chatType)
	case channels.PlatformQQ:
		return normalizeQQPluginConversationScope(scope, targetID, chatType)
	default:
		return ""
	}
}

func normalizeQQPluginConversationScope(scope string, targetID string, chatType string) string {
	switch strings.TrimSpace(strings.ToLower(scope)) {
	case "dm", "direct", "c2c", "private":
		return channels.ChannelConversationScopeDM
	case "group", "guild", "channel":
		return channels.ChannelConversationScopeGroup
	case "chat":
		lowerTarget := strings.ToLower(strings.TrimSpace(targetID))
		switch {
		case strings.HasPrefix(lowerTarget, "user:"):
			return channels.ChannelConversationScopeDM
		case strings.HasPrefix(lowerTarget, "group:"), strings.HasPrefix(lowerTarget, "guild:"), strings.HasPrefix(lowerTarget, "channel:"):
			return channels.ChannelConversationScopeGroup
		}

		lowerChatType := strings.ToLower(strings.TrimSpace(chatType))
		switch {
		case strings.Contains(lowerChatType, "group"),
			strings.Contains(lowerChatType, "guild"),
			strings.Contains(lowerChatType, "channel"):
			return channels.ChannelConversationScopeGroup
		case strings.Contains(lowerChatType, "c2c"),
			strings.Contains(lowerChatType, "private"),
			strings.Contains(lowerChatType, "dm"),
			strings.Contains(lowerChatType, "direct"):
			return channels.ChannelConversationScopeDM
		default:
			return ""
		}
	default:
		return ""
	}
}

func normalizeDingTalkPluginConversationScope(scope string, targetID string, chatType string) string {
	switch strings.TrimSpace(strings.ToLower(scope)) {
	case "dm", "direct", "p2p":
		return channels.ChannelConversationScopeDM
	case "group":
		return channels.ChannelConversationScopeGroup
	case "chat":
		lowerChatType := strings.ToLower(strings.TrimSpace(chatType))
		switch {
		case strings.Contains(lowerChatType, "group"):
			return channels.ChannelConversationScopeGroup
		case strings.Contains(lowerChatType, "p2p"),
			strings.Contains(lowerChatType, "single"),
			strings.Contains(lowerChatType, "dm"),
			strings.Contains(lowerChatType, "direct"):
			return channels.ChannelConversationScopeDM
		default:
			return ""
		}
	default:
		return ""
	}
}

func normalizeWeComPluginConversationScope(scope string) string {
	switch strings.TrimSpace(strings.ToLower(scope)) {
	case "dm", "direct":
		return channels.ChannelConversationScopeDM
	case "group":
		return channels.ChannelConversationScopeGroup
	default:
		return ""
	}
}

func normalizeFeishuPluginConversationScope(scope string, targetID string, chatType string) string {
	switch strings.TrimSpace(strings.ToLower(scope)) {
	case "dm", "direct", "p2p":
		return channels.ChannelConversationScopeDM
	case "group":
		return channels.ChannelConversationScopeGroup
	case "chat":
		lowerTarget := strings.ToLower(strings.TrimSpace(targetID))
		switch {
		case strings.HasPrefix(lowerTarget, "oc_"):
			return channels.ChannelConversationScopeGroup
		case strings.HasPrefix(lowerTarget, "ou_"), strings.HasPrefix(lowerTarget, "on_"):
			return channels.ChannelConversationScopeDM
		}

		lowerChatType := strings.ToLower(strings.TrimSpace(chatType))
		switch {
		case strings.Contains(lowerChatType, "group"):
			return channels.ChannelConversationScopeGroup
		case strings.Contains(lowerChatType, "p2p"),
			strings.Contains(lowerChatType, "single"),
			strings.Contains(lowerChatType, "dm"),
			strings.Contains(lowerChatType, "direct"):
			return channels.ChannelConversationScopeDM
		default:
			return ""
		}
	default:
		return ""
	}
}

func (s *OpenClawChannelService) buildSyncedConversationName(ch channels.Channel, entry openClawSessionStoreEntry, scope string, targetID string) string {
	title := channels.ConversationTitleFromFirstMessage(extractFirstSessionUserMessageText(entry.SessionFile))
	if title == "" {
		title = channels.ConversationTitleFromFirstMessage(extractLastSessionMessage(entry.SessionFile))
	}

	// WeChat: prefer the first user message as the conversation title (single and group).
	if ch.Platform == channels.PlatformWechat {
		if t := extractFirstSessionUserMessageText(entry.SessionFile); t != "" {
			return channels.ConversationTitleFromFirstMessage(t)
		}
	}

	name := strings.TrimSpace(entry.Origin.Label)
	if name != "" && !isSyncedConversationPlaceholderName(name, scope, targetID) {
		return name
	}

	if title != "" {
		return formatAssistantConversationName(scope, title)
	}
	return formatAssistantConversationName(scope, channels.NormalizeChannelConversationTargetID(targetID))
}

func loadConversationName(ctx context.Context, db *bun.DB, convID int64) (string, error) {
	var name string
	err := db.NewSelect().
		Table("conversations").
		Column("name").
		Where("id = ?", convID).
		Limit(1).
		Scan(ctx, &name)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(name), nil
}

func shouldUpdateSyncedConversationName(currentName, nextName, scope, targetID string) bool {
	currentName = strings.TrimSpace(currentName)
	nextName = strings.TrimSpace(nextName)
	if nextName == "" || currentName == nextName {
		return false
	}
	if currentName == "" {
		return true
	}
	currentPlaceholder := isSyncedConversationPlaceholderName(currentName, scope, targetID)
	nextPlaceholder := isSyncedConversationPlaceholderName(nextName, scope, targetID)
	if currentPlaceholder && !nextPlaceholder {
		return true
	}
	if !currentPlaceholder && nextPlaceholder {
		return false
	}
	return true
}

// shouldUpdateWechatSyncedConversationName applies the WeChat title from the first user message only
// while the row still has an empty or system-generated placeholder name. After the client sets a
// custom title (any non-placeholder), sync must not overwrite it on refresh.
func shouldUpdateWechatSyncedConversationName(currentName, nextName, scope, targetID string) bool {
	currentName = strings.TrimSpace(currentName)
	nextName = strings.TrimSpace(nextName)
	if nextName == "" || currentName == nextName {
		return false
	}
	if currentName == "" {
		return true
	}
	if !isSyncedConversationPlaceholderName(currentName, scope, targetID) {
		return false
	}
	return shouldUpdateSyncedConversationName(currentName, nextName, scope, targetID)
}

func isSyncedConversationPlaceholderName(name, scope, targetID string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return true
	}
	normalizedTargetID := channels.NormalizeChannelConversationTargetID(targetID)
	lowerName := strings.ToLower(name)
	if scope == channels.ChannelConversationScopeGroup {
		if strings.HasPrefix(name, "group:") || strings.HasPrefix(name, "「飞书群」") || strings.HasPrefix(name, "「企微群」") {
			return true
		}
		if strings.HasPrefix(name, "「群聊」") {
			rest := strings.TrimSpace(strings.TrimPrefix(name, "「群聊」"))
			if strings.EqualFold(rest, targetID) || strings.EqualFold(rest, normalizedTargetID) {
				return true
			}
		}
		if strings.EqualFold(name, targetID) || strings.EqualFold(name, normalizedTargetID) {
			return true
		}
		if strings.HasPrefix(lowerName, "feishu:g-oc_") || strings.HasPrefix(lowerName, "oc_") {
			return true
		}
		return false
	}
	if strings.HasPrefix(name, "user:") {
		return true
	}
	return strings.EqualFold(name, targetID) || strings.EqualFold(name, normalizedTargetID)
}

func formatAssistantConversationName(scope string, title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return ""
	}
	if scope == channels.ChannelConversationScopeGroup {
		return "「群聊」" + title
	}
	return title
}

func (s *OpenClawChannelService) resolveFeishuChatName(extraConfig string, chatID string) string {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return ""
	}
	cfg, err := channels.ParseFeishuConfig(extraConfig)
	if err != nil || strings.TrimSpace(cfg.AppID) == "" || strings.TrimSpace(cfg.AppSecret) == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := lark.NewClient(strings.TrimSpace(cfg.AppID), strings.TrimSpace(cfg.AppSecret))
	resp, err := client.Im.Chat.Get(ctx, larkim.NewGetChatReqBuilder().ChatId(chatID).Build())
	if err != nil || !resp.Success() || resp.Data == nil {
		return ""
	}
	if resp.Data.Name == nil {
		return ""
	}
	return strings.TrimSpace(*resp.Data.Name)
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
