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
	fmt.Print("resp: ", resp, "err: ", err)
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

	msgType, contentJSON, err := buildFeishuOutgoingMessage(content)
	if err != nil {
		return err
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
			MsgType(msgType).
			Content(contentJSON).
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
		contentBytes, err := json.Marshal(map[string]string{"text": raw})
		if err != nil {
			return "", "", fmt.Errorf("marshal feishu text content: %w", err)
		}
		return larkim.MsgTypeText, string(contentBytes), nil
	}

	var payload feishuOutgoingMessage
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		contentBytes, marshalErr := json.Marshal(map[string]string{"text": raw})
		if marshalErr != nil {
			return "", "", fmt.Errorf("marshal feishu text content: %w", marshalErr)
		}
		return larkim.MsgTypeText, string(contentBytes), nil
	}

	msgType := strings.TrimSpace(payload.MsgType)
	if msgType == "" {
		contentBytes, err := json.Marshal(map[string]string{"text": raw})
		if err != nil {
			return "", "", fmt.Errorf("marshal feishu text content: %w", err)
		}
		return larkim.MsgTypeText, string(contentBytes), nil
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
		contentBytes, err := json.Marshal(map[string]string{"text": text})
		if err != nil {
			return "", "", fmt.Errorf("marshal feishu text content: %w", err)
		}
		return larkim.MsgTypeText, string(contentBytes), nil
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

// UploadFile uploads a local file to Feishu and returns the file_key.
// fileType: opus, mp4, pdf, doc, xls, ppt, stream (default "stream").
func (a *FeishuAdapter) UploadFile(ctx context.Context, filePath string, fileType string) (string, error) {
	a.mu.Lock()
	client := a.client
	a.mu.Unlock()

	if client == nil {
		return "", fmt.Errorf("feishu client not initialized")
	}

	if fileType == "" {
		fileType = "stream"
	}

	fileName := filepath.Base(filePath)

	body, err := larkim.NewCreateFilePathReqBodyBuilder().
		FileType(fileType).
		FileName(fileName).
		FilePath(filePath).
		Build()
	if err != nil {
		return "", fmt.Errorf("build file upload body: %w", err)
	}

	req := larkim.NewCreateFileReqBuilder().Body(body).Build()
	resp, err := client.Im.File.Create(ctx, req)
	if err != nil {
		return "", fmt.Errorf("upload file to feishu: %w", err)
	}
	if !resp.Success() {
		return "", fmt.Errorf("upload file to feishu: code=%d msg=%s", resp.Code, resp.Msg)
	}
	if resp.Data == nil || resp.Data.FileKey == nil {
		return "", fmt.Errorf("upload file to feishu: empty file_key in response")
	}
	return *resp.Data.FileKey, nil
}

// UploadImage uploads a local image to Feishu and returns the image_key.
func (a *FeishuAdapter) UploadImage(ctx context.Context, imagePath string) (string, error) {
	a.mu.Lock()
	client := a.client
	a.mu.Unlock()

	if client == nil {
		return "", fmt.Errorf("feishu client not initialized")
	}

	body, err := larkim.NewCreateImagePathReqBodyBuilder().
		ImageType("message").
		ImagePath(imagePath).
		Build()
	if err != nil {
		return "", fmt.Errorf("build image upload body: %w", err)
	}

	req := larkim.NewCreateImageReqBuilder().Body(body).Build()
	resp, err := client.Im.Image.Create(ctx, req)
	if err != nil {
		return "", fmt.Errorf("upload image to feishu: %w", err)
	}
	if !resp.Success() {
		return "", fmt.Errorf("upload image to feishu: code=%d msg=%s", resp.Code, resp.Msg)
	}
	if resp.Data == nil || resp.Data.ImageKey == nil {
		return "", fmt.Errorf("upload image to feishu: empty image_key in response")
	}
	return *resp.Data.ImageKey, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
