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

// SectionBuilder produces a partial config map for one top-level section.
// Example return: {"models": {"mode": "replace", "providers": {...}}}
type SectionBuilder func(ctx context.Context) (map[string]any, error)

type sectionEntry struct {
	name    string
	builder SectionBuilder
}

// ConfigService manages all config sections and provides a unified
// config.get + merge + config.patch flow to the OpenClaw Gateway.
type ConfigService struct {
	manager      *Manager
	providersSvc ProvidersSvcProvider // set via SetProvidersService

	mu           sync.Mutex
	sections     []sectionEntry
	lastPatchRaw string
}

// ProvidersSvcProvider abstracts the subset of *providers.ProvidersService needed
// by ConfigService, avoiding a direct import that would cause a circular dependency.
type ProvidersSvcProvider interface {
	GetProviderWithModels(providerID string) (*providers.ProviderWithModels, error)
	ListProviders() ([]providers.Provider, error)
}

func NewConfigService(manager *Manager) *ConfigService {
	return &ConfigService{manager: manager}
}

// SetProvidersService injects the ProvidersService for per-model registration.
func (s *ConfigService) SetProvidersService(svc ProvidersSvcProvider) {
	s.providersSvc = svc
}

// ResponsesEndpointSection enables gateway HTTP OpenResponses via config.patch only.
// Do not use `openclaw config set` before gateway start: that writes openclaw.json once,
// then the process applies --auth/--token and persists again, then ChatClaw sends config.patch —
// multiple competing writes make the file watcher see gateway.auth/tailscale/meta churn and
// hybrid reload restarts the gateway in a loop.
func ResponsesEndpointSection() SectionBuilder {
	return func(ctx context.Context) (map[string]any, error) {
		_ = ctx
		return map[string]any{
			"gateway": map[string]any{
				"http": map[string]any{
					"endpoints": map[string]any{
						"responses": map[string]any{"enabled": true},
					},
				},
			},
		}, nil
	}
}

// Register adds a named section builder. The name is for logging only;
// the builder's returned map keys determine the actual config sections.
func (s *ConfigService) Register(name string, builder SectionBuilder) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sections = append(s.sections, sectionEntry{name: name, builder: builder})
}

// Sync collects all sections, merges them into one patch, and pushes to Gateway.
// Calls are serialised via mutex to avoid baseHash races. If the merged patch
// is identical to the last successful push, the call is a no-op.
func (s *ConfigService) Sync(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.manager.IsReady() {
		return fmt.Errorf("gateway not ready")
	}

	merged := make(map[string]any)
	for _, sec := range s.sections {
		partial, err := sec.builder(ctx)
		if err != nil {
			s.log("openclaw: section builder %q failed: %v", sec.name, err)
			continue
		}
		for k, v := range partial {
			merged[k] = v
		}
	}

	raw, err := json.Marshal(merged)
	if err != nil {
		return fmt.Errorf("marshal config patch: %w", err)
	}
	rawStr := string(raw)

	if rawStr == s.lastPatchRaw {
		return nil
	}

	syncCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	preview := rawStr
	if len(preview) > 512 {
		preview = preview[:512] + "...(truncated)"
	}
	s.log("openclaw: Sync pushing config len=%d preview=%s", len(rawStr), preview)

	var getResult struct {
		Hash string `json:"hash"`
	}
	if err := s.manager.Request(syncCtx, "config.get", map[string]any{}, &getResult); err != nil {
		return fmt.Errorf("config.get: %w", err)
	}

	if err := s.manager.Request(syncCtx, "config.patch", map[string]any{
		"raw":      rawStr,
		"baseHash": getResult.Hash,
	}, nil); err != nil {
		// Patch failed; do NOT update lastPatchRaw so next call can retry.
		return fmt.Errorf("config.patch: %w", err)
	}

	s.lastPatchRaw = rawStr
	s.log("openclaw: config sync completed")
	return nil
}

func (s *ConfigService) log(format string, args ...any) {
	if s.manager != nil && s.manager.app != nil {
		s.manager.app.Logger.Info(fmt.Sprintf(format, args...))
	}
}

// EnsureModelRegistered checks whether the given provider/model exists under
// config.models.providers on the Gateway (supports both top-level models and nested config.models).
// If missing, it pushes the same models section as full sync: {"models":{"mode":"replace","providers":{...}}}
// so the payload matches OpenClaw's schema (a bare {"models":{"chatwiki":...}} is ignored by the gateway).
func (s *ConfigService) EnsureModelRegistered(ctx context.Context, providerID, modelID string) error {
	providerID = strings.TrimSpace(providerID)
	modelID = strings.TrimSpace(modelID)
	if providerID == "" || modelID == "" {
		return nil
	}

	if !s.manager.IsReady() {
		return fmt.Errorf("gateway not ready")
	}

	ps, ok := s.providersSvc.(*providers.ProvidersService)
	if !ok || ps == nil {
		return fmt.Errorf("providers service not available for model sync")
	}

	// Force-refresh ChatWiki catalog so GetProviderWithModels sees latest data.
	// Without this, the in-memory cache may hold stale data that won't be in DB.
	ClearChatWikiSyncCache()

	syncCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	// Verify model exists in ChatClaw catalog (with refreshed cache).
	pwm, err := ps.GetProviderWithModels(providerID)
	if err != nil {
		return fmt.Errorf("get provider with models: %w", err)
	}
	modelFound := false
	for _, group := range pwm.ModelGroups {
		for _, m := range group.Models {
			if m.ModelID == modelID && m.Enabled {
				modelFound = true
				break
			}
		}
		if modelFound {
			break
		}
	}
	if !modelFound {
		return fmt.Errorf("model %s/%s not in ChatClaw catalog or disabled", providerID, modelID)
	}

	var live map[string]any
	if err := s.manager.Request(syncCtx, "config.get", map[string]any{}, &live); err != nil {
		return fmt.Errorf("config.get: %w", err)
	}

	hash := configGetHashFromResponse(live)
	liveProvs := extractModelsProvidersFromGatewayGet(live)
	if gatewayProviderHasModel(liveProvs, providerID, modelID) {
		s.log("openclaw: model already on gateway, skip models patch, provider=%s model=%s", providerID, modelID)
		return nil
	}

	s.log("openclaw: model missing on gateway, pushing full models section, provider=%s model=%s", providerID, modelID)

	// Build with fresh ChatWiki data (cache was cleared above).
	modelsSection, err := BuildModelsSectionPatch(ps)
	if err != nil {
		return fmt.Errorf("build models section: %w", err)
	}

	raw, err := json.Marshal(map[string]any{"models": modelsSection})
	if err != nil {
		return fmt.Errorf("marshal models patch: %w", err)
	}

	preview := string(raw)
	if len(preview) > 512 {
		preview = preview[:512] + "...(truncated)"
	}
	s.log("openclaw: models patch JSON len=%d preview=%s", len(raw), preview)

	if strings.TrimSpace(hash) == "" {
		return fmt.Errorf("config.get returned empty hash")
	}
	if err := s.manager.Request(syncCtx, "config.patch", map[string]any{
		"raw":      string(raw),
		"baseHash": hash,
	}, nil); err != nil {
		return fmt.Errorf("config.patch models: %w", err)
	}

	s.log("openclaw: models section patched for gateway, provider=%s model=%s", providerID, modelID)
	return nil
}

func configGetHashFromResponse(root map[string]any) string {
	if root == nil {
		return ""
	}
	if h, ok := root["hash"].(string); ok && strings.TrimSpace(h) != "" {
		return h
	}
	if inner, ok := root["result"].(map[string]any); ok {
		if h, ok := inner["hash"].(string); ok && strings.TrimSpace(h) != "" {
			return h
		}
	}
	if inner, ok := root["data"].(map[string]any); ok {
		if h, ok := inner["hash"].(string); ok && strings.TrimSpace(h) != "" {
			return h
		}
	}
	return ""
}

func extractModelsProvidersFromGatewayGet(root map[string]any) map[string]any {
	if root == nil {
		return nil
	}
	if inner, ok := root["result"].(map[string]any); ok {
		if p := extractModelsProvidersFromGatewayGet(inner); p != nil {
			return p
		}
	}
	if inner, ok := root["data"].(map[string]any); ok {
		if p := extractModelsProvidersFromGatewayGet(inner); p != nil {
			return p
		}
	}
	if cfg, ok := root["config"].(map[string]any); ok {
		if models, ok := cfg["models"].(map[string]any); ok {
			if p, ok := models["providers"].(map[string]any); ok {
				return p
			}
		}
	}
	if models, ok := root["models"].(map[string]any); ok {
		if p, ok := models["providers"].(map[string]any); ok {
			return p
		}
	}
	return nil
}

func gatewayProviderHasModel(providers map[string]any, providerID, modelID string) bool {
	if providers == nil {
		return false
	}
	pv, ok := providers[providerID]
	if !ok {
		return false
	}
	pm, ok := pv.(map[string]any)
	if !ok {
		return false
	}
	arr, ok := pm["models"].([]any)
	if !ok {
		return false
	}
	for _, it := range arr {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		id, _ := m["id"].(string)
		if id == modelID {
			return true
		}
	}
	return false
}
