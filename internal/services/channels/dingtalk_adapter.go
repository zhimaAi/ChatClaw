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

	"github.com/google/uuid"
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
	mu               sync.Mutex
	streamClient     *dingclient.StreamClient
	replier          *dingchatbot.ChatbotReplier
	connected        atomic.Bool
	cancel           context.CancelFunc
	channelID        int64
	handler          MessageHandler
	config           DingTalkConfig
	webhookCache     sync.Map // conversationId -> *webhookEntry
	nameCache        sync.Map // senderID -> displayName
	seenMsgs         sync.Map // msgId -> struct{}, dedup within TTL
	tokenMu          sync.Mutex
	tokenEntry       *dingTalkTokenEntry // cached api.dingtalk.com OAuth2 token
	oapiTokenMu      sync.Mutex
	oapiTokenEntry   *dingTalkOApiTokenEntry // cached oapi.dingtalk.com legacy token
	convTypeCache    sync.Map                // conversationId -> conversationType ("1"=private, "2"=group)
	convStaffIDCache sync.Map                // conversationId -> senderStaffId (for private chat card send)
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

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("invalid app_id or app_secret (http %d)", resp.StatusCode)
	}
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
			return []byte(""), nil
		}
		go func() {
			time.Sleep(5 * time.Minute)
			a.seenMsgs.Delete(msgID)
		}()
	}

	// Cache conversation metadata for interactive card sends.
	if data.ConversationId != "" {
		convType := data.ConversationType
		if convType == "" {
			convType = "2" // default: group chat
		}
		a.convTypeCache.Store(data.ConversationId, convType)
		if convType == "1" && data.SenderStaffId != "" {
			// Private chat: remember staffId to address the card recipient.
			a.convStaffIDCache.Store(data.ConversationId, data.SenderStaffId)
		}
	}

	// Cache the session webhook for this conversation (used for replies)
	if data.SessionWebhook != "" && data.ConversationId != "" {
		a.webhookCache.Store(data.ConversationId, &webhookEntry{
			url:       data.SessionWebhook,
			expiresAt: data.SessionWebhookExpiredTime,
		})
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
	webhook, err := a.resolveWebhook(targetID)
	if err != nil {
		slog.Error("[dingtalk] resolve webhook failed",
			"target_id", targetID,
			"error", err,
		)
		return err
	}

	_, requestBody, err := buildDingTalkOutgoingMessage(content)
	if err != nil {
		slog.Error("[dingtalk] build outgoing message failed",
			"target_id", targetID,
			"content", content,
			"error", err,
		)
		return err
	}

	replyCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	err = a.replier.ReplyMessage(replyCtx, webhook, requestBody)
	if err != nil {
		slog.Error("[dingtalk] send message response error",
			"target_id", targetID,
			"error", err,
		)
		return err
	}
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
	if trimmed == "" {
		slog.Warn("[dingtalk] build outgoing message failed: empty content")
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
		text := payload.Markdown
		if text == "" {
			text = raw
		}
		return "markdown", map[string]any{
			"msgtype": "markdown",
			"markdown": map[string]any{
				"title": title,
				"text":  text,
			},
		}, nil

	case "sampleImageMsg":
		return "sampleImageMsg", map[string]any{
			"msg_key": "image",
			"msgtype": "image",
			"image": map[string]any{
				"title":    "AI Reply",
				"photoURL": payload.MediaID,
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

// ---- Interactive card (typewriter mode) ------------------------------------

// streamingCardTemplate is the card JSON for the typewriter/streaming mode.
// %s is replaced with a properly JSON-encoded markdown string.
// The header logo is intentionally omitted to avoid dependency on any mediaId asset.
const streamingCardTemplate = `{"config":{"autoLayout":true,"enableForward":true},"header":{"title":{"type":"text","text":"AI 助手"}},"contents":[{"type":"markdown","text":%s,"id":"markdown_content"}]}`

// BuildStreamingCardData returns a card JSON string containing the given text content.
// It is exported so callers outside the package (e.g. bootstrap) can build card payloads
// when updating the card directly via UpdateInteractiveCard.
func BuildStreamingCardData(text string) string {
	textJSON, _ := json.Marshal(text)
	return fmt.Sprintf(streamingCardTemplate, string(textJSON))
}

// SendInteractiveCard sends a new robot interactive card to a DingTalk conversation.
// cardBizId is a caller-supplied UUID that uniquely identifies this card instance.
func (a *DingTalkAdapter) SendInteractiveCard(ctx context.Context, conversationID, cardBizID, cardData string) error {
	token, err := a.getAccessToken(ctx)
	if err != nil {
		return fmt.Errorf("send interactive card: %w", err)
	}

	convType := "2" // default: group chat
	if v, ok := a.convTypeCache.Load(conversationID); ok {
		convType = v.(string)
	}

	reqBody := map[string]any{
		"cardTemplateId": "StandardCard",
		"cardBizId":      cardBizID,
		"cardData":       cardData,
		"robotCode":      a.config.AppID,
		"pullStrategy":   false,
	}
	if convType == "2" {
		reqBody["openConversationId"] = conversationID
	} else {
		staffID := ""
		if v, ok := a.convStaffIDCache.Load(conversationID); ok {
			staffID = v.(string)
		}
		receiverBytes, _ := json.Marshal(map[string]string{"userId": staffID})
		reqBody["singleChatReceiver"] = string(receiverBytes)
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("send interactive card: marshal body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.dingtalk.com/v1.0/im/v1.0/robot/interactiveCards/send",
		bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("send interactive card: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-acs-dingtalk-access-token", token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("send interactive card: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("send interactive card: http %d: %s", resp.StatusCode, string(respBody))
	}

	// DingTalk returns {"processQueryKey":"..."} on success.
	// On failure it uses HTTP 4xx/5xx with {"code":"...","message":"..."}.
	// Guard against any unexpected 2xx body that contains a non-empty "code".
	var apiErr struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(respBody, &apiErr); err == nil && apiErr.Code != "" && apiErr.Code != "0" {
		return fmt.Errorf("send interactive card: api error (code: %s, msg: %s)", apiErr.Code, apiErr.Message)
	}

	slog.Debug("[dingtalk] interactive card sent",
		"conversation_id", conversationID,
		"card_biz_id", cardBizID,
		"conv_type", convType,
	)
	return nil
}

// UpdateInteractiveCard updates the content of an existing robot interactive card.
// SDK reference: PUT https://api.dingtalk.com/v1.0/im/robots/interactiveCards
// cardBizId goes in the request BODY (no path variable); robotCode is NOT required.
func (a *DingTalkAdapter) UpdateInteractiveCard(ctx context.Context, cardBizID, cardData string) error {
	token, err := a.getAccessToken(ctx)
	if err != nil {
		return fmt.Errorf("update interactive card: %w", err)
	}

	// Per DingTalk SDK (alibabacloud-go/dingtalk im_1_0):
	//   Pathname: "/v1.0/im/robots/interactiveCards"  (PUT, no path variable)
	//   Body:     cardBizId + cardData  (no robotCode)
	reqBody := map[string]any{
		"cardBizId": cardBizID,
		"cardData":  cardData,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("update interactive card: marshal body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut,
		"https://api.dingtalk.com/v1.0/im/robots/interactiveCards",
		bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("update interactive card: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-acs-dingtalk-access-token", token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("update interactive card: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("update interactive card: http %d: %s", resp.StatusCode, string(respBody))
	}

	var apiErr struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(respBody, &apiErr); err == nil && apiErr.Code != "" && apiErr.Code != "0" {
		return fmt.Errorf("update interactive card: api error (code: %s, msg: %s)", apiErr.Code, apiErr.Message)
	}

	return nil
}

// SendStreamingCard sends a message using typewriter (streaming card) mode.
// The full content is progressively revealed to simulate a typing animation.
// It first creates a card, then performs incremental updates.
func (a *DingTalkAdapter) SendStreamingCard(ctx context.Context, conversationID, content string) error {
	if strings.TrimSpace(content) == "" {
		return fmt.Errorf("streaming card: content is empty")
	}

	cardBizID := uuid.New().String()

	// Send initial card with a cursor to indicate the message is being composed.
	initialCard := BuildStreamingCardData("▌")
	if err := a.SendInteractiveCard(ctx, conversationID, cardBizID, initialCard); err != nil {
		return fmt.Errorf("streaming card: send initial card: %w", err)
	}

	runes := []rune(content)
	total := len(runes)

	// Target at most 25 intermediate updates to limit API calls.
	// Minimum chunk size is 30 runes so very short messages still animate.
	const maxUpdates = 25
	const minChunkSize = 30
	const updateIntervalMs = 150

	chunkSize := total / maxUpdates
	if chunkSize < minChunkSize {
		chunkSize = minChunkSize
	}

	for i := chunkSize; i < total; i += chunkSize {
		select {
		case <-ctx.Done():
			// Context cancelled: send final content and exit.
			_ = a.UpdateInteractiveCard(ctx, cardBizID, BuildStreamingCardData(content))
			return ctx.Err()
		default:
		}

		time.Sleep(updateIntervalMs * time.Millisecond)

		partial := string(runes[:i]) + "▌"
		if err := a.UpdateInteractiveCard(ctx, cardBizID, BuildStreamingCardData(partial)); err != nil {
			slog.Warn("[dingtalk] streaming card intermediate update failed",
				"card_biz_id", cardBizID,
				"error", err,
			)
			// Non-fatal: continue to next chunk.
		}
	}

	// Final update: full content without cursor.
	time.Sleep(updateIntervalMs * time.Millisecond)
	if err := a.UpdateInteractiveCard(ctx, cardBizID, BuildStreamingCardData(content)); err != nil {
		slog.Error("[dingtalk] streaming card final update failed",
			"card_biz_id", cardBizID,
			"error", err,
		)
		return fmt.Errorf("streaming card: final update: %w", err)
	}

	slog.Debug("[dingtalk] streaming card completed",
		"conversation_id", conversationID,
		"card_biz_id", cardBizID,
		"total_runes", total,
	)
	return nil
}
