package channels

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
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
	AppID     string `json:"app_id"`
	AppSecret string `json:"app_secret"`
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

	var cfg FeishuConfig
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return fmt.Errorf("parse feishu config: %w", err)
	}
	if cfg.AppID == "" || cfg.AppSecret == "" {
		return fmt.Errorf("feishu config: app_id and app_secret are required")
	}

	a.config = cfg
	a.channelID = channelID
	a.handler = handler

	a.client = lark.NewClient(cfg.AppID, cfg.AppSecret)

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
			a.connected.Store(false)
			return
		}
	}()

	a.connected.Store(true)
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

	// Guard: skip non-user messages (e.g. bot's own replies echoed back).
	if sender != nil && deref(sender.SenderType) != "user" {
		slog.Info("[feishu] skipping non-user message", "sender_type", deref(sender.SenderType))
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

	senderID := ""
	if sender != nil && sender.SenderId != nil {
		senderID = deref(sender.SenderId.OpenId)
	}

	senderName := a.resolveSenderName(ctx, senderID)

	chatID := deref(msg.ChatId)
	chatName := a.resolveChatName(ctx, chatID)
	content := deref(msg.Content)
	msgType := deref(msg.MessageType)

	fmt.Printf("[Feishu] 收到消息 - 发送者: %s(%s), 群聊: %s(%s), 类型: %s, 内容: %s\n", 
		senderName, senderID, chatName, chatID, msgType, content)

	a.handler(IncomingMessage{
		ChannelID:  a.channelID,
		Platform:   PlatformFeishu,
		MessageID:  messageID,
		SenderID:   senderID,
		SenderName: senderName,
		ChatID:     chatID,
		ChatName:   chatName,
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
	a.mu.Lock()
	client := a.client
	a.mu.Unlock()

	if client == nil {
		return fmt.Errorf("feishu client not initialized")
	}

	// Use json.Marshal for proper escaping of quotes, newlines, etc.
	textPayload := map[string]string{"text": content}
	contentBytes, err := json.Marshal(textPayload)
	if err != nil {
		return fmt.Errorf("marshal feishu content: %w", err)
	}

	// Detect ID type: chat IDs start with "oc_", open IDs start with "ou_"
	receiveIDType := larkim.ReceiveIdTypeOpenId
	if len(targetID) > 3 && targetID[:3] == "oc_" {
		receiveIDType = larkim.ReceiveIdTypeChatId
	}

	req := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType(receiveIDType).
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(targetID).
			MsgType(larkim.MsgTypeText).
			Content(string(contentBytes)).
			Build()).
		Build()

	resp, err := client.Im.Message.Create(ctx, req)
	if err != nil {
		return fmt.Errorf("send feishu message: %w", err)
	}
	if !resp.Success() {
		return fmt.Errorf("send feishu message: code=%d msg=%s", resp.Code, resp.Msg)
	}
	return nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
