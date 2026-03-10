package channels

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

func init() {
	RegisterAdapter(PlatformWeCom, func() PlatformAdapter {
		return &WeComAdapter{}
	})
}

const (
	wecomWSURL             = "wss://openws.work.weixin.qq.com"
	wecomHeartbeatInterval = 30 * time.Second
	wecomReconnectMax      = 30 * time.Second
	wecomReconnectBase     = 1 * time.Second
	wecomMaxReconnect      = 10
	wecomRequestTimeout    = 10 * time.Second
)

// WeComConfig contains credentials for a WeCom AI Bot.
type WeComConfig struct {
	AppID     string `json:"app_id"`     // Bot ID
	AppSecret string `json:"app_secret"` // Bot Secret
}

// WeComAdapter implements PlatformAdapter for WeCom using WebSocket.
type WeComAdapter struct {
	mu        sync.Mutex
	conn      *websocket.Conn
	connected atomic.Bool
	cancel    context.CancelFunc
	channelID int64
	handler   MessageHandler
	config    WeComConfig
	seenMsgs  sync.Map // messageID -> struct{}, dedup within TTL

	reconnectAttempts int
	lastReqID         string   // for reply
	lastResponseURL   string   // for HTTP reply fallback
	pendingReplies    sync.Map // reqID -> chan error
	authResult        chan error // channel for first auth result
	authReqID         string     // req_id of the pending auth request
}

func (a *WeComAdapter) Platform() string { return PlatformWeCom }

func (a *WeComAdapter) Connect(ctx context.Context, channelID int64, configJSON string, handler MessageHandler) error {
	a.mu.Lock()

	var cfg WeComConfig
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		a.mu.Unlock()
		return fmt.Errorf("parse wecom config: %w", err)
	}
	if cfg.AppID == "" || cfg.AppSecret == "" {
		a.mu.Unlock()
		return fmt.Errorf("wecom config: app_id and app_secret are required")
	}

	a.config = cfg
	a.channelID = channelID
	a.handler = handler
	a.authResult = make(chan error, 1)

	connCtx, cancel := context.WithCancel(ctx)
	a.cancel = cancel
	a.mu.Unlock()

	// Perform first connection synchronously to verify credentials
	if err := a.doConnect(connCtx); err != nil {
		cancel()
		return fmt.Errorf("wecom connection failed: %w", err)
	}

	// Start reading messages in background to receive auth response
	authDone := make(chan struct{})
	go func() {
		defer close(authDone)
		a.readMessagesUntilAuth(connCtx)
	}()

	// Wait for auth response with timeout
	select {
	case err := <-a.authResult:
		if err != nil {
			a.Disconnect(ctx)
			return fmt.Errorf("wecom authentication failed: %w", err)
		}
	case <-time.After(15 * time.Second):
		a.Disconnect(ctx)
		return fmt.Errorf("wecom authentication timeout")
	case <-ctx.Done():
		a.Disconnect(ctx)
		return ctx.Err()
	}

	a.reconnectAttempts = 0
	a.connected.Store(true)

	// Start background loop for message handling and reconnection
	go a.maintainLoop(connCtx)

	return nil
}

// readMessagesUntilAuth reads messages until auth result is received or context cancelled
func (a *WeComAdapter) readMessagesUntilAuth(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		a.mu.Lock()
		conn := a.conn
		authReqID := a.authReqID
		a.mu.Unlock()

		if conn == nil || authReqID == "" {
			return
		}

		conn.SetReadDeadline(time.Now().Add(20 * time.Second))
		_, message, err := conn.ReadMessage()
		if err != nil {
			slog.Warn("[wecom] read error during auth", "error", err)
			// Signal auth failure on connection error
			a.mu.Lock()
			authCh := a.authResult
			a.mu.Unlock()
			if authCh != nil {
				select {
				case authCh <- fmt.Errorf("connection error: %w", err):
				default:
				}
			}
			return
		}

		a.handleMessage(message)

		// Check if auth request ID has been cleared (auth processed)
		a.mu.Lock()
		currentAuthReqID := a.authReqID
		a.mu.Unlock()
		if currentAuthReqID == "" {
			return
		}
	}
}

// maintainLoop handles message receiving and reconnection after initial connect
func (a *WeComAdapter) maintainLoop(ctx context.Context) {
	// Run the message loop first (initial connection already established)
	err := a.runLoop(ctx)
	if err != nil {
		slog.Warn("[wecom] connection lost", "error", err)
		a.connected.Store(false)
	}

	// Reconnection loop
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		err := a.doConnect(ctx)
		if err != nil {
			slog.Warn("[wecom] reconnection failed", "error", err, "attempt", a.reconnectAttempts)
			a.connected.Store(false)

			if a.reconnectAttempts >= wecomMaxReconnect {
				slog.Error("[wecom] max reconnect attempts reached, giving up")
				return
			}

			delay := a.calcReconnectDelay()
			a.reconnectAttempts++

			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
				continue
			}
		}

		a.reconnectAttempts = 0
		a.connected.Store(true)

		err = a.runLoop(ctx)
		if err != nil {
			slog.Warn("[wecom] connection lost", "error", err)
			a.connected.Store(false)
		}
	}
}

func (a *WeComAdapter) calcReconnectDelay() time.Duration {
	delay := wecomReconnectBase * time.Duration(1<<a.reconnectAttempts)
	if delay > wecomReconnectMax {
		delay = wecomReconnectMax
	}
	return delay
}

func (a *WeComAdapter) doConnect(ctx context.Context) error {
	slog.Info("[wecom] dialing websocket", "url", wecomWSURL)

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, resp, err := dialer.DialContext(ctx, wecomWSURL, nil)
	if err != nil {
		if resp != nil {
			slog.Error("[wecom] dial failed", "error", err, "status", resp.StatusCode)
		}
		return fmt.Errorf("websocket dial: %w", err)
	}

	slog.Info("[wecom] websocket dial succeeded", "status", resp.StatusCode)

	a.mu.Lock()
	a.conn = conn
	a.mu.Unlock()

	slog.Info("[wecom] sending auth...")
	if err := a.sendAuth(); err != nil {
		conn.Close()
		return fmt.Errorf("auth failed: %w", err)
	}

	slog.Info("[wecom] auth sent, connected", "bot_id", a.config.AppID)
	return nil
}

// WeComFrame represents a WebSocket frame for WeCom.
type WeComFrame struct {
	Cmd     string                 `json:"cmd,omitempty"`
	Headers map[string]interface{} `json:"headers,omitempty"`
	Body    json.RawMessage        `json:"body,omitempty"`
	ErrCode int                    `json:"errcode,omitempty"`
	ErrMsg  string                 `json:"errmsg,omitempty"`
}

// WeComAuthBody is the authentication request body.
type WeComAuthBody struct {
	BotID  string `json:"bot_id"`
	Secret string `json:"secret"`
}

func (a *WeComAdapter) sendAuth() error {
	authBody, _ := json.Marshal(WeComAuthBody{
		BotID:  a.config.AppID,
		Secret: a.config.AppSecret,
	})

	reqID := generateReqID("aibot_subscribe")
	a.mu.Lock()
	a.authReqID = reqID
	a.mu.Unlock()

	frame := WeComFrame{
		Cmd:     "aibot_subscribe",
		Headers: map[string]interface{}{"req_id": reqID},
		Body:    authBody,
	}

	return a.sendFrame(frame)
}

func (a *WeComAdapter) sendFrame(frame WeComFrame) error {
	a.mu.Lock()
	conn := a.conn
	a.mu.Unlock()

	if conn == nil {
		return fmt.Errorf("not connected")
	}

	data, err := json.Marshal(frame)
	if err != nil {
		return err
	}

	return conn.WriteMessage(websocket.TextMessage, data)
}

func (a *WeComAdapter) runLoop(ctx context.Context) error {
	slog.Info("[wecom] entering runLoop")

	heartbeatTicker := time.NewTicker(wecomHeartbeatInterval)
	defer heartbeatTicker.Stop()

	errCh := make(chan error, 1)

	go func() {
		slog.Info("[wecom] message reader goroutine started")
		for {
			a.mu.Lock()
			conn := a.conn
			a.mu.Unlock()

			if conn == nil {
				slog.Warn("[wecom] connection is nil in reader goroutine")
				errCh <- fmt.Errorf("connection closed")
				return
			}

			slog.Info("[wecom] waiting for next message...")
			_, message, err := conn.ReadMessage()
			if err != nil {
				slog.Warn("[wecom] ReadMessage error", "error", err)
				errCh <- err
				return
			}

			slog.Info("[wecom] received message", "len", len(message))
			a.handleMessage(message)
		}
	}()

	slog.Info("[wecom] runLoop main loop started, heartbeat interval", "interval", wecomHeartbeatInterval)

	for {
		select {
		case <-ctx.Done():
			slog.Info("[wecom] context done, exiting runLoop")
			return ctx.Err()
		case err := <-errCh:
			slog.Warn("[wecom] error from reader", "error", err)
			return err
		case <-heartbeatTicker.C:
			slog.Info("[wecom] sending heartbeat")
			if err := a.sendHeartbeat(); err != nil {
				slog.Warn("[wecom] heartbeat failed", "error", err)
			}
		}
	}
}

func (a *WeComAdapter) sendHeartbeat() error {
	frame := WeComFrame{
		Cmd:     "ping",
		Headers: map[string]interface{}{"req_id": generateReqID("ping")},
	}
	err := a.sendFrame(frame)
	if err != nil {
		slog.Error("[wecom] sendHeartbeat failed", "error", err)
	} else {
		slog.Debug("[wecom] heartbeat (ping) frame sent")
	}
	return err
}

// WeComMessageBody represents the message body from WeCom.
type WeComMessageBody struct {
	MsgID       string      `json:"msgid"`
	AIBotID     string      `json:"aibotid"`
	ChatID      string      `json:"chatid"`
	ChatType    string      `json:"chattype"` // single, group
	From        WeComFrom   `json:"from"`
	MsgType     string      `json:"msgtype"`
	CreateTime  int64       `json:"create_time"`
	ResponseURL string      `json:"response_url"`
	Text        *WeComText  `json:"text,omitempty"`
	Image       *WeComImage `json:"image,omitempty"`
	File        *WeComFile  `json:"file,omitempty"`
	Voice       *WeComVoice `json:"voice,omitempty"`
	Mixed       *WeComMixed `json:"mixed,omitempty"`
	Event       *WeComEvent `json:"event,omitempty"`
}

type WeComFrom struct {
	UserID string `json:"userid"`
	CorpID string `json:"corpid,omitempty"`
}

type WeComText struct {
	Content string `json:"content"`
}

type WeComImage struct {
	URL    string `json:"url"`
	AESKey string `json:"aeskey"`
}

type WeComFile struct {
	URL      string `json:"url"`
	AESKey   string `json:"aeskey"`
	FileName string `json:"file_name"`
	FileSize int64  `json:"file_size"`
}

type WeComVoice struct {
	URL    string `json:"url"`
	AESKey string `json:"aeskey"`
	Text   string `json:"text"` // ASR result
}

type WeComMixed struct {
	Items []WeComMixedItem `json:"items"`
}

type WeComMixedItem struct {
	Type    string      `json:"type"` // text, image
	Content interface{} `json:"content"`
}

type WeComEvent struct {
	EventType string `json:"event_type"`
}

func (a *WeComAdapter) handleMessage(data []byte) {
	slog.Info("[wecom] received raw frame", "data", string(data))

	var frame WeComFrame
	if err := json.Unmarshal(data, &frame); err != nil {
		slog.Warn("[wecom] failed to parse frame", "error", err)
		return
	}

	slog.Info("[wecom] received frame", "cmd", frame.Cmd, "errcode", frame.ErrCode)

	// Message push: cmd is "aibot_msg_callback"
	if frame.Cmd == "aibot_msg_callback" {
		slog.Info("[wecom] processing incoming message")
		a.handleIncomingMessage(frame)
		return
	}

	// Event push: cmd is "aibot_event_callback"
	if frame.Cmd == "aibot_event_callback" {
		a.handleEvent(frame)
		return
	}

	// Response frames have no cmd, identify by req_id prefix
	reqID := ""
	if frame.Headers != nil {
		if v, ok := frame.Headers["req_id"].(string); ok {
			reqID = v
		}
	}

	// Check if this is a reply ack (req_id exists in pendingReplies)
	if _, exists := a.pendingReplies.Load(reqID); exists {
		a.handleReplyAck(frame)
		return
	}

	// Auth response: req_id starts with "aibot_subscribe"
	if strings.HasPrefix(reqID, "aibot_subscribe") {
		a.mu.Lock()
		authCh := a.authResult
		expectedReqID := a.authReqID
		a.authReqID = "" // Clear to signal auth processed
		a.mu.Unlock()

		if frame.ErrCode != 0 {
			slog.Error("[wecom] auth failed", "errcode", frame.ErrCode, "errmsg", frame.ErrMsg)
			// Signal auth failure if this is the pending auth request
			if authCh != nil && reqID == expectedReqID {
				select {
				case authCh <- fmt.Errorf("errcode %d: %s", frame.ErrCode, frame.ErrMsg):
				default:
				}
			}
		} else {
			slog.Info("[wecom] auth acknowledged successfully")
			// Signal auth success if this is the pending auth request
			if authCh != nil && reqID == expectedReqID {
				select {
				case authCh <- nil:
				default:
				}
			}
		}
		return
	}

	// Heartbeat response: req_id starts with "ping"
	if strings.HasPrefix(reqID, "ping") {
		if frame.ErrCode != 0 {
			slog.Warn("[wecom] heartbeat ack error", "errcode", frame.ErrCode, "errmsg", frame.ErrMsg)
		} else {
			slog.Debug("[wecom] heartbeat acknowledged")
		}
		return
	}

	// Unknown frame type
	slog.Warn("[wecom] unknown frame", "cmd", frame.Cmd, "reqID", reqID, "raw", string(data))
}

func (a *WeComAdapter) handleIncomingMessage(frame WeComFrame) {
	if a.handler == nil {
		return
	}

	var body WeComMessageBody
	if err := json.Unmarshal(frame.Body, &body); err != nil {
		slog.Warn("[wecom] failed to parse message body", "error", err)
		return
	}

	messageID := body.MsgID
	if messageID != "" {
		if _, loaded := a.seenMsgs.LoadOrStore(messageID, struct{}{}); loaded {
			slog.Info("[wecom] duplicate message_id, skipping", "message_id", messageID)
			return
		}
		go func() {
			time.Sleep(5 * time.Minute)
			a.seenMsgs.Delete(messageID)
		}()
	}

	reqID := ""
	if frame.Headers != nil {
		if v, ok := frame.Headers["req_id"].(string); ok {
			reqID = v
		}
	}

	a.mu.Lock()
	a.lastReqID = reqID
	a.lastResponseURL = body.ResponseURL
	a.mu.Unlock()

	content := ""
	msgType := body.MsgType

	switch msgType {
	case "text":
		if body.Text != nil {
			content = body.Text.Content
		}
	case "image":
		if body.Image != nil {
			contentJSON, _ := json.Marshal(body.Image)
			content = string(contentJSON)
		}
	case "file":
		if body.File != nil {
			contentJSON, _ := json.Marshal(body.File)
			content = string(contentJSON)
		}
	case "voice":
		if body.Voice != nil {
			content = body.Voice.Text
			if content == "" {
				contentJSON, _ := json.Marshal(body.Voice)
				content = string(contentJSON)
			}
		}
	case "mixed":
		if body.Mixed != nil {
			contentJSON, _ := json.Marshal(body.Mixed)
			content = string(contentJSON)
		}
	default:
		contentJSON, _ := json.Marshal(body)
		content = string(contentJSON)
	}

	senderID := body.From.UserID
	chatID := body.ChatID
	if chatID == "" && body.ChatType == "single" {
		chatID = senderID
	}

	fmt.Printf("[WeCom] 收到消息 - 发送者: %s, 群聊: %s, 类型: %s, 内容: %s\n",
		senderID, chatID, msgType, content)

	rawData, _ := json.Marshal(map[string]interface{}{
		"req_id":       reqID,
		"response_url": body.ResponseURL,
		"body":         body,
	})

	a.handler(IncomingMessage{
		ChannelID:  a.channelID,
		Platform:   PlatformWeCom,
		MessageID:  messageID,
		SenderID:   senderID,
		SenderName: senderID,
		ChatID:     chatID,
		ChatName:   "",
		Content:    content,
		MsgType:    msgType,
		RawData:    string(rawData),
	})
}

func (a *WeComAdapter) handleEvent(frame WeComFrame) {
	var body WeComMessageBody
	if err := json.Unmarshal(frame.Body, &body); err != nil {
		return
	}

	if body.Event != nil {
		slog.Info("[wecom] received event", "event_type", body.Event.EventType)
	}
}

func (a *WeComAdapter) handleReplyAck(frame WeComFrame) {
	if frame.Headers == nil {
		return
	}
	reqID, ok := frame.Headers["req_id"].(string)
	if !ok {
		return
	}

	if ch, loaded := a.pendingReplies.LoadAndDelete(reqID); loaded {
		errCh := ch.(chan error)
		if frame.ErrCode != 0 {
			errCh <- fmt.Errorf("reply failed: errcode=%d errmsg=%s", frame.ErrCode, frame.ErrMsg)
		} else {
			errCh <- nil
		}
	}
}

func (a *WeComAdapter) Disconnect(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.cancel != nil {
		a.cancel()
		a.cancel = nil
	}

	if a.conn != nil {
		a.conn.Close()
		a.conn = nil
	}

	a.connected.Store(false)
	a.handler = nil
	return nil
}

func (a *WeComAdapter) IsConnected() bool {
	return a.connected.Load()
}

// SendMessage sends a message to WeCom.
// For WeCom, the targetID is typically the chatid (group) or userid (single chat).
// Content can be plain text or JSON with msg_type and content fields.
func (a *WeComAdapter) SendMessage(ctx context.Context, targetID string, content string) error {
	a.mu.Lock()
	responseURL := a.lastResponseURL
	reqID := a.lastReqID
	a.mu.Unlock()

	msgType, payload, err := buildWeComOutgoingMessage(content)
	if err != nil {
		return err
	}

	if responseURL != "" {
		return a.sendViaHTTP(ctx, responseURL, msgType, payload)
	}

	if reqID != "" {
		return a.sendViaWS(ctx, reqID, targetID, msgType, payload)
	}

	return a.sendDirectMessage(ctx, targetID, msgType, payload)
}

func (a *WeComAdapter) sendViaHTTP(ctx context.Context, responseURL string, msgType string, payload interface{}) error {
	body := map[string]interface{}{
		"msgtype": msgType,
	}
	body[msgType] = payload

	data, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", responseURL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: wecomRequestTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("http response error: status=%d body=%s", resp.StatusCode, string(respBody))
	}

	return nil
}

func (a *WeComAdapter) sendViaWS(ctx context.Context, reqID string, targetID string, msgType string, payload interface{}) error {
	replyBody := map[string]interface{}{
		"msgtype": msgType,
	}
	replyBody[msgType] = payload

	bodyData, _ := json.Marshal(replyBody)

	frame := WeComFrame{
		Cmd: "aibot_respond_msg",
		Headers: map[string]interface{}{
			"req_id": reqID,
		},
		Body: bodyData,
	}

	errCh := make(chan error, 1)
	a.pendingReplies.Store(reqID, errCh)

	if err := a.sendFrame(frame); err != nil {
		a.pendingReplies.Delete(reqID)
		return err
	}

	select {
	case <-ctx.Done():
		a.pendingReplies.Delete(reqID)
		return ctx.Err()
	case err := <-errCh:
		return err
	case <-time.After(wecomRequestTimeout):
		a.pendingReplies.Delete(reqID)
		return fmt.Errorf("reply timeout")
	}
}

func (a *WeComAdapter) sendDirectMessage(ctx context.Context, targetID string, msgType string, payload interface{}) error {
	sendBody := map[string]interface{}{
		"chatid":  targetID,
		"msgtype": msgType,
	}
	sendBody[msgType] = payload

	bodyData, _ := json.Marshal(sendBody)

	frame := WeComFrame{
		Cmd: "aibot_send_msg",
		Headers: map[string]interface{}{
			"req_id": generateReqID("aibot_send_msg"),
		},
		Body: bodyData,
	}

	return a.sendFrame(frame)
}

type wecomOutgoingMessage struct {
	MsgType  string          `json:"msg_type"`
	Content  json.RawMessage `json:"content"`
	Text     string          `json:"text"`
	Markdown string          `json:"markdown"`
	FilePath string          `json:"file_path"`
	FileURL  string          `json:"file_url"`
}

func buildWeComOutgoingMessage(raw string) (string, interface{}, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil, fmt.Errorf("wecom message content is empty")
	}

	if !strings.HasPrefix(trimmed, "{") {
		return "markdown", map[string]string{"content": raw}, nil
	}

	var payload wecomOutgoingMessage
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return "markdown", map[string]string{"content": raw}, nil
	}

	msgType := strings.TrimSpace(payload.MsgType)
	if msgType == "" {
		return "markdown", map[string]string{"content": raw}, nil
	}

	if len(payload.Content) > 0 && string(payload.Content) != "null" {
		var contentMap map[string]interface{}
		if err := json.Unmarshal(payload.Content, &contentMap); err == nil {
			return msgType, contentMap, nil
		}
		return msgType, payload.Content, nil
	}

	switch msgType {
	case "text":
		text := payload.Text
		if text == "" {
			text = raw
		}
		return "text", map[string]string{"content": text}, nil
	case "markdown":
		md := payload.Markdown
		if md == "" {
			md = payload.Text
		}
		if md == "" {
			md = raw
		}
		return "markdown", map[string]string{"content": md}, nil
	default:
		return msgType, map[string]string{"content": raw}, nil
	}
}

// DownloadFile downloads and decrypts a file from WeCom.
func (a *WeComAdapter) DownloadFile(ctx context.Context, url string, aesKey string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, "", err
	}

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("download failed: status=%d", resp.StatusCode)
	}

	encrypted, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("read response failed: %w", err)
	}

	filename := ""
	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		if idx := strings.Index(cd, "filename="); idx != -1 {
			filename = strings.Trim(cd[idx+9:], "\"")
		}
	}

	if aesKey == "" {
		return encrypted, filename, nil
	}

	decrypted, err := decryptAES256CBC(encrypted, aesKey)
	if err != nil {
		return nil, "", fmt.Errorf("decrypt failed: %w", err)
	}

	return decrypted, filename, nil
}

func decryptAES256CBC(data []byte, aesKeyB64 string) ([]byte, error) {
	key, err := base64.StdEncoding.DecodeString(aesKeyB64)
	if err != nil {
		return nil, fmt.Errorf("decode aes key: %w", err)
	}

	if len(key) != 32 {
		return nil, fmt.Errorf("invalid aes key length: %d", len(key))
	}

	if len(data) < aes.BlockSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	iv := data[:aes.BlockSize]
	ciphertext := data[aes.BlockSize:]

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	mode := cipher.NewCBCDecrypter(block, iv)
	plaintext := make([]byte, len(ciphertext))
	mode.CryptBlocks(plaintext, ciphertext)

	plaintext = pkcs7Unpad(plaintext)
	return plaintext, nil
}

func pkcs7Unpad(data []byte) []byte {
	if len(data) == 0 {
		return data
	}
	padding := int(data[len(data)-1])
	if padding > len(data) || padding > aes.BlockSize {
		return data
	}
	return data[:len(data)-padding]
}

// UploadFile uploads a local file to WeCom and returns the media_id.
// WeCom AI Bot SDK doesn't have direct upload API; files are sent via markdown image URL or file message.
// This is a placeholder for future media upload API if available.
func (a *WeComAdapter) UploadFile(ctx context.Context, filePath string) (string, error) {
	return "", fmt.Errorf("wecom aibot does not support direct file upload, use file URL instead")
}

// SendStreamMessage sends a streaming message to WeCom.
func (a *WeComAdapter) SendStreamMessage(ctx context.Context, reqID string, streamID string, content string, finish bool) error {
	replyBody := map[string]interface{}{
		"stream_id":   streamID,
		"stream_text": content,
		"finish":      finish,
	}

	bodyData, _ := json.Marshal(replyBody)

	frame := WeComFrame{
		Cmd: "aibot_respond_msg",
		Headers: map[string]interface{}{
			"req_id": reqID,
		},
		Body: bodyData,
	}

	return a.sendFrame(frame)
}

// SendImage sends an image URL to WeCom as markdown.
func (a *WeComAdapter) SendImage(ctx context.Context, targetID string, imageURL string) error {
	content := fmt.Sprintf("![image](%s)", imageURL)
	return a.SendMessage(ctx, targetID, content)
}

// SendFile sends a file URL to WeCom as markdown link.
func (a *WeComAdapter) SendFile(ctx context.Context, targetID string, fileURL string, fileName string) error {
	if fileName == "" {
		fileName = filepath.Base(fileURL)
	}
	content := fmt.Sprintf("[%s](%s)", fileName, fileURL)
	return a.SendMessage(ctx, targetID, content)
}

var reqIDCounter atomic.Int64

func generateReqID(prefix ...string) string {
	p := "req"
	if len(prefix) > 0 && prefix[0] != "" {
		p = prefix[0]
	}
	return fmt.Sprintf("%s_%d_%d", p, time.Now().UnixNano(), reqIDCounter.Add(1))
}
