package chatwiki

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"chatclaw/internal/sqlite"
)

type ModelCatalogItem struct {
	ModelID       string   `json:"model_id"`
	Name          string   `json:"name"`
	Type          string   `json:"type"`
	Enabled       bool     `json:"enabled"`
	SortOrder     int      `json:"sort_order"`
	Capabilities  []string `json:"capabilities"`
	ModelSupplier string   `json:"model_supplier"`
	UniModelName  string   `json:"uni_model_name"`
	Price         string   `json:"price"`
	RegionScope   string   `json:"region_scope"`
}

type IntegralStats struct {
	Raw json.RawMessage `json:"raw"`
}

type ModelCatalog struct {
	Bound           bool               `json:"bound"`
	BindingUserID   string             `json:"binding_user_id"`
	LoadedAtUnix    int64              `json:"loaded_at_unix"`
	LLMModels       []ModelCatalogItem `json:"llm_models"`
	EmbeddingModels []ModelCatalogItem `json:"embedding_models"`
	RerankModels    []ModelCatalogItem `json:"rerank_models"`
	IntegralStats   *IntegralStats     `json:"integral_stats,omitempty"`
}

var (
	modelCatalogMu    sync.RWMutex
	modelCatalogCache *ModelCatalog
)

func chatWikiOpenAIBaseURL(serverURL string) string {
	baseURL := strings.TrimRight(strings.TrimSpace(serverURL), "/")
	if baseURL == "" {
		return ""
	}
	return baseURL + "/chatclaw/v1"
}

func (s *ChatWikiService) SyncProviderState() error {
	binding, err := s.GetBinding()
	if err != nil {
		return err
	}
	return syncChatWikiProviderCredentials(binding)
}

func syncChatWikiProviderCredentials(binding *Binding) error {
	db := sqlite.DB()
	if db == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	apiKey := ""
	apiEndpoint := ""
	if binding != nil {
		apiKey = strings.TrimSpace(binding.Token)
		apiEndpoint = chatWikiOpenAIBaseURL(binding.ServerURL)
	}

	_, err := db.NewUpdate().
		Table("providers").
		Where("provider_id = ?", "chatwiki").
		Set("api_key = ?", apiKey).
		Set("api_endpoint = ?", apiEndpoint).
		Set("updated_at = ?", sqlite.NowUTC()).
		Exec(ctx)
	return err
}

func (s *ChatWikiService) RefreshModelCatalog() (*ModelCatalog, error) {
	binding, err := s.GetBinding()
	if err != nil {
		return nil, err
	}
	if err := syncChatWikiProviderCredentials(binding); err != nil {
		return nil, err
	}
	if binding == nil {
		empty := &ModelCatalog{Bound: false, LoadedAtUnix: time.Now().Unix()}
		modelCatalogMu.Lock()
		modelCatalogCache = empty
		modelCatalogMu.Unlock()
		return cloneModelCatalog(empty), nil
	}

	catalog, err := s.fetchModelCatalog(binding)
	if err != nil {
		modelCatalogMu.RLock()
		cached := cloneModelCatalog(modelCatalogCache)
		modelCatalogMu.RUnlock()
		if cached != nil {
			return cached, nil
		}
		return nil, err
	}

	modelCatalogMu.Lock()
	modelCatalogCache = cloneModelCatalog(catalog)
	modelCatalogMu.Unlock()
	return cloneModelCatalog(catalog), nil
}

func (s *ChatWikiService) GetModelCatalog(forceRefresh bool) (*ModelCatalog, error) {
	if forceRefresh {
		return s.RefreshModelCatalog()
	}

	modelCatalogMu.RLock()
	cached := cloneModelCatalog(modelCatalogCache)
	modelCatalogMu.RUnlock()
	if cached != nil {
		return cached, nil
	}
	return s.RefreshModelCatalog()
}

func cloneModelCatalog(in *ModelCatalog) *ModelCatalog {
	if in == nil {
		return nil
	}
	out := *in
	out.LLMModels = append([]ModelCatalogItem(nil), in.LLMModels...)
	out.EmbeddingModels = append([]ModelCatalogItem(nil), in.EmbeddingModels...)
	out.RerankModels = append([]ModelCatalogItem(nil), in.RerankModels...)
	if in.IntegralStats != nil {
		raw := append(json.RawMessage(nil), in.IntegralStats.Raw...)
		out.IntegralStats = &IntegralStats{Raw: raw}
	}
	return &out
}

func (s *ChatWikiService) fetchModelCatalog(binding *Binding) (*ModelCatalog, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(binding.ServerURL), "/")
	modelURL := baseURL + "/manage/chatclaw/showModelConfigList"
	statsURL := baseURL + "/manage/chatclaw/getIntegralStats"

	modelBody, err := s.chatWikiGETLoose(binding.Token, modelURL)
	if err != nil {
		return nil, err
	}
	catalog, err := decodeModelCatalogResponse(modelBody)
	if err != nil {
		return nil, err
	}

	statsBody, statsErr := s.chatWikiGETLoose(binding.Token, statsURL)
	if statsErr == nil {
		catalog.IntegralStats = &IntegralStats{Raw: append(json.RawMessage(nil), statsBody...)}
	}

	catalog.Bound = true
	catalog.BindingUserID = binding.UserID
	catalog.LoadedAtUnix = time.Now().Unix()
	return catalog, nil
}

func decodeModelCatalogResponse(raw json.RawMessage) (*ModelCatalog, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || strings.EqualFold(trimmed, "null") {
		return &ModelCatalog{}, nil
	}

	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, err
	}

	catalog := &ModelCatalog{}
	seen := make(map[string]struct{})
	collectModelCatalogItems(decoded, "", catalog, seen)
	sortModelCatalogItems(catalog.LLMModels)
	sortModelCatalogItems(catalog.EmbeddingModels)
	sortModelCatalogItems(catalog.RerankModels)
	return catalog, nil
}

func sortModelCatalogItems(items []ModelCatalogItem) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].SortOrder != items[j].SortOrder {
			return items[i].SortOrder < items[j].SortOrder
		}
		return items[i].Name < items[j].Name
	})
}

func collectModelCatalogItems(value any, keyHint string, catalog *ModelCatalog, seen map[string]struct{}) {
	switch v := value.(type) {
	case map[string]any:
		if item, ok := parseModelCatalogItem(v, keyHint); ok {
			cacheKey := item.Type + "::" + item.ModelID
			if _, exists := seen[cacheKey]; !exists {
				seen[cacheKey] = struct{}{}
				switch item.Type {
				case "embedding":
					catalog.EmbeddingModels = append(catalog.EmbeddingModels, item)
				case "rerank":
					// Chatwiki provider currently only exposes llm + embedding in the app.
				default:
					catalog.LLMModels = append(catalog.LLMModels, item)
				}
			}
		}
		for key, child := range v {
			collectModelCatalogItems(child, key, catalog, seen)
		}
	case []any:
		for _, child := range v {
			collectModelCatalogItems(child, keyHint, catalog, seen)
		}
	}
}

func parseModelCatalogItem(item map[string]any, keyHint string) (ModelCatalogItem, bool) {
	modelID := firstNonEmptyString(item,
		"model_name", "modelName", "name", "model", "model_id", "modelId", "id",
	)
	if modelID == "" {
		return ModelCatalogItem{}, false
	}

	modelType := detectModelCatalogType(item, keyHint, modelID)
	name := firstNonEmptyString(item, "display_name", "displayName", "name", "model_name", "modelName", "model")
	if name == "" {
		name = modelID
	}
	modelSupplier := firstNonEmptyString(item,
		"model_supplier", "modelSupplier", "supplier", "provider", "provider_name", "vendor",
	)
	uniModelName := firstNonEmptyString(item,
		"uni_model_name", "uniModelName", "universal_model_name", "universalModelName",
		"normalized_model_name", "normalizedModelName",
	)
	regionScope := normalizeRegionScope(firstNonEmptyString(item,
		"region_scope", "regionScope", "scope", "region", "area_scope", "areaScope",
	))
	price := strings.TrimSpace(firstNonEmptyString(item, "price", "model_price", "modelPrice"))

	if modelSupplier == "" || uniModelName == "" {
		supplier, uni := splitModelDisplayName(name)
		if supplier == "" || uni == "" {
			fallbackSupplier, fallbackUni := splitModelDisplayName(modelID)
			if supplier == "" {
				supplier = fallbackSupplier
			}
			if uni == "" {
				uni = fallbackUni
			}
		}
		if modelSupplier == "" {
			modelSupplier = supplier
		}
		if uniModelName == "" {
			if uni != "" {
				uniModelName = uni
			} else {
				uniModelName = strings.TrimSpace(name)
			}
		}
	}

	capabilities := []string{"text"}
	if modelType == "llm" && modelSupportsImage(item) {
		capabilities = []string{"text", "image"}
	}

	return ModelCatalogItem{
		ModelID:      modelID,
		Name:         name,
		Type:         modelType,
		Enabled:      parseFlexibleBool(item, true, "enabled", "status", "switch_status", "chat_claw_switch_status"),
		SortOrder:    firstInt(item, "sort_order", "sortOrder", "order", "idx", "index"),
		Capabilities: capabilities,
		ModelSupplier: modelSupplier,
		UniModelName:  uniModelName,
		Price:         price,
		RegionScope:   regionScope,
	}, true
}

func splitModelDisplayName(raw string) (string, string) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", ""
	}
	parts := strings.SplitN(trimmed, "/", 2)
	if len(parts) != 2 {
		return "", trimmed
	}
	supplier := strings.TrimSpace(parts[0])
	modelName := strings.TrimSpace(parts[1])
	if supplier == "" || modelName == "" {
		return "", trimmed
	}
	return supplier, modelName
}

func normalizeRegionScope(raw string) string {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "CN", "DOMESTIC", "CHINA":
		return "CN"
	case "GLOBAL", "OVERSEA", "INTL", "INTERNATIONAL":
		return "Global"
	default:
		return ""
	}
}

func detectModelCatalogType(item map[string]any, keyHint string, modelID string) string {
	candidates := []string{
		firstNonEmptyString(item, "type", "model_type", "modelType", "category", "kind"),
		keyHint,
		modelID,
	}
	for _, candidate := range candidates {
		lower := strings.ToLower(strings.TrimSpace(candidate))
		switch {
		case strings.Contains(lower, "embedding"):
			return "embedding"
		case strings.Contains(lower, "rerank"):
			return "rerank"
		case strings.Contains(lower, "llm"), strings.Contains(lower, "language"), strings.Contains(lower, "chat"), strings.Contains(lower, "completion"):
			return "llm"
		}
	}
	return "llm"
}

func modelSupportsImage(item map[string]any) bool {
	if parseFlexibleBool(item, false, "support_image", "supportImage", "input_image", "inputImage", "vision", "multimodal", "is_multimodal") {
		return true
	}
	for _, key := range []string{"capabilities", "ability", "input_type", "input_types", "support_type"} {
		raw, ok := item[key]
		if !ok || raw == nil {
			continue
		}
		switch v := raw.(type) {
		case string:
			if strings.Contains(strings.ToLower(v), "image") {
				return true
			}
		case []any:
			for _, entry := range v {
				if strings.Contains(strings.ToLower(fmt.Sprintf("%v", entry)), "image") {
					return true
				}
			}
		}
	}
	return false
}

func firstNonEmptyString(item map[string]any, keys ...string) string {
	for _, key := range keys {
		raw, ok := item[key]
		if !ok || raw == nil {
			continue
		}
		value := strings.TrimSpace(fmt.Sprintf("%v", raw))
		if value != "" && value != "<nil>" {
			return value
		}
	}
	return ""
}

func firstInt(item map[string]any, keys ...string) int {
	for _, key := range keys {
		raw, ok := item[key]
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
		case json.Number:
			if n, err := v.Int64(); err == nil {
				return int(n)
			}
		case string:
			if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
				return n
			}
		}
	}
	return 0
}

func parseFlexibleBool(item map[string]any, defaultValue bool, keys ...string) bool {
	for _, key := range keys {
		raw, ok := item[key]
		if !ok || raw == nil {
			continue
		}
		switch v := raw.(type) {
		case bool:
			return v
		case int:
			return v != 0
		case int64:
			return v != 0
		case float64:
			return v != 0
		case json.Number:
			if n, err := v.Int64(); err == nil {
				return n != 0
			}
		case string:
			lower := strings.ToLower(strings.TrimSpace(v))
			switch lower {
			case "1", "true", "open", "enabled", "yes":
				return true
			case "0", "false", "close", "closed", "disabled", "no":
				return false
			}
		}
	}
	return defaultValue
}
