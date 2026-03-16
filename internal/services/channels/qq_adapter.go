package channels

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tencent-connect/botgo"
	"github.com/tencent-connect/botgo/dto"
	"github.com/tencent-connect/botgo/event"
	"github.com/tencent-connect/botgo/openapi"
	"github.com/tencent-connect/botgo/token"
)

func init() {
	RegisterAdapter(PlatformQQ, func() PlatformAdapter {
		return &QQAdapter{}
	})
}

// QQConfig contains credentials for a QQ bot.
type QQConfig struct {
	AppID     string `json:"app_id"`
	AppSecret string `json:"app_secret"`
}

// QQAdapter implements PlatformAdapter for QQ Bot using WebSocket.
type QQAdapter struct {
	mu        sync.Mutex
	api       openapi.OpenAPI
	connected atomic.Bool
	cancel    context.CancelFunc
	channelID int64
	handler   MessageHandler
	config    QQConfig
	seenMsgs  sync.Map // messageID -> struct{}, dedup within TTL
	msgSeqMap sync.Map // targetID -> *atomic.Uint32, monotonic sequence per target
}

func (a *QQAdapter) Platform() string { return PlatformQQ }

func (a *QQAdapter) Connect(ctx context.Context, channelID int64, configJSON string, handler MessageHandler) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	var cfg QQConfig
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return fmt.Errorf("parse qq config: %w", err)
	}
	if cfg.AppID == "" || cfg.AppSecret == "" {
		return fmt.Errorf("qq config: app_id and app_secret are required")
	}

	a.config = cfg
	a.channelID = channelID
	a.handler = handler

	tokenSource := token.NewQQBotTokenSource(&token.QQBotCredentials{
		AppID:     cfg.AppID,
		AppSecret: cfg.AppSecret,
	})

	connCtx, cancel := context.WithCancel(ctx)
	a.cancel = cancel

	if err := token.StartRefreshAccessToken(connCtx, tokenSource); err != nil {
		cancel()
		return fmt.Errorf("qq token refresh failed (check app_id/app_secret): %w", err)
	}

	a.api = botgo.NewOpenAPI(cfg.AppID, tokenSource).WithTimeout(10 * time.Second)

	// Register event handlers — these set global DefaultHandlers in the botgo SDK.
	// GROUP_AT_MESSAGE_CREATE (group @bot), C2C_MESSAGE_CREATE (private chat)
	intent := event.RegisterHandlers(
		a.groupATMessageHandler(),
		a.c2cMessageHandler(),
	)

	wsInfo, err := a.api.WS(connCtx, nil, "")
	if err != nil {
		cancel()
		return fmt.Errorf("qq get websocket info failed: %w", err)
	}

	go func() {
		sm := botgo.NewSessionManager()
		if startErr := sm.Start(wsInfo, tokenSource, &intent); startErr != nil {
			slog.Error("[qq] websocket session error", "error", startErr)
			a.connected.Store(false)
		}
	}()

	a.connected.Store(true)
	return nil
}

func (a *QQAdapter) groupATMessageHandler() event.GroupATMessageEventHandler {
	return func(payload *dto.WSPayload, data *dto.WSGroupATMessageData) error {
		if !a.connected.Load() || a.handler == nil || data == nil {
			return nil
		}

		msg := (*dto.Message)(data)
		messageID := msg.ID
		if messageID != "" {
			if _, loaded := a.seenMsgs.LoadOrStore(messageID, struct{}{}); loaded {
				return nil
			}
			go func() {
				time.Sleep(5 * time.Minute)
				a.seenMsgs.Delete(messageID)
			}()
		}

		senderID := ""
		senderName := ""
		if msg.Author != nil {
			senderID = msg.Author.ID
			senderName = msg.Author.Username
		}

		content := strings.TrimSpace(msg.Content)

		slog.Info("[qq] group @bot message received",
			"sender", senderName, "group_id", msg.GroupID, "content", content)

		a.handler(IncomingMessage{
			ChannelID:  a.channelID,
			Platform:   PlatformQQ,
			MessageID:  messageID,
			SenderID:   senderID,
			SenderName: senderName,
			ChatID:     "group:" + msg.GroupID,
			ChatName:   msg.GroupID,
			Content:    content,
			MsgType:    "text",
			RawData:    string(payload.RawMessage),
		})
		return nil
	}
}

func (a *QQAdapter) c2cMessageHandler() event.C2CMessageEventHandler {
	return func(payload *dto.WSPayload, data *dto.WSC2CMessageData) error {
		if !a.connected.Load() || a.handler == nil || data == nil {
			return nil
		}

		msg := (*dto.Message)(data)
		messageID := msg.ID
		if messageID != "" {
			if _, loaded := a.seenMsgs.LoadOrStore(messageID, struct{}{}); loaded {
				return nil
			}
			go func() {
				time.Sleep(5 * time.Minute)
				a.seenMsgs.Delete(messageID)
			}()
		}

		senderID := ""
		senderName := ""
		if msg.Author != nil {
			senderID = msg.Author.ID
			senderName = msg.Author.Username
		}

		content := strings.TrimSpace(msg.Content)

		slog.Info("[qq] C2C message received",
			"sender", senderName, "sender_id", senderID, "content", content)

		a.handler(IncomingMessage{
			ChannelID:  a.channelID,
			Platform:   PlatformQQ,
			MessageID:  messageID,
			SenderID:   senderID,
			SenderName: senderName,
			ChatID:     "user:" + senderID,
			ChatName:   senderName,
			Content:    content,
			MsgType:    "text",
			RawData:    string(payload.RawMessage),
		})
		return nil
	}
}

func (a *QQAdapter) Disconnect(ctx context.Context) error {
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

func (a *QQAdapter) IsConnected() bool {
	return a.connected.Load()
}

// nextMsgSeq returns a monotonically increasing msg_seq per target to avoid dedup rejection.
func (a *QQAdapter) nextMsgSeq(targetID string) uint32 {
	val, _ := a.msgSeqMap.LoadOrStore(targetID, &atomic.Uint32{})
	counter := val.(*atomic.Uint32)
	return counter.Add(1)
}

// SendMessage sends a text message to a QQ group or C2C user.
// targetID format: "group:{groupOpenID}" for groups, "user:{userOpenID}" for C2C.
func (a *QQAdapter) SendMessage(ctx context.Context, targetID string, content string) error {
	a.mu.Lock()
	api := a.api
	a.mu.Unlock()

	if api == nil {
		return fmt.Errorf("qq api not initialized")
	}

	content = strings.TrimSpace(content)
	if content == "" {
		return fmt.Errorf("qq message content is empty")
	}

	if strings.HasPrefix(targetID, "group:") {
		groupID := strings.TrimPrefix(targetID, "group:")
		return a.sendGroupMessage(ctx, api, groupID, content)
	}
	if strings.HasPrefix(targetID, "user:") {
		userID := strings.TrimPrefix(targetID, "user:")
		return a.sendC2CMessage(ctx, api, userID, content)
	}

	return fmt.Errorf("qq target_id must have 'group:' or 'user:' prefix, got: %s", targetID)
}

func (a *QQAdapter) sendGroupMessage(ctx context.Context, api openapi.OpenAPI, groupID, content string) error {
	msg := dto.MessageToCreate{
		Content: content,
		MsgType: dto.TextMsg,
		MsgSeq:  a.nextMsgSeq("group:" + groupID),
	}
	_, err := api.PostGroupMessage(ctx, groupID, msg)
	if err != nil {
		return fmt.Errorf("send qq group message: %w", err)
	}
	return nil
}

func (a *QQAdapter) sendC2CMessage(ctx context.Context, api openapi.OpenAPI, userID, content string) error {
	msg := dto.MessageToCreate{
		Content: content,
		MsgType: dto.TextMsg,
		MsgSeq:  a.nextMsgSeq("user:" + userID),
	}
	_, err := api.PostC2CMessage(ctx, userID, msg)
	if err != nil {
		return fmt.Errorf("send qq c2c message: %w", err)
	}
	return nil
}
