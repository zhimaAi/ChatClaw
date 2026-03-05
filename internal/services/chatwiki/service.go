package chatwiki

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"chatclaw/internal/sqlite"

	"github.com/uptrace/bun"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// Binding represents a ChatWiki binding record exposed to the frontend.
type Binding struct {
	ID        int64  `json:"id"`
	ServerURL string `json:"server_url"`
	Token     string `json:"token"`
	TTL       int64  `json:"ttl"`
	Exp       int64  `json:"exp"`
	UserID    string `json:"user_id"`
	UserName  string `json:"user_name"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
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

	ID        int64     `bun:"id,pk,autoincrement"`
	ServerURL string    `bun:"server_url,notnull"`
	Token     string    `bun:"token,notnull"`
	TTL       int64     `bun:"ttl,notnull"`
	Exp       int64     `bun:"exp,notnull"`
	UserID    string    `bun:"user_id,notnull"`
	UserName  string    `bun:"user_name,notnull"`
	CreatedAt time.Time `bun:"created_at,notnull"`
	UpdatedAt time.Time `bun:"updated_at,notnull"`
}

// ChatWikiService exposes ChatWiki binding operations to the frontend via Wails.
type ChatWikiService struct {
	app *application.App
}

func NewChatWikiService(app *application.App) *ChatWikiService {
	return &ChatWikiService{app: app}
}

// GetBinding returns the current binding, or nil if none exists.
func (s *ChatWikiService) GetBinding() (*Binding, error) {
	db := sqlite.DB()
	if db == nil {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	m := new(bindingModel)
	err := db.NewSelect().Model(m).OrderExpr("id DESC").Limit(1).Scan(ctx)
	if err != nil {
		return nil, nil
	}
	return toBinding(m), nil
}

// SaveBinding creates or replaces the binding. Called from deeplink handler.
func SaveBinding(app *application.App, serverURL, token, ttl, exp, userID, userName string) error {
	db := sqlite.DB()
	if db == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ttlInt, _ := strconv.ParseInt(ttl, 10, 64)
	expInt, _ := strconv.ParseInt(exp, 10, 64)
	now := time.Now().UTC()

	// Delete old bindings, keep only latest
	if _, err := db.NewDelete().Model((*bindingModel)(nil)).Where("1=1").Exec(ctx); err != nil {
		app.Logger.Warn("Failed to delete old chatwiki bindings", "error", err)
	}

	m := &bindingModel{
		ServerURL: serverURL,
		Token:     token,
		TTL:       ttlInt,
		Exp:       expInt,
		UserID:    userID,
		UserName:  userName,
		CreatedAt: now,
		UpdatedAt: now,
	}
	_, err := db.NewInsert().Model(m).Exec(ctx)
	if err != nil {
		app.Logger.Error("Failed to save chatwiki binding", "error", err)
		return err
	}
	app.Logger.Info("ChatWiki binding saved", "user_id", userID, "user_name", userName)
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

	_, err := db.NewDelete().Model((*bindingModel)(nil)).Where("1=1").Exec(ctx)
	return err
}

// GetRobotList fetches the robot/application list from ChatWiki API.
func (s *ChatWikiService) GetRobotList() ([]Robot, error) {
	binding, err := s.GetBinding()
	if err != nil || binding == nil {
		return nil, fmt.Errorf("no binding found")
	}

	baseURL := strings.TrimRight(binding.ServerURL, "/")
	q := url.Values{}
	q.Set("application_type", "-1")
	q.Set("only_open", "0")
	apiURL := baseURL + "/manage/chatclaw/getRobotList?" + q.Encode()

	s.app.Logger.Info("[ChatWiki] GetRobotList request",
		"url", apiURL,
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
		s.app.Logger.Error("[ChatWiki] GetRobotList request failed", "error", err)
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	s.app.Logger.Info("[ChatWiki] GetRobotList response",
		"status", resp.StatusCode,
		"body", string(body),
	)

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

// GetLibraryList fetches the knowledge base list from ChatWiki API.
// libType: 0=normal, 2=QA, 3=wechat-official-account
func (s *ChatWikiService) GetLibraryList(libType int) ([]Library, error) {
	binding, err := s.GetBinding()
	if err != nil || binding == nil {
		return nil, fmt.Errorf("no binding found")
	}

	baseURL := strings.TrimRight(binding.ServerURL, "/")
	q := url.Values{}
	q.Set("type", strconv.Itoa(libType))
	q.Set("only_open", "0")
	apiURL := baseURL + "/manage/chatclaw/getLibraryList?" + q.Encode()

	s.app.Logger.Info("[ChatWiki] GetLibraryList request",
		"url", apiURL,
		"type", libType,
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
		"body", string(body),
	)

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
		ID:        m.ID,
		ServerURL: m.ServerURL,
		Token:     m.Token,
		TTL:       m.TTL,
		Exp:       m.Exp,
		UserID:    m.UserID,
		UserName:  m.UserName,
		CreatedAt: m.CreatedAt.Format(sqlite.DateTimeFormat),
		UpdatedAt: m.UpdatedAt.Format(sqlite.DateTimeFormat),
	}
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
