package channels

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkauth "github.com/larksuite/oapi-sdk-go/v3/service/auth/v3"
	larkcardkit "github.com/larksuite/oapi-sdk-go/v3/service/cardkit/v1"
	larkcontact "github.com/larksuite/oapi-sdk-go/v3/service/contact/v3"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
)

func init() {
	RegisterAdapter(PlatformFeishu, func() PlatformAdapter {
		return &FeishuAdapter{}
	})
}

// FeishuConfig contains credentials for a Feishu/Lark bot.
type FeishuConfig struct {
	AppID               string `json:"app_id"`
	AppSecret           string `json:"app_secret"`
	StreamOutputEnabled *bool  `json:"stream_output_enabled,omitempty"`
}

const (
	feishuStreamCardType          = "card_json"
	feishuStreamCardElementID     = "agent_reply_md"
	feishuStreamCardSummaryMaxLen = 120
)

// FeishuStreamCardHandle tracks the card entity used for streaming replies.
type FeishuStreamCardHandle struct {
	MessageID string
	CardID    string
	sequence  int
}

func (h *FeishuStreamCardHandle) nextSequence() int {
	h.sequence++
	return h.sequence
}

func ParseFeishuConfig(configJSON string) (FeishuConfig, error) {
	var cfg FeishuConfig
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return FeishuConfig{}, fmt.Errorf("parse feishu config: %w", err)
	}
	return cfg, nil
}

func (c FeishuConfig) StreamOutputEnabledOrDefault() bool {
	return c.StreamOutputEnabled == nil || *c.StreamOutputEnabled
}

// FeishuAdapter implements PlatformAdapter for Feishu/Lark using WebSocket (长连接).
type FeishuAdapter struct {
	mu        sync.Mutex
	client    *lark.Client
	wsClient  *larkws.Client
	connected atomic.Bool
	cancel    context.CancelFunc
	channelID int64
	handler   MessageHandler
	config    FeishuConfig
	nameCache sync.Map // openID -> display name
	chatCache sync.Map // chatID -> chat name
	seenMsgs  sync.Map // messageID -> struct{}, dedup within TTL
}

func (a *FeishuAdapter) Platform() string { return PlatformFeishu }

func (a *FeishuAdapter) Connect(ctx context.Context, channelID int64, configJSON string, handler MessageHandler) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	cfg, err := ParseFeishuConfig(configJSON)
	if err != nil {
		return err
	}
	if cfg.AppID == "" || cfg.AppSecret == "" {
		return fmt.Errorf("feishu config: app_id and app_secret are required")
	}

	a.config = cfg
	a.channelID = channelID
	a.handler = handler

	a.client = lark.NewClient(cfg.AppID, cfg.AppSecret)

	// Verify credentials by fetching tenant access token
	if err := a.verifyCredentials(ctx); err != nil {
		return fmt.Errorf("feishu credentials verification failed: %w", err)
	}

	eventHandler := dispatcher.NewEventDispatcher("", "").
		OnP2MessageReceiveV1(a.onMessageReceive)

	wsClient := larkws.NewClient(cfg.AppID, cfg.AppSecret,
		larkws.WithEventHandler(eventHandler),
		larkws.WithLogLevel(larkcore.LogLevelInfo),
	)

	connCtx, cancel := context.WithCancel(ctx)
	a.cancel = cancel
	a.wsClient = wsClient

	go func() {
		err := wsClient.Start(connCtx)
		if err != nil {
			slog.Error("[feishu] websocket connection error", "error", err)
			a.connected.Store(false)
			return
		}
	}()

	a.connected.Store(true)
	return nil
}

// verifyCredentials checks if the app_id and app_secret are valid by requesting a tenant access token.
func (a *FeishuAdapter) verifyCredentials(ctx context.Context) error {
	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Use the auth API to verify credentials by explicitly requesting a tenant access token
	req := larkauth.NewInternalTenantAccessTokenReqBuilder().
		Body(larkauth.NewInternalTenantAccessTokenReqBodyBuilder().
			AppId(a.config.AppID).
			AppSecret(a.config.AppSecret).
			Build()).
		Build()

	resp, err := a.client.Auth.TenantAccessToken.Internal(reqCtx, req)
	if err != nil {
		return fmt.Errorf("feishu auth request failed: %w", err)
	}
	if !resp.Success() {
		return fmt.Errorf("invalid app_id or app_secret (code: %d, msg: %s)", resp.Code, resp.Msg)
	}

	return nil
}

func (a *FeishuAdapter) onMessageReceive(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
	// Guard: SDK ws client does not stop on context cancel in current version.
	// Ignore all events after adapter is marked disconnected.
	if !a.connected.Load() {
		return nil
	}

	if a.handler == nil || event == nil || event.Event == nil || event.Event.Message == nil {
		return nil
	}

	msg := event.Event.Message
	sender := event.Event.Sender

	// Skip only app-originated messages (bot self / echoed replies). Feishu may omit
	// sender_type or send values like "unknown" in some group payloads; requiring
	// exactly "user" would drop those legitimate messages.
	if sender != nil && deref(sender.SenderType) == "app" {
		slog.Info("[feishu] skipping app-originated message")
		return nil
	}

	messageID := deref(msg.MessageId)

	// Guard: deduplicate by message_id (Feishu may deliver at-least-once).
	if messageID != "" {
		if _, loaded := a.seenMsgs.LoadOrStore(messageID, struct{}{}); loaded {
			slog.Info("[feishu] duplicate message_id, skipping", "message_id", messageID)
			return nil
		}
		// Auto-expire after 5 minutes to bound memory.
		go func() {
			time.Sleep(5 * time.Minute)
			a.seenMsgs.Delete(messageID)
		}()
	}

	content := deref(msg.Content)
	msgType := deref(msg.MessageType)
	chatID := deref(msg.ChatId)

	senderID := ""
	if sender != nil && sender.SenderId != nil {
		senderID = deref(sender.SenderId.OpenId)
	}

	fmt.Printf("[Feishu] 收到消息 - message_id: %s, 发送者: %s, 群聊: %s, 类型: %s, 内容: %s\n",
		messageID, senderID, chatID, msgType, content)

	senderName := a.resolveSenderName(ctx, senderID)
	chatName := a.resolveChatName(ctx, chatID)

	chatType := deref(msg.ChatType)

	a.handler(IncomingMessage{
		ChannelID:  a.channelID,
		Platform:   PlatformFeishu,
		MessageID:  messageID,
		SenderID:   senderID,
		SenderName: senderName,
		ChatID:     chatID,
		ChatName:   chatName,
		IsGroup:    chatType == "group" || chatType == "topic_group",
		Content:    content,
		MsgType:    msgType,
		RawData:    content,
	})

	return nil
}

// resolveSenderName fetches the display name for a Feishu user via the Contact API.
// Results are cached in nameCache; failures are silently ignored.
func (a *FeishuAdapter) resolveSenderName(ctx context.Context, openID string) string {
	if openID == "" {
		return ""
	}

	if cached, ok := a.nameCache.Load(openID); ok {
		return cached.(string)
	}

	a.mu.Lock()
	client := a.client
	a.mu.Unlock()

	if client == nil {
		return ""
	}

	req := larkcontact.NewGetUserReqBuilder().
		UserId(openID).
		UserIdType(larkcontact.UserIdTypeGetUserOpenId).
		Build()

	resp, err := client.Contact.User.Get(ctx, req)
	if err != nil || !resp.Success() || resp.Data == nil || resp.Data.User == nil {
		return ""
	}

	name := deref(resp.Data.User.Name)
	if name == "" {
		name = deref(resp.Data.User.Nickname)
	}
	if name != "" {
		a.nameCache.Store(openID, name)
	}
	return name
}

// resolveChatName fetches the display name for a Feishu chat/group via the IM API.
// Results are cached in chatCache; failures are silently ignored.
func (a *FeishuAdapter) resolveChatName(ctx context.Context, chatID string) string {
	if chatID == "" {
		return ""
	}

	if cached, ok := a.chatCache.Load(chatID); ok {
		return cached.(string)
	}

	a.mu.Lock()
	client := a.client
	a.mu.Unlock()

	if client == nil {
		return ""
	}

	req := larkim.NewGetChatReqBuilder().
		ChatId(chatID).
		Build()

	resp, err := client.Im.Chat.Get(ctx, req)
	if err != nil || !resp.Success() || resp.Data == nil {
		slog.Info("[feishu] failed to get chat info", "chat_id", chatID, "error", err)
		return ""
	}

	name := deref(resp.Data.Name)
	if name != "" {
		a.chatCache.Store(chatID, name)
	}
	return name
}

// isTokenExpiredCode returns true for Feishu error codes that indicate a stale/missing access token.
func isTokenExpiredCode(code int) bool {
	switch code {
	case 99991661, // access_token expired
		99991663, // invalid access token / not attached
		99991664: // token not found
		return true
	}
	return false
}

// rebuildClient creates a fresh lark.Client, forcing the SDK to obtain a new tenant access token.
func (a *FeishuAdapter) rebuildClient() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.client = lark.NewClient(a.config.AppID, a.config.AppSecret)
	slog.Info("[feishu] rebuilt lark client to refresh access token", "channel_id", a.channelID)
}

// withTokenRetry executes fn with the current lark.Client. If the Feishu API returns
// a token-expired error code, it rebuilds the client and retries once.
func (a *FeishuAdapter) withTokenRetry(ctx context.Context, fn func(client *lark.Client) (apiCode int, err error)) error {
	a.mu.Lock()
	client := a.client
	a.mu.Unlock()

	if client == nil {
		return fmt.Errorf("feishu client not initialized")
	}

	code, err := fn(client)
	if err != nil && isTokenExpiredCode(code) {
		slog.Warn("[feishu] token expired, rebuilding client and retrying", "code", code, "channel_id", a.channelID)
		a.rebuildClient()

		a.mu.Lock()
		client = a.client
		a.mu.Unlock()

		_, err = fn(client)
	}
	return err
}

func (a *FeishuAdapter) Disconnect(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.cancel != nil {
		a.cancel()
		a.cancel = nil
	}
	a.connected.Store(false)
	a.handler = nil
	return nil
}

func (a *FeishuAdapter) IsConnected() bool {
	return a.connected.Load()
}

func (a *FeishuAdapter) SendMessage(ctx context.Context, targetID string, content string) error {
	_, err := a.sendMessage(ctx, targetID, content)
	return err
}

func (a *FeishuAdapter) SendTextMessage(ctx context.Context, targetID string, text string) (string, error) {
	return a.sendMessage(ctx, targetID, text)
}

func (a *FeishuAdapter) ReplyMessage(ctx context.Context, replyToMessageID string, content string) (string, error) {
	msgType, contentJSON, err := buildFeishuOutgoingMessage(content)
	if err != nil {
		return "", err
	}

	var messageID string
	err = a.withTokenRetry(ctx, func(client *lark.Client) (int, error) {
		req := larkim.NewReplyMessageReqBuilder().
			MessageId(replyToMessageID).
			Body(larkim.NewReplyMessageReqBodyBuilder().
				MsgType(msgType).
				Content(contentJSON).
				Uuid(fmt.Sprintf("feishu_reply_%d", time.Now().UnixNano())).
				Build()).
			Build()

		resp, err := client.Im.Message.Reply(ctx, req)
		if err != nil {
			return 0, fmt.Errorf("reply feishu message: %w", err)
		}
		if !resp.Success() {
			return resp.Code, fmt.Errorf("reply feishu message: code=%d msg=%s", resp.Code, resp.Msg)
		}
		if resp.Data == nil || resp.Data.MessageId == nil {
			return 0, fmt.Errorf("reply feishu message: empty message_id in response")
		}
		messageID = *resp.Data.MessageId
		return 0, nil
	})
	return messageID, err
}

func (a *FeishuAdapter) UpdateTextMessage(ctx context.Context, messageID string, text string) error {
	contentJSON, err := marshalFeishuTextContent(text)
	if err != nil {
		return err
	}

	return a.withTokenRetry(ctx, func(client *lark.Client) (int, error) {
		req := larkim.NewUpdateMessageReqBuilder().
			MessageId(messageID).
			Body(larkim.NewUpdateMessageReqBodyBuilder().
				MsgType(larkim.MsgTypeText).
				Content(contentJSON).
				Build()).
			Build()

		resp, err := client.Im.Message.Update(ctx, req)
		if err != nil {
			return 0, fmt.Errorf("update feishu message: %w", err)
		}
		if !resp.Success() {
			return resp.Code, fmt.Errorf("update feishu message: code=%d msg=%s", resp.Code, resp.Msg)
		}
		return 0, nil
	})
}

func (a *FeishuAdapter) CreateStreamCardMessage(ctx context.Context, targetID string, replyToMessageID string, placeholder string) (*FeishuStreamCardHandle, error) {
	cardID, err := a.createStreamCard(ctx, placeholder)
	if err != nil {
		return nil, err
	}

	contentJSON, err := marshalFeishuCardContent(cardID)
	if err != nil {
		return nil, err
	}

	var messageID string
	if replyToMessageID != "" {
		err = a.withTokenRetry(ctx, func(client *lark.Client) (int, error) {
			req := larkim.NewReplyMessageReqBuilder().
				MessageId(replyToMessageID).
				Body(larkim.NewReplyMessageReqBodyBuilder().
					MsgType(larkim.MsgTypeInteractive).
					Content(contentJSON).
					Uuid(fmt.Sprintf("feishu_stream_reply_%d", time.Now().UnixNano())).
					Build()).
				Build()

			resp, err := client.Im.Message.Reply(ctx, req)
			if err != nil {
				return 0, fmt.Errorf("reply feishu stream card message: %w", err)
			}
			if !resp.Success() {
				return resp.Code, fmt.Errorf("reply feishu stream card message: code=%d msg=%s", resp.Code, resp.Msg)
			}
			if resp.Data == nil || resp.Data.MessageId == nil {
				return 0, fmt.Errorf("reply feishu stream card message: empty message_id in response")
			}
			messageID = *resp.Data.MessageId
			return 0, nil
		})
	} else {
		messageID, err = a.sendInteractiveMessage(ctx, targetID, contentJSON)
	}
	if err != nil {
		return nil, err
	}

	return &FeishuStreamCardHandle{
		MessageID: messageID,
		CardID:    cardID,
	}, nil
}

func (a *FeishuAdapter) UpdateStreamCardMessage(ctx context.Context, handle *FeishuStreamCardHandle, text string, finish bool) error {
	if handle == nil || strings.TrimSpace(handle.CardID) == "" {
		return fmt.Errorf("feishu stream card handle is invalid")
	}
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("feishu stream card content is empty")
	}

	if err := a.updateStreamCardContent(ctx, handle, text); err != nil {
		return err
	}
	if !finish {
		return nil
	}
	return a.finishStreamCard(ctx, handle, text)
}

func (a *FeishuAdapter) createStreamCard(ctx context.Context, text string) (string, error) {
	cardJSON, err := marshalFeishuStreamCardJSON(text)
	if err != nil {
		return "", err
	}

	var cardID string
	err = a.withTokenRetry(ctx, func(client *lark.Client) (int, error) {
		req := larkcardkit.NewCreateCardReqBuilder().
			Body(larkcardkit.NewCreateCardReqBodyBuilder().
				Type(feishuStreamCardType).
				Data(cardJSON).
				Build()).
			Build()

		resp, err := client.Cardkit.V1.Card.Create(ctx, req)
		if err != nil {
			return 0, fmt.Errorf("create feishu stream card: %w", err)
		}
		if !resp.Success() {
			return resp.Code, fmt.Errorf("create feishu stream card: code=%d msg=%s", resp.Code, resp.Msg)
		}
		if resp.Data == nil || resp.Data.CardId == nil {
			return 0, fmt.Errorf("create feishu stream card: empty card_id in response")
		}
		cardID = *resp.Data.CardId
		return 0, nil
	})
	return cardID, err
}

func (a *FeishuAdapter) updateStreamCardContent(ctx context.Context, handle *FeishuStreamCardHandle, text string) error {
	sequence := handle.nextSequence()
	return a.withTokenRetry(ctx, func(client *lark.Client) (int, error) {
		req := larkcardkit.NewContentCardElementReqBuilder().
			CardId(handle.CardID).
			ElementId(feishuStreamCardElementID).
			Body(larkcardkit.NewContentCardElementReqBodyBuilder().
				Uuid(fmt.Sprintf("feishu_stream_content_%d", time.Now().UnixNano())).
				Content(text).
				Sequence(sequence).
				Build()).
			Build()

		resp, err := client.Cardkit.V1.CardElement.Content(ctx, req)
		if err != nil {
			return 0, fmt.Errorf("update feishu stream card content: %w", err)
		}
		if !resp.Success() {
			return resp.Code, fmt.Errorf("update feishu stream card content: code=%d msg=%s", resp.Code, resp.Msg)
		}
		return 0, nil
	})
}

func (a *FeishuAdapter) finishStreamCard(ctx context.Context, handle *FeishuStreamCardHandle, finalText string) error {
	settingsJSON, err := marshalFeishuStreamCardSettings(finalText)
	if err != nil {
		return err
	}

	sequence := handle.nextSequence()
	return a.withTokenRetry(ctx, func(client *lark.Client) (int, error) {
		req := larkcardkit.NewSettingsCardReqBuilder().
			CardId(handle.CardID).
			Body(larkcardkit.NewSettingsCardReqBodyBuilder().
				Settings(settingsJSON).
				Uuid(fmt.Sprintf("feishu_stream_finish_%d", time.Now().UnixNano())).
				Sequence(sequence).
				Build()).
			Build()

		resp, err := client.Cardkit.V1.Card.Settings(ctx, req)
		if err != nil {
			return 0, fmt.Errorf("finish feishu stream card: %w", err)
		}
		if !resp.Success() {
			return resp.Code, fmt.Errorf("finish feishu stream card: code=%d msg=%s", resp.Code, resp.Msg)
		}
		return 0, nil
	})
}

func (a *FeishuAdapter) sendInteractiveMessage(ctx context.Context, targetID string, contentJSON string) (string, error) {
	receiveIDType := larkim.ReceiveIdTypeOpenId
	if len(targetID) > 3 && targetID[:3] == "oc_" {
		receiveIDType = larkim.ReceiveIdTypeChatId
	}

	var messageID string
	err := a.withTokenRetry(ctx, func(client *lark.Client) (int, error) {
		req := larkim.NewCreateMessageReqBuilder().
			ReceiveIdType(receiveIDType).
			Body(larkim.NewCreateMessageReqBodyBuilder().
				ReceiveId(targetID).
				MsgType(larkim.MsgTypeInteractive).
				Content(contentJSON).
				Build()).
			Build()

		resp, err := client.Im.Message.Create(ctx, req)
		if err != nil {
			return 0, fmt.Errorf("send feishu interactive message: %w", err)
		}
		if !resp.Success() {
			return resp.Code, fmt.Errorf("send feishu interactive message: code=%d msg=%s", resp.Code, resp.Msg)
		}
		if resp.Data == nil || resp.Data.MessageId == nil {
			return 0, fmt.Errorf("send feishu interactive message: empty message_id in response")
		}
		messageID = *resp.Data.MessageId
		return 0, nil
	})
	return messageID, err
}

func (a *FeishuAdapter) sendMessage(ctx context.Context, targetID string, content string) (string, error) {
	msgType, contentJSON, err := buildFeishuOutgoingMessage(content)
	if err != nil {
		return "", err
	}

	receiveIDType := larkim.ReceiveIdTypeOpenId
	if len(targetID) > 3 && targetID[:3] == "oc_" {
		receiveIDType = larkim.ReceiveIdTypeChatId
	}

	var messageID string
	err = a.withTokenRetry(ctx, func(client *lark.Client) (int, error) {
		req := larkim.NewCreateMessageReqBuilder().
			ReceiveIdType(receiveIDType).
			Body(larkim.NewCreateMessageReqBodyBuilder().
				ReceiveId(targetID).
				MsgType(msgType).
				Content(contentJSON).
				Build()).
			Build()

		resp, err := client.Im.Message.Create(ctx, req)
		if err != nil {
			return 0, fmt.Errorf("send feishu message: %w", err)
		}
		if !resp.Success() {
			return resp.Code, fmt.Errorf("send feishu message: code=%d msg=%s", resp.Code, resp.Msg)
		}
		if resp.Data == nil || resp.Data.MessageId == nil {
			return 0, fmt.Errorf("send feishu message: empty message_id in response")
		}
		messageID = *resp.Data.MessageId
		return 0, nil
	})
	return messageID, err
}

type feishuOutgoingMessage struct {
	MsgType string          `json:"msg_type"`
	Content json.RawMessage `json:"content"`
	Text    string          `json:"text"`

	ImageKey string `json:"image_key"`
	FileKey  string `json:"file_key"`
	FileName string `json:"file_name"`
	Duration int    `json:"duration"`
}

func buildFeishuOutgoingMessage(raw string) (string, string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", "", fmt.Errorf("feishu message content is empty")
	}

	if !strings.HasPrefix(trimmed, "{") {
		contentJSON, err := marshalFeishuTextContent(raw)
		if err != nil {
			return "", "", err
		}
		return larkim.MsgTypeText, contentJSON, nil
	}

	var payload feishuOutgoingMessage
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		contentJSON, marshalErr := marshalFeishuTextContent(raw)
		if marshalErr != nil {
			return "", "", marshalErr
		}
		return larkim.MsgTypeText, contentJSON, nil
	}

	msgType := strings.TrimSpace(payload.MsgType)
	if msgType == "" {
		contentJSON, err := marshalFeishuTextContent(raw)
		if err != nil {
			return "", "", err
		}
		return larkim.MsgTypeText, contentJSON, nil
	}

	if len(payload.Content) > 0 && string(payload.Content) != "null" {
		switch msgType {
		case "text":
			return larkim.MsgTypeText, string(payload.Content), nil
		case "image":
			return larkim.MsgTypeImage, string(payload.Content), nil
		case "file":
			return larkim.MsgTypeFile, string(payload.Content), nil
		case "audio":
			return larkim.MsgTypeAudio, string(payload.Content), nil
		case "media":
			return larkim.MsgTypeMedia, string(payload.Content), nil
		case "sticker":
			return larkim.MsgTypeSticker, string(payload.Content), nil
		default:
			return "", "", fmt.Errorf("unsupported feishu msg_type: %s", msgType)
		}
	}

	switch msgType {
	case "text":
		text := payload.Text
		if text == "" {
			text = raw
		}
		contentJSON, err := marshalFeishuTextContent(text)
		if err != nil {
			return "", "", err
		}
		return larkim.MsgTypeText, contentJSON, nil
	case "image":
		if strings.TrimSpace(payload.ImageKey) == "" {
			return "", "", fmt.Errorf("feishu image message requires image_key")
		}
		contentBytes, err := json.Marshal(map[string]string{"image_key": payload.ImageKey})
		if err != nil {
			return "", "", fmt.Errorf("marshal feishu image content: %w", err)
		}
		return larkim.MsgTypeImage, string(contentBytes), nil
	case "file":
		if strings.TrimSpace(payload.FileKey) == "" {
			return "", "", fmt.Errorf("feishu file message requires file_key")
		}
		contentBytes, err := json.Marshal(map[string]string{"file_key": payload.FileKey})
		if err != nil {
			return "", "", fmt.Errorf("marshal feishu file content: %w", err)
		}
		return larkim.MsgTypeFile, string(contentBytes), nil
	case "audio":
		if strings.TrimSpace(payload.FileKey) == "" {
			return "", "", fmt.Errorf("feishu audio message requires file_key")
		}
		contentBytes, err := json.Marshal(map[string]string{"file_key": payload.FileKey})
		if err != nil {
			return "", "", fmt.Errorf("marshal feishu audio content: %w", err)
		}
		return larkim.MsgTypeAudio, string(contentBytes), nil
	case "media":
		if strings.TrimSpace(payload.FileKey) == "" {
			return "", "", fmt.Errorf("feishu media message requires file_key")
		}
		mediaPayload := map[string]any{
			"file_key": payload.FileKey,
		}
		if payload.ImageKey != "" {
			mediaPayload["image_key"] = payload.ImageKey
		}
		if payload.FileName != "" {
			mediaPayload["file_name"] = payload.FileName
		}
		if payload.Duration > 0 {
			mediaPayload["duration"] = payload.Duration
		}
		contentBytes, err := json.Marshal(mediaPayload)
		if err != nil {
			return "", "", fmt.Errorf("marshal feishu media content: %w", err)
		}
		return larkim.MsgTypeMedia, string(contentBytes), nil
	case "sticker":
		if strings.TrimSpace(payload.FileKey) == "" {
			return "", "", fmt.Errorf("feishu sticker message requires file_key")
		}
		contentBytes, err := json.Marshal(map[string]string{"file_key": payload.FileKey})
		if err != nil {
			return "", "", fmt.Errorf("marshal feishu sticker content: %w", err)
		}
		return larkim.MsgTypeSticker, string(contentBytes), nil
	default:
		return "", "", fmt.Errorf("unsupported feishu msg_type: %s", msgType)
	}
}

func marshalFeishuTextContent(text string) (string, error) {
	contentBytes, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		return "", fmt.Errorf("marshal feishu text content: %w", err)
	}
	return string(contentBytes), nil
}

func marshalFeishuCardContent(cardID string) (string, error) {
	contentBytes, err := json.Marshal(map[string]any{
		"type": "card",
		"data": map[string]string{
			"card_id": cardID,
		},
	})
	if err != nil {
		return "", fmt.Errorf("marshal feishu card content: %w", err)
	}
	return string(contentBytes), nil
}

func marshalFeishuStreamCardJSON(text string) (string, error) {
	cardBytes, err := json.Marshal(map[string]any{
		"schema": "2.0",
		"config": map[string]any{
			"streaming_mode": true,
			"summary": map[string]string{
				"content": "",
			},
			"streaming_config": map[string]any{
				"print_frequency_ms": map[string]int{"default": 70},
				"print_step":         map[string]int{"default": 1},
				"print_strategy":     "fast",
			},
		},
		"body": map[string]any{
			"elements": []map[string]string{
				{
					"tag":        "markdown",
					"content":    text,
					"element_id": feishuStreamCardElementID,
				},
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("marshal feishu stream card json: %w", err)
	}
	return string(cardBytes), nil
}

func marshalFeishuStreamCardSettings(finalText string) (string, error) {
	settingsBytes, err := json.Marshal(map[string]any{
		"config": map[string]any{
			"streaming_mode": false,
			"summary": map[string]string{
				"content": buildFeishuStreamCardSummary(finalText),
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("marshal feishu stream card settings: %w", err)
	}
	return string(settingsBytes), nil
}

func buildFeishuStreamCardSummary(text string) string {
	cleaned := strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if cleaned == "" {
		return ""
	}
	runes := []rune(cleaned)
	if len(runes) <= feishuStreamCardSummaryMaxLen {
		return cleaned
	}
	return string(runes[:feishuStreamCardSummaryMaxLen]) + "..."
}

// UploadFile uploads a local file to Feishu and returns the file_key.
// fileType: opus, mp4, pdf, doc, xls, ppt, stream (default "stream").
func (a *FeishuAdapter) UploadFile(ctx context.Context, filePath string, fileType string) (string, error) {
	if fileType == "" {
		fileType = "stream"
	}

	fileName := filepath.Base(filePath)

	var fileKey string
	err := a.withTokenRetry(ctx, func(client *lark.Client) (int, error) {
		body, err := larkim.NewCreateFilePathReqBodyBuilder().
			FileType(fileType).
			FileName(fileName).
			FilePath(filePath).
			Build()
		if err != nil {
			return 0, fmt.Errorf("build file upload body: %w", err)
		}

		req := larkim.NewCreateFileReqBuilder().Body(body).Build()
		resp, err := client.Im.File.Create(ctx, req)
		if err != nil {
			return 0, fmt.Errorf("upload file to feishu: %w", err)
		}
		if !resp.Success() {
			return resp.Code, fmt.Errorf("upload file to feishu: code=%d msg=%s", resp.Code, resp.Msg)
		}
		if resp.Data == nil || resp.Data.FileKey == nil {
			return 0, fmt.Errorf("upload file to feishu: empty file_key in response")
		}
		fileKey = *resp.Data.FileKey
		return 0, nil
	})
	return fileKey, err
}

// UploadImage uploads a local image to Feishu and returns the image_key.
func (a *FeishuAdapter) UploadImage(ctx context.Context, imagePath string) (string, error) {
	var imageKey string
	err := a.withTokenRetry(ctx, func(client *lark.Client) (int, error) {
		body, err := larkim.NewCreateImagePathReqBodyBuilder().
			ImageType("message").
			ImagePath(imagePath).
			Build()
		if err != nil {
			return 0, fmt.Errorf("build image upload body: %w", err)
		}

		req := larkim.NewCreateImageReqBuilder().Body(body).Build()
		resp, err := client.Im.Image.Create(ctx, req)
		if err != nil {
			return 0, fmt.Errorf("upload image to feishu: %w", err)
		}
		if !resp.Success() {
			return resp.Code, fmt.Errorf("upload image to feishu: code=%d msg=%s", resp.Code, resp.Msg)
		}
		if resp.Data == nil || resp.Data.ImageKey == nil {
			return 0, fmt.Errorf("upload image to feishu: empty image_key in response")
		}
		imageKey = *resp.Data.ImageKey
		return 0, nil
	})
	return imageKey, err
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
