package openclawruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"chatclaw/internal/services/providers"
)

// configSyncer synchronizes ChatClaw provider/model configuration to OpenClaw Gateway
// via WebSocket RPC (config.get + config.patch).
type configSyncer struct {
	manager      *Manager
	providersSvc *providers.ProvidersService

	mu           sync.Mutex
	debounce     *time.Timer
	lastPatchRaw string // cache last synced patch JSON to avoid redundant restarts
}

func newConfigSyncer(m *Manager, providersSvc *providers.ProvidersService) *configSyncer {
	return &configSyncer{manager: m, providersSvc: providersSvc}
}

// RequestSync schedules a debounced config sync (500ms).
// Multiple calls within the debounce window are coalesced into a single sync.
func (s *configSyncer) RequestSync() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.debounce != nil {
		s.debounce.Stop()
	}
	s.debounce = time.AfterFunc(500*time.Millisecond, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := s.doSync(ctx); err != nil {
			s.manager.app.Logger.Warn("openclaw: config sync failed", "error", err)
		}
	})
}

// SyncNow performs an immediate config sync (blocking).
func (s *configSyncer) SyncNow(ctx context.Context) error {
	return s.doSync(ctx)
}

// doSync reads all enabled providers/models from DB, builds the OpenClaw config patch,
// and applies it via Gateway RPC.
func (s *configSyncer) doSync(ctx context.Context) error {
	s.manager.mu.RLock()
	client := s.manager.client
	s.manager.mu.RUnlock()
	if client == nil {
		return fmt.Errorf("gateway client not connected")
	}

	// Read providers from DB
	allProviders, err := s.providersSvc.ListProviders()
	if err != nil {
		return fmt.Errorf("list providers: %w", err)
	}

	// Build the OpenClaw config patch
	patch := s.buildOpenClawModelsPatch(allProviders)

	// Marshal patch to JSON
	raw, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("marshal patch: %w", err)
	}
	rawStr := string(raw)

	// Skip if patch is identical to last sync (avoids restart loop)
	s.mu.Lock()
	unchanged := rawStr == s.lastPatchRaw
	s.mu.Unlock()
	if unchanged {
		s.manager.app.Logger.Info("openclaw: config unchanged, skipping sync")
		return nil
	}

	// Step 1: config.get to obtain baseHash
	var getResult struct {
		Hash string `json:"hash"`
	}
	if err := client.Request(ctx, "config.get", map[string]any{}, &getResult); err != nil {
		return fmt.Errorf("config.get: %w", err)
	}

	// Step 2: config.patch with the models section
	patchParams := map[string]any{
		"raw":      rawStr,
		"baseHash": getResult.Hash,
	}
	if err := client.Request(ctx, "config.patch", patchParams, nil); err != nil {
		return fmt.Errorf("config.patch: %w", err)
	}

	// Cache the synced patch
	s.mu.Lock()
	s.lastPatchRaw = rawStr
	s.mu.Unlock()

	s.manager.app.Logger.Info("openclaw: config sync completed")
	return nil
}

// openclawProviderConfig represents a single provider entry in OpenClaw's models.providers.
type openclawProviderConfig struct {
	BaseURL string               `json:"baseUrl,omitempty"`
	APIKey  string               `json:"apiKey,omitempty"`
	API     string               `json:"api"`
	Headers map[string]string    `json:"headers,omitempty"`
	Models  []openclawModelEntry `json:"models,omitempty"`
}

// openclawModelEntry represents a single model in an OpenClaw provider.
type openclawModelEntry struct {
	ID    string   `json:"id"`
	Name  string   `json:"name"`
	Input []string `json:"input,omitempty"`
}

// chatclawTypeToOpenClawAPI maps ChatClaw provider type to OpenClaw api adapter.
var chatclawTypeToOpenClawAPI = map[string]string{
	"openai":    "openai-completions",
	"azure":     "openai-completions",
	"anthropic": "anthropic-messages",
	"gemini":    "google-generative-ai",
	"ollama":    "openai-completions",
	"qwen":      "openai-completions",
}

// buildOpenClawModelsPatch converts ChatClaw providers/models to an OpenClaw config patch.
// Only enabled providers with valid credentials are included.
func (s *configSyncer) buildOpenClawModelsPatch(allProviders []providers.Provider) map[string]any {
	providerMap := make(map[string]any)

	for _, p := range allProviders {
		if !p.Enabled {
			continue
		}
		// Ollama doesn't need API key; others do
		if p.ProviderID != "ollama" && strings.TrimSpace(p.APIKey) == "" {
			continue
		}

		ocProvider := buildSingleProvider(p)
		if ocProvider == nil {
			continue
		}

		// Fetch models for this provider
		pwm, err := s.providersSvc.GetProviderWithModels(p.ProviderID)
		if err == nil {
			for _, group := range pwm.ModelGroups {
				// Only sync LLM models to OpenClaw for now
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

// buildSingleProvider converts a ChatClaw provider to an OpenClaw provider config.
func buildSingleProvider(p providers.Provider) *openclawProviderConfig {
	api, ok := chatclawTypeToOpenClawAPI[p.Type]
	if !ok {
		// Unknown type, try openai-completions as fallback
		api = "openai-completions"
	}

	oc := &openclawProviderConfig{
		API: api,
	}

	// Set baseUrl
	endpoint := strings.TrimSpace(p.APIEndpoint)
	if endpoint != "" {
		// Anthropic client appends /v1 itself, so strip it for anthropic-messages
		if api == "anthropic-messages" {
			endpoint = strings.TrimSuffix(endpoint, "/v1")
		}
		oc.BaseURL = endpoint
	}

	// Set apiKey (skip for Ollama)
	if p.ProviderID != "ollama" {
		oc.APIKey = p.APIKey
	}

	// Azure special handling: inject api_version via baseUrl query param
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
