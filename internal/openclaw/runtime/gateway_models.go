package openclawruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"chatclaw/internal/services/providers"
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
	"gemini":    "google-generative-ai",
	"ollama":    "openai-completions",
	"qwen":      "openai-completions",
}

func buildModelsPatch(providersSvc *providers.ProvidersService, allProviders []providers.Provider) map[string]any {
	providerMap := make(map[string]any)

	for _, p := range allProviders {
		if !p.Enabled {
			continue
		}
		if p.ProviderID != "ollama" && strings.TrimSpace(p.APIKey) == "" {
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
					if len(m.Capabilities) > 0 {
						entry.Input = m.Capabilities
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
