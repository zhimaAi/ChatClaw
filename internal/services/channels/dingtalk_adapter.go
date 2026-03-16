package channels

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	dingchatbot "github.com/open-dingtalk/dingtalk-stream-sdk-go/chatbot"
	dingclient "github.com/open-dingtalk/dingtalk-stream-sdk-go/client"
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

// dingTalkTokenEntry caches a new-style api.dingtalk.com OAuth2 access token.
type dingTalkTokenEntry struct {
	token     string
	expiresAt time.Time
}

// dingTalkOApiTokenEntry caches the legacy oapi.dingtalk.com access token,
// which is required by older endpoints such as /media/upload.
type dingTalkOApiTokenEntry struct {
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
	mu             sync.Mutex
	streamClient   *dingclient.StreamClient
	replier        *dingchatbot.ChatbotReplier
	connected      atomic.Bool
	cancel         context.CancelFunc
	channelID      int64
	handler        MessageHandler
	config         DingTalkConfig
	webhookCache   sync.Map // conversationId -> *webhookEntry
	nameCache      sync.Map // senderID -> displayName
	seenMsgs       sync.Map // msgId -> struct{}, dedup within TTL
	tokenMu        sync.Mutex
	tokenEntry     *dingTalkTokenEntry // cached api.dingtalk.com OAuth2 token
	oapiTokenMu    sync.Mutex
	oapiTokenEntry *dingTalkOApiTokenEntry // cached oapi.dingtalk.com legacy token
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
		nowMs := time.Now().UnixMilli()
		slog.Info("[dingtalk] session webhook cached",
			"conversation_id", data.ConversationId,
			"webhook_len", len(data.SessionWebhook),
			"expires_at", data.SessionWebhookExpiredTime,
			"now", nowMs,
			"remaining_ms", data.SessionWebhookExpiredTime-nowMs,
		)
	} else {
		slog.Warn("[dingtalk] session webhook missing in callback",
			"conversation_id", data.ConversationId,
			"has_session_webhook", data.SessionWebhook != "",
			"session_webhook_expired_time", data.SessionWebhookExpiredTime,
			"msg_id", data.MsgId,
			"msg_type", data.Msgtype,
		)
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

// getOApiAccessToken returns a valid legacy oapi.dingtalk.com access token,
// refreshing when expired. This token is required by the media upload endpoint
// (https://oapi.dingtalk.com/media/upload) which does not accept the newer
// api.dingtalk.com OAuth2 token.
func (a *DingTalkAdapter) getOApiAccessToken(ctx context.Context) (string, error) {
	a.oapiTokenMu.Lock()
	defer a.oapiTokenMu.Unlock()

	if a.oapiTokenEntry != nil && time.Now().Before(a.oapiTokenEntry.expiresAt) {
		return a.oapiTokenEntry.token, nil
	}

	a.mu.Lock()
	appKey := a.config.AppID
	appSecret := a.config.AppSecret
	a.mu.Unlock()

	tokenURL := fmt.Sprintf("https://oapi.dingtalk.com/gettoken?appkey=%s&appsecret=%s", appKey, appSecret)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tokenURL, nil)
	if err != nil {
		return "", fmt.Errorf("build oapi token request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("oapi token request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"` // seconds
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parse oapi token response: %w", err)
	}
	if result.ErrCode != 0 {
		return "", fmt.Errorf("get oapi access token failed (code: %d, msg: %s)", result.ErrCode, result.ErrMsg)
	}
	if result.AccessToken == "" {
		return "", fmt.Errorf("get oapi access token failed: empty token in response")
	}

	// Cache with 3-minute safety margin to avoid using an almost-expired token.
	expiry := time.Duration(result.ExpiresIn)*time.Second - 3*time.Minute
	if expiry < 0 {
		expiry = 0
	}
	a.oapiTokenEntry = &dingTalkOApiTokenEntry{
		token:     result.AccessToken,
		expiresAt: time.Now().Add(expiry),
	}
	return result.AccessToken, nil
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
	slog.Info("[dingtalk] send message start",
		"target_id", targetID,
		"content_len", len(content),
	)
	webhook, err := a.resolveWebhook(targetID)
	if err != nil {
		slog.Error("[dingtalk] resolve webhook failed",
			"target_id", targetID,
			"error", err,
		)
		return err
	}
	slog.Info("[dingtalk] resolve webhook ok",
		"target_id", targetID,
		"content", content,
		"webhook_len", len(webhook),
	)

	msgType, requestBody, err := buildDingTalkOutgoingMessage(content)
	if err != nil {
		slog.Error("[dingtalk] build outgoing message failed",
			"target_id", targetID,
			"content", content,
			"error", err,
		)
		return err
	}

	reqJSON, _ := json.Marshal(requestBody)
	const maxLogLen = 1024
	reqLog := string(reqJSON)
	if len(reqLog) > maxLogLen {
		reqLog = reqLog[:maxLogLen] + "...(truncated)"
	}
	slog.Error("[dingtalk] send message request",
		"target_id", targetID,
		"msg_type", msgType,
		"body", reqLog,
	)

	replyCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if deadline, ok := replyCtx.Deadline(); ok {
		slog.Info("[dingtalk] reply context prepared",
			"target_id", targetID,
			"msg_type", msgType,
			"deadline", deadline.UnixMilli(),
		)
	}

	err = a.replier.ReplyMessage(replyCtx, webhook, requestBody)
	if err != nil {
		slog.Error("[dingtalk] send message response error",
			"target_id", targetID,
			"msg_type", msgType,
			"error", err,
		)
		return err
	}
	slog.Info("[dingtalk] send message response ok",
		"target_id", targetID,
		"msg_type", msgType,
	)
	return nil
}

// detectDingTalkMediaType infers the DingTalk media type from a file name.
// DingTalk supports: image (jpg/jpeg/gif/png/bmp ≤1 MB), voice (amr/mp3/wav ≤2 MB),
// video (mp4 ≤10 MB), and file (doc/docx/xls/xlsx/ppt/pptx/zip/pdf/rar ≤20 MB).
func detectDingTalkMediaType(fileName string) string {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(fileName), "."))
	switch ext {
	case "jpg", "jpeg", "gif", "png", "bmp":
		return "image"
	case "amr", "mp3", "wav":
		return "voice"
	case "mp4":
		return "video"
	default:
		return "file"
	}
}

// UploadMessageFile uploads a local file to DingTalk and returns (mediaId, mediaType, error).
// mediaId is the DingTalk media resource identifier used when sending file/audio/video messages.
// mediaType is one of: image, voice, video, file (derived from the file extension).
//
// Uses the legacy oapi endpoint which accepts a single multipart form with:
//   - media: the file binary
//   - type:  one of image / voice / video / file
//
// Reference: https://open.dingtalk.com/document/development/upload-media-files
// Equivalent curl:
//
//	curl -X POST 'https://oapi.dingtalk.com/media/upload?access_token=TOKEN' \
//	  --form 'media=@"/path/to/file"' \
//	  --form 'type="file"'
func (a *DingTalkAdapter) UploadMessageFile(ctx context.Context, filePath string) (mediaID string, mediaType string, err error) {
	stat, err := os.Stat(filePath)
	if err != nil {
		return "", "", fmt.Errorf("dingtalk upload: file not accessible: %w", err)
	}
	if stat.IsDir() {
		return "", "", fmt.Errorf("dingtalk upload: path is a directory")
	}

	token, err := a.getOApiAccessToken(ctx)
	if err != nil {
		return "", "", fmt.Errorf("dingtalk upload: %w", err)
	}

	fileName := filepath.Base(filePath)
	mediaType = detectDingTalkMediaType(fileName)

	file, err := os.Open(filePath)
	if err != nil {
		return "", "", fmt.Errorf("dingtalk upload: open file: %w", err)
	}
	defer file.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	part, err := writer.CreateFormFile("media", fileName)
	if err != nil {
		return "", "", fmt.Errorf("dingtalk upload: create multipart field: %w", err)
	}
	if _, err := io.Copy(part, file); err != nil {
		return "", "", fmt.Errorf("dingtalk upload: copy file data: %w", err)
	}
	if err := writer.WriteField("type", mediaType); err != nil {
		return "", "", fmt.Errorf("dingtalk upload: write type field: %w", err)
	}
	if err := writer.Close(); err != nil {
		return "", "", fmt.Errorf("dingtalk upload: finalize multipart body: %w", err)
	}

	uploadURL := fmt.Sprintf("https://oapi.dingtalk.com/media/upload?access_token=%s", token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL, &body)
	if err != nil {
		return "", "", fmt.Errorf("dingtalk upload: build request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("dingtalk upload: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("dingtalk upload failed (http %d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		ErrCode   int    `json:"errcode"`
		ErrMsg    string `json:"errmsg"`
		Type      string `json:"type"`
		MediaID   string `json:"media_id"`
		CreatedAt int64  `json:"created_at"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", "", fmt.Errorf("dingtalk upload: parse response: %w", err)
	}
	if result.ErrCode != 0 {
		return "", "", fmt.Errorf("dingtalk upload failed (code: %d, msg: %s)", result.ErrCode, result.ErrMsg)
	}

	mediaID = strings.TrimSpace(result.MediaID)
	if mediaID == "" {
		return "", "", fmt.Errorf("dingtalk upload failed: empty media_id in response")
	}
	return mediaID, mediaType, nil
}

// resolveWebhook retrieves a cached session webhook for the given conversationId.
// DingTalk session webhooks are temporary (tied to individual message events);
// they expire after SessionWebhookExpiredTime and cannot be used for proactive messages.
// For proactive messages, the DingTalk OpenAPI with an access token is required (TODO).
func (a *DingTalkAdapter) resolveWebhook(conversationID string) (string, error) {
	nowMs := time.Now().UnixMilli()
	if v, ok := a.webhookCache.Load(conversationID); ok {
		entry := v.(*webhookEntry)
		slog.Info("[dingtalk] webhook cache hit",
			"conversation_id", conversationID,
			"expires_at", entry.expiresAt,
			"now", nowMs,
			"remaining_ms", entry.expiresAt-nowMs,
		)
		// Allow 30s grace period before expiry
		if entry.expiresAt == 0 || nowMs < entry.expiresAt-30000 {
			return entry.url, nil
		}
		slog.Warn("[dingtalk] webhook expired, evict from cache",
			"conversation_id", conversationID,
			"expires_at", entry.expiresAt,
			"now", nowMs,
		)
		a.webhookCache.Delete(conversationID)
	} else {
		slog.Warn("[dingtalk] webhook cache miss",
			"conversation_id", conversationID,
		)
	}
	return "", fmt.Errorf("no valid session webhook for conversation %s (webhook expired or not yet received)", conversationID)
}

// buildDingTalkOutgoingMessage constructs the DingTalk reply message body.
// Supported msg_type values (in JSON payload):
//   - "text"     → plain text message
//   - "markdown" → Markdown message (default for plain strings)
//   - "image"    → image by public URL (pic_url/photo_url) or media_id
//
// JSON format examples:
//
//	{"msg_type":"text","text":"hello"}
//	{"msg_type":"markdown","title":"Title","markdown":"## Hello\nworld"}
//	{"msg_type":"image","pic_url":"https://example.com/img.png"}
//	{"msg_type":"image","photo_url":"https://example.com/img.png"}
//	{"msg_type":"image","media_id":"@lALPDfJ6..."}
func buildDingTalkOutgoingMessage(raw string) (string, map[string]any, error) {
	trimmed := strings.TrimSpace(raw)
	slog.Info("[dingtalk] build outgoing message start",
		"raw_len", len(raw),
		"trimmed_len", len(trimmed),
		"looks_like_json", strings.HasPrefix(trimmed, "{"),
	)
	if trimmed == "" {
		slog.Warn("[dingtalk] build outgoing message failed: empty content")
		return "", nil, fmt.Errorf("dingtalk message content is empty")
	}

	// If content is plain text (not JSON), send as markdown for richer rendering
	if !strings.HasPrefix(trimmed, "{") {
		slog.Info("[dingtalk] build outgoing message fallback plain text -> markdown")
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
		PhotoURL string `json:"photo_url"`
		// For file type.
		MediaID      string `json:"media_id"`
		FileName     string `json:"file_name"`
		DownloadCode string `json:"download_code"` // backward compatibility
	}
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		slog.Warn("[dingtalk] build outgoing message json parse failed, fallback markdown",
			"error", err,
			"raw", raw,
		)
		// Fall back to markdown
		return "markdown", map[string]any{
			"msgtype": "markdown",
			"markdown": map[string]any{
				"title": "AI Reply",
				"text":  raw,
			},
		}, nil
	}
	// Compatibility: allow camelCase image URL keys in JSON payload.
	if payload.MsgType == "image" && strings.TrimSpace(payload.PicURL) == "" && strings.TrimSpace(payload.PhotoURL) == "" {
		var rawMap map[string]any
		if err := json.Unmarshal([]byte(trimmed), &rawMap); err == nil {
			if v, ok := rawMap["photoURL"].(string); ok {
				payload.PhotoURL = v
			}
			if v, ok := rawMap["picURL"].(string); ok {
				payload.PicURL = v
			}
		}
	}
	slog.Info("[dingtalk] build outgoing message parsed",
		"msg_type", payload.MsgType,
		"text_len", len(payload.Text),
		"title", payload.Title,
		"markdown_len", len(payload.Markdown),
		"pic_url", strings.TrimSpace(payload.PicURL),
		"photo_url", strings.TrimSpace(payload.PhotoURL),
		"media_id", strings.TrimSpace(payload.MediaID),
		"download_code", strings.TrimSpace(payload.DownloadCode),
	)

	switch payload.MsgType {
	case "text":
		text := payload.Text
		if text == "" {
			text = raw
		}
		slog.Info("[dingtalk] build outgoing message -> text")
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
		slog.Info("[dingtalk] build outgoing message -> markdown",
			"title", title,
			"markdown_len", len(md),
		)
		return "markdown", map[string]any{
			"msgtype": "markdown",
			"markdown": map[string]any{
				"title": title,
				"text":  md,
			},
		}, nil

	case "image":
		// Per DingTalk robot single-chat message spec, image template key is
		// sampleImageMsg and parameter key is photoURL.
		picURL := strings.TrimSpace(payload.PicURL)
		if picURL == "" {
			picURL = strings.TrimSpace(payload.PhotoURL)
		}
		if picURL != "" {
			slog.Info("[dingtalk] build outgoing message -> sampleImageMsg",
				"pic_url", picURL,
			)
			return "sampleImageMsg", map[string]any{
				"msgtype": "sampleImageMsg",
				"sampleImageMsg": map[string]any{
					"photoURL": picURL,
					// Keep backward compatibility for old clients that used picURL.
					"picURL": picURL,
				},
			}, nil
		}
		// Fallback: support image by media_id (for uploaded local image files).
		mediaID := strings.TrimSpace(payload.MediaID)
		if mediaID == "" {
			mediaID = strings.TrimSpace(payload.DownloadCode)
		}
		if mediaID == "" {
			slog.Warn("[dingtalk] build outgoing image failed: empty pic_url and media_id")
			return "", nil, fmt.Errorf("dingtalk image message requires pic_url or media_id")
		}
		slog.Info("[dingtalk] build outgoing message -> image(media_id)",
			"media_id", mediaID,
		)
		return "image", map[string]any{
			"msgtype": "image",
			"image": map[string]any{
				"media_id": mediaID,
			},
		}, nil

	case "file":
		mediaID := strings.TrimSpace(payload.MediaID)
		if mediaID == "" {
			// Backward compatibility with old internal payload key.
			mediaID = strings.TrimSpace(payload.DownloadCode)
		}
		if mediaID == "" {
			slog.Warn("[dingtalk] build outgoing file failed: empty media_id")
			return "", nil, fmt.Errorf("dingtalk file message requires media_id")
		}
		fileBody := map[string]any{
			"media_id": mediaID,
		}
		slog.Info("[dingtalk] build outgoing message -> file",
			"media_id", mediaID,
		)
		return "file", map[string]any{
			"msgtype": "file",
			"file":    fileBody,
		}, nil

	default:
		// Default to markdown
		slog.Info("[dingtalk] build outgoing message unknown msg_type, fallback markdown",
			"msg_type", payload.MsgType,
		)
		return "markdown", map[string]any{
			"msgtype": "markdown",
			"markdown": map[string]any{
				"title": "AI Reply",
				"text":  raw,
			},
		}, nil
	}
}
