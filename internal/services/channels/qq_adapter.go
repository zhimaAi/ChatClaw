package channels

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"chatclaw/internal/services/oss"

	"github.com/tencent-connect/botgo"
	"github.com/tencent-connect/botgo/constant"
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

// qqTransportAPI is the minimal API needed for file upload; allows tests to mock Transport only.
type qqTransportAPI interface {
	Transport(ctx context.Context, method, url string, body interface{}) ([]byte, error)
}

var qqMediaURLUploader = oss.UploadImage

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
			ChatName:   "",
			IsGroup:    true,
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

// qqOutgoingPayload is the JSON shape for rich/markdown messages (same convention as Feishu).
// For image/file: prefer upload-then-send (srv_send_msg=false, recommended by QQ API).
// "url" accepts a public HTTP/HTTPS URL or a local file path; local files are uploaded
// to the configured OSS provider first. "file_path" is the explicit local-path form.
// "file_info" can be used when you already have file_info from a previous upload.
type qqOutgoingPayload struct {
	MsgType    string `json:"msg_type"`
	Content    string `json:"content"`
	Text       string `json:"text"`
	URL        string `json:"url"`
	FilePath   string `json:"file_path"`
	FileInfo   string `json:"file_info"`    // pre-obtained from upload API
	SrvSendMsg *bool  `json:"srv_send_msg"` // kept for backward compatibility; rich media is always sent with srv_send_msg=false
	MsgID      string `json:"msg_id"`       // id of message to reply to; empty = active message, non-empty = passive reply
}

// qqFileUploadResponse is the response from POST /v2/groups/{id}/files or /v2/users/{id}/files when srv_send_msg=false.
type qqFileUploadResponse struct {
	FileInfo string `json:"file_info"`
	FileUUID string `json:"file_uuid"`
	TTL      int    `json:"ttl"`
}

// qqSendInput holds either a ready-to-send message or params for upload-then-send.
type qqSendInput struct {
	Msg        dto.APIMessage
	NeedUpload bool
	UploadURL  string
	FileType   uint64
	MsgSeq     uint32
	MsgID      string // id of message to reply to; empty = active, non-empty = passive reply
}

// InjectQQMsgID ensures the outgoing content carries the given msgID for QQ passive replies.
// If content is plain text, it is wrapped into a JSON text payload with msg_id set.
// If content is already JSON, msg_id is added unless it is already present.
// This must be called before SendMessage when replying to a user-initiated QQ message.
func InjectQQMsgID(content, msgID string) string {
	if msgID == "" {
		return content
	}
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return content
	}

	if strings.HasPrefix(trimmed, "{") {
		var m map[string]any
		if err := json.Unmarshal([]byte(trimmed), &m); err == nil {
			if existing, ok := m["msg_id"]; !ok || existing == "" {
				m["msg_id"] = msgID
			}
			if b, err := json.Marshal(m); err == nil {
				return string(b)
			}
		}
		return content
	}

	// Plain text — wrap as a JSON text payload with msg_id.
	b, _ := json.Marshal(map[string]any{
		"msg_type": "text",
		"text":     trimmed,
		"msg_id":   msgID,
	})
	return string(b)
}

// buildQQOutgoingMessage parses content (plain text or JSON with msg_type) and returns qqSendInput.
// For image/file: use url, file_path, or file_info from prior upload. Local paths are uploaded
// to the configured OSS provider first so QQ always receives a public URL.
func buildQQOutgoingMessage(ctx context.Context, content string, nextSeq uint32) (*qqSendInput, error) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return nil, fmt.Errorf("qq message content is empty")
	}

	// Plain text: no JSON (active message, no MsgID)
	if !strings.HasPrefix(trimmed, "{") {
		return &qqSendInput{
			Msg: &dto.MessageToCreate{
				Content: trimmed,
				MsgType: dto.TextMsg,
				MsgSeq:  nextSeq,
			},
		}, nil
	}

	var payload qqOutgoingPayload
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return &qqSendInput{
			Msg: &dto.MessageToCreate{
				Content: content,
				MsgType: dto.TextMsg,
				MsgSeq:  nextSeq,
			},
		}, nil
	}

	msgID := strings.TrimSpace(payload.MsgID)
	msgType := strings.TrimSpace(strings.ToLower(payload.MsgType))
	if msgType == "" {
		msgType = "text"
	}

	switch msgType {
	case "text":
		body := payload.Text
		if body == "" {
			body = payload.Content
		}
		if body == "" {
			body = trimmed
		}
		return &qqSendInput{
			MsgID: msgID,
			Msg: &dto.MessageToCreate{
				Content: body,
				MsgType: dto.TextMsg,
				MsgSeq:  nextSeq,
				MsgID:   msgID,
			},
		}, nil
	case "markdown":
		body := payload.Content
		if body == "" {
			body = payload.Text
		}
		if body == "" {
			return nil, fmt.Errorf("qq markdown message requires content or text")
		}
		return &qqSendInput{
			MsgID: msgID,
			Msg: &dto.MessageToCreate{
				MsgType:  dto.MarkdownMsg,
				MsgSeq:   nextSeq,
				MsgID:    msgID,
				Markdown: &dto.Markdown{Content: body},
			},
		}, nil
	case "image":
		return buildQQRichMediaInput(ctx, payload, msgID, nextSeq, 1, "image")
	case "file":
		return buildQQRichMediaInput(ctx, payload, msgID, nextSeq, 4, "file")
	default:
		return nil, fmt.Errorf("qq unsupported msg_type: %s", msgType)
	}
}

func isQQNetworkURL(raw string) bool {
	return strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://")
}

func isQQLoopbackHost(host string) bool {
	host = strings.TrimSpace(strings.ToLower(host))
	switch host {
	case "", "localhost", "127.0.0.1", "0.0.0.0", "::1":
		return true
	default:
		return false
	}
}

func resolveQQMediaPublicURL(ctx context.Context, raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", fmt.Errorf("qq media url is empty")
	}
	if isQQNetworkURL(value) {
		parsed, err := url.Parse(value)
		if err != nil {
			return "", fmt.Errorf("parse qq media url: %w", err)
		}
		if isQQLoopbackHost(parsed.Hostname()) {
			return "", fmt.Errorf("qq media url must be publicly accessible, got local URL: %s", value)
		}
		return value, nil
	}
	if _, err := os.Stat(value); err != nil {
		return "", fmt.Errorf("qq media file not accessible: %w", err)
	}

	uploadedURL, err := qqMediaURLUploader(ctx, value)
	if err != nil {
		return "", fmt.Errorf("upload qq local media to OSS: %w", err)
	}
	return uploadedURL, nil
}

// buildQQRichMediaInput builds qqSendInput for image (fileType=1) or file (fileType=4).
// QQ rich media prefers upload-then-send (srv_send_msg=false) as recommended by the QQ API:
// media is uploaded first to get file_info, then sent via the message API (msg_type=7).
// If that path fails, SendMessage falls back to direct send with srv_send_msg=true.
func buildQQRichMediaInput(ctx context.Context, payload qqOutgoingPayload, msgID string, nextSeq uint32, fileType uint64, label string) (*qqSendInput, error) {
	fileInfo := strings.TrimSpace(payload.FileInfo)
	mediaRef := strings.TrimSpace(payload.FilePath)
	if mediaRef == "" {
		mediaRef = strings.TrimSpace(payload.URL)
	}

	if fileInfo != "" {
		return &qqSendInput{
			MsgID: msgID,
			Msg: &dto.MessageToCreate{
				MsgType: dto.RichMediaMsg,
				MsgSeq:  nextSeq,
				MsgID:   msgID,
				Media:   &dto.MediaInfo{FileInfo: []byte(fileInfo)},
			},
		}, nil
	}
	if mediaRef == "" {
		return nil, fmt.Errorf("qq %s message requires url, file_path, or file_info", label)
	}
	mediaURL, err := resolveQQMediaPublicURL(ctx, mediaRef)
	if err != nil {
		return nil, fmt.Errorf("qq resolve %s media url: %w", label, err)
	}

	return &qqSendInput{NeedUpload: true, UploadURL: mediaURL, FileType: fileType, MsgSeq: nextSeq, MsgID: msgID}, nil
}

// SendMessage sends a message to a QQ group or C2C user.
// targetID format: "group:{groupOpenID}" for groups, "user:{userOpenID}" for C2C.
// content can be: plain text; or JSON with msg_type (text, markdown, image, file).
// For image/file use a public URL, a local file_path, or file_info.
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

// uploadGroupFile uploads media to QQ group files endpoint (srv_send_msg=false) and returns file_info for later send.
func (a *QQAdapter) uploadGroupFile(ctx context.Context, api qqTransportAPI, groupID string, fileType uint64, url string) ([]byte, error) {
	body := &dto.RichMediaMessage{
		FileType:   fileType,
		URL:        url,
		SrvSendMsg: false,
	}
	fullURL := constant.APIDomain + "/v2/groups/" + groupID + "/files"
	respBody, err := api.Transport(ctx, http.MethodPost, fullURL, body)
	if err != nil {
		return nil, fmt.Errorf("qq upload group file: %w", err)
	}
	var uploadResp qqFileUploadResponse
	if err := json.Unmarshal(respBody, &uploadResp); err != nil {
		return nil, fmt.Errorf("qq parse upload response: %w", err)
	}
	if uploadResp.FileInfo == "" {
		return nil, fmt.Errorf("qq upload response missing file_info")
	}
	return []byte(uploadResp.FileInfo), nil
}

// uploadC2CFile uploads media to QQ C2C files endpoint (srv_send_msg=false) and returns file_info for later send.
func (a *QQAdapter) uploadC2CFile(ctx context.Context, api qqTransportAPI, userID string, fileType uint64, url string) ([]byte, error) {
	body := &dto.RichMediaMessage{
		FileType:   fileType,
		URL:        url,
		SrvSendMsg: false,
	}
	fullURL := constant.APIDomain + "/v2/users/" + userID + "/files"
	respBody, err := api.Transport(ctx, http.MethodPost, fullURL, body)
	if err != nil {
		return nil, fmt.Errorf("qq upload c2c file: %w", err)
	}
	var uploadResp qqFileUploadResponse
	if err := json.Unmarshal(respBody, &uploadResp); err != nil {
		return nil, fmt.Errorf("qq parse upload response: %w", err)
	}
	if uploadResp.FileInfo == "" {
		return nil, fmt.Errorf("qq upload response missing file_info")
	}
	return []byte(uploadResp.FileInfo), nil
}

func (a *QQAdapter) sendGroupMessage(ctx context.Context, api openapi.OpenAPI, groupID, content string) error {
	seq := a.nextMsgSeq("group:" + groupID)
	input, err := buildQQOutgoingMessage(ctx, content, seq)
	if err != nil {
		return err
	}
	if input.NeedUpload {
		fileInfo, uploadErr := a.uploadGroupFile(ctx, api, groupID, input.FileType, input.UploadURL)
		if uploadErr == nil {
			msg := &dto.MessageToCreate{
				MsgType: dto.RichMediaMsg,
				MsgSeq:  input.MsgSeq,
				MsgID:   input.MsgID,
				Media:   &dto.MediaInfo{FileInfo: fileInfo},
			}
			_, sendErr := api.PostGroupMessage(ctx, groupID, msg)
			if sendErr == nil {
				return nil
			}
			slog.Warn("[qq] upload-then-send failed for group, falling back to direct send",
				"group_id", groupID, "error", sendErr)
		} else {
			slog.Warn("[qq] file upload failed for group, falling back to direct send",
				"group_id", groupID, "error", uploadErr)
		}
		fallbackSeq := a.nextMsgSeq("group:" + groupID)
		directMsg := &dto.RichMediaMessage{
			FileType:   input.FileType,
			URL:        input.UploadURL,
			SrvSendMsg: true,
			MsgSeq:     int64(fallbackSeq),
		}
		_, fallbackErr := api.PostGroupMessage(ctx, groupID, directMsg)
		if fallbackErr != nil {
			return fmt.Errorf("send qq group message: upload-then-send failed and direct send fallback failed: %w", fallbackErr)
		}
		return nil
	}
	_, err = api.PostGroupMessage(ctx, groupID, input.Msg)
	if err != nil {
		return fmt.Errorf("send qq group message: %w", err)
	}
	return nil
}

func (a *QQAdapter) sendC2CMessage(ctx context.Context, api openapi.OpenAPI, userID, content string) error {
	seq := a.nextMsgSeq("user:" + userID)
	input, err := buildQQOutgoingMessage(ctx, content, seq)
	if err != nil {
		return err
	}
	if input.NeedUpload {
		fileInfo, uploadErr := a.uploadC2CFile(ctx, api, userID, input.FileType, input.UploadURL)
		if uploadErr == nil {
			msg := &dto.MessageToCreate{
				MsgType: dto.RichMediaMsg,
				MsgSeq:  input.MsgSeq,
				MsgID:   input.MsgID,
				Media:   &dto.MediaInfo{FileInfo: fileInfo},
			}
			_, sendErr := api.PostC2CMessage(ctx, userID, msg)
			if sendErr == nil {
				return nil
			}
			slog.Warn("[qq] upload-then-send failed for C2C, falling back to direct send",
				"user_id", userID, "error", sendErr)
		} else {
			slog.Warn("[qq] file upload failed for C2C, falling back to direct send",
				"user_id", userID, "error", uploadErr)
		}
		fallbackSeq := a.nextMsgSeq("user:" + userID)
		directMsg := &dto.RichMediaMessage{
			FileType:   input.FileType,
			URL:        input.UploadURL,
			SrvSendMsg: true,
			MsgSeq:     int64(fallbackSeq),
		}
		_, fallbackErr := api.PostC2CMessage(ctx, userID, directMsg)
		if fallbackErr != nil {
			return fmt.Errorf("send qq c2c message: upload-then-send failed and direct send fallback failed: %w", fallbackErr)
		}
		return nil
	}
	_, err = api.PostC2CMessage(ctx, userID, input.Msg)
	if err != nil {
		return fmt.Errorf("send qq c2c message: %w", err)
	}
	return nil
}
