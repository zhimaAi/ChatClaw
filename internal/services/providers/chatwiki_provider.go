package providers

import (
	"strings"

	"chatclaw/internal/services/chatwiki"
)

func (s *ProvidersService) buildChatWikiProviderWithModels(provider *Provider, catalog *chatwiki.ModelCatalog) *ProviderWithModels {
	if provider == nil {
		return nil
	}
	groups := make([]ModelGroup, 0, 2)
	if catalog != nil && len(catalog.LLMModels) > 0 {
		groups = append(groups, ModelGroup{
			Type:   "llm",
			Models: s.chatWikiCatalogItemsToModels(provider.ProviderID, catalog.LLMModels),
		})
	}
	if catalog != nil && len(catalog.EmbeddingModels) > 0 {
		groups = append(groups, ModelGroup{
			Type:   "embedding",
			Models: s.chatWikiCatalogItemsToModels(provider.ProviderID, catalog.EmbeddingModels),
		})
	}
	return &ProviderWithModels{
		Provider:    *provider,
		ModelGroups: groups,
	}
}

func (s *ProvidersService) chatWikiCatalogItemsToModels(providerID string, items []chatwiki.ModelCatalogItem) []Model {
	models := make([]Model, 0, len(items))
	for idx, item := range items {
		if !item.Enabled {
			continue
		}
		models = append(models, Model{
			ID:            int64(idx + 1),
			ProviderID:    providerID,
			ModelID:       item.ModelID,
			Name:          item.Name,
			ModelSupplier: item.ModelSupplier,
			UniModelName:  item.UniModelName,
			Type:          item.Type,
			Capabilities:  filterOpenClawCapabilities(item.Capabilities),
			IsBuiltin:     true,
			Enabled:       item.Enabled,
			SortOrder:     item.SortOrder,
		})
	}
	return models
}

// filterOpenClawCapabilities keeps only capabilities that OpenClaw accepts:
// "text" and "image". All other values (document, video, audio, function_call,
// etc.) are stripped because OpenClaw validates this field strictly and rejects
// any unknown value with "INVALID_REQUEST: invalid config".
func filterOpenClawCapabilities(capabilities []string) []string {
	if len(capabilities) == 0 {
		return []string{"text"}
	}
	valid := make([]string, 0, 2)
	for _, c := range capabilities {
		lower := strings.ToLower(strings.TrimSpace(c))
		if lower == "text" || lower == "image" {
			valid = append(valid, lower)
		}
	}
	if len(valid) == 0 {
		return []string{"text"}
	}
	return valid
}
