package openclawruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
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
	manager *Manager

	mu           sync.Mutex
	sections     []sectionEntry
	lastPatchRaw string
}

func NewConfigService(manager *Manager) *ConfigService {
	return &ConfigService{manager: manager}
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
