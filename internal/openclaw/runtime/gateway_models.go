package openclawruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"chatclaw/internal/define"
	"chatclaw/internal/services/chatwiki"
	"chatclaw/internal/services/providers"
	"chatclaw/internal/sqlite"

	"github.com/uptrace/bun"
)

// NewModelsSectionBuilder returns a SectionBuilder that produces the OpenClaw
// "models" config section from ChatClaw's enabled providers and LLM models.
func NewModelsSectionBuilder(providersSvc *providers.ProvidersService) SectionBuilder {
	return func(ctx context.Context) (map[string]any, error) {
		allProviders, err := providersSvc.ListProviders()
		if err != nil {
			return nil, fmt.Errorf("list providers: %w", err)
		}
		return buildModelsPatch(providersSvc, allProviders), nil
	}
}

type openclawProviderConfig struct {
	BaseURL string               `json:"baseUrl,omitempty"`
	APIKey  string               `json:"apiKey,omitempty"`
	API     string               `json:"api"`
	Headers map[string]string    `json:"headers,omitempty"`
	Models  []openclawModelEntry `json:"models,omitempty"`
}

type openclawModelEntry struct {
	ID    string   `json:"id"`
	Name  string   `json:"name"`
	Input []string `json:"input,omitempty"`
}

var chatclawTypeToOpenClawAPI = map[string]string{
	"openai":    "openai-completions",
	"azure":     "openai-completions",
	"anthropic": "anthropic-messages",
	"gemini":    "google-generativeai",
	"ollama":    "openai-completions",
	"qwen":      "openai-completions",
}

// chatWikiSyncMu protects the ChatWiki model catalog cache during sync.
var chatWikiSyncMu sync.Mutex

// chatWikiSyncCache caches the ChatWiki model catalog for the duration of a sync cycle.
var chatWikiSyncCache *chatWikiSyncData

type chatWikiSyncData struct {
	Binding  *chatWikiBindingDTO
	Catalog  *chatwiki.ModelCatalog
	CachedAt time.Time
}

// chatWikiBindingDTO is a minimal binding struct for sync.
type chatWikiBindingDTO struct {
	ID        int64
	ServerURL string
	Token     string
	UserID    string
}

func buildModelsPatch(providersSvc *providers.ProvidersService, allProviders []providers.Provider) map[string]any {
	providerMap := make(map[string]any)

	// Pre-fetch ChatWiki binding and catalog for sync.
	// Force refresh to ensure we have the latest models when building config.
	chatWikiData := fetchChatWikiSyncData(true)
	hasChatWikiBinding := chatWikiData != nil && chatWikiData.Binding != nil && chatWikiData.Binding.Token != ""

	for _, p := range allProviders {
		if !p.Enabled {
			continue
		}

		// Skip providers without API key (except ollama and chatwiki which have special handling).
		if p.ProviderID != "ollama" && p.ProviderID != "chatwiki" && strings.TrimSpace(p.APIKey) == "" {
			continue
		}

		// Special handling for ChatWiki: get APIKey from binding and models from catalog.
		if p.ProviderID == "chatwiki" {
			if !hasChatWikiBinding {
				continue // No ChatWiki binding, skip.
			}
			ocProvider := buildChatWikiProvider(chatWikiData)
			if ocProvider != nil && len(ocProvider.Models) > 0 {
				providerMap[p.ProviderID] = ocProvider
			}
			continue
		}

		ocProvider := buildSingleProvider(p)
		if ocProvider == nil {
			continue
		}

		pwm, err := providersSvc.GetProviderWithModels(p.ProviderID)
		if err == nil {
			for _, group := range pwm.ModelGroups {
				if group.Type != "llm" {
					continue
				}
				for _, m := range group.Models {
					if !m.Enabled {
						continue
					}
					entry := openclawModelEntry{
						ID:   m.ModelID,
						Name: m.Name,
					}
					// Strip to OpenClaw-allowed values only; omit input field entirely
					// if nothing valid remains so OpenClaw defaults to ["text"].
					if ok, filtered := validOpenClawInputs(m.Capabilities); ok {
						entry.Input = filtered
					}
					ocProvider.Models = append(ocProvider.Models, entry)
				}
			}
		}

		providerMap[p.ProviderID] = ocProvider
	}

	return map[string]any{
		"models": map[string]any{
			"mode":      "replace",
			"providers": providerMap,
		},
	}
}

// validOpenClawInputs strips model capabilities to only those accepted by OpenClaw.
// OpenClaw rejects any input value outside ["text", "image"]; everything else (file,
// audio, video, document, function_call, rerank, etc.) is stripped. Returns ok=false
// when the input field should be omitted entirely (no valid capabilities remain),
// allowing OpenClaw to default to ["text"].
func validOpenClawInputs(capabilities []string) (ok bool, filtered []string) {
	if len(capabilities) == 0 {
		return false, nil
	}
	valid := make([]string, 0, 2)
	for _, c := range capabilities {
		lower := strings.ToLower(strings.TrimSpace(c))
		if lower == "text" || lower == "image" {
			valid = append(valid, lower)
		}
	}
	if len(valid) == 0 {
		return false, nil
	}
	return true, valid
}

// BuildModelsSectionPatch returns the same "models" object as full config sync (mode + providers).
// Used when pushing only the models slice to the Gateway (must match OpenClaw schema).
func BuildModelsSectionPatch(svc *providers.ProvidersService) (map[string]any, error) {
	if svc == nil {
		return nil, fmt.Errorf("providers service is nil")
	}
	allProviders, err := svc.ListProviders()
	if err != nil {
		return nil, err
	}
	patch := buildModelsPatch(svc, allProviders)
	models, ok := patch["models"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("buildModelsPatch missing models section")
	}
	return models, nil
}

// fetchChatWikiSyncData fetches ChatWiki binding and model catalog for sync.
// It first triggers a catalog refresh to ensure we have the latest models.
// Parameters:
//   - forceRefresh: if true, bypasses cache and forces a fresh fetch
func fetchChatWikiSyncData(forceRefresh bool) *chatWikiSyncData {
	chatWikiSyncMu.Lock()
	defer chatWikiSyncMu.Unlock()

	// Check if cache is still valid (within last 5 minutes) - only use cache if not forcing refresh.
	if !forceRefresh && chatWikiSyncCache != nil && time.Since(chatWikiSyncCache.CachedAt) < 5*time.Minute {
		return chatWikiSyncCache
	}

	// Refresh the ChatWiki model catalog to ensure we have the latest models.
	// This handles cases where models were added/removed/updated in ChatWiki.
	if chatwiki.RefreshChatWikiModelCatalog != nil {
		_ = chatwiki.RefreshChatWikiModelCatalog()
	}

	result := &chatWikiSyncData{CachedAt: time.Now()}

	// Fetch binding from DB.
	binding, err := fetchChatWikiBindingFromDB()
	if err != nil || binding == nil || binding.Token == "" {
		chatWikiSyncCache = result
		return result
	}
	result.Binding = binding

	// Fetch model catalog (now should be fresh after refresh).
	catalog, err := chatwiki.GetModelCatalogForSync()
	if err != nil || catalog == nil {
		chatWikiSyncCache = result
		return result
	}
	result.Catalog = catalog

	chatWikiSyncCache = result
	return result
}

// ClearChatWikiSyncCache clears the cached ChatWiki sync data.
func ClearChatWikiSyncCache() {
	chatWikiSyncMu.Lock()
	defer chatWikiSyncMu.Unlock()
	chatWikiSyncCache = nil
}

// fetchChatWikiBindingFromDB reads the ChatWiki binding from sqlite.
func fetchChatWikiBindingFromDB() (*chatWikiBindingDTO, error) {
	db := sqlite.DB()
	if db == nil {
		return nil, fmt.Errorf("sqlite not initialized")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var m struct {
		bun.BaseModel `bun:"table:chatwiki_bindings"`
		ID            int64  `bun:"id,pk,autoincrement"`
		ServerURL     string `bun:"server_url,notnull"`
		Token         string `bun:"token,notnull"`
		UserID        string `bun:"user_id,notnull"`
	}
	err := db.NewSelect().Model(&m).OrderExpr("id DESC").Limit(1).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return &chatWikiBindingDTO{
		ID:        m.ID,
		ServerURL: m.ServerURL,
		Token:    m.Token,
		UserID:   m.UserID,
	}, nil
}

// buildChatWikiProvider builds an OpenClaw provider config for ChatWiki.
func buildChatWikiProvider(data *chatWikiSyncData) *openclawProviderConfig {
	if data == nil || data.Binding == nil || data.Binding.Token == "" {
		return nil
	}

	// Build base URL for ChatWiki OpenAI-compatible API.
	baseURL := strings.TrimRight(strings.TrimSpace(define.GetModelChatWikiURL()), "/")
	if baseURL == "" {
		baseURL = strings.TrimRight(data.Binding.ServerURL, "/")
	}
	apiBaseURL := baseURL + "/chatclaw/v1"

	ocProvider := &openclawProviderConfig{
		BaseURL: apiBaseURL,
		APIKey:  data.Binding.Token,
		API:     "openai-completions",
	}

	// Add models from catalog.
	if data.Catalog != nil && len(data.Catalog.LLMModels) > 0 {
		for _, m := range data.Catalog.LLMModels {
			if !m.Enabled {
				continue
			}
			entry := openclawModelEntry{
				ID:   m.ModelID,
				Name: m.Name,
			}
			// CRITICAL: OpenClaw only accepts ["text"] and/or ["image"] for input.
			// ChatWiki API capabilities include "document", "video", "audio", "function_call",
			// "rerank" etc. which all cause INVALID_REQUEST. Force to ["text"] for all ChatWiki
			// models until multi-capability support is confirmed safe. This also sidesteps any
			// caching issues in buildModelsPatch where the DB catalog may hold stale values.
			entry.Input = []string{"text"}
			ocProvider.Models = append(ocProvider.Models, entry)
		}
	}

	return ocProvider
}

func buildSingleProvider(p providers.Provider) *openclawProviderConfig {
	api, ok := chatclawTypeToOpenClawAPI[p.Type]
	if !ok {
		api = "openai-completions"
	}

	oc := &openclawProviderConfig{API: api}

	endpoint := strings.TrimSpace(p.APIEndpoint)
	if endpoint != "" {
		if api == "anthropic-messages" {
			endpoint = strings.TrimSuffix(endpoint, "/v1")
		}
		oc.BaseURL = endpoint
	}

	if p.ProviderID != "ollama" {
		oc.APIKey = p.APIKey
	}

	if p.Type == "azure" {
		var extra struct {
			APIVersion string `json:"api_version"`
		}
		if p.ExtraConfig != "" {
			_ = json.Unmarshal([]byte(p.ExtraConfig), &extra)
		}
		if extra.APIVersion != "" && endpoint != "" {
			sep := "?"
			if strings.Contains(endpoint, "?") {
				sep = "&"
			}
			oc.BaseURL = endpoint + sep + "api-version=" + extra.APIVersion
		}
	}

	return oc
}
