package providers

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"willchat/internal/define"
	"willchat/internal/device"
	"willchat/internal/errs"
	"willchat/internal/sqlite"

	"github.com/cloudwego/eino-ext/components/model/claude"
	einogemini "github.com/cloudwego/eino-ext/components/model/gemini"
	"github.com/cloudwego/eino-ext/components/model/ollama"
	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/uptrace/bun"
	"github.com/wailsapp/wails/v3/pkg/application"
	"google.golang.org/genai"
)

var (
	// Enable provider HTTP request logs (may include endpoint URLs and response previews).
	// DO NOT enable in production by default.
	debugProviders = os.Getenv("WILLCHAT_DEBUG_PROVIDERS") == "1"
)

// ProvidersService 供应商服务（暴露给前端调用）
type ProvidersService struct {
	app *application.App
}

func validateModelID(modelID string) error {
	if strings.Contains(modelID, "::") {
		return errs.New("error.model_id_invalid_chars")
	}
	return nil
}

func NewProvidersService(app *application.App) *ProvidersService {
	return &ProvidersService{app: app}
}

func (s *ProvidersService) db() (*bun.DB, error) {
	db := sqlite.DB()
	if db == nil {
		return nil, errs.New("error.sqlite_not_initialized")
	}
	return db, nil
}

// ListProviders 获取所有供应商列表
func (s *ProvidersService) ListProviders() ([]Provider, error) {
	db, err := s.db()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	models := make([]providerModel, 0)
	err = db.NewSelect().
		Model(&models).
		OrderExpr("sort_order ASC, id ASC").
		Scan(ctx)
	if err != nil {
		return nil, errs.Wrap("error.provider_list_failed", err)
	}

	out := make([]Provider, 0, len(models))
	for i := range models {
		out = append(out, models[i].toDTO())
	}
	return out, nil
}

// chatWikiAPIKeyPayload ChatWiki API key plaintext structure
type chatWikiAPIKeyPayload struct {
	UserID string `json:"user_id"`
	UserIP string `json:"user_ip"`
}

// generateChatWikiAPIKeyInternal generates API key (used by both GenerateChatWikiAPIKey and EnsureChatWikiInitialized)
func generateChatWikiAPIKeyInternal() (string, error) {
	clientID, err := device.GetClientID()
	if err != nil {
		return "", errs.Wrap("error.chatwiki_generate_key_failed", err)
	}

	userIP, err := fetchPublicIP()
	if err != nil {
		// Network may be restricted (e.g., no global internet access). Do not block key generation.
		// ChatWiki model list endpoint can be public; other endpoints may still work without IP binding.
		userIP = ""
	}

	payload := chatWikiAPIKeyPayload{
		UserID: clientID,
		UserIP: strings.TrimSpace(userIP),
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return "", errs.Wrap("error.chatwiki_generate_key_failed", err)
	}

	return base64.StdEncoding.EncodeToString(b), nil
}

// GenerateChatWikiAPIKey generates API key for chatwiki provider.
// Plaintext: {"user_id":"<device_id>","user_ip":"<public_ip>"}, then base64 encoded.
func (s *ProvidersService) GenerateChatWikiAPIKey() (string, error) {
	return generateChatWikiAPIKeyInternal()
}

// EnsureChatWikiInitialized ensures chatwiki provider has API key at app startup.
// Called during bootstrap after sqlite init. If api_key is empty, generates and saves.
func EnsureChatWikiInitialized() error {
	db := sqlite.DB()
	if db == nil {
		return nil // sqlite not ready, skip
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	var m providerModel
	err := db.NewSelect().
		Model(&m).
		Where("provider_id = ?", "chatwiki").
		Limit(1).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}

	if strings.TrimSpace(m.APIKey) != "" {
		return nil // already has key
	}

	key, err := generateChatWikiAPIKeyInternal()
	if err != nil {
		return err
	}

	_, err = db.NewUpdate().
		Model((*providerModel)(nil)).
		Where("provider_id = ?", "chatwiki").
		Set("api_key = ?", key).
		Set("updated_at = ?", sqlite.NowUTC()).
		Exec(ctx)
	return err
}

// fetchPublicIP fetches the machine's public IP via api.ipify.org
func fetchPublicIP() (string, error) {
	// Multiple fallbacks for restricted networks (CN-only, captive portals, etc.).
	// We only need a best-effort IP string for telemetry/binding. If all fail, caller may proceed without IP.
	endpoints := []string{
		// Global providers (may be blocked in CN-only networks)
		"https://api.ipify.org?format=text",
		"https://ipv4.icanhazip.com/",
		"https://ifconfig.me/ip",
		"https://ident.me/",
		// CN-friendly providers (often reachable inside CN)
		"https://myip.ipip.net",                  // contains IP in text
		"http://pv.sohu.com/cityjson?ie=utf-8",   // contains "cip": "x.x.x.x"
		"https://ip.3322.net",                    // plain text
	}

	var lastErr error
	for _, u := range endpoints {
		ip, err := fetchIPFromEndpoint(u)
		if err == nil && strings.TrimSpace(ip) != "" {
			return ip, nil
		}
		if err != nil {
			lastErr = err
		}
	}
	if lastErr == nil {
		lastErr = errors.New("failed to fetch public IP")
	}
	return "", lastErr
}

var ipCandidateRe = regexp.MustCompile(`(?i)\b(?:\d{1,3}\.){3}\d{1,3}\b|(?:[0-9a-f]{0,4}:){2,7}[0-9a-f]{0,4}\b`)

func fetchIPFromEndpoint(url string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "text/plain,application/json,*/*")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", errors.New("non-200 when fetching public IP")
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return "", err
	}

	raw := strings.TrimSpace(string(b))
	if raw == "" {
		return "", errors.New("empty response when fetching public IP")
	}

	// Try exact parse first.
	if ip := net.ParseIP(raw); ip != nil {
		return ip.String(), nil
	}

	// Extract first parsable IP candidate from response body.
	cands := ipCandidateRe.FindAllString(raw, -1)
	for _, c := range cands {
		c = strings.TrimSpace(strings.Trim(c, `"'`))
		if ip := net.ParseIP(c); ip != nil {
			return ip.String(), nil
		}
	}
	return "", errors.New("no ip found in response")
}

// GetProvider 获取单个供应商详情
func (s *ProvidersService) GetProvider(providerID string) (*Provider, error) {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return nil, errs.New("error.provider_id_required")
	}

	db, err := s.db()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var m providerModel
	err = db.NewSelect().
		Model(&m).
		Where("provider_id = ?", providerID).
		Limit(1).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.Newf("error.provider_not_found", map[string]any{"ProviderID": providerID})
		}
		return nil, errs.Wrap("error.provider_read_failed", err)
	}
	dto := m.toDTO()
	return &dto, nil
}

// GetProviderWithModels 获取供应商及其模型列表
func (s *ProvidersService) GetProviderWithModels(providerID string) (*ProviderWithModels, error) {
	provider, err := s.GetProvider(providerID)
	if err != nil {
		return nil, err
	}

	// ChatWiki: fetch models from /custom-model/list API
	if providerID == "chatwiki" {
		groups, err := s.fetchChatWikiModels(provider)
		if err != nil {
			return nil, err
		}
		// Persist fetched models into local DB (add/update/delete) so they can be reused offline.
		if err := s.syncChatWikiModelsToDB(provider.ProviderID, groups); err != nil {
			return nil, err
		}
		return &ProviderWithModels{
			Provider:    *provider,
			ModelGroups: groups,
		}, nil
	}

	db, err := s.db()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// 获取该供应商的所有模型
	models := make([]modelModel, 0)
	err = db.NewSelect().
		Model(&models).
		Where("provider_id = ?", providerID).
		OrderExpr("type ASC, sort_order ASC, id ASC").
		Scan(ctx)
	if err != nil {
		return nil, errs.Wrap("error.model_list_failed", err)
	}

	// 按类型分组
	groupMap := make(map[string][]Model)
	for i := range models {
		dto := models[i].toDTO()
		groupMap[dto.Type] = append(groupMap[dto.Type], dto)
	}

	// 转换为有序的分组列表（llm 在前，embedding 次之，rerank 在后）
	typeOrder := []string{"llm", "embedding", "rerank"}
	groups := make([]ModelGroup, 0)
	for _, t := range typeOrder {
		if ms, ok := groupMap[t]; ok {
			groups = append(groups, ModelGroup{
				Type:   t,
				Models: ms,
			})
		}
	}

	return &ProviderWithModels{
		Provider:    *provider,
		ModelGroups: groups,
	}, nil
}

// SyncChatWikiModels fetches ChatWiki model list and syncs it to local `models` table.
// This is intended to be called at app startup to keep the local cache fresh.
func (s *ProvidersService) SyncChatWikiModels() error {
	provider, err := s.GetProvider("chatwiki")
	if err != nil {
		// If ChatWiki provider doesn't exist, skip.
		// (Return nil instead of surfacing provider_not_found at startup.)
		return nil
	}
	groups, err := s.fetchChatWikiModels(provider)
	if err != nil {
		return err
	}
	return s.syncChatWikiModelsToDB(provider.ProviderID, groups)
}

type chatWikiRemoteModel struct {
	ModelID   string
	Name      string
	Type      string
	SortOrder int
}

func (s *ProvidersService) flattenChatWikiGroups(groups []ModelGroup) []chatWikiRemoteModel {
	// Use per-type ordering so `sort_order` is stable within each type.
	perTypeOrder := map[string]int{
		"llm":       0,
		"embedding": 0,
		"rerank":    0,
	}
	out := make([]chatWikiRemoteModel, 0, 64)
	for _, g := range groups {
		t := strings.TrimSpace(strings.ToLower(g.Type))
		for _, m := range g.Models {
			modelID := strings.TrimSpace(m.ModelID)
			if modelID == "" {
				continue
			}
			// Prevent breaking frontend key format "provider::model".
			if err := validateModelID(modelID); err != nil {
				continue
			}
			name := strings.TrimSpace(m.Name)
			if name == "" {
				name = modelID
			}
			if t != "llm" && t != "embedding" && t != "rerank" {
				t = "llm"
			}
			sort := perTypeOrder[t]
			perTypeOrder[t] = sort + 1
			out = append(out, chatWikiRemoteModel{
				ModelID:   modelID,
				Name:      name,
				Type:      t,
				SortOrder: sort,
			})
		}
	}
	return out
}

func chunkStrings(in []string, size int) [][]string {
	if size <= 0 || len(in) == 0 {
		return nil
	}
	out := make([][]string, 0, (len(in)+size-1)/size)
	for i := 0; i < len(in); i += size {
		j := i + size
		if j > len(in) {
			j = len(in)
		}
		out = append(out, in[i:j])
	}
	return out
}

func truncateString(s string, max int) string {
	if max <= 0 || s == "" {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "...(truncated)"
}

func (s *ProvidersService) syncChatWikiModelsToDB(providerID string, groups []ModelGroup) error {
	db, err := s.db()
	if err != nil {
		return err
	}

	remote := s.flattenChatWikiGroups(groups)
	remoteMap := make(map[string]chatWikiRemoteModel, len(remote))
	for _, r := range remote {
		remoteMap[r.ModelID] = r
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		// Load existing cached models for this provider.
		existing := make([]modelModel, 0)
		if err := tx.NewSelect().
			Model(&existing).
			Where("provider_id = ?", providerID).
			Scan(ctx); err != nil {
			return errs.Wrap("error.chatwiki_model_sync_failed", err)
		}

		existingMap := make(map[string]modelModel, len(existing))
		for _, e := range existing {
			existingMap[e.ModelID] = e
		}

		// Compute deletes (local but not in remote).
		toDelete := make([]string, 0)
		for modelID := range existingMap {
			if _, ok := remoteMap[modelID]; !ok {
				toDelete = append(toDelete, modelID)
			}
		}
		for _, part := range chunkStrings(toDelete, 200) {
			if _, err := tx.NewDelete().
				Table("models").
				Where("provider_id = ?", providerID).
				Where("model_id IN (?)", bun.In(part)).
				Exec(ctx); err != nil {
				return errs.Wrap("error.chatwiki_model_sync_failed", err)
			}
		}

		// Inserts and updates.
		toInsert := make([]modelModel, 0)
		for _, r := range remote {
			if e, ok := existingMap[r.ModelID]; ok {
				// Force readonly/cache-managed attributes.
				needUpdate := false
				if strings.TrimSpace(e.Name) != r.Name {
					needUpdate = true
				}
				if strings.TrimSpace(strings.ToLower(e.Type)) != r.Type {
					needUpdate = true
				}
				if e.SortOrder != r.SortOrder {
					needUpdate = true
				}
				if !e.Enabled {
					needUpdate = true
				}
				if !e.IsBuiltin {
					needUpdate = true
				}
				if needUpdate {
					if _, err := tx.NewUpdate().
						Model((*modelModel)(nil)).
						Where("provider_id = ?", providerID).
						Where("model_id = ?", r.ModelID).
						Set("name = ?", r.Name).
						Set("type = ?", r.Type).
						Set("sort_order = ?", r.SortOrder).
						Set("enabled = ?", true).
						Set("is_builtin = ?", true).
						Set("updated_at = ?", sqlite.NowUTC()).
						Exec(ctx); err != nil {
						return errs.Wrap("error.chatwiki_model_sync_failed", err)
					}
				}
				continue
			}

			toInsert = append(toInsert, modelModel{
				ProviderID: providerID,
				ModelID:    r.ModelID,
				Name:       r.Name,
				Type:       r.Type,
				IsBuiltin:  true,
				Enabled:    true,
				SortOrder:  r.SortOrder,
			})
		}

		for _, part := range func(ms []modelModel, size int) [][]modelModel {
			if size <= 0 || len(ms) == 0 {
				return nil
			}
			out := make([][]modelModel, 0, (len(ms)+size-1)/size)
			for i := 0; i < len(ms); i += size {
				j := i + size
				if j > len(ms) {
					j = len(ms)
				}
				out = append(out, ms[i:j])
			}
			return out
		}(toInsert, 200) {
			if _, err := tx.NewInsert().
				Model(&part).
				Exec(ctx); err != nil {
				return errs.Wrap("error.chatwiki_model_sync_failed", err)
			}
		}

		return nil
	})
}

// chatWikiModelItem /custom-model/list API response item
type chatWikiModelItem struct {
	ModelID   string `json:"model_id"`
	ID        string `json:"id"` // alternative field
	ModelName string `json:"modelName"` // display name
	Name      string `json:"name"`      // fallback
	ModelType string `json:"modelType"` // for grouping: llm, embedding, rerank
	Type      string `json:"type"`      // fallback
}

// chatWikiModelListResponse /custom-model/list API response (supports data or models array)
type chatWikiModelListResponse struct {
	Data   []chatWikiModelItem `json:"data"`
	Models []chatWikiModelItem `json:"models"`
}

// fetchChatWikiModels fetches models from ChatWiki /custom-model/list API
func (s *ProvidersService) fetchChatWikiModels(provider *Provider) ([]ModelGroup, error) {
	baseURL := strings.TrimSuffix(strings.TrimSpace(provider.APIEndpoint), "/")
	if baseURL == "" {
		return nil, errs.New("error.chatwiki_api_endpoint_required")
	}

	// ChatWiki model list endpoint lives under the OpenAPI base path.
	// Users may configure api_endpoint as either:
	// - https://dev1.willchat.chatwiki.com/openapi   (recommended)
	// - https://dev1.willchat.chatwiki.com          (legacy / mis-config)
	// Normalize here so we always hit the JSON API instead of the admin HTML.
	var url string
	if strings.Contains(baseURL, "/openapi") {
		// already includes openapi base
		url = strings.TrimSuffix(baseURL, "/") + "/custom-model/list"
	} else {
		url = strings.TrimSuffix(baseURL, "/") + "/openapi/custom-model/list"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, errs.Newf("error.chatwiki_model_list_failed", map[string]any{"Status": err.Error()})
	}
	// Model list endpoint may be public. Only attach Authorization if we have a key.
	hasAuth := strings.TrimSpace(provider.APIKey) != ""
	if hasAuth {
		req.Header.Set("Authorization", "Bearer "+provider.APIKey)
	}
	req.Header.Set("Content-Type", "application/json")

	if debugProviders {
		s.app.Logger.Info(
			"ChatWiki model list request",
			"provider_id", provider.ProviderID,
			"url", url,
			"timeout_seconds", 15,
			"has_authorization", hasAuth,
		)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if debugProviders {
			s.app.Logger.Warn(
				"ChatWiki model list request failed",
				"provider_id", provider.ProviderID,
				"url", url,
				"elapsed_ms", time.Since(start).Milliseconds(),
				"error", err,
			)
		}
		return nil, errs.Newf("error.chatwiki_model_list_failed", map[string]any{"Status": err.Error()})
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Best-effort response preview for debugging (may be empty for redirects).
		var preview string
		if debugProviders {
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
			preview = truncateString(string(b), 512)
		}
		if debugProviders {
			s.app.Logger.Warn(
				"ChatWiki model list unexpected status",
				"provider_id", provider.ProviderID,
				"url", url,
				"status", resp.StatusCode,
				"elapsed_ms", time.Since(start).Milliseconds(),
				"response_preview", preview,
			)
		}
		return nil, errs.Newf("error.chatwiki_model_list_failed", map[string]any{"Status": resp.StatusCode})
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		if debugProviders {
			s.app.Logger.Warn(
				"ChatWiki model list read body failed",
				"provider_id", provider.ProviderID,
				"url", url,
				"elapsed_ms", time.Since(start).Milliseconds(),
				"error", err,
			)
		}
		return nil, errs.Newf("error.chatwiki_model_list_failed", map[string]any{"Status": err.Error()})
	}

	if debugProviders {
		s.app.Logger.Info(
			"ChatWiki model list response received",
			"provider_id", provider.ProviderID,
			"url", url,
			"status", resp.StatusCode,
			"elapsed_ms", time.Since(start).Milliseconds(),
			"response_bytes", len(b),
			"response_preview", truncateString(string(b), 512),
		)
	}

	// Try object with data/models
	var respObj chatWikiModelListResponse
	if err := json.Unmarshal(b, &respObj); err == nil {
		items := respObj.Data
		if len(items) == 0 {
			items = respObj.Models
		}
		return s.chatWikiModelsToGroups(items, provider.ProviderID)
	}

	// Try direct array
	var items []chatWikiModelItem
	if err := json.Unmarshal(b, &items); err != nil {
		return nil, errs.Newf("error.chatwiki_model_list_failed", map[string]any{"Status": err.Error()})
	}
	return s.chatWikiModelsToGroups(items, provider.ProviderID)
}

func (s *ProvidersService) chatWikiModelsToGroups(items []chatWikiModelItem, providerID string) ([]ModelGroup, error) {
	groupMap := make(map[string][]Model)
	perTypeOrder := map[string]int{
		"llm":       0,
		"embedding": 0,
		"rerank":    0,
	}
	for _, item := range items {
		// IMPORTANT:
		// ChatWiki: Persist into local `models` table using modelName as `model_id`.
		// Do NOT use API `id` value, since the frontend expects a stable, human-readable model key.
		modelID := strings.TrimSpace(item.ModelName)
		if modelID == "" {
			continue
		}
		// Display name: keep consistent with model_id (UI selection key uses model_id).
		name := modelID
		// Group by modelType, fallback to type
		modelType := strings.TrimSpace(strings.ToLower(item.ModelType))
		if modelType == "" {
			modelType = strings.TrimSpace(strings.ToLower(item.Type))
		}
		if modelType == "" {
			modelType = "llm"
		}
		if modelType != "llm" && modelType != "embedding" && modelType != "rerank" {
			modelType = "llm"
		}
		// Skip invalid IDs (would break frontend selection key format).
		if err := validateModelID(modelID); err != nil {
			continue
		}
		sort := perTypeOrder[modelType]
		perTypeOrder[modelType] = sort + 1
		m := Model{
			ID:         0,
			ProviderID: providerID,
			ModelID:    modelID,
			Name:       name,
			Type:       modelType,
			IsBuiltin:  true, // system-managed cache
			Enabled:    true,
			SortOrder:  sort,
		}
		groupMap[modelType] = append(groupMap[modelType], m)
	}

	typeOrder := []string{"llm", "embedding", "rerank"}
	groups := make([]ModelGroup, 0)
	for _, t := range typeOrder {
		if ms, ok := groupMap[t]; ok {
			groups = append(groups, ModelGroup{Type: t, Models: ms})
		}
	}
	return groups, nil
}

// UpdateProvider 更新供应商信息
func (s *ProvidersService) UpdateProvider(providerID string, input UpdateProviderInput) (*Provider, error) {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return nil, errs.New("error.provider_id_required")
	}

	db, err := s.db()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// 禁止关闭正在作为"全局嵌入模型"使用的供应商（ChatWiki 关闭时无需验证）
	if input.Enabled != nil && !*input.Enabled && providerID != "chatwiki" {
		type row struct {
			Key   string         `bun:"key"`
			Value sql.NullString `bun:"value"`
		}
		rows := make([]row, 0, 2)
		if err := db.NewSelect().
			Table("settings").
			Column("key", "value").
			Where("key IN (?)", bun.In([]string{"embedding_provider_id", "embedding_model_id"})).
			Scan(ctx, &rows); err != nil {
			return nil, errs.Wrap("error.setting_read_failed", err)
		}

		var embeddingProviderID, embeddingModelID string
		for _, r := range rows {
			if !r.Value.Valid {
				continue
			}
			switch r.Key {
			case "embedding_provider_id":
				embeddingProviderID = strings.TrimSpace(r.Value.String)
			case "embedding_model_id":
				embeddingModelID = strings.TrimSpace(r.Value.String)
			}
		}
		if embeddingProviderID != "" && embeddingModelID != "" && embeddingProviderID == providerID {
			return nil, errs.New("error.cannot_disable_global_embedding_provider")
		}

	// 禁止关闭其语义分段模型正被知识库使用的供应商
	var libraryName string
	if err := db.NewSelect().
		Table("library").
		Column("name").
		Where("raptor_llm_provider_id = ?", providerID).
		Limit(1).
		Scan(ctx, &libraryName); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, errs.Wrap("error.library_read_failed", err)
	}
	if libraryName != "" {
		return nil, errs.Newf("error.cannot_disable_provider_with_semantic_segment_in_use", map[string]any{"LibraryName": libraryName})
	}
	}

	// 构建更新语句
	q := db.NewUpdate().
		Model((*providerModel)(nil)).
		Where("provider_id = ?", providerID).
		Set("updated_at = ?", time.Now().UTC())

	if input.Enabled != nil {
		q = q.Set("enabled = ?", *input.Enabled)
	}
	if input.APIKey != nil {
		q = q.Set("api_key = ?", *input.APIKey)
	}
	if input.APIEndpoint != nil {
		q = q.Set("api_endpoint = ?", *input.APIEndpoint)
	}
	if input.ExtraConfig != nil {
		q = q.Set("extra_config = ?", *input.ExtraConfig)
	}

	result, err := q.Exec(ctx)
	if err != nil {
		return nil, errs.Wrap("error.provider_update_failed", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return nil, errs.Newf("error.provider_not_found", map[string]any{"ProviderID": providerID})
	}

	return s.GetProvider(providerID)
}

// ResetAPIEndpoint 重置供应商的 API 地址为默认值
func (s *ProvidersService) ResetAPIEndpoint(providerID string) (*Provider, error) {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return nil, errs.New("error.provider_id_required")
	}

	// 从共享配置获取默认 API 地址
	defaultEndpoint, ok := define.GetBuiltinProviderDefaultEndpoint(providerID)
	if !ok {
		// 非内置供应商，清空地址
		defaultEndpoint = ""
	}

	input := UpdateProviderInput{
		APIEndpoint: &defaultEndpoint,
	}
	return s.UpdateProvider(providerID, input)
}

// CheckAPIKeyInput 检测 API Key 的输入参数
type CheckAPIKeyInput struct {
	APIKey      string `json:"api_key"`
	APIEndpoint string `json:"api_endpoint"`
	ExtraConfig string `json:"extra_config"`
}

// CheckAPIKeyResult 检测 API Key 的结果
type CheckAPIKeyResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// CheckAPIKey 检测供应商的 API Key 是否有效
func (s *ProvidersService) CheckAPIKey(providerID string, input CheckAPIKeyInput) (*CheckAPIKeyResult, error) {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return nil, errs.New("error.provider_id_required")
	}

	// 获取供应商信息
	provider, err := s.GetProvider(providerID)
	if err != nil {
		return nil, err
	}

	// 获取该供应商的第一个 LLM 模型作为测试模型
	testModelID, err := s.getFirstLLMModel(providerID)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 根据供应商类型调用不同的 SDK
	switch provider.Type {
	case "openai":
		return s.checkOpenAI(ctx, input, testModelID)
	case "azure":
		return s.checkAzure(ctx, input, testModelID)
	case "anthropic":
		return s.checkClaude(ctx, input, testModelID)
	case "gemini":
		return s.checkGemini(ctx, input, testModelID)
	case "ollama":
		// Ollama 本地运行，直接尝试连接检测
		return s.checkOllama(ctx, input, testModelID)
	default:
		return nil, errs.Newf("error.unsupported_provider_type", map[string]any{"Type": provider.Type})
	}
}

// getFirstLLMModel 获取供应商的第一个 LLM 模型
func (s *ProvidersService) getFirstLLMModel(providerID string) (string, error) {
	db, err := s.db()
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var m modelModel
	err = db.NewSelect().
		Model(&m).
		Where("provider_id = ?", providerID).
		Where("type = ?", "llm").
		OrderExpr("sort_order ASC, id ASC").
		Limit(1).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", errs.Newf("error.no_llm_model", map[string]any{"ProviderID": providerID})
		}
		return "", errs.Wrap("error.model_read_failed", err)
	}
	return m.ModelID, nil
}

// ChatModelGenerator 定义可生成消息的聊天模型接口
type ChatModelGenerator interface {
	Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error)
}

// testChatModel 使用聊天模型发送测试消息
func testChatModel(ctx context.Context, chatModel ChatModelGenerator) *CheckAPIKeyResult {
	_, err := chatModel.Generate(ctx, []*schema.Message{
		{
			Role:    schema.User,
			Content: "hi",
		},
	})
	if err != nil {
		return &CheckAPIKeyResult{
			Success: false,
			Message: err.Error(),
		}
	}
	return &CheckAPIKeyResult{
		Success: true,
		Message: "",
	}
}

// checkOpenAI 使用 OpenAI SDK 检测
func (s *ProvidersService) checkOpenAI(ctx context.Context, input CheckAPIKeyInput, modelID string) (*CheckAPIKeyResult, error) {
	chatModel, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		APIKey:  input.APIKey,
		Model:   modelID,
		BaseURL: input.APIEndpoint,
	})
	if err != nil {
		return &CheckAPIKeyResult{
			Success: false,
			Message: err.Error(),
		}, nil
	}
	return testChatModel(ctx, chatModel), nil
}

// checkAzure 使用 Azure OpenAI SDK 检测
func (s *ProvidersService) checkAzure(ctx context.Context, input CheckAPIKeyInput, modelID string) (*CheckAPIKeyResult, error) {
	// 解析 Azure 的额外配置
	var extraConfig struct {
		APIVersion string `json:"api_version"`
	}
	if input.ExtraConfig != "" {
		if err := json.Unmarshal([]byte(input.ExtraConfig), &extraConfig); err != nil {
			return &CheckAPIKeyResult{
				Success: false,
				Message: "invalid extra_config: " + err.Error(),
			}, nil
		}
	}

	chatModel, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		APIKey:     input.APIKey,
		Model:      modelID,
		BaseURL:    input.APIEndpoint,
		ByAzure:    true,
		APIVersion: extraConfig.APIVersion,
	})
	if err != nil {
		return &CheckAPIKeyResult{
			Success: false,
			Message: err.Error(),
		}, nil
	}
	return testChatModel(ctx, chatModel), nil
}

// checkClaude 使用 Claude SDK 检测
func (s *ProvidersService) checkClaude(ctx context.Context, input CheckAPIKeyInput, modelID string) (*CheckAPIKeyResult, error) {
	var baseURL *string
	if input.APIEndpoint != "" {
		baseURL = &input.APIEndpoint
	}

	chatModel, err := claude.NewChatModel(ctx, &claude.Config{
		APIKey:    input.APIKey,
		Model:     modelID,
		BaseURL:   baseURL,
		MaxTokens: 100,
	})
	if err != nil {
		return &CheckAPIKeyResult{
			Success: false,
			Message: err.Error(),
		}, nil
	}
	return testChatModel(ctx, chatModel), nil
}

// checkGemini 使用 Gemini SDK 检测
func (s *ProvidersService) checkGemini(ctx context.Context, input CheckAPIKeyInput, modelID string) (*CheckAPIKeyResult, error) {
	config := &genai.ClientConfig{
		APIKey: input.APIKey,
	}
	if input.APIEndpoint != "" {
		config.HTTPOptions = genai.HTTPOptions{
			BaseURL: input.APIEndpoint,
		}
	}
	client, err := genai.NewClient(ctx, config)
	if err != nil {
		return &CheckAPIKeyResult{
			Success: false,
			Message: err.Error(),
		}, nil
	}

	chatModel, err := einogemini.NewChatModel(ctx, &einogemini.Config{
		Client: client,
		Model:  modelID,
	})
	if err != nil {
		return &CheckAPIKeyResult{
			Success: false,
			Message: err.Error(),
		}, nil
	}
	return testChatModel(ctx, chatModel), nil
}

// checkOllama 使用 Ollama SDK 检测
func (s *ProvidersService) checkOllama(ctx context.Context, input CheckAPIKeyInput, modelID string) (*CheckAPIKeyResult, error) {
	chatModel, err := ollama.NewChatModel(ctx, &ollama.ChatModelConfig{
		BaseURL: input.APIEndpoint,
		Model:   modelID,
	})
	if err != nil {
		return &CheckAPIKeyResult{
			Success: false,
			Message: err.Error(),
		}, nil
	}
	return testChatModel(ctx, chatModel), nil
}

// CreateModel 创建模型
func (s *ProvidersService) CreateModel(providerID string, input CreateModelInput) (*Model, error) {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return nil, errs.New("error.provider_id_required")
	}
	if providerID == "chatwiki" {
		return nil, errs.New("error.chatwiki_models_readonly")
	}

	input.ModelID = strings.TrimSpace(input.ModelID)
	if input.ModelID == "" {
		return nil, errs.New("error.model_id_required")
	}
	if len([]rune(input.ModelID)) > 40 {
		return nil, errs.New("error.model_id_too_long")
	}
	if err := validateModelID(input.ModelID); err != nil {
		return nil, err
	}

	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		return nil, errs.New("error.model_name_required")
	}
	if len([]rune(input.Name)) > 40 {
		return nil, errs.New("error.model_name_too_long")
	}

	input.Type = strings.TrimSpace(input.Type)
	if input.Type != "llm" && input.Type != "embedding" && input.Type != "rerank" {
		return nil, errs.New("error.model_type_invalid")
	}

	db, err := s.db()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// 检查供应商是否存在
	_, err = s.GetProvider(providerID)
	if err != nil {
		return nil, err
	}

	// 检查模型是否已存在
	var existingCount int
	existingCount, err = db.NewSelect().
		Model((*modelModel)(nil)).
		Where("provider_id = ?", providerID).
		Where("model_id = ?", input.ModelID).
		Count(ctx)
	if err != nil {
		return nil, errs.Wrap("error.model_check_failed", err)
	}
	if existingCount > 0 {
		return nil, errs.New("error.model_already_exists")
	}

	// 获取最大排序值
	var maxSortOrder int
	err = db.NewSelect().
		Model((*modelModel)(nil)).
		Where("provider_id = ?", providerID).
		Where("type = ?", input.Type).
		ColumnExpr("COALESCE(MAX(sort_order), 0)").
		Scan(ctx, &maxSortOrder)
	if err != nil {
		return nil, errs.Wrap("error.model_sort_order_failed", err)
	}

	m := &modelModel{
		ProviderID: providerID,
		ModelID:    input.ModelID,
		Name:       input.Name,
		Type:       input.Type,
		IsBuiltin:  false,
		Enabled:    true,
		SortOrder:  maxSortOrder + 1,
	}

	_, err = db.NewInsert().Model(m).Exec(ctx)
	if err != nil {
		return nil, errs.Wrap("error.model_create_failed", err)
	}

	dto := m.toDTO()
	return &dto, nil
}

// UpdateModel 更新模型
func (s *ProvidersService) UpdateModel(providerID string, modelID string, input UpdateModelInput) (*Model, error) {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return nil, errs.New("error.provider_id_required")
	}
	if providerID == "chatwiki" {
		return nil, errs.New("error.chatwiki_models_readonly")
	}

	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return nil, errs.New("error.model_id_required")
	}
	if err := validateModelID(modelID); err != nil {
		return nil, err
	}

	db, err := s.db()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// 构建更新语句
	q := db.NewUpdate().
		Model((*modelModel)(nil)).
		Where("provider_id = ?", providerID).
		Where("model_id = ?", modelID).
		Set("updated_at = ?", time.Now().UTC())

	if input.Name != nil {
		newName := strings.TrimSpace(*input.Name)
		if newName == "" {
			return nil, errs.New("error.model_name_required")
		}
		if len([]rune(newName)) > 40 {
			return nil, errs.New("error.model_name_too_long")
		}
		q = q.Set("name = ?", newName)
	}
	if input.Enabled != nil {
		q = q.Set("enabled = ?", *input.Enabled)
	}

	result, err := q.Exec(ctx)
	if err != nil {
		return nil, errs.Wrap("error.model_update_failed", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return nil, errs.Newf("error.model_not_found", map[string]any{"ModelID": modelID})
	}

	return s.GetModel(providerID, modelID)
}

// GetModel 获取单个模型
func (s *ProvidersService) GetModel(providerID string, modelID string) (*Model, error) {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return nil, errs.New("error.provider_id_required")
	}

	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return nil, errs.New("error.model_id_required")
	}

	db, err := s.db()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var m modelModel
	err = db.NewSelect().
		Model(&m).
		Where("provider_id = ?", providerID).
		Where("model_id = ?", modelID).
		Limit(1).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.Newf("error.model_not_found", map[string]any{"ModelID": modelID})
		}
		return nil, errs.Wrap("error.model_read_failed", err)
	}

	dto := m.toDTO()
	return &dto, nil
}

// DeleteModel 删除模型
func (s *ProvidersService) DeleteModel(providerID string, modelID string) error {
	providerID = strings.TrimSpace(providerID)
	if providerID == "chatwiki" {
		return errs.New("error.chatwiki_models_readonly")
	}
	if providerID == "" {
		return errs.New("error.provider_id_required")
	}

	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return errs.New("error.model_id_required")
	}

	db, err := s.db()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// 先检查模型是否存在以及是否为内置模型
	var m modelModel
	err = db.NewSelect().
		Model(&m).
		Where("provider_id = ?", providerID).
		Where("model_id = ?", modelID).
		Limit(1).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errs.Newf("error.model_not_found", map[string]any{"ModelID": modelID})
		}
		return errs.Wrap("error.model_read_failed", err)
	}

	// 禁止删除内置模型
	if m.IsBuiltin {
		return errs.New("error.cannot_delete_builtin_model")
	}

	// 禁止删除正在作为"全局嵌入模型"使用的模型
	if m.Type == "embedding" {
		type row struct {
			Key   string         `bun:"key"`
			Value sql.NullString `bun:"value"`
		}
		rows := make([]row, 0, 2)
		if err := db.NewSelect().
			Table("settings").
			Column("key", "value").
			Where("key IN (?)", bun.In([]string{"embedding_provider_id", "embedding_model_id"})).
			Scan(ctx, &rows); err != nil {
			return errs.Wrap("error.setting_read_failed", err)
		}

		var embeddingProviderID, embeddingModelID string
		for _, r := range rows {
			if !r.Value.Valid {
				continue
			}
			switch r.Key {
			case "embedding_provider_id":
				embeddingProviderID = strings.TrimSpace(r.Value.String)
			case "embedding_model_id":
				embeddingModelID = strings.TrimSpace(r.Value.String)
			}
		}

		if embeddingProviderID != "" && embeddingModelID != "" &&
			providerID == embeddingProviderID && modelID == embeddingModelID {
			return errs.New("error.cannot_delete_global_embedding_model")
		}
	}

	// 禁止删除正在被知识库使用的语义分段模型（LLM 类型）
	if m.Type == "llm" {
		var libraryName string
		if err := db.NewSelect().
			Table("library").
			Column("name").
			Where("raptor_llm_provider_id = ?", providerID).
			Where("raptor_llm_model_id = ?", modelID).
			Limit(1).
			Scan(ctx, &libraryName); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return errs.Wrap("error.library_read_failed", err)
		}
		if libraryName != "" {
			return errs.Newf("error.cannot_delete_semantic_segment_model_in_use", map[string]any{"LibraryName": libraryName})
		}
	}

	_, err = db.NewDelete().
		Model((*modelModel)(nil)).
		Where("provider_id = ?", providerID).
		Where("model_id = ?", modelID).
		Exec(ctx)
	if err != nil {
		return errs.Wrap("error.model_delete_failed", err)
	}

	return nil
}
