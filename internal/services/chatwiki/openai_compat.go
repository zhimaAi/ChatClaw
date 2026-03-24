package chatwiki

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const chatWikiModelCatalogCacheTTL = 2 * time.Minute

type cachedOpenAIModelCatalog struct {
	catalog   *ModelCatalog
	expiresAt time.Time
}

var openAIModelCatalogCache sync.Map

func ResetOpenAIModelCatalogCacheForTest() {
	openAIModelCatalogCache = sync.Map{}
}

func ResolveSelfOwnedModelConfigID(apiKey, apiEndpoint, modelID, modelType string) (int, error) {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return 0, fmt.Errorf("chatwiki model_id is required")
	}

	catalog, err := loadModelCatalogForOpenAI(apiKey, apiEndpoint)
	if err != nil {
		return 0, err
	}

	for _, item := range catalogItemsByType(catalog, modelType) {
		if strings.TrimSpace(item.ModelID) != modelID {
			continue
		}
		if item.SelfOwnedModelConfigID <= 0 {
			return 0, fmt.Errorf("chatwiki model %q missing self_owned_model_config_id", modelID)
		}
		return item.SelfOwnedModelConfigID, nil
	}

	return 0, fmt.Errorf("chatwiki model %q not found in model catalog", modelID)
}

func loadModelCatalogForOpenAI(apiKey, apiEndpoint string) (*ModelCatalog, error) {
	baseURL := normalizeManagementBaseURL(apiEndpoint)
	if baseURL == "" {
		return nil, fmt.Errorf("invalid chatwiki api endpoint: %q", apiEndpoint)
	}

	cacheKey := strings.TrimSpace(apiKey) + "|" + baseURL
	if cached, ok := openAIModelCatalogCache.Load(cacheKey); ok {
		entry := cached.(cachedOpenAIModelCatalog)
		if time.Now().Before(entry.expiresAt) && entry.catalog != nil {
			return cloneModelCatalog(entry.catalog), nil
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/manage/chatclaw/showModelConfigList", nil)
	if err != nil {
		return nil, fmt.Errorf("create chatwiki model catalog request: %w", err)
	}
	req.Header.Set("Token", strings.TrimSpace(apiKey))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request chatwiki model catalog: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read chatwiki model catalog: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("chatwiki model catalog status=%d body=%s", resp.StatusCode, previewChatWikiLogBody(body))
	}

	catalog, err := decodeModelCatalogResponse(json.RawMessage(body))
	if err != nil {
		return nil, fmt.Errorf("decode chatwiki model catalog: %w", err)
	}
	if err := syncModelCatalogToLocalDB(catalog); err != nil {
		return nil, fmt.Errorf("sync chatwiki model catalog to db: %w", err)
	}

	openAIModelCatalogCache.Store(cacheKey, cachedOpenAIModelCatalog{
		catalog:   cloneModelCatalog(catalog),
		expiresAt: time.Now().Add(chatWikiModelCatalogCacheTTL),
	})
	return cloneModelCatalog(catalog), nil
}

func normalizeManagementBaseURL(apiEndpoint string) string {
	baseURL := strings.TrimRight(strings.TrimSpace(apiEndpoint), "/")
	switch {
	case strings.HasSuffix(baseURL, "/chatclaw/v1"):
		return strings.TrimSuffix(baseURL, "/chatclaw/v1")
	case strings.HasSuffix(baseURL, "/openapi/chatclaw/v1"):
		return strings.TrimSuffix(baseURL, "/chatclaw/v1")
	default:
		return baseURL
	}
}

func catalogItemsByType(catalog *ModelCatalog, modelType string) []ModelCatalogItem {
	switch strings.ToLower(strings.TrimSpace(modelType)) {
	case "embedding":
		return catalog.EmbeddingModels
	case "rerank":
		return catalog.RerankModels
	default:
		return catalog.LLMModels
	}
}
