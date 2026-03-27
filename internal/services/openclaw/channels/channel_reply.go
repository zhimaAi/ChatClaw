package openclawchannels

import (
	"context"
	"strings"
	"time"

	"chatclaw/internal/errs"
	"chatclaw/internal/services/channels"
	"chatclaw/internal/services/chat"
	"chatclaw/internal/services/conversations"
	"chatclaw/internal/services/i18n"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// feishuStreamingAdapter is the subset of FeishuAdapter used for streaming replies.
type feishuStreamingAdapter interface {
	CreateStreamCardMessage(ctx context.Context, targetID string, replyToMessageID string, placeholder string) (*channels.FeishuStreamCardHandle, error)
	UpdateStreamCardMessage(ctx context.Context, handle *channels.FeishuStreamCardHandle, text string, finish bool) error
}

// RunChannelReply handles channel messages for OpenClaw agents.
// It requires the OpenClaw Gateway to be running.
// When the Feishu channel has streaming enabled, it creates a streaming card
// and pushes incremental updates as the agent generates tokens.
func RunChannelReply(
	app *application.App,
	chatService *chat.ChatService,
	convService *conversations.ConversationsService,
	gateway *channels.Gateway,
	conversationID int64,
	agentID int64,
	content string,
	msg channels.IncomingMessage,
	replyTarget string,
	extraConfig string,
	useQuickMode bool,
	sendReply func(string),
) {
	res, err := chatService.SendOpenClawMessage(chat.SendMessageInput{
		ConversationID: conversationID,
		Content:        content,
		TabID:          "channel_backend",
	})
	if err != nil {
		errMsg := i18n.T("error.openclaw_gateway_not_ready_channel")
		if ie, ok := err.(*errs.I18nError); ok {
			switch ie.Key {
			case "error.chat_generation_in_progress", "error.chat_generation_in_progress_other_tab":
				errMsg = ie.Message
			}
		}
		app.Logger.Warn("openclaw channel: send openclaw message failed", "conv", conversationID, "error", err)
		sendReply(errMsg)
		return
	}

	app.Logger.Info("openclaw channel: SendOpenClawMessage ok, waiting for generation",
		"conv", conversationID, "requestID", res.RequestID)

	// OpenClaw channel runs do not persist channel user messages into local DB.
	// Push conversation/message refresh signals immediately so frontend can
	// sync the channel-triggered user turn without waiting for final completion.
	syncChannelIncomingState(app, convService, conversationID, agentID, content, msg.SenderName)

	if !useQuickMode && msg.Platform == channels.PlatformFeishu && replyTarget != "" {
		streamEnabled := feishuStreamOutputEnabled(extraConfig)
		if streamEnabled {
			if adapter := gateway.GetAdapter(msg.ChannelID); adapter != nil {
				if fa, ok := adapter.(feishuStreamingAdapter); ok {
					streamFeishuReply(app, chatService, convService, conversationID, agentID, res.RequestID, fa, msg, replyTarget)
					return
				}
			}
		}
	}

	if err := chatService.WaitForGeneration(conversationID, res.RequestID); err != nil {
		app.Logger.Error("openclaw channel: generation wait failed", "conv", conversationID, "error", err)
		sendReply(i18n.Tf("error.channel_ai_reply_failed", map[string]any{"Error": err}))
		return
	}

	finalResponse := finalResponse(chatService, conversationID, res.RequestID)

	app.Logger.Info("openclaw channel: after WaitForGeneration",
		"conv", conversationID, "response_len", len(finalResponse))

	updateConversationMeta(app, convService, conversationID, agentID, finalResponse)

	if finalResponse == "" {
		app.Logger.Warn("openclaw channel: empty AI response", "conv", conversationID)
		sendReply(i18n.T("error.channel_ai_reply_empty"))
		return
	}

	sendReply(finalResponse)
}

func syncChannelIncomingState(
	app *application.App,
	convService *conversations.ConversationsService,
	conversationID int64,
	agentID int64,
	content string,
	senderName string,
) {
	preview := chat.CleanOpenClawChannelUserMessage(strings.TrimSpace(content))
	senderName = strings.TrimSpace(senderName)
	if senderName != "" {
		prefix := senderName + "："
		preview = strings.TrimSpace(strings.TrimPrefix(preview, prefix))
	}

	if preview != "" {
		_, _ = convService.UpdateConversation(conversationID, conversations.UpdateConversationInput{
			LastMessage: &preview,
		})
	}

	app.Event.Emit("conversations:changed", map[string]any{
		"agent_id": agentID,
	})
	emitConversationMessagesChanged(app, conversationID)
	// The OpenClaw gateway transcript may lag behind the generation events.
	// Re-emit a few refreshes so the assistant UI can pick up late-arriving
	// user/assistant turns without waiting for another manual refresh.
	scheduleConversationMessageRefreshes(app, conversationID, 800*time.Millisecond, 2*time.Second, 4*time.Second)
}

// streamFeishuReply creates a Feishu streaming card and pushes
// incremental content updates as the OpenClaw agent generates tokens.
func streamFeishuReply(
	app *application.App,
	chatService *chat.ChatService,
	convService *conversations.ConversationsService,
	conversationID int64,
	agentID int64,
	requestID string,
	adapter feishuStreamingAdapter,
	msg channels.IncomingMessage,
	replyTarget string,
) {
	createCtx, createCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer createCancel()

	placeholder := i18n.T("channel.feishu_streaming_generating")
	handle, err := adapter.CreateStreamCardMessage(createCtx, replyTarget, msg.MessageID, placeholder)
	if err != nil {
		app.Logger.Warn("openclaw channel: create stream card failed, falling back to plain reply",
			"conv", conversationID, "error", err)
		_ = chatService.WaitForGeneration(conversationID, requestID)
		resp := finalResponse(chatService, conversationID, requestID)
		updateConversationMeta(app, convService, conversationID, agentID, resp)
		return
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
			if waitErr != nil {
				app.Logger.Error("openclaw channel: generation wait failed", "conv", conversationID, "error", waitErr)
			}

			resp := finalResponse(chatService, conversationID, requestID)
			if strings.TrimSpace(resp) == "" {
				resp = i18n.T("error.channel_ai_reply_empty")
			}

			updateCtx, updateCancel := context.WithTimeout(context.Background(), 10*time.Second)
			if updateErr := adapter.UpdateStreamCardMessage(updateCtx, handle, resp, true); updateErr != nil {
				app.Logger.Error("openclaw channel: final stream update failed",
					"conv", conversationID, "error", updateErr)
			}
			updateCancel()

			updateConversationMeta(app, convService, conversationID, agentID, resp)
			return

		case <-ticker.C:
			current, ok := chatService.GetGenerationContent(conversationID, requestID)
			if !ok || strings.TrimSpace(current) == "" || current == lastSent {
				continue
			}

			updateCtx, updateCancel := context.WithTimeout(context.Background(), 10*time.Second)
			if updateErr := adapter.UpdateStreamCardMessage(updateCtx, handle, current, false); updateErr != nil {
				app.Logger.Warn("openclaw channel: stream update failed",
					"conv", conversationID, "error", updateErr)
				updateCancel()
				continue
			}
			updateCancel()

			lastSent = current
		}
	}
}

// finalResponse retrieves the final response text for an OpenClaw generation.
// It first checks the in-memory generation content, then falls back to sessions.get.
func finalResponse(chatService *chat.ChatService, conversationID int64, requestID string) string {
	if content, ok := chatService.GetGenerationContent(conversationID, requestID); ok && strings.TrimSpace(content) != "" {
		return strings.TrimSpace(content)
	}
	if reply := chatService.GetOpenClawLastAssistantReply(conversationID); reply != "" {
		return strings.TrimSpace(reply)
	}
	return ""
}

// updateConversationMeta updates conversation metadata and notifies the frontend.
func updateConversationMeta(
	app *application.App,
	convService *conversations.ConversationsService,
	conversationID int64,
	agentID int64,
	lastMessage string,
) {
	_, _ = convService.UpdateConversation(conversationID, conversations.UpdateConversationInput{
		LastMessage: &lastMessage,
	})
	app.Event.Emit("conversations:changed", map[string]any{
		"agent_id": agentID,
	})
	emitConversationMessagesChanged(app, conversationID)
	scheduleConversationMessageRefreshes(app, conversationID, 600*time.Millisecond, 1500*time.Millisecond)
}

func emitConversationMessagesChanged(app *application.App, conversationID int64) {
	app.Event.Emit("chat:messages-changed", map[string]any{
		"conversation_id": conversationID,
	})
}

func scheduleConversationMessageRefreshes(app *application.App, conversationID int64, delays ...time.Duration) {
	if len(delays) == 0 {
		return
	}
	go func(convID int64, refreshDelays []time.Duration) {
		for _, delay := range refreshDelays {
			if delay > 0 {
				time.Sleep(delay)
			}
			emitConversationMessagesChanged(app, convID)
		}
	}(conversationID, append([]time.Duration(nil), delays...))
}

func feishuStreamOutputEnabled(extraConfig string) bool {
	cfg, err := channels.ParseFeishuConfig(extraConfig)
	if err != nil {
		return false
	}
	return cfg.StreamOutputEnabledOrDefault()
}
