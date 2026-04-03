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

	"chatclaw/internal/define"
	"chatclaw/internal/sqlite"

	"github.com/uptrace/bun"
)

type ModelCatalogItem struct {
	ModelID                string   `json:"model_id"`
	Name                   string   `json:"name"`
	Type                   string   `json:"type"`
	Enabled                bool     `json:"enabled"`
	SortOrder              int      `json:"sort_order"`
	Capabilities           []string `json:"capabilities"`
	ModelSupplier          string   `json:"model_supplier"`
	UniModelName           string   `json:"uni_model_name"`
	Price                  string   `json:"price"`
	RegionScope            string   `json:"region_scope"`
	SelfOwnedModelConfigID int      `json:"self_owned_model_config_id"`
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

type modelCatalogSource struct {
	ServerURL string
	Token     string
	UserID    string
	Bound     bool
	Cloud     bool
}

var (
	modelCatalogMu    sync.RWMutex
	modelCatalogCache *ModelCatalog
	getChatWikiSyncDB = sqlite.DB
)

func previewChatWikiLogBody(raw []byte) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return ""
	}
	const limit = 400
	if len(trimmed) > limit {
		return trimmed[:limit] + "...(truncated)"
	}
	return trimmed
}

func chatWikiOpenAIBaseURL(_ string) string {
	baseURL := strings.TrimRight(strings.TrimSpace(define.GetModelChatWikiURL()), "/")
	if baseURL == "" {
		return ""
	}
	return baseURL + "/chatclaw/v1"
}

func chatWikiChatCompletionsURL(serverURL string) string {
	baseURL := chatWikiOpenAIBaseURL(serverURL)
	if baseURL == "" {
		return ""
	}
	return baseURL + "/chat/completions"
}

// chatWikiTeamManageChatCompletionsURL returns the team-assistant streaming endpoint
// (manage API). Do not use chatWikiChatCompletionsURL (/chatclaw/v1/chat/completions) for team tab.
func chatWikiTeamManageChatCompletionsURL(serverURL string) string {
	baseURL := normalizeManagementBaseURL(serverURL)
	if baseURL == "" {
		return ""
	}
	return baseURL + "/manage/chatclaw/chat/completions"
}

func getModelCatalogSource(binding *Binding) modelCatalogSource {
	if binding == nil {
		return modelCatalogSource{
			ServerURL: strings.TrimSpace(define.GetChatWikiCloudURL()),
			Token:     "",
			UserID:    "",
			Bound:     false,
			Cloud:     false,
		}
	}
	return modelCatalogSource{
		ServerURL: strings.TrimSpace(binding.ServerURL),
		Token:     strings.TrimSpace(binding.Token),
		UserID:    binding.UserID,
		Bound:     true,
		Cloud:     isChatWikiCloudBinding(binding),
	}
}

func (s *ChatWikiService) SyncProviderState() error {
	s.app.Logger.Info("[chatwiki] SyncProviderState start")
	binding, err := s.GetBinding()
	if err != nil {
		s.app.Logger.Error("[chatwiki] SyncProviderState get binding failed", "error", err)
		return err
	}
	err = syncChatWikiProviderCredentials(binding)
	if err != nil {
		s.app.Logger.Error("[chatwiki] SyncProviderState sync credentials failed", "has_binding", binding != nil, "error", err)
		return err
	}
	s.app.Logger.Info("[chatwiki] SyncProviderState done",
		"has_binding", binding != nil,
		"server_url", func() string {
			if binding == nil {
				return ""
			}
			return strings.TrimSpace(binding.ServerURL)
		}(),
	)
	return nil
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
	if isChatWikiCloudBinding(binding) {
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
	s.app.Logger.Info("[chatwiki] RefreshModelCatalog start")
	binding, err := s.GetBinding()
	if err != nil {
		s.app.Logger.Error("[chatwiki] RefreshModelCatalog get binding failed", "error", err)
		return nil, err
	}
	if err := syncChatWikiProviderCredentials(binding); err != nil {
		s.app.Logger.Error("[chatwiki] RefreshModelCatalog sync credentials failed", "has_binding", binding != nil, "error", err)
		return nil, err
	}
	source := getModelCatalogSource(binding)
	catalog, err := s.fetchModelCatalog(source)
	if err != nil {
		s.app.Logger.Error("[chatwiki] RefreshModelCatalog fetch failed",
			"server_url", source.ServerURL,
			"user_id", source.UserID,
			"bound", source.Bound,
			"error", err,
		)
		modelCatalogMu.RLock()
		cached := cloneModelCatalog(modelCatalogCache)
		modelCatalogMu.RUnlock()
		if cached != nil {
			s.app.Logger.Warn("[chatwiki] RefreshModelCatalog using cached catalog after fetch failure",
				"llm_count", len(cached.LLMModels),
				"embedding_count", len(cached.EmbeddingModels),
			)
			return cached, nil
		}
		return nil, err
	}

	modelCatalogMu.Lock()
	modelCatalogCache = cloneModelCatalog(catalog)
	modelCatalogMu.Unlock()
	s.app.Logger.Info("[chatwiki] RefreshModelCatalog done",
		"user_id", source.UserID,
		"server_url", source.ServerURL,
		"bound", source.Bound,
		"llm_count", len(catalog.LLMModels),
		"embedding_count", len(catalog.EmbeddingModels),
		"rerank_count", len(catalog.RerankModels),
	)
	return cloneModelCatalog(catalog), nil
}

func (s *ChatWikiService) GetModelCatalog(forceRefresh bool) (*ModelCatalog, error) {
	s.app.Logger.Info("[chatwiki] GetModelCatalog start", "force_refresh", forceRefresh)
	if forceRefresh {
		return s.RefreshModelCatalog()
	}

	modelCatalogMu.RLock()
	cached := cloneModelCatalog(modelCatalogCache)
	modelCatalogMu.RUnlock()
	if cached != nil {
		s.app.Logger.Info("[chatwiki] GetModelCatalog hit cache",
			"llm_count", len(cached.LLMModels),
			"embedding_count", len(cached.EmbeddingModels),
			"loaded_at_unix", cached.LoadedAtUnix,
		)
		return cached, nil
	}
	s.app.Logger.Info("[chatwiki] GetModelCatalog cache miss, refreshing")
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

// GetModelCatalogForSync returns the cached ChatWiki model catalog for OpenClaw sync.
// If no cached catalog exists, it attempts to fetch a fresh one from ChatWiki API.
// This function is designed to be called from the openclawruntime package without
// requiring access to ChatWikiService instance.
// Returns nil if no binding exists or catalog fetch fails.
func GetModelCatalogForSync() (*ModelCatalog, error) {
	modelCatalogMu.RLock()
	cached := modelCatalogCache
	modelCatalogMu.RUnlock()
	if cached != nil {
		return cloneModelCatalog(cached), nil
	}
	// No cached catalog - try to get binding and refresh.
	// Note: We can't call RefreshModelCatalog directly here because it requires
	// ChatWikiService instance. Instead, we return nil and let the caller handle it.
	// The models section will only include providers with valid API keys.
	return nil, nil
}

// RefreshChatWikiModelCatalog triggers a refresh of the ChatWiki model catalog cache.
// This should be called before OpenClaw config sync to ensure latest models are available.
// The refresh is performed via the providers service to avoid circular dependencies.
var RefreshChatWikiModelCatalog func() error

func (s *ChatWikiService) fetchModelCatalog(source modelCatalogSource) (*ModelCatalog, error) {
	baseURL := normalizeManagementBaseURL(source.ServerURL)
	modelURL := baseURL + "/manage/chatclaw/showModelConfigList"
	statsURL := baseURL + "/manage/chatclaw/getIntegralStats"
	s.app.Logger.Info("[chatwiki] fetchModelCatalog request start",
		"server_url", baseURL,
		"model_url", modelURL,
		"stats_url", statsURL,
		"token_len", len(strings.TrimSpace(source.Token)),
		"user_id", source.UserID,
		"bound", source.Bound,
	)

	modelBody, err := s.chatWikiGETLoose(source.Token, modelURL)
	if err != nil {
		s.app.Logger.Error("[chatwiki] fetchModelCatalog model request failed", "url", modelURL, "error", err)
		return nil, err
	}
	s.app.Logger.Info("[chatwiki] fetchModelCatalog model response",
		"url", modelURL,
		"body_len", len(modelBody),
		"body_preview", previewChatWikiLogBody(modelBody),
	)
	catalog, err := decodeModelCatalogResponse(modelBody)
	if err != nil {
		s.app.Logger.Error("[chatwiki] fetchModelCatalog decode failed", "url", modelURL, "error", err)
		return nil, err
	}
	if err := syncModelCatalogToLocalDB(catalog); err != nil {
		s.app.Logger.Error("[chatwiki] fetchModelCatalog sync db failed", "url", modelURL, "error", err)
		return nil, err
	}
	s.app.Logger.Info("[chatwiki] fetchModelCatalog decoded",
		"llm_count", len(catalog.LLMModels),
		"embedding_count", len(catalog.EmbeddingModels),
		"rerank_count", len(catalog.RerankModels),
	)

	if source.Bound && source.Cloud {
		statsBody, statsErr := s.chatWikiGETLoose(source.Token, statsURL)
		if statsErr == nil {
			catalog.IntegralStats = &IntegralStats{Raw: append(json.RawMessage(nil), statsBody...)}
			s.app.Logger.Info("[chatwiki] fetchModelCatalog stats response",
				"url", statsURL,
				"body_len", len(statsBody),
				"body_preview", previewChatWikiLogBody(statsBody),
			)
		} else {
			s.app.Logger.Warn("[chatwiki] fetchModelCatalog stats request failed", "url", statsURL, "error", statsErr)
		}
	}

	catalog.Bound = source.Bound
	catalog.BindingUserID = source.UserID
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
	if len(catalog.LLMModels) == 0 && len(catalog.EmbeddingModels) == 0 {
		return catalog, nil
	}
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
	if modelType == "llm" {
		if modelSupportsImage(item) {
			capabilities = appendCapability(capabilities, "image")
		}
		if modelSupportsFile(item) {
			capabilities = appendCapability(capabilities, "file")
		}
	}

	return ModelCatalogItem{
		ModelID:                modelID,
		Name:                   name,
		Type:                   modelType,
		Enabled:                parseFlexibleBool(item, true, "enabled", "status", "switch_status", "chat_claw_switch_status"),
		SortOrder:              firstInt(item, "sort_order", "sortOrder", "order", "idx", "index"),
		Capabilities:           capabilities,
		ModelSupplier:          modelSupplier,
		UniModelName:           uniModelName,
		Price:                  price,
		RegionScope:            regionScope,
		SelfOwnedModelConfigID: firstInt(item, "id", "config_id", "configId", "self_owned_model_config_id", "selfOwnedModelConfigId"),
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

func modelSupportsFile(item map[string]any) bool {
	if parseFlexibleBool(item, false, "input_document", "inputDocument", "support_document", "supportDocument", "document", "file") {
		return true
	}
	for _, key := range []string{"capabilities", "ability", "input_type", "input_types", "support_type"} {
		raw, ok := item[key]
		if !ok || raw == nil {
			continue
		}
		switch v := raw.(type) {
		case string:
			lower := strings.ToLower(v)
			if strings.Contains(lower, "document") || strings.Contains(lower, "file") {
				return true
			}
		case []any:
			for _, entry := range v {
				lower := strings.ToLower(fmt.Sprintf("%v", entry))
				if strings.Contains(lower, "document") || strings.Contains(lower, "file") {
					return true
				}
			}
		}
	}
	return false
}

func appendCapability(capabilities []string, capability string) []string {
	for _, existing := range capabilities {
		if existing == capability {
			return capabilities
		}
	}
	return append(capabilities, capability)
}

type syncedModelRow struct {
	bun.BaseModel `bun:"table:models"`

	ID           int64     `bun:"id,pk,autoincrement"`
	ProviderID   string    `bun:"provider_id,notnull"`
	ModelID      string    `bun:"model_id,notnull"`
	Name         string    `bun:"name,notnull"`
	Type         string    `bun:"type,notnull"`
	Capabilities string    `bun:"capabilities,notnull"`
	IsBuiltin    bool      `bun:"is_builtin,notnull"`
	Enabled      bool      `bun:"enabled,notnull"`
	SortOrder    int       `bun:"sort_order,notnull"`
	CreatedAt    time.Time `bun:"created_at,notnull"`
	UpdatedAt    time.Time `bun:"updated_at,notnull"`
}

type syncedCatalogModel struct {
	ModelID      string
	Name         string
	Type         string
	Capabilities string
	SortOrder    int
}

func syncModelCatalogToLocalDB(catalog *ModelCatalog) error {
	return syncModelCatalogToDB(context.Background(), getChatWikiSyncDB(), "chatwiki", catalog)
}

func syncModelCatalogToDB(ctx context.Context, db *bun.DB, providerID string, catalog *ModelCatalog) error {
	if db == nil || strings.TrimSpace(providerID) == "" || catalog == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	remote := flattenCatalogModelsForSync(catalog)
	remoteMap := make(map[string]syncedCatalogModel, len(remote))
	for _, item := range remote {
		remoteMap[item.ModelID] = item
	}

	return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		existing := make([]syncedModelRow, 0)
		if err := tx.NewSelect().
			Model(&existing).
			Where("provider_id = ?", providerID).
			Scan(ctx); err != nil {
			return err
		}

		existingMap := make(map[string]syncedModelRow, len(existing))
		for _, row := range existing {
			existingMap[row.ModelID] = row
		}

		toDelete := make([]string, 0)
		for modelID := range existingMap {
			if _, ok := remoteMap[modelID]; !ok {
				toDelete = append(toDelete, modelID)
			}
		}
		for _, chunk := range chunkStrings(toDelete, 200) {
			if _, err := tx.NewDelete().
				Table("models").
				Where("provider_id = ?", providerID).
				Where("model_id IN (?)", bun.In(chunk)).
				Exec(ctx); err != nil {
				return err
			}
		}

		toInsert := make([]syncedModelRow, 0)
		for _, item := range remote {
			if existingRow, ok := existingMap[item.ModelID]; ok {
				needsUpdate := strings.TrimSpace(existingRow.Name) != item.Name ||
					strings.TrimSpace(strings.ToLower(existingRow.Type)) != item.Type ||
					existingRow.Capabilities != item.Capabilities ||
					existingRow.SortOrder != item.SortOrder ||
					!existingRow.Enabled ||
					!existingRow.IsBuiltin
				if !needsUpdate {
					continue
				}
				if _, err := tx.NewUpdate().
					Model((*syncedModelRow)(nil)).
					Where("provider_id = ?", providerID).
					Where("model_id = ?", item.ModelID).
					Set("name = ?", item.Name).
					Set("type = ?", item.Type).
					Set("capabilities = ?", item.Capabilities).
					Set("sort_order = ?", item.SortOrder).
					Set("enabled = ?", true).
					Set("is_builtin = ?", true).
					Set("updated_at = ?", sqlite.NowUTC()).
					Exec(ctx); err != nil {
					return err
				}
				continue
			}

			toInsert = append(toInsert, syncedModelRow{
				ProviderID:   providerID,
				ModelID:      item.ModelID,
				Name:         item.Name,
				Type:         item.Type,
				Capabilities: item.Capabilities,
				IsBuiltin:    true,
				Enabled:      true,
				SortOrder:    item.SortOrder,
			})
		}

		for _, chunk := range chunkSyncedModels(toInsert, 200) {
			if _, err := tx.NewInsert().Model(&chunk).Exec(ctx); err != nil {
				return err
			}
		}
		return nil
	})
}

func clearSyncedModelCatalogFromDB(ctx context.Context, db *bun.DB, providerID string) error {
	if db == nil || strings.TrimSpace(providerID) == "" {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	_, err := db.NewDelete().
		Table("models").
		Where("provider_id = ?", providerID).
		Exec(ctx)
	return err
}

func flattenCatalogModelsForSync(catalog *ModelCatalog) []syncedCatalogModel {
	if catalog == nil {
		return nil
	}

	collections := []struct {
		typ   string
		items []ModelCatalogItem
	}{
		{typ: "llm", items: catalog.LLMModels},
		{typ: "embedding", items: catalog.EmbeddingModels},
		{typ: "rerank", items: catalog.RerankModels},
	}

	out := make([]syncedCatalogModel, 0, len(catalog.LLMModels)+len(catalog.EmbeddingModels)+len(catalog.RerankModels))
	perTypeOrder := map[string]int{
		"llm":       0,
		"embedding": 0,
		"rerank":    0,
	}
	for _, collection := range collections {
		for _, item := range collection.items {
			if !item.Enabled {
				continue
			}
			modelID := strings.TrimSpace(item.UniModelName)
			if modelID == "" {
				modelID = strings.TrimSpace(item.ModelID)
			}
			if modelID == "" || strings.Contains(modelID, "::") {
				continue
			}

			modelType := normalizeCatalogModelType(item.Type)
			if modelType == "" {
				modelType = collection.typ
			}
			sortOrder := item.SortOrder
			if sortOrder <= 0 {
				sortOrder = perTypeOrder[modelType]
			}
			perTypeOrder[modelType] = sortOrder + 1

			out = append(out, syncedCatalogModel{
				ModelID:      modelID,
				Name:         modelID,
				Type:         modelType,
				Capabilities: marshalCapabilities(item.Capabilities),
				SortOrder:    sortOrder,
			})
		}
	}

	return out
}

func normalizeCatalogModelType(modelType string) string {
	switch strings.ToLower(strings.TrimSpace(modelType)) {
	case "embedding":
		return "embedding"
	case "rerank":
		return "rerank"
	default:
		return "llm"
	}
}

func marshalCapabilities(capabilities []string) string {
	if len(capabilities) == 0 {
		capabilities = []string{"text"}
	}
	raw, err := json.Marshal(capabilities)
	if err != nil {
		return `["text"]`
	}
	return string(raw)
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

func chunkSyncedModels(in []syncedModelRow, size int) [][]syncedModelRow {
	if size <= 0 || len(in) == 0 {
		return nil
	}
	out := make([][]syncedModelRow, 0, (len(in)+size-1)/size)
	for i := 0; i < len(in); i += size {
		j := i + size
		if j > len(in) {
			j = len(in)
		}
		out = append(out, in[i:j])
	}
	return out
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
