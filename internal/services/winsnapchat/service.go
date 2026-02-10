package winsnapchat

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	"willclaw/internal/errs"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	// EventName is the frontend subscription channel.
	// Frontend listens via: Events.On("winsnap:chat", handler)
	EventName = "winsnap:chat"
)

// StreamPayload follows an SSE-like structure: { event, data } plus requestId
// to support concurrent requests.
type StreamPayload struct {
	RequestID string `json:"requestId"`
	Event     string `json:"event"`
	Data      string `json:"data"`
}

// WinsnapChatService provides streaming chat events for the winsnap window.
//
// Current implementation simulates the willclaw-manage streaming API.
// Later we can replace simulateStream() with an actual HTTP streaming client.
type WinsnapChatService struct {
	app *application.App

	mu      sync.Mutex
	streams map[string]context.CancelFunc
}

func NewWinsnapChatService(app *application.App) *WinsnapChatService {
	return &WinsnapChatService{
		app:     app,
		streams: make(map[string]context.CancelFunc),
	}
}

// Ask starts a streaming request and returns a request ID immediately.
// The stream is delivered via app events (EventName).
func (s *WinsnapChatService) Ask(question string) (string, error) {
	question = strings.TrimSpace(question)
	if question == "" {
		return "", errs.New("error.question_required")
	}
	if s.app == nil {
		return "", errs.New("error.app_required")
	}

	requestID := fmt.Sprintf("%d", time.Now().UnixNano())
	s.app.Logger.Info("WinsnapChatService.Ask called", "question", question, "requestID", requestID)

	// Cancel any previous stream with the same ID (should never happen, but safe).
	s.mu.Lock()
	if cancel, ok := s.streams[requestID]; ok && cancel != nil {
		cancel()
		delete(s.streams, requestID)
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.streams[requestID] = cancel
	s.mu.Unlock()

	go func() {
		defer func() {
			s.mu.Lock()
			delete(s.streams, requestID)
			s.mu.Unlock()
			s.app.Logger.Debug("stream goroutine finished", "requestID", requestID)
		}()
		s.simulateStream(ctx, requestID, question)
	}()

	return requestID, nil
}

// Cancel stops a running stream (best-effort).
func (s *WinsnapChatService) Cancel(requestID string) error {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return errs.New("error.request_id_required")
	}

	s.mu.Lock()
	cancel := s.streams[requestID]
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}

func (s *WinsnapChatService) emit(requestID string, eventName string, data string) {
	payload := StreamPayload{
		RequestID: requestID,
		Event:     eventName,
		Data:      data,
	}
	s.app.Logger.Debug("emit winsnap:chat", "requestId", requestID, "event", eventName, "dataLen", len(data))
	_ = s.app.Event.Emit(EventName, payload)
}

func (s *WinsnapChatService) simulateStream(ctx context.Context, requestID string, question string) {
	// Helper sleep that respects cancellation.
	sleep := func(d time.Duration) bool {
		t := time.NewTimer(d)
		defer t.Stop()
		select {
		case <-ctx.Done():
			return false
		case <-t.C:
			return true
		}
	}

	// Give the frontend a brief moment to receive the requestId and start filtering.
	if !sleep(60 * time.Millisecond) {
		return
	}

	now := time.Now().Unix()
	dialogueID := fmt.Sprintf("%d", 100000+rand.Intn(900000))
	sessionID := fmt.Sprintf("%d", 100000+rand.Intn(900000))

	s.emit(requestID, "ping", fmt.Sprintf("%d", now))
	if !sleep(30 * time.Millisecond) {
		return
	}
	s.emit(requestID, "dialogue_id", dialogueID)
	if !sleep(30 * time.Millisecond) {
		return
	}
	s.emit(requestID, "session_id", sessionID)
	if !sleep(30 * time.Millisecond) {
		return
	}

	// Minimal customer/c_message payloads for future usage.
	customer, _ := json.Marshal(map[string]any{
		"id":          "1",
		"openid":      "1",
		"name":        "访客",
		"avatar":      "/public/user_avatar_2x.png",
		"create_time": fmt.Sprintf("%d", now),
	})
	s.emit(requestID, "customer", string(customer))

	cmsg, _ := json.Marshal(map[string]any{
		"id":          fmt.Sprintf("%d", 10000000+rand.Intn(90000000)),
		"content":     question,
		"is_customer": "1",
		"dialogue_id": dialogueID,
		"session_id":  sessionID,
		"create_time": fmt.Sprintf("%d", now),
	})
	s.emit(requestID, "c_message", string(cmsg))

	// Search-not-found simulation: no `sending`, only `ai_message.content`.
	searchNotFound := strings.Contains(question, "数据公式") || strings.Contains(strings.ToLower(question), "notfound")
	if searchNotFound {
		aiMessage, _ := json.Marshal(map[string]any{
			"content":  "哎呀，这个问题我暂时还不太清楚呢～（对手指）",
			"msg_type": "2",
		})
		s.emit(requestID, "ai_message", string(aiMessage))
		// Small delay to ensure ai_message arrives before finish (Wails event order issue)
		if !sleep(30 * time.Millisecond) {
			return
		}
		s.emit(requestID, "finish", fmt.Sprintf("%d", time.Now().Unix()))
		return
	}

	// Normal streaming simulation: send `sending` chunks (typing effect on frontend),
	// then send a final `ai_message` as a fallback.
	answer := "您好！请问您是想了解关于流式输出的相关信息吗？还是遇到了什么具体问题需要帮助呢？"
	chunks := splitToChunks(answer, 2, 6)
	for _, c := range chunks {
		if !sleep(90 * time.Millisecond) {
			return
		}
		s.emit(requestID, "sending", c)
	}

	if !sleep(80 * time.Millisecond) {
		return
	}
	aiMessage, _ := json.Marshal(map[string]any{
		"content":  answer,
		"msg_type": "1",
	})
	s.emit(requestID, "ai_message", string(aiMessage))
	// Small delay to ensure ai_message arrives before finish (Wails event order issue)
	if !sleep(30 * time.Millisecond) {
		return
	}
	s.emit(requestID, "finish", fmt.Sprintf("%d", time.Now().Unix()))
}

func splitToChunks(s string, minLen, maxLen int) []string {
	if minLen <= 0 {
		minLen = 1
	}
	if maxLen < minLen {
		maxLen = minLen
	}
	runes := []rune(s)
	out := make([]string, 0, len(runes)/minLen+1)
	for i := 0; i < len(runes); {
		n := minLen
		if maxLen > minLen {
			n = minLen + rand.Intn(maxLen-minLen+1)
		}
		if i+n > len(runes) {
			n = len(runes) - i
		}
		out = append(out, string(runes[i:i+n]))
		i += n
	}
	return out
}
