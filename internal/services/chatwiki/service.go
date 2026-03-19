package chatwiki

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"chatclaw/internal/define"
	"chatclaw/internal/sqlite"
	"chatclaw/internal/sysinfo"

	"github.com/uptrace/bun"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// Binding represents a ChatWiki binding record exposed to the frontend.
type Binding struct {
	ID              int64  `json:"id"`
	ServerURL       string `json:"server_url"`
	Token           string `json:"token"`
	TTL             int64  `json:"ttl"`
	Exp             int64  `json:"exp"`
	UserID          string `json:"user_id"`
	UserName        string `json:"user_name"`
	ChatWikiVersion string `json:"chatwiki_version"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

// Robot represents a ChatWiki robot/application item returned to the frontend.
type Robot struct {
	ID                  string `json:"id"`
	RobotKey            string `json:"robot_key"`
	Name                string `json:"name"`
	Intro               string `json:"intro"`
	Type                string `json:"type"`
	Icon                string `json:"icon"`
	SwitchStatus        int    `json:"chat_claw_switch_status"`
	ApplicationTypeCode string `json:"application_type"`
}

// Library represents a ChatWiki knowledge base item returned to the frontend.
type Library struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Intro        string `json:"intro"`
	Type         string `json:"type"`
	TypeName     string `json:"type_name"`
	SwitchStatus int    `json:"chat_claw_switch_status"`
}

// LibraryGroup represents a ChatWiki file group in a knowledge base.
type LibraryGroup struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Total int    `json:"total"`
}

// LibraryFile represents a ChatWiki file item in a knowledge base.
type LibraryFile struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Extension string `json:"extension"`
	Status    int    `json:"status"`
	UpdatedAt string `json:"updated_at"`
	ThumbPath string `json:"thumb_path"`
}

// LibraryParagraph represents a QA paragraph item in a knowledge base.
type LibraryParagraph struct {
	ID       string   `json:"id"`
	Question string   `json:"question"`
	Answer   string   `json:"answer"`
	Images   []string `json:"images"`
}

// LibraryParagraphPage represents a page result for QA paragraphs.
// Total is -1 when upstream does not return a reliable total count.
type LibraryParagraphPage struct {
	List  []LibraryParagraph `json:"list"`
	Total int                `json:"total"`
}

// LibraryFilePage represents a page result for library file list.
// Total is -1 when upstream does not return a reliable total count.
type LibraryFilePage struct {
	List  []LibraryFile `json:"list"`
	Total int           `json:"total"`
}

// TeamChatMessage is a single history message for team chat.
type TeamChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// TeamChatInput is the frontend input for team mode streaming chat.
type TeamChatInput struct {
	ConversationID int64             `json:"conversation_id"`
	TeamAgentID    int64             `json:"team_agent_id"` // team conversation group agent id
	TabID          string            `json:"tab_id"`
	RobotKey       string            `json:"robot_key"`
	Content        string            `json:"content"`
	Messages       []TeamChatMessage `json:"messages"`
	UseNewDialogue int               `json:"use_new_dialogue"` // 1 = new session; 0 or omit = continue existing
	DialogueID     string            `json:"dialogue_id"`      // from previous SSE response when continuing
}

// TeamChatResult returns request/message IDs so frontend can correlate state.
type TeamChatResult struct {
	RequestID string `json:"request_id"`
	MessageID int64  `json:"message_id"`
}

type chatWikiRobotRaw struct {
	ID              string `json:"id"`
	RobotKey        string `json:"robot_key"`
	RobotName       string `json:"robot_name"`
	RobotIntro      string `json:"robot_intro"`
	RobotAvatar     string `json:"robot_avatar"`
	ApplicationType string `json:"application_type"`
	SwitchStatus    string `json:"chat_claw_switch_status"`
}

type chatWikiLibraryRaw struct {
	ID           string `json:"id"`
	LibraryName  string `json:"library_name"`
	LibraryIntro string `json:"library_intro"`
	Type         string `json:"type"`
	SwitchStatus string `json:"chat_claw_switch_status"`
}

// bindingModel is the bun ORM model for the chatwiki_bindings table.
type bindingModel struct {
	bun.BaseModel `bun:"table:chatwiki_bindings"`

	ID              int64     `bun:"id,pk,autoincrement"`
	ServerURL       string    `bun:"server_url,notnull"`
	Token           string    `bun:"token,notnull"`
	TTL             int64     `bun:"ttl,notnull"`
	Exp             int64     `bun:"exp,notnull"`
	UserID          string    `bun:"user_id,notnull"`
	UserName        string    `bun:"user_name,notnull"`
	ChatWikiVersion string    `bun:"chatwiki_version,notnull"`
	CreatedAt       time.Time `bun:"created_at,notnull"`
	UpdatedAt       time.Time `bun:"updated_at,notnull"`
}

// ChatWikiService exposes ChatWiki binding operations to the frontend via Wails.
type ChatWikiService struct {
	app *application.App

	teamMu      sync.Mutex
	teamCancels map[int64]context.CancelFunc
	teamSeq     map[string]int32

	refreshMu sync.Mutex // serializes token refresh inside GetBinding
}

var bindingWriteMu sync.Mutex

// NewChatWikiService creates a new ChatWikiService instance (used by bootstrap and Wails bindings).
func NewChatWikiService(app *application.App) *ChatWikiService {
	return &ChatWikiService{
		app:         app,
		teamCancels: make(map[int64]context.CancelFunc),
		teamSeq:     make(map[string]int32),
	}
}

// GetCloudURL returns the ChatWiki Cloud server URL for this build (dev or production).
// The frontend uses this to open the correct auth page instead of relying on a hardcoded URL.
func (s *ChatWikiService) GetCloudURL() string {
	return define.GetChatWikiCloudURL()
}

// getBindingFromDB reads the latest binding from DB only (no refresh logic). Returns (nil, nil) when no row.
func (s *ChatWikiService) getBindingFromDB() (*bindingModel, error) {
	db := sqlite.DB()
	if db == nil {
		return nil, nil
	}
	if err := ensureChatWikiBindingVersionColumn(db); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	m := new(bindingModel)
	err := db.NewSelect().Model(m).OrderExpr("id DESC").Limit(1).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return m, nil
}

// GetBinding returns the current binding, or nil if none exists.
// If the token expires within 24 hours, it is refreshed first and the updated binding is returned.
func (s *ChatWikiService) GetBinding() (*Binding, error) {
	s.app.Logger.Info("[chatwiki] GetBinding start")
	m, err := s.getBindingFromDB()
	if err != nil {
		s.app.Logger.Error("[chatwiki] GetBinding db read failed", "error", err)
		return nil, err
	}
	if m == nil {
		s.app.Logger.Warn("[chatwiki] GetBinding no binding found")
		return nil, nil
	}
	b := toBinding(m)
	if b.Exp <= 0 {
		s.app.Logger.Info("[chatwiki] GetBinding returning binding without exp",
			"user_id", b.UserID,
			"server_url", strings.TrimSpace(b.ServerURL),
			"token_len", len(strings.TrimSpace(b.Token)),
		)
		return b, nil
	}
	now := time.Now().Unix()
	if b.Exp-now >= refreshTokenExpireThreshold {
		s.app.Logger.Info("[chatwiki] GetBinding using cached binding",
			"user_id", b.UserID,
			"server_url", strings.TrimSpace(b.ServerURL),
			"token_len", len(strings.TrimSpace(b.Token)),
			"remaining_sec", b.Exp-now,
		)
		return b, nil
	}

	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	// Re-fetch in case another goroutine already refreshed
	m2, err2 := s.getBindingFromDB()
	if err2 != nil {
		s.app.Logger.Warn("[ChatWiki] Failed to re-fetch binding after refresh lock", "error", err2)
		return b, nil
	}
	if m2 == nil {
		return b, nil
	}
	b2 := toBinding(m2)
	if b2.Exp-time.Now().Unix() >= refreshTokenExpireThreshold {
		return b2, nil
	}

	s.app.Logger.Info("[ChatWiki] Token expires within 24h, refreshing",
		"exp", b2.Exp, "remaining_sec", b2.Exp-time.Now().Unix())
	newToken, newExp, err := s.callRefreshTokenAPI(b2)
	if err != nil {
		s.app.Logger.Warn("[ChatWiki] Token refresh failed", "error", err)
		return b2, nil
	}
	if err := s.updateBindingTokenAndExp(newToken, newExp); err != nil {
		s.app.Logger.Warn("[ChatWiki] Failed to save refreshed token", "error", err)
		return b2, nil
	}
	m3, _ := s.getBindingFromDB()
	if m3 == nil {
		return b2, nil
	}
	return toBinding(m3), nil
}

// SaveBinding creates or replaces the binding. Called from deeplink handler.
func SaveBinding(app *application.App, serverURL, token, ttl, exp, userID, userName, chatWikiVersion string) error {
	db := sqlite.DB()
	if db == nil {
		return nil
	}
	if err := ensureChatWikiBindingVersionColumn(db); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := saveBindingWithDB(ctx, db, serverURL, token, ttl, exp, userID, userName, chatWikiVersion)
	if err != nil {
		if app != nil {
			app.Logger.Error("Failed to save chatwiki binding", "error", err)
		}
		return err
	}
	if app != nil {
		app.Logger.Info("ChatWiki binding saved", "user_id", userID, "user_name", userName)
	}
	return nil
}

func saveBindingWithDB(ctx context.Context, db bun.IDB, serverURL, token, ttl, exp, userID, userName, chatWikiVersion string) error {
	ttlInt, _ := strconv.ParseInt(ttl, 10, 64)
	expInt, _ := strconv.ParseInt(exp, 10, 64)
	now := time.Now().UTC()
	chatWikiVersion = normalizeChatWikiVersion(chatWikiVersion)

	bindingWriteMu.Lock()
	defer bindingWriteMu.Unlock()

	return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewDelete().Model((*bindingModel)(nil)).Where("1=1").Exec(ctx); err != nil {
			return err
		}

		m := &bindingModel{
			ServerURL:       serverURL,
			Token:           token,
			TTL:             ttlInt,
			Exp:             expInt,
			UserID:          userID,
			UserName:        userName,
			ChatWikiVersion: chatWikiVersion,
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		_, err := tx.NewInsert().Model(m).Exec(ctx)
		return err
	})
}

func resolveChatWikiVersionFromLoginSource(loginSource string) string {
	switch strings.TrimSpace(loginSource) {
	case "cloud":
		return "yun"
	case "open-source":
		return "dev"
	default:
		return "dev"
	}
}

// SaveBindingFromCallback persists a ChatWiki auth callback using the frontend login source
// instead of trusting any upstream version field.
func (s *ChatWikiService) SaveBindingFromCallback(serverURL, token, ttl, exp, userID, userName, loginSource string) error {
	var app *application.App
	if s != nil {
		app = s.app
	}
	return SaveBinding(
		app,
		serverURL,
		token,
		ttl,
		exp,
		userID,
		userName,
		resolveChatWikiVersionFromLoginSource(loginSource),
	)
}

// TokenForceOffline calls POST /manage/chatclaw/tokenForceOffline with reason "logout"
// so the server can invalidate the current token. Call before DeleteBinding when user unbinds.
// If there is no binding or the request fails, it returns nil so local unbind can still proceed.
func (s *ChatWikiService) TokenForceOffline() error {
	binding, err := s.GetBinding()
	if err != nil || binding == nil {
		return nil
	}
	baseURL := strings.TrimRight(binding.ServerURL, "/")
	apiURL := baseURL + "/manage/chatclaw/tokenForceOffline"

	body := map[string]string{"reason": "logout"}
	bodyBytes, _ := json.Marshal(body)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		s.app.Logger.Warn("[ChatWiki] tokenForceOffline create request failed", "error", err)
		return nil
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Token", binding.Token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		s.app.Logger.Warn("[ChatWiki] tokenForceOffline request failed", "error", err)
		return nil
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	bodyPreview := strings.TrimSpace(string(respBody))
	if len(bodyPreview) > 256 {
		bodyPreview = bodyPreview[:256] + "...(truncated)"
	}
	s.app.Logger.Info(
		"[ChatWiki] tokenForceOffline response",
		"status",
		resp.StatusCode,
		"body_size",
		len(respBody),
		"body_preview",
		bodyPreview,
	)
	return nil
}

// DeleteBinding removes the current binding.
func (s *ChatWikiService) DeleteBinding() error {
	db := sqlite.DB()
	if db == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := db.NewDelete().Model((*bindingModel)(nil)).Where("1=1").Exec(ctx); err != nil {
		return err
	}
	return clearSyncedModelCatalogFromDB(ctx, db, "chatwiki")
}

// errChatWikiAuthExpired is returned when the server returns 401 (e.g. "账号未获取登录信息").
// Frontend should show re-auth hint and treat binding as expired.
var errChatWikiAuthExpired = errors.New("CHATWIKI_AUTH_EXPIRED")

// markBindingExpired sets the latest binding's exp to 0 so the client treats it as expired
// and guides the user to re-authorize.
func (s *ChatWikiService) markBindingExpired() {
	db := sqlite.DB()
	if db == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Update the latest row (max id) to exp=0
	_, err := db.NewUpdate().Model((*bindingModel)(nil)).
		Set("exp = ?", int64(0)).
		Set("updated_at = ?", time.Now().UTC()).
		Where("id = (SELECT MAX(id) FROM chatwiki_bindings)").
		Exec(ctx)
	if err != nil {
		s.app.Logger.Warn("Failed to mark chatwiki binding expired", "error", err)
		return
	}
	s.app.Logger.Info("[ChatWiki] Binding marked expired (exp=0) for re-auth")
}

// refreshTokenExpireThreshold is the remaining time (seconds) below which we refresh the token.
// If exp - now < refreshTokenExpireThreshold, we call the refresh API.
const refreshTokenExpireThreshold = 24 * 3600 // 24 hours

// updateBindingTokenAndExp updates the latest binding row with new token and exp (and TTL derived from exp).
func (s *ChatWikiService) updateBindingTokenAndExp(token string, exp int64) error {
	db := sqlite.DB()
	if db == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now().UTC()
	ttl := exp - now.Unix()
	if ttl < 0 {
		ttl = 0
	}
	_, err := db.NewUpdate().Model((*bindingModel)(nil)).
		Set("token = ?", token).
		Set("ttl = ?", ttl).
		Set("exp = ?", exp).
		Set("updated_at = ?", now).
		Where("id = (SELECT MAX(id) FROM chatwiki_bindings)").
		Exec(ctx)
	if err != nil {
		s.app.Logger.Warn("Failed to update chatwiki binding token/exp", "error", err)
		return err
	}
	s.app.Logger.Info("[ChatWiki] Binding token refreshed", "exp", exp)
	return nil
}

// callRefreshTokenAPI calls POST /manage/chatclaw/refreshToken and returns new token and exp, or error.
// On 401 it marks the binding expired and returns errChatWikiAuthExpired.
func (s *ChatWikiService) callRefreshTokenAPI(binding *Binding) (newToken string, newExp int64, err error) {
	baseURL := strings.TrimRight(binding.ServerURL, "/")
	apiURL := baseURL + "/manage/chatclaw/refreshToken"

	body := map[string]string{
		"os_type":    sysinfo.OSType(),
		"os_version": sysinfo.OSVersion(),
	}
	bodyBytes, _ := json.Marshal(body)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", 0, fmt.Errorf("create refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Token", binding.Token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("refresh request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, fmt.Errorf("read refresh response: %w", err)
	}

	var apiResp struct {
		Res  int    `json:"res"`
		Msg  string `json:"msg"`
		Data struct {
			Token string `json:"token"`
			Exp   int64  `json:"exp"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return "", 0, fmt.Errorf("decode refresh response: %w", err)
	}

	s.app.Logger.Info("[ChatWiki] refreshToken response",
		"status", resp.StatusCode,
		"res", apiResp.Res,
	)

	if apiResp.Res == 401 || resp.StatusCode == http.StatusUnauthorized {
		s.markBindingExpired()
		msg := apiResp.Msg
		if msg == "" {
			msg = "账号未获取登录信息，请重新授权"
		}
		return "", 0, fmt.Errorf("%w: %s", errChatWikiAuthExpired, msg)
	}

	if apiResp.Res != 0 {
		return "", 0, fmt.Errorf("refresh API error res=%d msg=%s", apiResp.Res, apiResp.Msg)
	}

	if apiResp.Data.Token == "" || apiResp.Data.Exp <= 0 {
		return "", 0, fmt.Errorf("refresh response missing token or exp")
	}

	return apiResp.Data.Token, apiResp.Data.Exp, nil
}

// GetRobotList fetches the robot/application list from ChatWiki API (only robots with switch_status open).
func (s *ChatWikiService) GetRobotList() ([]Robot, error) {
	return s.getRobotList(1)
}

// GetRobotListAll fetches all robots/applications from ChatWiki API (no only_open filter).
// Used by account management settings to show and manage all applications.
func (s *ChatWikiService) GetRobotListAll() ([]Robot, error) {
	return s.getRobotList(0)
}

func (s *ChatWikiService) getRobotList(onlyOpen int) ([]Robot, error) {
	binding, err := s.GetBinding()
	if err != nil || binding == nil {
		return nil, fmt.Errorf("no binding found")
	}

	baseURL := strings.TrimRight(binding.ServerURL, "/")
	q := url.Values{}
	q.Set("application_type", "-1")
	if onlyOpen != 0 {
		q.Set("only_open", "1")
	}
	apiURL := baseURL + "/manage/chatclaw/getRobotList?" + q.Encode()

	s.app.Logger.Info("[ChatWiki] getRobotList request",
		"url", apiURL,
		"only_open", onlyOpen,
		"token_length", len(binding.Token),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Token", binding.Token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		s.app.Logger.Error("[ChatWiki] getRobotList request failed", "error", err)
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	s.app.Logger.Info("[ChatWiki] getRobotList response",
		"status", resp.StatusCode,
		"body", string(body),
	)

	if resp.StatusCode == http.StatusUnauthorized {
		s.markBindingExpired()
		var apiErr struct {
			Res int    `json:"res"`
			Msg string `json:"msg"`
		}
		_ = json.Unmarshal(body, &apiErr)
		msg := apiErr.Msg
		if msg == "" {
			msg = "账号未获取登录信息，请重新授权"
		}
		return nil, fmt.Errorf("%w: %s", errChatWikiAuthExpired, msg)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var apiResp struct {
		Res  int                `json:"res"`
		Code int                `json:"code"`
		Msg  string             `json:"msg"`
		Data []chatWikiRobotRaw `json:"data"`
	}
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	resultCode := apiResp.Res
	if resultCode == 0 && apiResp.Code != 0 {
		resultCode = apiResp.Code
	}
	if resultCode == 401 {
		s.markBindingExpired()
		msg := apiResp.Msg
		if msg == "" {
			msg = "账号未获取登录信息，请重新授权"
		}
		return nil, fmt.Errorf("%w: %s", errChatWikiAuthExpired, msg)
	}
	if resultCode != 0 {
		return nil, fmt.Errorf("API error code=%d msg=%s", resultCode, apiResp.Msg)
	}

	robots := make([]Robot, 0, len(apiResp.Data))
	for _, item := range apiResp.Data {
		robotType := "chat"
		if item.ApplicationType == "1" {
			robotType = "workflow"
		}
		switchStatus := parseSwitchStatus(item.SwitchStatus)
		fullAvatar := normalizeAssetURL(binding.ServerURL, item.RobotAvatar)
		s.app.Logger.Info("[ChatWiki] robot avatar resolved",
			"robot_id", item.ID,
			"robot_name", item.RobotName,
			"raw_robot_avatar", item.RobotAvatar,
			"full_robot_avatar", fullAvatar,
			"switch_status", switchStatus,
		)
		robots = append(robots, Robot{
			ID:                  item.ID,
			RobotKey:            item.RobotKey,
			Name:                item.RobotName,
			Intro:               item.RobotIntro,
			Type:                robotType,
			Icon:                fullAvatar,
			SwitchStatus:        switchStatus,
			ApplicationTypeCode: item.ApplicationType,
		})
	}
	return robots, nil
}

// SendTeamMessageStream sends a team-mode message to ChatWiki and forwards SSE chunks
// to frontend chat events (chat:start/chat:chunk/chat:complete/chat:error/chat:stopped).
func (s *ChatWikiService) SendTeamMessageStream(input TeamChatInput) (*TeamChatResult, error) {
	conversationID := input.ConversationID
	if conversationID == 0 {
		return nil, fmt.Errorf("conversation_id is required")
	}
	content := strings.TrimSpace(input.Content)
	if content == "" {
		return nil, fmt.Errorf("content is required")
	}
	robotKey := strings.TrimSpace(input.RobotKey)
	if robotKey == "" {
		return nil, fmt.Errorf("robot_key is required")
	}
	tabID := strings.TrimSpace(input.TabID)
	if tabID == "" {
		tabID = "team"
	}

	binding, err := s.GetBinding()
	if err != nil || binding == nil {
		return nil, fmt.Errorf("no binding found")
	}

	requestID := fmt.Sprintf("team-%d", time.Now().UnixNano())
	messageID := -time.Now().UnixMilli()
	if messageID >= 0 {
		messageID = -1
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.storeTeamCancel(conversationID, cancel)

	s.app.Logger.Info("[ChatWiki][TeamChat] stream start",
		"conversation_id", conversationID,
		"tab_id", tabID,
		"request_id", requestID,
		"robot_key", robotKey,
		"history_count", len(input.Messages),
		"content_len", len(content),
	)

	go s.runTeamChatStream(
		ctx,
		conversationID,
		input.TeamAgentID,
		tabID,
		requestID,
		messageID,
		robotKey,
		content,
		input.Messages,
		input.UseNewDialogue,
		input.DialogueID,
		binding,
	)

	return &TeamChatResult{
		RequestID: requestID,
		MessageID: messageID,
	}, nil
}

// StopTeamMessageStream stops active team-mode SSE stream for a conversation.
func (s *ChatWikiService) StopTeamMessageStream(conversationID int64) error {
	if conversationID == 0 {
		return nil
	}
	s.teamMu.Lock()
	cancel, ok := s.teamCancels[conversationID]
	s.teamMu.Unlock()
	if ok && cancel != nil {
		cancel()
	}
	return nil
}

func (s *ChatWikiService) runTeamChatStream(
	ctx context.Context,
	conversationID int64,
	teamAgentID int64,
	tabID string,
	requestID string,
	messageID int64,
	robotKey string,
	content string,
	history []TeamChatMessage,
	useNewDialogue int,
	dialogueID string,
	binding *Binding,
) {
	defer s.clearTeamCancel(conversationID)

	emitBase := func() map[string]any {
		return map[string]any{
			"conversation_id": conversationID,
			"tab_id":          tabID,
			"request_id":      requestID,
			"message_id":      messageID,
			"seq":             s.nextTeamSeq(requestID),
			"ts":              time.Now().UnixMilli(),
		}
	}

	startPayload := emitBase()
	startPayload["status"] = "streaming"
	s.app.Event.Emit("chat:start", startPayload)

	candidates := []string{chatWikiChatCompletionsURL(binding.ServerURL)}

	reqBody := map[string]any{
		"robot_key": robotKey,
		"content":   content,
		"messages":  history,
		"quote_lib": true,
	}
	if useNewDialogue == 1 {
		reqBody["use_new_dialogue"] = 1
	} else if dialogueID != "" {
		parsedDialogueID, parseErr := strconv.ParseInt(strings.TrimSpace(dialogueID), 10, 64)
		if parseErr != nil || parsedDialogueID <= 0 {
			s.app.Logger.Warn("[ChatWiki][TeamChat] invalid dialogue_id for continue chat",
				"dialogue_id", dialogueID,
				"error", parseErr,
			)
		} else {
			reqBody["dialogue_id"] = parsedDialogueID
		}
	}
	b, err := json.Marshal(reqBody)
	if err != nil {
		s.emitTeamError(emitBase, "encode request body failed: "+err.Error())
		return
	}

	var (
		resp       *http.Response
		lastErrMsg string
	)
	for idx, candidate := range candidates {
		u, parseErr := url.Parse(candidate)
		if parseErr != nil {
			lastErrMsg = "invalid request url: " + parseErr.Error()
			continue
		}

		s.app.Logger.Info("[ChatWiki][TeamChat] request",
			"url", u.String(),
			"candidate_index", idx,
			"token_length", len(binding.Token),
			"conversation_id", conversationID,
			"request_id", requestID,
			"auth_mode", "header_token",
		)

		req, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(b))
		if reqErr != nil {
			lastErrMsg = "create request failed: " + reqErr.Error()
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Token", binding.Token)
		req.Header.Set("AppType", "chat_claw_client")

		tryResp, doErr := http.DefaultClient.Do(req)
		if doErr != nil {
			if ctx.Err() == context.Canceled {
				stopped := emitBase()
				stopped["status"] = "cancelled"
				s.app.Event.Emit("chat:stopped", stopped)
				return
			}
			lastErrMsg = "request failed: " + doErr.Error()
			continue
		}

		contentType := strings.ToLower(strings.TrimSpace(tryResp.Header.Get("Content-Type")))
		if tryResp.StatusCode == http.StatusOK && strings.Contains(contentType, "text/event-stream") {
			resp = tryResp
			break
		}

		raw, _ := io.ReadAll(io.LimitReader(tryResp.Body, 4096))
		_ = tryResp.Body.Close()
		bodyPreview := strings.TrimSpace(string(raw))
		lastErrMsg = fmt.Sprintf("unexpected response status=%d content_type=%s body=%s", tryResp.StatusCode, contentType, bodyPreview)
		s.app.Logger.Warn("[ChatWiki][TeamChat] request not matched",
			"url", u.String(),
			"candidate_index", idx,
			"status", tryResp.StatusCode,
			"content_type", contentType,
			"body_preview", bodyPreview,
		)
	}
	if resp == nil {
		if lastErrMsg == "" {
			lastErrMsg = "all request candidates failed"
		}
		if strings.Contains(strings.ToLower(lastErrMsg), "body=noroute") {
			lastErrMsg = "remote route not found: /manage/chatclaw/chat/completions returned NoRoute"
		}
		s.emitTeamError(emitBase, lastErrMsg)
		return
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
	eventName := ""
	dataLines := make([]string, 0, 4)
	finished := false
	var fullSendingText strings.Builder
	var sseDialogueID string // dialogue_id from SSE, forwarded to frontend for next request

	flush := func() {
		if eventName == "" {
			dataLines = dataLines[:0]
			return
		}
		data := strings.Join(dataLines, "\n")
		s.app.Logger.Info("[ChatWiki][TeamChat] sse event",
			"conversation_id", conversationID,
			"request_id", requestID,
			"event", eventName,
			"data_len", len(data),
			"data", data, // full raw data for debugging
		)
		switch eventName {
		case "sending":
			fullSendingText.WriteString(data)
			chunk := emitBase()
			chunk["delta"] = data
			s.app.Event.Emit("chat:chunk", chunk)
		case "dialogue_id":
			sseDialogueID = strings.TrimSpace(data)
		case "finish", "data":
			// Try to extract dialogue_id from JSON payload (e.g. {"dialogue_id": "xxx", ...})
			if data != "" {
				var parsed map[string]any
				if err := json.Unmarshal([]byte(data), &parsed); err == nil {
					if id, ok := parsed["dialogue_id"]; ok {
						switch v := id.(type) {
						case string:
							sseDialogueID = strings.TrimSpace(v)
						case float64:
							sseDialogueID = strings.TrimSpace(strconv.FormatInt(int64(v), 10))
						}
					}
				}
			}
			if fullSendingText.Len() > 0 {
				s.app.Logger.Info("[ChatWiki][TeamChat] full sending content",
					"conversation_id", conversationID,
					"request_id", requestID,
					"content", fullSendingText.String(), // full assembled content
				)
			}
			complete := emitBase()
			complete["status"] = "success"
			complete["finish_reason"] = "stop"
			if sseDialogueID != "" {
				s.persistTeamMessageRecords(conversationID, teamAgentID, sseDialogueID, content, fullSendingText.String())
				complete["dialogue_id"] = sseDialogueID
			}
			s.app.Event.Emit("chat:complete", complete)
			finished = true
		case "error":
			s.emitTeamError(emitBase, data)
			finished = true
		}
		eventName = ""
		dataLines = dataLines[:0]
	}

	for scanner.Scan() {
		if ctx.Err() == context.Canceled {
			stopped := emitBase()
			stopped["status"] = "cancelled"
			s.app.Event.Emit("chat:stopped", stopped)
			return
		}
		line := strings.TrimRight(scanner.Text(), "\r")
		if line == "" {
			flush()
			if finished {
				return
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if strings.HasPrefix(line, "event:") {
			eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	flush()

	if err := scanner.Err(); err != nil {
		if ctx.Err() == context.Canceled {
			stopped := emitBase()
			stopped["status"] = "cancelled"
			s.app.Event.Emit("chat:stopped", stopped)
			return
		}
		s.emitTeamError(emitBase, "stream read failed: "+err.Error())
		return
	}

	if !finished {
		complete := emitBase()
		complete["status"] = "success"
		complete["finish_reason"] = "stop"
		if sseDialogueID != "" {
			s.persistTeamMessageRecords(conversationID, teamAgentID, sseDialogueID, content, fullSendingText.String())
			complete["dialogue_id"] = sseDialogueID
		}
		s.app.Event.Emit("chat:complete", complete)
	}
}

func (s *ChatWikiService) persistTeamMessageRecords(conversationID int64, teamAgentID int64, dialogueIDRaw string, userContent string, assistantContent string) {
	dialogueID, ok := parsePositiveInt64(strings.TrimSpace(dialogueIDRaw))
	if !ok {
		s.app.Logger.Warn("[ChatWiki][TeamChat] skip persisting messages: invalid dialogue_id",
			"dialogue_id", dialogueIDRaw,
		)
		return
	}
	db := sqlite.DB()
	if db == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conversationID, err := s.ensureTeamConversationByDialogueID(ctx, db, conversationID, teamAgentID, dialogueID, userContent)
	if err != nil {
		s.app.Logger.Warn("[ChatWiki][TeamChat] ensure team conversation failed",
			"dialogue_id", dialogueID,
			"error", err,
		)
		return
	}

	trimmedUserContent := strings.TrimSpace(userContent)
	if trimmedUserContent != "" {
		if _, err := db.ExecContext(
			ctx,
			`INSERT INTO messages (conversation_id, role, content, status, tool_calls) VALUES (?, ?, ?, ?, ?)`,
			conversationID,
			"user",
			trimmedUserContent,
			"success",
			"[]",
		); err != nil {
			s.app.Logger.Warn("[ChatWiki][TeamChat] persist user message failed",
				"conversation_id", conversationID,
				"dialogue_id", dialogueID,
				"error", err,
			)
		}
	}

	trimmedAssistantContent := strings.TrimSpace(assistantContent)
	if trimmedAssistantContent != "" {
		if _, err := db.ExecContext(
			ctx,
			`INSERT INTO messages (conversation_id, role, content, status, finish_reason, tool_calls) VALUES (?, ?, ?, ?, ?, ?)`,
			conversationID,
			"assistant",
			trimmedAssistantContent,
			"success",
			"stop",
			"[]",
		); err != nil {
			s.app.Logger.Warn("[ChatWiki][TeamChat] persist assistant message failed",
				"conversation_id", conversationID,
				"dialogue_id", dialogueID,
				"error", err,
			)
		}
	}
}

func (s *ChatWikiService) ensureTeamConversationByDialogueID(
	ctx context.Context,
	db *bun.DB,
	preferredConversationID int64,
	preferredAgentID int64,
	dialogueID int64,
	lastMessage string,
) (int64, error) {
	if preferredConversationID > 0 {
		var byPreferredID int64
		err := db.NewSelect().
			Table("conversations").
			Column("id").
			Where("id = ?", preferredConversationID).
			Limit(1).
			Scan(ctx, &byPreferredID)
		if err == nil && byPreferredID > 0 {
			trimmedLastMessage := strings.TrimSpace(lastMessage)
			var updateErr error
			if preferredAgentID > 0 {
				_, updateErr = db.ExecContext(
					ctx,
					`UPDATE conversations
SET agent_id = ?, last_message = ?, chat_mode = ?, team_type = ?, dialogue_id = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ?`,
					preferredAgentID,
					trimmedLastMessage,
					"chat",
					"team",
					dialogueID,
					byPreferredID,
				)
			} else {
				_, updateErr = db.ExecContext(
					ctx,
					`UPDATE conversations
SET last_message = ?, chat_mode = ?, team_type = ?, dialogue_id = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ?`,
					trimmedLastMessage,
					"chat",
					"team",
					dialogueID,
					byPreferredID,
				)
			}
			if updateErr != nil {
				return 0, updateErr
			}
			return byPreferredID, nil
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return 0, err
		}
	}

	var existingID int64
	err := db.NewSelect().
		Table("conversations").
		Column("id").
		Where("team_type = ?", "team").
		Where("dialogue_id = ?", dialogueID).
		Limit(1).
		Scan(ctx, &existingID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}

	if existingID > 0 {
		trimmedLastMessage := strings.TrimSpace(lastMessage)
		if trimmedLastMessage != "" || preferredAgentID > 0 {
			var updateErr error
			if preferredAgentID > 0 {
				_, updateErr = db.ExecContext(
					ctx,
					`UPDATE conversations
SET agent_id = ?, last_message = ?, chat_mode = ?, team_type = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ?`,
					preferredAgentID,
					trimmedLastMessage,
					"chat",
					"team",
					existingID,
				)
			} else {
				_, updateErr = db.ExecContext(
					ctx,
					`UPDATE conversations
SET last_message = ?, chat_mode = ?, team_type = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ?`,
					trimmedLastMessage,
					"chat",
					"team",
					existingID,
				)
			}
			if updateErr != nil {
				s.app.Logger.Warn("[ChatWiki][TeamChat] update team conversation failed",
					"conversation_id", existingID,
					"dialogue_id", dialogueID,
					"error", updateErr,
				)
			}
		}
		return existingID, nil
	}

	trimmedLastMessage := strings.TrimSpace(lastMessage)
	title := trimmedLastMessage
	if title == "" {
		title = fmt.Sprintf("Team %d", dialogueID)
	}
	titleRunes := []rune(title)
	if len(titleRunes) > 100 {
		title = string(titleRunes[:100])
	}

	result, err := db.ExecContext(
		ctx,
		`INSERT INTO conversations (agent_id, name, last_message, chat_mode, team_type, dialogue_id)
VALUES (?, ?, ?, ?, ?, ?)`,
		preferredAgentID,
		title,
		trimmedLastMessage,
		"chat",
		"team",
		dialogueID,
	)
	if err != nil {
		return 0, err
	}
	conversationID, err := result.LastInsertId()
	if err != nil || conversationID <= 0 {
		return 0, fmt.Errorf("get inserted conversation id failed: %w", err)
	}

	s.app.Logger.Info("[ChatWiki][TeamChat] persisted team conversation",
		"conversation_id", conversationID,
		"dialogue_id", dialogueID,
		"chat_mode", "chat",
		"team_type", "team",
	)
	return conversationID, nil
}

func parsePositiveInt64(raw string) (int64, bool) {
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || v <= 0 {
		return 0, false
	}
	return v, true
}

func (s *ChatWikiService) emitTeamError(emitBase func() map[string]any, message string) {
	s.app.Logger.Error("[ChatWiki][TeamChat] stream error", "error", message)
	errPayload := emitBase()
	errPayload["status"] = "error"
	errPayload["error_key"] = "error.chat_stream_failed"
	errPayload["error_data"] = map[string]any{
		"Error": message,
	}
	s.app.Event.Emit("chat:error", errPayload)
}

func (s *ChatWikiService) storeTeamCancel(conversationID int64, cancel context.CancelFunc) {
	s.teamMu.Lock()
	defer s.teamMu.Unlock()
	if old, ok := s.teamCancels[conversationID]; ok && old != nil {
		old()
	}
	s.teamCancels[conversationID] = cancel
}

func (s *ChatWikiService) clearTeamCancel(conversationID int64) {
	s.teamMu.Lock()
	defer s.teamMu.Unlock()
	delete(s.teamCancels, conversationID)
}

func (s *ChatWikiService) nextTeamSeq(requestID string) int {
	s.teamMu.Lock()
	defer s.teamMu.Unlock()
	next := s.teamSeq[requestID] + 1
	s.teamSeq[requestID] = next
	return int(next)
}

// GetLibraryList fetches the full knowledge base list from ChatWiki API.
// libType: 0=normal, 2=QA, 3=wechat-official-account
func (s *ChatWikiService) GetLibraryList(libType int) ([]Library, error) {
	return s.getLibraryList(libType, 0)
}

// GetLibraryListOnlyOpen fetches only enabled knowledge bases from ChatWiki API.
// libType: 0=normal, 2=QA, 3=wechat-official-account
func (s *ChatWikiService) GetLibraryListOnlyOpen(libType int) ([]Library, error) {
	return s.getLibraryList(libType, 1)
}

// GetLibraryListOnlyOpenAll fetches all enabled knowledge bases without type filter.
// Same endpoint as getLibraryList but omits type so the server returns the full list (personal + team categories).
func (s *ChatWikiService) GetLibraryListOnlyOpenAll() ([]Library, error) {
	return s.getLibraryList(-1, 1)
}

func (s *ChatWikiService) getLibraryList(libType int, onlyOpen int) ([]Library, error) {
	binding, err := s.GetBinding()
	if err != nil || binding == nil {
		return nil, fmt.Errorf("no binding found")
	}

	baseURL := strings.TrimRight(binding.ServerURL, "/")
	q := url.Values{}
	// Omit type when libType < 0 so server returns full list (no type filter).
	if libType >= 0 {
		q.Set("type", strconv.Itoa(libType))
	}
	q.Set("only_open", strconv.Itoa(onlyOpen))
	apiURL := baseURL + "/manage/chatclaw/getLibraryList?" + q.Encode()

	s.app.Logger.Info("[ChatWiki] GetLibraryList request",
		"url", apiURL,
		"type", libType,
		"only_open", onlyOpen,
		"type_omitted", libType < 0,
		"token_length", len(binding.Token),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Token", binding.Token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		s.app.Logger.Error("[ChatWiki] GetLibraryList request failed", "error", err)
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	s.app.Logger.Info("[ChatWiki] GetLibraryList response",
		"status", resp.StatusCode,
		"type", libType,
		"only_open", onlyOpen,
		"body", string(body),
	)

	if resp.StatusCode == http.StatusUnauthorized {
		s.markBindingExpired()
		var apiErr struct {
			Res int    `json:"res"`
			Msg string `json:"msg"`
		}
		_ = json.Unmarshal(body, &apiErr)
		msg := apiErr.Msg
		if msg == "" {
			msg = "账号未获取登录信息，请重新授权"
		}
		return nil, fmt.Errorf("%w: %s", errChatWikiAuthExpired, msg)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var apiResp struct {
		Res  int                  `json:"res"`
		Code int                  `json:"code"`
		Msg  string               `json:"msg"`
		Data []chatWikiLibraryRaw `json:"data"`
	}
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	resultCode := apiResp.Res
	if resultCode == 0 && apiResp.Code != 0 {
		resultCode = apiResp.Code
	}
	if resultCode == 401 {
		s.markBindingExpired()
		msg := apiResp.Msg
		if msg == "" {
			msg = "账号未获取登录信息，请重新授权"
		}
		return nil, fmt.Errorf("%w: %s", errChatWikiAuthExpired, msg)
	}
	if resultCode != 0 {
		return nil, fmt.Errorf("API error code=%d msg=%s", resultCode, apiResp.Msg)
	}

	libraries := make([]Library, 0, len(apiResp.Data))
	for _, item := range apiResp.Data {
		typeName := "normal"
		switch item.Type {
		case "2":
			typeName = "qa"
		case "3":
			typeName = "wechat"
		}
		libraries = append(libraries, Library{
			ID:           item.ID,
			Name:         item.LibraryName,
			Intro:        item.LibraryIntro,
			Type:         item.Type,
			TypeName:     typeName,
			SwitchStatus: parseSwitchStatus(item.SwitchStatus),
		})
	}
	return libraries, nil
}

// GetLibraryGroup fetches folder groups for the specified knowledge base.
// groupType is fixed to 1 for chatclaw integration.
func (s *ChatWikiService) GetLibraryGroup(libraryID string, groupType int) ([]LibraryGroup, error) {
	binding, err := s.GetBinding()
	if err != nil || binding == nil {
		return nil, fmt.Errorf("no binding found")
	}
	libraryID = strings.TrimSpace(libraryID)
	if libraryID == "" {
		return nil, fmt.Errorf("library_id is required")
	}
	if groupType <= 0 {
		groupType = 1
	}

	baseURL := strings.TrimRight(binding.ServerURL, "/")
	q := url.Values{}
	q.Set("library_id", libraryID)
	q.Set("group_type", strconv.Itoa(groupType))
	apiURL := baseURL + "/manage/chatclaw/getLibraryGroup?" + q.Encode()

	s.app.Logger.Info("[ChatWiki] GetLibraryGroup request",
		"url", apiURL,
		"library_id", libraryID,
		"group_type", groupType,
	)

	body, err := s.chatWikiGET(binding.Token, apiURL)
	if err != nil {
		return nil, err
	}

	items, err := decodeAPIDataObjectArray(body)
	if err != nil {
		return nil, err
	}

	result := make([]LibraryGroup, 0, len(items)+1)
	allGroupTotal := 0
	for _, item := range items {
		id := getStringByKeys(item, "group_id", "id")
		if id == "" {
			continue
		}
		name := getStringByKeys(item, "group_name", "name")
		total := getIntByKeys(item, "total", "count", "total_count", "total_num", "file_count")
		allGroupTotal += total
		result = append(result, LibraryGroup{
			ID:    id,
			Name:  name,
			Total: total,
		})
	}
	result = append([]LibraryGroup{{
		ID:    "-1",
		Name:  "全部分组",
		Total: allGroupTotal,
	}}, result...)
	return result, nil
}

// GetLibFileList fetches the file list in a knowledge base.
// groupID is optional; pass an empty string to query all files.
// Total in the result is -1 when upstream does not return a total count.
func (s *ChatWikiService) GetLibFileList(
	libraryID string,
	status string,
	page int,
	size int,
	sortField string,
	sortType string,
	groupID string,
	fileName string,
) (LibraryFilePage, error) {
	out := LibraryFilePage{Total: -1}
	binding, err := s.GetBinding()
	if err != nil || binding == nil {
		return out, fmt.Errorf("no binding found")
	}
	libraryID = strings.TrimSpace(libraryID)
	if libraryID == "" {
		return out, fmt.Errorf("library_id is required")
	}
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 10
	}

	baseURL := strings.TrimRight(binding.ServerURL, "/")
	q := url.Values{}
	q.Set("library_id", libraryID)
	if strings.TrimSpace(status) != "" {
		q.Set("status", strings.TrimSpace(status))
	}
	q.Set("page", strconv.Itoa(page))
	q.Set("size", strconv.Itoa(size))
	if strings.TrimSpace(sortField) != "" {
		q.Set("sort_field", strings.TrimSpace(sortField))
	}
	if strings.TrimSpace(sortType) != "" {
		q.Set("sort_type", strings.TrimSpace(sortType))
	}
	if strings.TrimSpace(groupID) != "" {
		q.Set("group_id", strings.TrimSpace(groupID))
	}
	if strings.TrimSpace(fileName) != "" {
		q.Set("file_name", strings.TrimSpace(fileName))
	}
	apiURL := baseURL + "/manage/chatclaw/getLibFileList?" + q.Encode()

	s.app.Logger.Info("[ChatWiki] GetLibFileList request",
		"url", apiURL,
		"library_id", libraryID,
		"status", strings.TrimSpace(status),
		"page", page,
		"size", size,
		"sort_field", strings.TrimSpace(sortField),
		"sort_type", strings.TrimSpace(sortType),
		"group_id", strings.TrimSpace(groupID),
		"file_name", strings.TrimSpace(fileName),
	)

	body, err := s.chatWikiGETLoose(binding.Token, apiURL)
	if err != nil {
		return out, err
	}

	items, total, hasTotal, err := decodeAPIDataObjectArrayWithTotal(body)
	if err != nil {
		return out, err
	}

	result := make([]LibraryFile, 0, len(items))
	for _, item := range items {
		id := getStringByKeys(item, "id", "file_id")
		if id == "" {
			continue
		}
		name := getStringByKeys(item, "file_name", "name", "origin_name")
		if name == "" {
			name = id
		}
		ext := strings.TrimPrefix(getStringByKeys(item, "extension", "ext"), ".")
		statusInt := getIntByKeys(item, "status", "parse_status")
		updatedAt := getStringByKeys(item, "updated_at", "create_time", "created_at")
		thumbPath := normalizeAssetURL(binding.ServerURL, getStringByKeys(item, "thumb_path"))

		result = append(result, LibraryFile{
			ID:        id,
			Name:      name,
			Extension: ext,
			Status:    statusInt,
			UpdatedAt: updatedAt,
			ThumbPath: thumbPath,
		})
	}
	out.List = result
	if hasTotal {
		out.Total = total
	}
	return out, nil
}

// GetParagraphList fetches paragraph list for QA knowledge base.
// libraryID and fileID are mutually optional, but at least one must be provided.
func (s *ChatWikiService) GetParagraphList(
	libraryID string,
	fileID string,
	page int,
	size int,
	status int,
	graphStatus int,
	categoryID int,
	groupID int,
	sortField string,
	sortType string,
	search string,
) (LibraryParagraphPage, error) {
	binding, err := s.GetBinding()
	if err != nil || binding == nil {
		return LibraryParagraphPage{}, fmt.Errorf("no binding found")
	}
	libraryID = strings.TrimSpace(libraryID)
	fileID = strings.TrimSpace(fileID)
	if libraryID == "" && fileID == "" {
		return LibraryParagraphPage{}, fmt.Errorf("library_id or file_id is required")
	}
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 10
	}

	baseURL := strings.TrimRight(binding.ServerURL, "/")
	q := url.Values{}
	if libraryID != "" {
		q.Set("library_id", libraryID)
	}
	if fileID != "" {
		q.Set("file_id", fileID)
	}
	q.Set("page", strconv.Itoa(page))
	q.Set("size", strconv.Itoa(size))
	q.Set("status", strconv.Itoa(status))
	q.Set("graph_status", strconv.Itoa(graphStatus))
	q.Set("category_id", strconv.Itoa(categoryID))
	q.Set("group_id", strconv.Itoa(groupID))
	if strings.TrimSpace(sortField) != "" {
		q.Set("sort_field", strings.TrimSpace(sortField))
	}
	if strings.TrimSpace(sortType) != "" {
		q.Set("sort_type", strings.TrimSpace(sortType))
	}
	if strings.TrimSpace(search) != "" {
		q.Set("search", strings.TrimSpace(search))
	}
	apiURL := baseURL + "/manage/chatclaw/getParagraphList?" + q.Encode()

	s.app.Logger.Info("[ChatWiki] GetParagraphList request",
		"url", apiURL,
		"library_id", libraryID,
		"file_id", fileID,
		"page", page,
		"size", size,
		"status", status,
		"graph_status", graphStatus,
		"category_id", categoryID,
		"group_id", groupID,
		"sort_field", strings.TrimSpace(sortField),
		"sort_type", strings.TrimSpace(sortType),
		"search", strings.TrimSpace(search),
	)

	body, err := s.chatWikiGETLoose(binding.Token, apiURL)
	if err != nil {
		return LibraryParagraphPage{}, err
	}

	items, total, hasTotal, err := decodeAPIDataObjectArrayWithTotal(body)
	if err != nil {
		return LibraryParagraphPage{}, err
	}

	result := make([]LibraryParagraph, 0, len(items))
	for _, item := range items {
		id := getStringByKeys(item, "id", "paragraph_id", "qa_id")
		question := getStringByKeys(item, "question", "title", "q", "query")
		answer := getStringByKeys(item, "answer", "content", "a")
		images := getStringSliceByKeys(item, "images", "image_list", "pics", "pic_list", "attachments", "files")
		normalizedImages := make([]string, 0, len(images))
		for _, rawURL := range images {
			fullURL := normalizeAssetURL(binding.ServerURL, rawURL)
			if strings.TrimSpace(fullURL) == "" {
				continue
			}
			normalizedImages = append(normalizedImages, fullURL)
		}
		result = append(result, LibraryParagraph{
			ID:       id,
			Question: question,
			Answer:   answer,
			Images:   normalizedImages,
		})
	}
	if !hasTotal {
		total = -1
	}
	return LibraryParagraphPage{
		List:  result,
		Total: total,
	}, nil
}

// UpdateRobotSwitchStatus updates the robot switch status.
func (s *ChatWikiService) UpdateRobotSwitchStatus(id string, switchStatus int) error {
	binding, err := s.GetBinding()
	if err != nil || binding == nil {
		return fmt.Errorf("no binding found")
	}
	baseURL := strings.TrimRight(binding.ServerURL, "/")
	apiURL := baseURL + "/manage/chatclaw/updateRobotSwitchStatus"

	idInt, err := strconv.Atoi(strings.TrimSpace(id))
	if err != nil {
		return fmt.Errorf("invalid robot id %q: %w", id, err)
	}
	payload := map[string]any{
		"id":            idInt,
		"switch_status": switchStatus,
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode payload: %w", err)
	}

	s.app.Logger.Info("[ChatWiki] UpdateRobotSwitchStatus request",
		"url", apiURL,
		"payload", string(bodyBytes),
		"token_length", len(binding.Token),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Token", binding.Token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}
	s.app.Logger.Info("[ChatWiki] UpdateRobotSwitchStatus response",
		"status", resp.StatusCode,
		"body", string(respBody),
	)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	var apiResp struct {
		Res  int    `json:"res"`
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	resultCode := apiResp.Res
	if resultCode == 0 && apiResp.Code != 0 {
		resultCode = apiResp.Code
	}
	if resultCode != 0 {
		return fmt.Errorf("API error code=%d msg=%s", resultCode, apiResp.Msg)
	}
	return nil
}

// UpdateLibrarySwitchStatus updates the knowledge base switch status.
func (s *ChatWikiService) UpdateLibrarySwitchStatus(id string, switchStatus int) error {
	binding, err := s.GetBinding()
	if err != nil || binding == nil {
		return fmt.Errorf("no binding found")
	}
	baseURL := strings.TrimRight(binding.ServerURL, "/")
	apiURL := baseURL + "/manage/chatclaw/updateLibrarySwitchStatus"

	idInt, err := strconv.Atoi(strings.TrimSpace(id))
	if err != nil {
		return fmt.Errorf("invalid library id %q: %w", id, err)
	}
	payload := map[string]any{
		"id":            idInt,
		"switch_status": switchStatus,
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode payload: %w", err)
	}

	s.app.Logger.Info("[ChatWiki] UpdateLibrarySwitchStatus request",
		"url", apiURL,
		"payload", string(bodyBytes),
		"token_length", len(binding.Token),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Token", binding.Token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}
	s.app.Logger.Info("[ChatWiki] UpdateLibrarySwitchStatus response",
		"status", resp.StatusCode,
		"body", string(respBody),
	)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	var apiResp struct {
		Res  int    `json:"res"`
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	resultCode := apiResp.Res
	if resultCode == 0 && apiResp.Code != 0 {
		resultCode = apiResp.Code
	}
	if resultCode != 0 {
		return fmt.Errorf("API error code=%d msg=%s", resultCode, apiResp.Msg)
	}
	return nil
}

func toBinding(m *bindingModel) *Binding {
	return &Binding{
		ID:              m.ID,
		ServerURL:       m.ServerURL,
		Token:           m.Token,
		TTL:             m.TTL,
		Exp:             m.Exp,
		UserID:          m.UserID,
		UserName:        m.UserName,
		ChatWikiVersion: normalizeChatWikiVersion(m.ChatWikiVersion),
		CreatedAt:       m.CreatedAt.Format(sqlite.DateTimeFormat),
		UpdatedAt:       m.UpdatedAt.Format(sqlite.DateTimeFormat),
	}
}

func normalizeChatWikiVersion(version string) string {
	version = strings.TrimSpace(version)
	if version == "" {
		return "dev"
	}
	return version
}

func isChatWikiCloudBinding(binding *Binding) bool {
	if binding == nil {
		return false
	}
	return normalizeChatWikiVersion(binding.ChatWikiVersion) == "yun"
}

func ensureChatWikiBindingVersionColumn(db *bun.DB) error {
	if db == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var columns []struct {
		CID        int            `bun:"cid"`
		Name       string         `bun:"name"`
		Type       string         `bun:"type"`
		NotNull    int            `bun:"notnull"`
		Default    sql.NullString `bun:"dflt_value"`
		PrimaryKey int            `bun:"pk"`
	}
	if err := db.NewRaw("PRAGMA table_info(chatwiki_bindings)").Scan(ctx, &columns); err != nil {
		return err
	}
	for _, column := range columns {
		if column.Name == "chatwiki_version" {
			return nil
		}
	}
	_, err := db.ExecContext(ctx, `
ALTER TABLE chatwiki_bindings
ADD COLUMN chatwiki_version TEXT NOT NULL DEFAULT 'dev';
`)
	return err
}

func normalizeAssetURL(serverURL, assetPath string) string {
	assetPath = strings.TrimSpace(assetPath)
	if assetPath == "" {
		return ""
	}
	if strings.HasPrefix(assetPath, "http://") || strings.HasPrefix(assetPath, "https://") {
		return assetPath
	}
	base := normalizeAssetBase(serverURL)
	if base == "" {
		return assetPath
	}
	return base + "/" + strings.TrimLeft(assetPath, "/")
}

func normalizeAssetBase(serverURL string) string {
	serverURL = strings.TrimSpace(serverURL)
	if serverURL == "" {
		return ""
	}

	parsed, err := url.Parse(serverURL)
	if err == nil && parsed.Scheme != "" && parsed.Host != "" {
		return parsed.Scheme + "://" + parsed.Host
	}

	// Fallback for non-standard inputs.
	return strings.TrimRight(serverURL, "/")
}

func parseSwitchStatus(v string) int {
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return 0
	}
	if n == 1 {
		return 1
	}
	return 0
}

func (s *ChatWikiService) chatWikiGET(token string, apiURL string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Token", token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var apiResp struct {
		Res  int             `json:"res"`
		Code int             `json:"code"`
		Msg  string          `json:"msg"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	resultCode := apiResp.Res
	if resultCode == 0 && apiResp.Code != 0 {
		resultCode = apiResp.Code
	}
	if resultCode != 0 {
		return nil, fmt.Errorf("API error code=%d msg=%s", resultCode, apiResp.Msg)
	}

	return apiResp.Data, nil
}

// chatWikiGETLoose accepts both standard API envelopes and raw JSON payloads.
// Some ChatWiki endpoints may return direct arrays/objects or even "NULL".
func (s *ChatWikiService) chatWikiGETLoose(token string, apiURL string) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	s.app.Logger.Info("[chatwiki] GET start", "url", apiURL, "token_len", len(strings.TrimSpace(token)))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Token", token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		s.app.Logger.Error("[chatwiki] GET request failed", "url", apiURL, "error", err)
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		s.app.Logger.Error("[chatwiki] GET read response failed", "url", apiURL, "error", err)
		return nil, fmt.Errorf("read response body: %w", err)
	}
	s.app.Logger.Info("[chatwiki] GET response",
		"url", apiURL,
		"status", resp.StatusCode,
		"body_len", len(body),
		"body_preview", previewChatWikiLogBody(body),
	)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return json.RawMessage("[]"), nil
	}

	var apiResp struct {
		Res  int             `json:"res"`
		Code int             `json:"code"`
		Msg  string          `json:"msg"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &apiResp); err == nil {
		resultCode := apiResp.Res
		if resultCode == 0 && apiResp.Code != 0 {
			resultCode = apiResp.Code
		}
		if resultCode != 0 {
			s.app.Logger.Warn("[chatwiki] GET API envelope error",
				"url", apiURL,
				"result_code", resultCode,
				"msg", apiResp.Msg,
			)
			return nil, fmt.Errorf("API error code=%d msg=%s", resultCode, apiResp.Msg)
		}
		dataTrimmed := strings.TrimSpace(string(apiResp.Data))
		if dataTrimmed == "" || strings.EqualFold(dataTrimmed, "null") {
			s.app.Logger.Info("[chatwiki] GET API envelope empty data", "url", apiURL)
			return json.RawMessage("[]"), nil
		}
		s.app.Logger.Info("[chatwiki] GET API envelope ok",
			"url", apiURL,
			"data_len", len(apiResp.Data),
			"data_preview", previewChatWikiLogBody(apiResp.Data),
		)
		return apiResp.Data, nil
	}

	if json.Valid([]byte(trimmed)) {
		s.app.Logger.Info("[chatwiki] GET raw JSON payload", "url", apiURL)
		return json.RawMessage(trimmed), nil
	}

	// Some deployments may return plain "NULL" as empty payload.
	if strings.EqualFold(trimmed, "NULL") {
		s.app.Logger.Info("[chatwiki] GET payload NULL treated as empty", "url", apiURL)
		return json.RawMessage("[]"), nil
	}

	if len(trimmed) > 240 {
		trimmed = trimmed[:240] + "..."
	}
	return nil, fmt.Errorf("decode response: non-JSON body: %s", trimmed)
}

func decodeAPIDataObjectArray(data json.RawMessage) ([]map[string]any, error) {
	var arr []map[string]any
	if err := json.Unmarshal(data, &arr); err == nil {
		return arr, nil
	}

	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil, fmt.Errorf("decode data: %w", err)
	}
	for _, key := range []string{"list", "rows", "items", "data"} {
		raw, ok := obj[key]
		if !ok || raw == nil {
			continue
		}
		items, ok := raw.([]any)
		if !ok {
			continue
		}
		result := make([]map[string]any, 0, len(items))
		for _, item := range items {
			asMap, ok := item.(map[string]any)
			if !ok {
				continue
			}
			result = append(result, asMap)
		}
		return result, nil
	}

	return []map[string]any{}, nil
}

func decodeAPIDataObjectArrayWithTotal(data json.RawMessage) ([]map[string]any, int, bool, error) {
	var arr []map[string]any
	if err := json.Unmarshal(data, &arr); err == nil {
		return arr, 0, false, nil
	}

	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil, 0, false, fmt.Errorf("decode data: %w", err)
	}

	items := []map[string]any{}
	for _, key := range []string{"list", "rows", "items", "data"} {
		raw, ok := obj[key]
		if !ok || raw == nil {
			continue
		}
		list, ok := raw.([]any)
		if !ok {
			continue
		}
		result := make([]map[string]any, 0, len(list))
		for _, item := range list {
			asMap, ok := item.(map[string]any)
			if !ok {
				continue
			}
			result = append(result, asMap)
		}
		items = result
		break
	}

	total, hasTotal := getIntWithPresenceByKeys(obj, "total", "count", "total_count", "total_num", "records")
	return items, total, hasTotal, nil
}

func getStringByKeys(data map[string]any, keys ...string) string {
	for _, key := range keys {
		raw, ok := data[key]
		if !ok || raw == nil {
			continue
		}
		switch v := raw.(type) {
		case string:
			if strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		case float64:
			return strconv.FormatInt(int64(v), 10)
		case int:
			return strconv.Itoa(v)
		case int64:
			return strconv.FormatInt(v, 10)
		case json.Number:
			return v.String()
		default:
			value := strings.TrimSpace(fmt.Sprintf("%v", v))
			if value != "" && value != "<nil>" {
				return value
			}
		}
	}
	return ""
}

func getIntByKeys(data map[string]any, keys ...string) int {
	for _, key := range keys {
		raw, ok := data[key]
		if !ok || raw == nil {
			continue
		}
		switch v := raw.(type) {
		case int:
			return v
		case int64:
			return int(v)
		case float64:
			return int(v)
		case string:
			n, err := strconv.Atoi(strings.TrimSpace(v))
			if err == nil {
				return n
			}
		case json.Number:
			n, err := v.Int64()
			if err == nil {
				return int(n)
			}
		}
	}
	return 0
}

func getIntWithPresenceByKeys(data map[string]any, keys ...string) (int, bool) {
	for _, key := range keys {
		raw, ok := data[key]
		if !ok || raw == nil {
			continue
		}
		switch v := raw.(type) {
		case int:
			return v, true
		case int64:
			return int(v), true
		case float64:
			return int(v), true
		case string:
			n, err := strconv.Atoi(strings.TrimSpace(v))
			if err == nil {
				return n, true
			}
		case json.Number:
			n, err := v.Int64()
			if err == nil {
				return int(n), true
			}
		}
	}
	return 0, false
}

func getStringSliceByKeys(data map[string]any, keys ...string) []string {
	for _, key := range keys {
		raw, ok := data[key]
		if !ok || raw == nil {
			continue
		}
		items, ok := raw.([]any)
		if !ok {
			continue
		}
		result := make([]string, 0, len(items))
		for _, item := range items {
			if item == nil {
				continue
			}
			if m, ok := item.(map[string]any); ok {
				s := getStringByKeys(m, "url", "path", "src", "thumb_path", "image", "file_url")
				if s != "" {
					result = append(result, s)
				}
				continue
			}
			s := strings.TrimSpace(fmt.Sprintf("%v", item))
			if s == "" || s == "<nil>" {
				continue
			}
			result = append(result, s)
		}
		return result
	}
	return []string{}
}
