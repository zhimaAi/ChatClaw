package bootstrap

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"chatclaw/internal/services/agents"
	"chatclaw/internal/services/channels"
	"chatclaw/internal/services/chat"
	"chatclaw/internal/services/conversations"
	"chatclaw/internal/services/i18n"
	"chatclaw/internal/sqlite"

	"github.com/uptrace/bun"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// ensureChannelAgent creates a new AI agent for a channel if needed.
// The agent is assigned the first available enabled LLM model as its default.
func ensureChannelAgent(agentsService *agents.AgentsService, channelName string) (int64, error) {
	agent, err := agentsService.CreateAgent(agents.CreateAgentInput{
		Name:   channelName,
		Prompt: "You are a helpful AI assistant connected to a messaging channel. Be concise and friendly in your responses.",
	})
	if err != nil {
		return 0, fmt.Errorf("create agent: %w", err)
	}

	// Try to assign the first available LLM model as default
	db := sqlite.DB()
	if db != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		type modelRow struct {
			ProviderID string `bun:"provider_id"`
			ModelID    string `bun:"model_id"`
		}
		var m modelRow
		err := db.NewSelect().
			Table("models").
			Column("provider_id", "model_id").
			Where("type = ?", "llm").
			Where("enabled = ?", true).
			Where("provider_id IN (SELECT provider_id FROM providers WHERE enabled = true)").
			OrderExpr("sort_order ASC").
			Limit(1).
			Scan(ctx, &m)
		if err == nil && m.ProviderID != "" && m.ModelID != "" {
			providerID := m.ProviderID
			modelID := m.ModelID
			_, _ = agentsService.UpdateAgent(agent.ID, agents.UpdateAgentInput{
				DefaultLLMProviderID: &providerID,
				DefaultLLMModelID:    &modelID,
			})
		}
	}

	return agent.ID, nil
}

// handleChannelMessage processes an incoming message from a channel platform.
// It extracts text content, finds or creates a conversation for the sender,
// generates an AI reply, and sends it back through the channel adapter.
func handleChannelMessage(
	app *application.App,
	chatService *chat.ChatService,
	convService *conversations.ConversationsService,
	gateway *channels.Gateway,
	msg channels.IncomingMessage,
) {
	app.Logger.Info("channel message received",
		"channel_id", msg.ChannelID,
		"platform", msg.Platform,
		"message_id", msg.MessageID,
		"sender", msg.SenderID,
		"sender_name", msg.SenderName,
		"chat_id", msg.ChatID,
		"msg_type", msg.MsgType,
	)

	if gateway == nil {
		app.Logger.Warn("channel gateway not ready, dropping message")
		return
	}

	replyTarget := msg.ChatID
	if replyTarget == "" {
		replyTarget = msg.SenderID
	}

	sendReply := func(text string) {
		if replyTarget == "" {
			app.Logger.Warn("channel message: no reply target available", "channel_id", msg.ChannelID)
			return
		}

		adapter := gateway.GetAdapter(msg.ChannelID)
		if adapter == nil {
			app.Logger.Warn("channel adapter not found, cannot reply", "channel_id", msg.ChannelID)
			return
		}

		replyCtx, replyCancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer replyCancel()

		if err := sendChannelReply(replyCtx, adapter, msg, replyTarget, text); err != nil {
			app.Logger.Error("channel message: send reply failed", "channel_id", msg.ChannelID, "target", replyTarget, "error", err)
			return
		}

		app.Logger.Info("channel reply sent", "channel_id", msg.ChannelID, "target", replyTarget, "response_len", len(text))
	}

	// Extract text content from platform-specific format
	textContent := extractTextContent(msg)
	if textContent == "" {
		app.Logger.Info("channel message has no text content, skipping", "msg_type", msg.MsgType)
		return
	}

	// Look up channel to get agent_id
	db := sqlite.DB()
	if db == nil {
		app.Logger.Error("channel message: database not initialized")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := channels.UpdateChannelLastReplyTarget(ctx, db, msg.ChannelID, msg.ChatID, msg.SenderID); err != nil {
		app.Logger.Warn(
			"channel message: failed to update last reply target",
			"channel_id", msg.ChannelID,
			"chat_id", msg.ChatID,
			"sender_id", msg.SenderID,
			"error", err,
		)
	}

	var channelRow struct {
		AgentID     int64  `bun:"agent_id"`
		ExtraConfig string `bun:"extra_config"`
	}
	err := db.NewSelect().
		Table("channels").
		Column("agent_id", "extra_config").
		Where("id = ?", msg.ChannelID).
		Scan(ctx, &channelRow)
	if err != nil {
		app.Logger.Warn("channel message: failed to load channel config", "channel_id", msg.ChannelID, "error", err)
		return
	}

	if supportsChannelStreamOutput(msg.Platform) {
		if enabled, matched := parseChannelStreamToggleCommand(textContent); matched {
			if err := updateChannelStreamOutputSetting(ctx, db, msg.ChannelID, msg.Platform, channelRow.ExtraConfig, enabled); err != nil {
				app.Logger.Error("channel message: failed to update stream setting", "channel_id", msg.ChannelID, "platform", msg.Platform, "enabled", enabled, "error", err)
				sendReply(i18n.Tf("error.channel_ai_reply_failed", map[string]any{"Error": err}))
				return
			}

			sendReply(i18n.T(streamOutputSettingMessageKey(msg.Platform, enabled)))
			return
		}
	}

	agentID := channelRow.AgentID
	if agentID == 0 {
		app.Logger.Warn("channel has no linked agent, dropping message", "channel_id", msg.ChannelID, "error", err)
		return
	}

	// Use ChatID as conversation key so different platform chats get separate conversations.
	// Fall back to SenderID for direct messages (where ChatID may be empty).
	convKey := msg.ChatID
	if convKey == "" {
		convKey = msg.SenderID
	}
	externalID := fmt.Sprintf("ch:%d:%s", msg.ChannelID, convKey)

	// Build display name: prefer resolved ChatName/SenderName;
	// fall back to "「<platform><type>」<excerpt>" when unavailable.
	displayName := msg.ChatName
	if displayName == "" && msg.IsGroup {
		displayName = channelDisplayName(msg.Platform, true, textContent)
	}
	if displayName == "" {
		displayName = msg.SenderName
	}
	if displayName == "" {
		displayName = channelDisplayName(msg.Platform, false, textContent)
	}

	conv, err := findOrCreateConversation(ctx, db, convService, agentID, externalID, displayName)
	if err != nil {
		app.Logger.Error("channel message: find/create conversation failed", "error", err)
		return
	}

	// Notify frontend that conversation list has changed (e.g. new conversation created)
	app.Event.Emit("conversations:changed", map[string]any{
		"agent_id": agentID,
	})

	// Check for quick-mode prefix: "/q " or "/quick " forces non-streaming reply for this message.
	// Useful when the user wants an instant, one-shot response without the typewriter animation.
	useQuickMode := false
	for _, prefix := range []string{"/q ", "/quick "} {
		if strings.HasPrefix(strings.ToLower(textContent), prefix) {
			textContent = strings.TrimSpace(textContent[len(prefix):])
			useQuickMode = true
			break
		}
	}

	// Prepend sender name so the AI can distinguish who sent the message in a group chat.
	aiContent := textContent
	if msg.SenderName != "" {
		aiContent = fmt.Sprintf("%s：%s", msg.SenderName, textContent)
	}

	// DingTalk: use real-time streaming interactive card by default.
	// Tokens are pushed to the card as the AI generates them instead of waiting for the full response.
	// Pass useQuickMode=true (via /q or /quick prefix) to bypass streaming and send a plain reply.
	if msg.Platform == channels.PlatformDingTalk {
		if replyTarget != "" {
			if adapter := gateway.GetAdapter(msg.ChannelID); adapter != nil {
				if dingAdapter, ok := adapter.(*channels.DingTalkAdapter); ok {
					if useQuickMode {
						runNormalDingTalkReply(app, chatService, convService, db, dingAdapter, msg, agentID, conv.ID, replyTarget, aiContent)
					} else {
						runDingTalkStreamingReply(app, chatService, convService, db, dingAdapter, msg, agentID, conv.ID, replyTarget, aiContent)
					}
					return
				}
			}
		}
	}
	res, err := chatService.SendMessage(chat.SendMessageInput{
		ConversationID: conv.ID,
		Content:        aiContent,
		TabID:          "channel_backend", // Special tab ID to prevent frontend errors
	})
	if err != nil {
		app.Logger.Error("channel message: AI generation failed to start", "conv", conv.ID, "error", err)
		sendReply(i18n.Tf("error.channel_ai_reply_failed", map[string]any{"Error": err}))
		return
	}

	// Notify frontend that there's a new user message
	app.Event.Emit("chat:messages-changed", map[string]any{
		"conversation_id": conv.ID,
	})

	streamOutputEnabled, cfgErr := getChannelStreamOutputEnabled(msg.Platform, channelRow.ExtraConfig)
	if cfgErr != nil {
		app.Logger.Warn("channel message: failed to parse channel config, fallback to non-stream", "channel_id", msg.ChannelID, "platform", msg.Platform, "error", cfgErr)
	}

	var (
		finalResponse      string
		streamReplyHandled bool
		shouldFetchFinal   = true
	)
	if streamOutputEnabled {
		if adapter := gateway.GetAdapter(msg.ChannelID); adapter != nil {
			switch msg.Platform {
			case channels.PlatformFeishu:
				if feishuAdapter, ok := adapter.(feishuStreamingReplyAdapter); ok {
					streamedResponse, handled, streamErr := streamFeishuReply(app, db, chatService, conv.ID, res.RequestID, feishuAdapter, msg, replyTarget)
					if streamErr != nil {
						app.Logger.Error("channel message: feishu stream reply failed", "conv", conv.ID, "error", streamErr)
					}
					if handled {
						streamReplyHandled = true
						finalResponse = streamedResponse
						shouldFetchFinal = false
					}
				}
			case channels.PlatformWeCom:
				if wecomAdapter, ok := adapter.(wecomStreamingReplyAdapter); ok {
					streamedResponse, handled, streamErr := streamWeComReply(app, db, chatService, conv.ID, res.RequestID, wecomAdapter, msg)
					if streamErr != nil {
						app.Logger.Error("channel message: wecom stream reply failed", "conv", conv.ID, "error", streamErr)
					}
					if handled {
						streamReplyHandled = true
						finalResponse = streamedResponse
						shouldFetchFinal = false
					}
				}
			}
		}
	}

	if shouldFetchFinal {
		if err := chatService.WaitForGeneration(conv.ID, res.RequestID); err != nil {
			app.Logger.Error("channel message: AI generation wait failed", "conv", conv.ID, "error", err)
			sendReply(i18n.Tf("error.channel_ai_reply_failed", map[string]any{"Error": err}))
			// Not returning here in case some partial response was generated
		}

		assistantMsg, fetchErr := fetchLatestAssistantMessage(ctx, db, conv.ID)
		if fetchErr == nil {
			finalResponse = assistantMsg.Content
		} else {
			app.Logger.Warn("channel message: fetch final assistant message failed", "conv", conv.ID, "error", fetchErr)
		}
	}

	finalResponse = strings.TrimSpace(finalResponse)

	// Notify frontend again after AI reply is generated
	_, _ = convService.UpdateConversation(conv.ID, conversations.UpdateConversationInput{
		LastMessage: &finalResponse,
	})
	app.Event.Emit("conversations:changed", map[string]any{
		"agent_id": agentID,
	})
	app.Event.Emit("chat:messages-changed", map[string]any{
		"conversation_id": conv.ID,
	})

	if finalResponse == "" {
		app.Logger.Warn("channel message: empty AI response", "conv", conv.ID)
		if !streamReplyHandled {
			sendReply(i18n.T("error.channel_ai_reply_empty"))
		}
		return
	}

	if !streamReplyHandled {
		sendReply(finalResponse)
	}
}

type feishuStreamingReplyAdapter interface {
	CreateStreamCardMessage(ctx context.Context, targetID string, replyToMessageID string, placeholder string) (*channels.FeishuStreamCardHandle, error)
	UpdateStreamCardMessage(ctx context.Context, handle *channels.FeishuStreamCardHandle, text string, finish bool) error
}

type wecomStreamingReplyAdapter interface {
	SendStreamMessage(ctx context.Context, reqID string, streamID string, content string, finish bool) error
}

type assistantMessageSnapshot struct {
	Content string `bun:"content"`
	Status  string `bun:"status"`
}

func sendChannelReply(ctx context.Context, adapter channels.PlatformAdapter, msg channels.IncomingMessage, targetID string, text string) error {
	if msg.Platform == channels.PlatformFeishu && msg.MessageID != "" {
		if feishuAdapter, ok := adapter.(interface {
			ReplyMessage(context.Context, string, string) (string, error)
		}); ok {
			_, err := feishuAdapter.ReplyMessage(ctx, msg.MessageID, text)
			return err
		}
	}

	content := text
	if msg.Platform == channels.PlatformQQ && msg.MessageID != "" {
		content = channels.InjectQQMsgID(text, msg.MessageID)
	}
	return adapter.SendMessage(ctx, targetID, content)
}

func supportsChannelStreamOutput(platform string) bool {
	return platform == channels.PlatformFeishu || platform == channels.PlatformWeCom
}

func parseChannelStreamToggleCommand(text string) (bool, bool) {
	switch strings.TrimSpace(text) {
	case "关闭流式输出", "关闭流式回复", "关闭流式":
		return false, true
	case "开启流式输出", "打开流式输出", "启用流式输出", "开启流式回复", "开启流式":
		return true, true
	default:
		return false, false
	}
}

func streamOutputSettingMessageKey(platform string, enabled bool) string {
	switch platform {
	case channels.PlatformWeCom:
		if enabled {
			return "channel.wecom_stream_output_enabled"
		}
		return "channel.wecom_stream_output_disabled"
	default:
		if enabled {
			return "channel.feishu_stream_output_enabled"
		}
		return "channel.feishu_stream_output_disabled"
	}
}

func getChannelStreamOutputEnabled(platform string, extraConfig string) (bool, error) {
	switch platform {
	case channels.PlatformFeishu:
		cfg, err := channels.ParseFeishuConfig(extraConfig)
		if err != nil {
			return false, err
		}
		return cfg.StreamOutputEnabledOrDefault(), nil
	case channels.PlatformWeCom:
		cfg, err := channels.ParseWeComConfig(extraConfig)
		if err != nil {
			return false, err
		}
		return cfg.StreamOutputEnabledOrDefault(), nil
	default:
		return false, nil
	}
}

func updateChannelStreamOutputSetting(ctx context.Context, db *bun.DB, channelID int64, platform string, extraConfig string, enabled bool) error {
	var (
		configBytes []byte
		err         error
	)

	switch platform {
	case channels.PlatformFeishu:
		cfg, parseErr := channels.ParseFeishuConfig(extraConfig)
		if parseErr != nil {
			return parseErr
		}
		cfg.StreamOutputEnabled = boolPtr(enabled)
		configBytes, err = json.Marshal(cfg)
		if err != nil {
			return fmt.Errorf("marshal feishu config: %w", err)
		}
	case channels.PlatformWeCom:
		cfg, parseErr := channels.ParseWeComConfig(extraConfig)
		if parseErr != nil {
			return parseErr
		}
		cfg.StreamOutputEnabled = boolPtr(enabled)
		configBytes, err = json.Marshal(cfg)
		if err != nil {
			return fmt.Errorf("marshal wecom config: %w", err)
		}
	default:
		return fmt.Errorf("platform %s does not support stream output setting", platform)
	}

	_, err = db.NewUpdate().
		Table("channels").
		Set("extra_config = ?", string(configBytes)).
		Set("updated_at = ?", sqlite.NowUTC()).
		Where("id = ?", channelID).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("update channel extra_config: %w", err)
	}
	return nil
}

func streamFeishuReply(
	app *application.App,
	db *bun.DB,
	chatService *chat.ChatService,
	conversationID int64,
	requestID string,
	adapter feishuStreamingReplyAdapter,
	msg channels.IncomingMessage,
	replyTarget string,
) (string, bool, error) {
	if replyTarget == "" {
		return "", false, fmt.Errorf("no reply target available")
	}

	replyCtx, replyCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer replyCancel()

	placeholder := i18n.T("channel.feishu_streaming_generating")

	var (
		streamHandle *channels.FeishuStreamCardHandle
		err          error
	)
	streamHandle, err = adapter.CreateStreamCardMessage(replyCtx, replyTarget, msg.MessageID, placeholder)
	if err != nil {
		return "", false, err
	}

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- chatService.WaitForGeneration(conversationID, requestID)
	}()

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	lastSent := placeholder

	for {
		select {
		case waitErr := <-waitCh:
			finalCtx, finalCancel := context.WithTimeout(context.Background(), 5*time.Second)
			snapshot, fetchErr := fetchLatestAssistantMessage(finalCtx, db, conversationID)
			finalCancel()

			finalResponse := ""
			if fetchErr == nil {
				finalResponse = snapshot.Content
			}

			if strings.TrimSpace(finalResponse) == "" {
				finalResponse = i18n.T("error.channel_ai_reply_empty")
			}

			updateCtx, updateCancel := context.WithTimeout(context.Background(), 10*time.Second)
			updateErr := adapter.UpdateStreamCardMessage(updateCtx, streamHandle, finalResponse, true)
			updateCancel()
			if updateErr != nil {
				app.Logger.Error("channel message: final feishu stream update failed", "conv", conversationID, "card_id", streamHandle.CardID, "error", updateErr)
			}

			return finalResponse, true, waitErr
		case <-ticker.C:
			currentContent, ok := chatService.GetGenerationContent(conversationID, requestID)
			if !ok || strings.TrimSpace(currentContent) == "" || currentContent == lastSent {
				continue
			}

			updateCtx, updateCancel := context.WithTimeout(context.Background(), 10*time.Second)
			updateErr := adapter.UpdateStreamCardMessage(updateCtx, streamHandle, currentContent, false)
			updateCancel()
			if updateErr != nil {
				app.Logger.Warn("channel message: feishu stream update failed", "conv", conversationID, "card_id", streamHandle.CardID, "error", updateErr)
				continue
			}

			lastSent = currentContent
		}
	}
}

type wecomReplyContext struct {
	ReqID string `json:"req_id"`
}

func streamWeComReply(
	app *application.App,
	db *bun.DB,
	chatService *chat.ChatService,
	conversationID int64,
	requestID string,
	adapter wecomStreamingReplyAdapter,
	msg channels.IncomingMessage,
) (string, bool, error) {
	replyCtx, err := extractWeComReplyContext(msg.RawData)
	if err != nil {
		return "", false, err
	}

	streamID := channels.GenerateWeComReqID("stream")

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- chatService.WaitForGeneration(conversationID, requestID)
	}()

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	lastSent := ""
	sentAny := false

	sendChunk := func(content string, finish bool) error {
		sendCtx, sendCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer sendCancel()
		return adapter.SendStreamMessage(sendCtx, replyCtx.ReqID, streamID, content, finish)
	}

	for {
		select {
		case waitErr := <-waitCh:
			finalCtx, finalCancel := context.WithTimeout(context.Background(), 5*time.Second)
			snapshot, fetchErr := fetchLatestAssistantMessage(finalCtx, db, conversationID)
			finalCancel()

			finalResponse := ""
			if fetchErr == nil {
				finalResponse = snapshot.Content
			}

			if strings.TrimSpace(finalResponse) == "" {
				finalResponse = i18n.T("error.channel_ai_reply_empty")
			}

			if err := sendChunk(finalResponse, true); err != nil {
				if !sentAny {
					return "", false, err
				}
				app.Logger.Error("channel message: final wecom stream update failed", "conv", conversationID, "error", err)
			} else {
				lastSent = finalResponse
				sentAny = true
			}

			return finalResponse, sentAny, waitErr
		case <-ticker.C:
			currentContent, ok := chatService.GetGenerationContent(conversationID, requestID)
			if !ok || strings.TrimSpace(currentContent) == "" || currentContent == lastSent {
				continue
			}

			if err := sendChunk(currentContent, false); err != nil {
				if !sentAny {
					return "", false, err
				}
				app.Logger.Warn("channel message: wecom stream update failed", "conv", conversationID, "error", err)
				continue
			}

			lastSent = currentContent
			sentAny = true
		}
	}
}

func extractWeComReplyContext(rawData string) (wecomReplyContext, error) {
	var replyCtx wecomReplyContext
	if err := json.Unmarshal([]byte(rawData), &replyCtx); err != nil {
		return wecomReplyContext{}, fmt.Errorf("parse wecom reply context: %w", err)
	}
	if strings.TrimSpace(replyCtx.ReqID) == "" {
		return wecomReplyContext{}, fmt.Errorf("wecom reply context missing req_id")
	}
	return replyCtx, nil
}

func fetchLatestAssistantMessage(ctx context.Context, db *bun.DB, conversationID int64) (assistantMessageSnapshot, error) {
	var assistantMsg assistantMessageSnapshot
	err := db.NewSelect().
		Table("messages").
		Column("content", "status").
		Where("conversation_id = ?", conversationID).
		Where("role = ?", chat.RoleAssistant).
		OrderExpr("id DESC").
		Limit(1).
		Scan(ctx, &assistantMsg)
	if err != nil {
		return assistantMsg, err
	}
	return assistantMsg, nil
}

func boolPtr(v bool) *bool {
	return &v
}

// runDingTalkStreamingReply handles the DingTalk-specific real-time streaming reply path.
// It creates an interactive card immediately, then updates it in real-time as the AI generates
// tokens, delivering a typewriter effect without waiting for the full response.
func runDingTalkStreamingReply(
	app *application.App,
	chatService *chat.ChatService,
	convService *conversations.ConversationsService,
	db *bun.DB,
	dingAdapter *channels.DingTalkAdapter,
	msg channels.IncomingMessage,
	agentID int64,
	convID int64,
	replyTarget string,
	aiContent string,
) {
	// Generate a unique card instance ID.
	cardBizID := generateCardBizID()

	// Send the initial card placeholder with a cursor.
	cardCtx, cardCancel := context.WithTimeout(context.Background(), 10*time.Second)
	err := dingAdapter.SendInteractiveCard(cardCtx, replyTarget, cardBizID, channels.BuildStreamingCardData("▌"))
	cardCancel()
	if err != nil {
		app.Logger.Warn("[channel] DingTalk streaming card create failed, falling back to normal webhook",
			"channel_id", msg.ChannelID,
			"target", replyTarget,
			"error", err,
		)
		// Graceful fallback: run normal blocking generation + webhook reply.
		runNormalDingTalkReply(app, chatService, convService, db, dingAdapter, msg, agentID, convID, replyTarget, aiContent)
		return
	}

	// Debounced card updater: flush latest accumulated text to DingTalk at most every 300ms.
	// This avoids spamming the interactive card update API while still providing a real-time feel.
	var (
		mu         sync.Mutex
		latestText string
	)
	flushCh := make(chan struct{})

	chatService.RegisterChunkCallback(convID, func(accumulated string) {
		mu.Lock()
		latestText = accumulated
		mu.Unlock()
	})

	go func() {
		ticker := time.NewTicker(300 * time.Millisecond)
		defer ticker.Stop()
		var lastSent string
		for {
			select {
			case <-ticker.C:
				mu.Lock()
				text := latestText
				mu.Unlock()
				if text != lastSent && text != "" {
					lastSent = text
					updateCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
					_ = dingAdapter.UpdateInteractiveCard(updateCtx, cardBizID,
						channels.BuildStreamingCardData(text+"▌"))
					cancel()
				}
			case <-flushCh:
				return
			}
		}
	}()

	// Start AI generation.
	res, genErr := chatService.SendMessage(chat.SendMessageInput{
		ConversationID: convID,
		Content:        aiContent,
		TabID:          "channel_backend",
	})
	if genErr != nil {
		close(flushCh)
		chatService.UnregisterChunkCallback(convID)
		app.Logger.Error("[channel] DingTalk AI generation failed to start",
			"channel_id", msg.ChannelID, "conv", convID, "error", genErr)
		failCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		_ = dingAdapter.UpdateInteractiveCard(failCtx, cardBizID,
			channels.BuildStreamingCardData(i18n.Tf("error.channel_ai_reply_failed", map[string]any{"Error": genErr})))
		cancel()
		return
	}

	// Notify frontend of the new user message.
	app.Event.Emit("chat:messages-changed", map[string]any{"conversation_id": convID})

	// Wait for the AI generation to finish.
	_ = chatService.WaitForGeneration(convID, res.RequestID)

	// Stop the debounced updater goroutine.
	close(flushCh)
	chatService.UnregisterChunkCallback(convID)

	// Fetch the final assistant message.
	var assistantMsg struct {
		Content string `bun:"content"`
	}
	fetchCtx, fetchCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer fetchCancel()
	dbErr := db.NewSelect().
		Table("messages").
		Column("content").
		Where("conversation_id = ?", convID).
		Where("role = ?", "assistant").
		Where("status = ?", "success").
		Where("TRIM(content) <> ''").
		OrderExpr("id DESC").
		Limit(1).
		Scan(fetchCtx, &assistantMsg)
	if errors.Is(dbErr, sql.ErrNoRows) {
		_ = db.NewSelect().
			Table("messages").
			Column("content").
			Where("conversation_id = ?", convID).
			Where("role = ?", "assistant").
			OrderExpr("id DESC").
			Limit(1).
			Scan(fetchCtx, &assistantMsg)
	}
	finalResponse := strings.TrimSpace(assistantMsg.Content)

	// Final card update: complete content, no cursor.
	updateCtx, updateCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer updateCancel()
	if finalResponse != "" {
		if updateErr := dingAdapter.UpdateInteractiveCard(updateCtx, cardBizID,
			channels.BuildStreamingCardData(finalResponse)); updateErr != nil {
			app.Logger.Error("[channel] DingTalk final card update failed, trying webhook fallback",
				"channel_id", msg.ChannelID, "error", updateErr)
			// Best-effort webhook fallback so the message is never lost.
			fallbackCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			_ = dingAdapter.SendMessage(fallbackCtx, replyTarget, finalResponse)
			cancel()
		}
	} else {
		app.Logger.Warn("[channel] DingTalk: empty AI response", "conv", convID)
		_ = dingAdapter.UpdateInteractiveCard(updateCtx, cardBizID,
			channels.BuildStreamingCardData(i18n.T("error.channel_ai_reply_empty")))
	}

	// Update conversation metadata and notify the frontend.
	_, _ = convService.UpdateConversation(convID, conversations.UpdateConversationInput{
		LastMessage: &finalResponse,
	})
	app.Event.Emit("conversations:changed", map[string]any{"agent_id": agentID})
	app.Event.Emit("chat:messages-changed", map[string]any{"conversation_id": convID})

	app.Logger.Info("[channel] DingTalk streaming reply completed",
		"channel_id", msg.ChannelID,
		"conv", convID,
		"response_len", len(finalResponse),
	)
}

// runNormalDingTalkReply is the fallback path when the streaming card cannot be created.
// It runs the same blocking generation + webhook reply used by other platforms.
func runNormalDingTalkReply(
	app *application.App,
	chatService *chat.ChatService,
	convService *conversations.ConversationsService,
	db *bun.DB,
	dingAdapter *channels.DingTalkAdapter,
	msg channels.IncomingMessage,
	agentID int64,
	convID int64,
	replyTarget string,
	aiContent string,
) {
	res, genErr := chatService.SendMessage(chat.SendMessageInput{
		ConversationID: convID,
		Content:        aiContent,
		TabID:          "channel_backend",
	})
	if genErr != nil {
		app.Logger.Error("[channel] DingTalk normal reply: AI generation failed to start",
			"conv", convID, "error", genErr)
		replyCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_ = dingAdapter.SendMessage(replyCtx, replyTarget,
			i18n.Tf("error.channel_ai_reply_failed", map[string]any{"Error": genErr}))
		cancel()
		return
	}

	app.Event.Emit("chat:messages-changed", map[string]any{"conversation_id": convID})
	_ = chatService.WaitForGeneration(convID, res.RequestID)

	var assistantMsg struct {
		Content string `bun:"content"`
	}
	fetchCtx, fetchCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer fetchCancel()
	dbErr := db.NewSelect().
		Table("messages").Column("content").
		Where("conversation_id = ?", convID).
		Where("role = ?", "assistant").
		Where("status = ?", "success").
		Where("TRIM(content) <> ''").
		OrderExpr("id DESC").Limit(1).
		Scan(fetchCtx, &assistantMsg)
	if errors.Is(dbErr, sql.ErrNoRows) {
		_ = db.NewSelect().Table("messages").Column("content").
			Where("conversation_id = ?", convID).Where("role = ?", "assistant").
			OrderExpr("id DESC").Limit(1).Scan(fetchCtx, &assistantMsg)
	}
	finalResponse := strings.TrimSpace(assistantMsg.Content)

	_, _ = convService.UpdateConversation(convID, conversations.UpdateConversationInput{LastMessage: &finalResponse})
	app.Event.Emit("conversations:changed", map[string]any{"agent_id": agentID})
	app.Event.Emit("chat:messages-changed", map[string]any{"conversation_id": convID})

	replyCtx, replyCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer replyCancel()
	if finalResponse == "" {
		_ = dingAdapter.SendMessage(replyCtx, replyTarget, i18n.T("error.channel_ai_reply_empty"))
	} else {
		_ = dingAdapter.SendMessage(replyCtx, replyTarget, finalResponse)
	}
}

// generateCardBizID returns a unique string identifying a DingTalk interactive card instance.
// Combines unix nanoseconds with a monotonic atomic counter to avoid collisions.
var cardBizIDCounter int64

func generateCardBizID() string {
	seq := atomic.AddInt64(&cardBizIDCounter, 1)
	return fmt.Sprintf("card-%d-%d", time.Now().UnixNano(), seq)
}

// extractTextContent extracts plain text from platform-specific message formats.
func extractTextContent(msg channels.IncomingMessage) string {
	// DingTalk media types (picture/file/audio/video) are handled below;
	// skip the text-only guard for DingTalk so media descriptions reach the AI.
	if msg.Platform != channels.PlatformDingTalk {
		if msg.MsgType != "text" && msg.MsgType != "post" && msg.MsgType != "" {
			return ""
		}
	}

	content := strings.TrimSpace(msg.Content)
	if content == "" {
		return ""
	}

	// DingTalk: content is already extracted/described by the adapter.
	// - text/audio(ASR) -> plain string, pass directly
	// - picture/file/video/audio-no-ASR -> descriptive string like "[图片]", "[文件: xxx.pdf]"
	// Non-text media types (picture/file/video) are described but not useful as AI input;
	// we pass the description so the AI can at least acknowledge the message.
	if msg.Platform == channels.PlatformDingTalk {
		return content
	}

	if strings.HasPrefix(content, "{") {
		// Handle Feishu text messages: {"text":"actual message"}
		if msg.MsgType == "text" || msg.MsgType == "" {
			var parsed struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal([]byte(content), &parsed); err == nil && parsed.Text != "" {
				return strings.TrimSpace(parsed.Text)
			}
		}

		// Handle Feishu post (rich text) messages
		if msg.MsgType == "post" {
			var parsed struct {
				Title   string `json:"title"`
				Content [][]struct {
					Tag  string `json:"tag"`
					Text string `json:"text"`
				} `json:"content"`
			}
			if err := json.Unmarshal([]byte(content), &parsed); err == nil {
				var sb strings.Builder
				if parsed.Title != "" {
					sb.WriteString(parsed.Title)
					sb.WriteString("\n")
				}
				for _, line := range parsed.Content {
					for _, item := range line {
						if item.Tag == "text" || item.Tag == "a" {
							sb.WriteString(item.Text)
						}
					}
					sb.WriteString("\n")
				}
				return strings.TrimSpace(sb.String())
			}
		}
	}

	return content
}

// findOrCreateConversation finds an existing conversation by agent_id and external_id,
// or creates a new one if it doesn't exist.
// externalID is a stable key (e.g., "ch:1:oc_xxx") for lookup.
// displayName is a human-readable name (e.g., group/user name) for display.
func findOrCreateConversation(
	ctx context.Context,
	db *bun.DB,
	convService *conversations.ConversationsService,
	agentID int64,
	externalID string,
	displayName string,
) (*conversations.Conversation, error) {
	// Try to find existing conversation by external_id
	type convRow struct {
		ID int64 `bun:"id"`
	}
	var existing convRow
	err := db.NewSelect().
		Table("conversations").
		Column("id").
		Where("agent_id = ?", agentID).
		Where("external_id = ?", externalID).
		Limit(1).
		Scan(ctx, &existing)
	if err == nil && existing.ID > 0 {
		conv, err := convService.GetConversation(existing.ID)
		if err == nil {
			return conv, nil
		}
	}

	// Fall back to legacy lookup by name for backward compatibility
	err = db.NewSelect().
		Table("conversations").
		Column("id").
		Where("agent_id = ?", agentID).
		Where("name = ?", externalID).
		Limit(1).
		Scan(ctx, &existing)
	if err == nil && existing.ID > 0 {
		conv, err := convService.GetConversation(existing.ID)
		if err == nil {
			return conv, nil
		}
	}

	// Create new conversation with chat mode (simple LLM, no tools)
	conv, err := convService.CreateConversation(conversations.CreateConversationInput{
		AgentID:    agentID,
		Name:       displayName,
		ExternalID: externalID,
		ChatMode:   "chat",
	})
	if err != nil {
		return nil, fmt.Errorf("create conversation: %w", err)
	}

	return conv, nil
}

var platformLabel = map[string]string{
	channels.PlatformFeishu: "飞书",
	channels.PlatformWeCom:  "企微",
	channels.PlatformQQ:     "QQ",
}

func channelDisplayName(platform string, isGroup bool, content string) string {
	label, ok := platformLabel[platform]
	if !ok {
		label = platform
	}
	suffix := "群"
	if !isGroup {
		suffix = ""
	}
	excerpt := content
	if rs := []rune(excerpt); len(rs) > 20 {
		excerpt = string(rs[:20]) + "…"
	}
	return "「" + label + suffix + "」" + excerpt
}
