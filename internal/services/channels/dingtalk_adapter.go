package channels

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	dingclient "github.com/open-dingtalk/dingtalk-stream-sdk-go/client"
	dingchatbot "github.com/open-dingtalk/dingtalk-stream-sdk-go/chatbot"
)

// ---- Incoming media content models ----------------------------------------

// DingTalkPictureContent is the Content payload for msgtype=picture.
type DingTalkPictureContent struct {
	DownloadCode string `json:"downloadCode"`
	PicURL       string `json:"picURL"` // may be empty for private images
}

// DingTalkFileContent is the Content payload for msgtype=file.
type DingTalkFileContent struct {
	DownloadCode string `json:"downloadCode"`
	FileName     string `json:"fileName"`
	FileSize     int64  `json:"fileSize"`
	FileType     string `json:"fileType"` // e.g. "pdf", "docx"
}

// DingTalkAudioContent is the Content payload for msgtype=audio.
type DingTalkAudioContent struct {
	DownloadCode      string `json:"downloadCode"`
	Duration          int    `json:"duration"`          // seconds
	RecognizedContent string `json:"recognizedContent"` // ASR text (may be empty)
}

// DingTalkVideoContent is the Content payload for msgtype=video.
type DingTalkVideoContent struct {
	DownloadCode string `json:"downloadCode"`
	VideoType    string `json:"videoType"` // e.g. "mp4"
	Duration     int    `json:"duration"`  // seconds
}

// ---- Access token cache ----------------------------------------------------

type dingTalkTokenEntry struct {
	token     string
	expiresAt time.Time
}

func init() {
	RegisterAdapter(PlatformDingTalk, func() PlatformAdapter {
		return &DingTalkAdapter{}
	})
}

// DingTalkConfig contains credentials for a DingTalk bot (Stream mode).
// AppID maps to ClientID (AppKey), AppSecret maps to ClientSecret.
type DingTalkConfig struct {
	AppID     string `json:"app_id"`
	AppSecret string `json:"app_secret"`
}

// webhookEntry caches a per-conversation reply webhook URL with its expiry time.
type webhookEntry struct {
	url       string
	expiresAt int64 // unix milliseconds
}

// DingTalkAdapter implements PlatformAdapter for DingTalk using Stream (长连接).
type DingTalkAdapter struct {
	mu           sync.Mutex
	streamClient *dingclient.StreamClient
	replier      *dingchatbot.ChatbotReplier
	connected    atomic.Bool
	cancel       context.CancelFunc
	channelID    int64
	handler      MessageHandler
	config       DingTalkConfig
	webhookCache sync.Map // conversationId -> *webhookEntry
	nameCache    sync.Map // senderID -> displayName
	seenMsgs     sync.Map // msgId -> struct{}, dedup within TTL
	tokenMu      sync.Mutex
	tokenEntry   *dingTalkTokenEntry // cached access token
}

func (a *DingTalkAdapter) Platform() string { return PlatformDingTalk }

func (a *DingTalkAdapter) Connect(ctx context.Context, channelID int64, configJSON string, handler MessageHandler) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	var cfg DingTalkConfig
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return fmt.Errorf("parse dingtalk config: %w", err)
	}
	if cfg.AppID == "" || cfg.AppSecret == "" {
		return fmt.Errorf("dingtalk config: app_id and app_secret are required")
	}

	a.config = cfg
	a.channelID = channelID
	a.handler = handler
	a.replier = dingchatbot.NewChatbotReplier()

	streamClient := dingclient.NewStreamClient(
		dingclient.WithAppCredential(dingclient.NewAppCredentialConfig(cfg.AppID, cfg.AppSecret)),
		dingclient.WithAutoReconnect(true),
	)
	streamClient.RegisterChatBotCallbackRouter(a.onMessageReceive)

	// Verify credentials by attempting to fetch connection endpoint
	verifyCtx, verifyCancel := context.WithTimeout(ctx, 10*time.Second)
	defer verifyCancel()
	if err := a.verifyCredentials(verifyCtx, cfg.AppID, cfg.AppSecret); err != nil {
		return fmt.Errorf("dingtalk credentials verification failed: %w", err)
	}

	a.streamClient = streamClient

	connCtx, cancel := context.WithCancel(ctx)
	a.cancel = cancel

	go func() {
		if err := streamClient.Start(connCtx); err != nil {
			slog.Error("[dingtalk] stream connection error", "error", err)
			a.connected.Store(false)
			return
		}
	}()

	a.connected.Store(true)
	return nil
}

// verifyCredentials checks that the ClientID and ClientSecret are valid by
// calling the DingTalk connection-endpoint API (same endpoint the SDK uses).
func (a *DingTalkAdapter) verifyCredentials(ctx context.Context, clientID, clientSecret string) error {
	body, _ := json.Marshal(map[string]string{
		"clientId":     clientID,
		"clientSecret": clientSecret,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.dingtalk.com/v1.0/gateway/connections/open",
		bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build verify request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("verify request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("invalid app_id or app_secret (http %d)", resp.StatusCode)
	}

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		Endpoint string `json:"endpoint"`
		Ticket   string `json:"ticket"`
		// DingTalk returns errcode/errmsg on failure
		ErrCode int    `json:"errCode"`
		ErrMsg  string `json:"message"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("parse verify response: %w", err)
	}
	if result.ErrCode != 0 {
		return fmt.Errorf("invalid credentials (code: %d, msg: %s)", result.ErrCode, result.ErrMsg)
	}
	if result.Endpoint == "" {
		return fmt.Errorf("invalid credentials: empty endpoint in response")
	}
	return nil
}

func (a *DingTalkAdapter) onMessageReceive(ctx context.Context, data *dingchatbot.BotCallbackDataModel) ([]byte, error) {
	if !a.connected.Load() {
		return []byte(""), nil
	}
	if a.handler == nil || data == nil {
		return []byte(""), nil
	}

	msgID := data.MsgId

	// Deduplicate by msgId
	if msgID != "" {
		if _, loaded := a.seenMsgs.LoadOrStore(msgID, struct{}{}); loaded {
			slog.Info("[dingtalk] duplicate msg_id, skipping", "msg_id", msgID)
			return []byte(""), nil
		}
		go func() {
			time.Sleep(5 * time.Minute)
			a.seenMsgs.Delete(msgID)
		}()
	}

	// Cache the session webhook for this conversation (used for replies)
	if data.SessionWebhook != "" && data.ConversationId != "" {
		a.webhookCache.Store(data.ConversationId, &webhookEntry{
			url:       data.SessionWebhook,
			expiresAt: data.SessionWebhookExpiredTime,
		})
	}

	senderID := data.SenderId
	senderName := data.SenderNick
	if senderName == "" {
		senderName = data.SenderStaffId
	}

	chatID := data.ConversationId
	chatName := data.ConversationTitle
	msgType := data.Msgtype
	if msgType == "" {
		msgType = "text"
	}

	// Extract content based on message type
	content, rawData := a.extractIncomingContent(data)

	slog.Info("[dingtalk] message received",
		"sender", senderName,
		"sender_id", senderID,
		"chat_id", chatID,
		"chat_name", chatName,
		"msg_type", msgType,
		"content_len", len(content),
	)

	a.handler(IncomingMessage{
		ChannelID:  a.channelID,
		Platform:   PlatformDingTalk,
		MessageID:  msgID,
		SenderID:   senderID,
		SenderName: senderName,
		ChatID:     chatID,
		ChatName:   chatName,
		Content:    content,
		MsgType:    msgType,
		RawData:    rawData,
	})

	return []byte(""), nil
}

// extractIncomingContent returns (displayContent, rawData) from a DingTalk message.
// displayContent is human-readable text passed to the AI pipeline.
// rawData is the original JSON payload for media messages.
func (a *DingTalkAdapter) extractIncomingContent(data *dingchatbot.BotCallbackDataModel) (string, string) {
	msgType := data.Msgtype

	switch msgType {
	case "text", "":
		text := strings.TrimSpace(data.Text.Content)
		return text, text

	case "picture":
		rawBytes, _ := json.Marshal(data.Content)
		var pic DingTalkPictureContent
		if err := json.Unmarshal(rawBytes, &pic); err == nil && pic.DownloadCode != "" {
			desc := "[图片]"
			if pic.PicURL != "" {
				desc = fmt.Sprintf("[图片] %s", pic.PicURL)
			}
			mediaJSON, _ := json.Marshal(map[string]any{
				"type":          "picture",
				"download_code": pic.DownloadCode,
				"pic_url":       pic.PicURL,
			})
			return desc, string(mediaJSON)
		}
		return "[图片]", ""

	case "file":
		rawBytes, _ := json.Marshal(data.Content)
		var f DingTalkFileContent
		if err := json.Unmarshal(rawBytes, &f); err == nil && f.DownloadCode != "" {
			desc := fmt.Sprintf("[文件: %s]", f.FileName)
			mediaJSON, _ := json.Marshal(map[string]any{
				"type":          "file",
				"download_code": f.DownloadCode,
				"file_name":     f.FileName,
				"file_size":     f.FileSize,
				"file_type":     f.FileType,
			})
			return desc, string(mediaJSON)
		}
		return "[文件]", ""

	case "audio":
		rawBytes, _ := json.Marshal(data.Content)
		var au DingTalkAudioContent
		if err := json.Unmarshal(rawBytes, &au); err == nil && au.DownloadCode != "" {
			// Prefer ASR-recognized text if available
			desc := fmt.Sprintf("[语音 %ds]", au.Duration)
			if au.RecognizedContent != "" {
				desc = au.RecognizedContent
			}
			mediaJSON, _ := json.Marshal(map[string]any{
				"type":               "audio",
				"download_code":      au.DownloadCode,
				"duration":           au.Duration,
				"recognized_content": au.RecognizedContent,
			})
			return desc, string(mediaJSON)
		}
		return "[语音]", ""

	case "video":
		rawBytes, _ := json.Marshal(data.Content)
		var v DingTalkVideoContent
		if err := json.Unmarshal(rawBytes, &v); err == nil && v.DownloadCode != "" {
			desc := fmt.Sprintf("[视频 %ds]", v.Duration)
			mediaJSON, _ := json.Marshal(map[string]any{
				"type":          "video",
				"download_code": v.DownloadCode,
				"video_type":    v.VideoType,
				"duration":      v.Duration,
			})
			return desc, string(mediaJSON)
		}
		return "[视频]", ""

	default:
		// Unknown types: pass raw content string as-is
		rawBytes, _ := json.Marshal(data.Content)
		return string(rawBytes), string(rawBytes)
	}
}

// getAccessToken returns a valid DingTalk access token, refreshing if expired.
// Tokens are cached per adapter instance (expire after ~2 hours).
func (a *DingTalkAdapter) getAccessToken(ctx context.Context) (string, error) {
	a.tokenMu.Lock()
	defer a.tokenMu.Unlock()

	if a.tokenEntry != nil && time.Now().Before(a.tokenEntry.expiresAt) {
		return a.tokenEntry.token, nil
	}

	body, _ := json.Marshal(map[string]string{
		"appKey":    a.config.AppID,
		"appSecret": a.config.AppSecret,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.dingtalk.com/v1.0/oauth2/accessToken",
		bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		AccessToken string `json:"accessToken"`
		ExpireIn    int    `json:"expireIn"` // seconds
		Code        string `json:"code"`
		Message     string `json:"message"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parse token response: %w", err)
	}
	if result.AccessToken == "" {
		return "", fmt.Errorf("get access token failed (code: %s, msg: %s)", result.Code, result.Message)
	}

	// Cache with 3-minute safety margin
	expiry := time.Duration(result.ExpireIn)*time.Second - 3*time.Minute
	if expiry < 0 {
		expiry = 0
	}
	a.tokenEntry = &dingTalkTokenEntry{
		token:     result.AccessToken,
		expiresAt: time.Now().Add(expiry),
	}
	return result.AccessToken, nil
}

// DownloadFile downloads a DingTalk media file by its downloadCode and returns the raw bytes.
// downloadCode is obtained from incoming picture/file/audio/video messages.
func (a *DingTalkAdapter) DownloadFile(ctx context.Context, downloadCode string) ([]byte, error) {
	a.mu.Lock()
	clientID := a.config.AppID
	a.mu.Unlock()

	token, err := a.getAccessToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("dingtalk download: %w", err)
	}

	url := fmt.Sprintf(
		"https://api.dingtalk.com/v1.0/robot/messageFiles/download?downloadCode=%s&robotCode=%s",
		downloadCode, clientID,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build download request: %w", err)
	}
	req.Header.Set("x-acs-dingtalk-access-token", token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("download failed (http %d): %s", resp.StatusCode, string(body))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read download body: %w", err)
	}
	return data, nil
}

func (a *DingTalkAdapter) Disconnect(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.cancel != nil {
		a.cancel()
		a.cancel = nil
	}
	a.connected.Store(false)
	a.handler = nil
	a.streamClient = nil
	return nil
}

func (a *DingTalkAdapter) IsConnected() bool {
	return a.connected.Load()
}

// SendMessage sends a text reply to a DingTalk conversation.
// targetID should be the conversationId (as stored in chatID from IncomingMessage).
func (a *DingTalkAdapter) SendMessage(ctx context.Context, targetID string, content string) error {
	webhook, err := a.resolveWebhook(targetID)
	if err != nil {
		return err
	}

	msgType, requestBody, err := buildDingTalkOutgoingMessage(content)
	if err != nil {
		return err
	}
	_ = msgType

	replyCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	return a.replier.ReplyMessage(replyCtx, webhook, requestBody)
}

// resolveWebhook retrieves a cached session webhook for the given conversationId.
// DingTalk session webhooks are temporary (tied to individual message events);
// they expire after SessionWebhookExpiredTime and cannot be used for proactive messages.
// For proactive messages, the DingTalk OpenAPI with an access token is required (TODO).
func (a *DingTalkAdapter) resolveWebhook(conversationID string) (string, error) {
	if v, ok := a.webhookCache.Load(conversationID); ok {
		entry := v.(*webhookEntry)
		// Allow 30s grace period before expiry
		if entry.expiresAt == 0 || time.Now().UnixMilli() < entry.expiresAt-30000 {
			return entry.url, nil
		}
		a.webhookCache.Delete(conversationID)
	}
	return "", fmt.Errorf("no valid session webhook for conversation %s (webhook expired or not yet received)", conversationID)
}

// buildDingTalkOutgoingMessage constructs the DingTalk reply message body.
// Supported msg_type values (in JSON payload):
//   - "text"     → plain text message
//   - "markdown" → Markdown message (default for plain strings)
//   - "image"    → image by public URL (uses sampleImageMsg via session webhook)
//
// JSON format examples:
//
//	{"msg_type":"text","text":"hello"}
//	{"msg_type":"markdown","title":"Title","markdown":"## Hello\nworld"}
//	{"msg_type":"image","pic_url":"https://example.com/img.png"}
func buildDingTalkOutgoingMessage(raw string) (string, map[string]any, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil, fmt.Errorf("dingtalk message content is empty")
	}

	// If content is plain text (not JSON), send as markdown for richer rendering
	if !strings.HasPrefix(trimmed, "{") {
		return "markdown", map[string]any{
			"msgtype": "markdown",
			"markdown": map[string]any{
				"title": "AI Reply",
				"text":  raw,
			},
		}, nil
	}

	// Try to parse as DingTalk outgoing message JSON
	var payload struct {
		MsgType  string `json:"msg_type"`
		Text     string `json:"text"`
		Title    string `json:"title"`
		Markdown string `json:"markdown"`
		PicURL   string `json:"pic_url"` // for image type
	}
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		// Fall back to markdown
		return "markdown", map[string]any{
			"msgtype": "markdown",
			"markdown": map[string]any{
				"title": "AI Reply",
				"text":  raw,
			},
		}, nil
	}

	switch payload.MsgType {
	case "text":
		text := payload.Text
		if text == "" {
			text = raw
		}
		return "text", map[string]any{
			"msgtype": "text",
			"text": map[string]any{
				"content": text,
			},
		}, nil

	case "markdown":
		title := payload.Title
		if title == "" {
			title = "AI Reply"
		}
		md := payload.Markdown
		if md == "" {
			md = raw
		}
		return "markdown", map[string]any{
			"msgtype": "markdown",
			"markdown": map[string]any{
				"title": title,
				"text":  md,
			},
		}, nil

	case "image":
		// DingTalk session webhook uses sampleImageMsg for image-by-URL
		picURL := strings.TrimSpace(payload.PicURL)
		if picURL == "" {
			return "", nil, fmt.Errorf("dingtalk image message requires pic_url")
		}
		return "sampleImageMsg", map[string]any{
			"msgtype": "sampleImageMsg",
			"sampleImageMsg": map[string]any{
				"picURL": picURL,
			},
		}, nil

	default:
		// Default to markdown
		return "markdown", map[string]any{
			"msgtype": "markdown",
			"markdown": map[string]any{
				"title": "AI Reply",
				"text":  raw,
			},
		}, nil
	}
}
