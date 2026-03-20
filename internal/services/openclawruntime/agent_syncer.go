package openclawruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"chatclaw/internal/define"
	"chatclaw/internal/services/agents"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	agentSyncDebounce       = 1 * time.Second
	agentSyncReadyStability = 5 * time.Second
	agentSyncRetryBackoff   = 3 * time.Second
	agentSyncWriteWindow    = 60 * time.Second
	agentSyncWriteLimit     = 3
)

var retryAfterPattern = regexp.MustCompile(`retryAfterMs[^0-9]*(\d+)`)

type AgentSyncer struct {
	app       *application.App
	manager   *Manager
	agentsSvc *agents.AgentsService

	wakeCh chan struct{}
	stopCh chan struct{}

	mu               sync.Mutex
	generation       uint64
	syncedGeneration uint64
	lastDirtyAt      time.Time
	writeAttempts    []time.Time
	closed           bool
}

type configGetResult struct {
	Config map[string]any `json:"config"`
	Hash   string         `json:"hash"`
}

func NewAgentSyncer(app *application.App, manager *Manager, agentsSvc *agents.AgentsService) *AgentSyncer {
	s := &AgentSyncer{
		app:       app,
		manager:   manager,
		agentsSvc: agentsSvc,
		wakeCh:    make(chan struct{}, 1),
		stopCh:    make(chan struct{}),
	}
	if manager != nil {
		manager.registerReadyHook(s.MarkDirty)
	}
	go s.loop()
	return s
}

func (s *AgentSyncer) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	close(s.stopCh)
	s.mu.Unlock()
}

func (s *AgentSyncer) MarkDirty() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.generation++
	s.lastDirtyAt = time.Now()
	s.mu.Unlock()

	select {
	case s.wakeCh <- struct{}{}:
	default:
	}
}

func (s *AgentSyncer) loop() {
	for {
		select {
		case <-s.stopCh:
			return
		case <-s.wakeCh:
		}

		for {
			gen, synced, lastDirty, ok := s.snapshot()
			if !ok || gen == 0 || gen == synced {
				break
			}
			if s.manager == nil || !s.manager.isReady() {
				break
			}

			if wait := time.Until(lastDirty.Add(agentSyncDebounce)); wait > 0 {
				if !s.sleep(wait) {
					return
				}
				continue
			}

			readyAt := s.manager.readySince()
			if readyAt.IsZero() {
				break
			}
			if wait := time.Until(readyAt.Add(agentSyncReadyStability)); wait > 0 {
				if !s.sleep(wait) {
					return
				}
				continue
			}

			if wait := s.writeWindowWait(); wait > 0 {
				if !s.sleep(wait) {
					return
				}
				continue
			}

			if err := s.syncOnce(gen); err != nil {
				s.logWarn("openclaw agent sync failed", "error", err)
				if !s.sleep(retryDelay(err)) {
					return
				}
				continue
			}
		}
	}
}

func (s *AgentSyncer) syncOnce(gen uint64) error {
	if s.agentsSvc == nil {
		return nil
	}
	if err := s.agentsSvc.EnsureMainAgent(); err != nil {
		return err
	}

	agentsList, err := s.agentsSvc.ListAgentsForOpenClawSync()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var cfg configGetResult
	if err := s.manager.request(ctx, "config.get", map[string]any{}, &cfg); err != nil {
		return err
	}

	existingList := extractAgentList(cfg.Config)
	desiredList := append(buildManagedAgents(agentsList), filterUnmanagedAgents(existingList)...)
	if agentListsEqual(normalizeForCompare(existingList), normalizeForCompare(desiredList)) {
		s.markSynced(gen)
		return nil
	}

	patchRaw, err := json.Marshal(map[string]any{
		"agents": map[string]any{
			"list": desiredList,
		},
	})
	if err != nil {
		return fmt.Errorf("marshal agent config patch: %w", err)
	}

	s.recordWriteAttempt()
	var patchResp map[string]any
	if err := s.manager.request(ctx, "config.patch", map[string]any{
		"raw":      string(patchRaw),
		"baseHash": cfg.Hash,
	}, &patchResp); err != nil {
		return err
	}

	s.markSynced(gen)
	return nil
}

func extractAgentList(cfg map[string]any) []any {
	if len(cfg) == 0 {
		return nil
	}
	agentsCfg, _ := cfg["agents"].(map[string]any)
	if len(agentsCfg) == 0 {
		return nil
	}
	list, _ := agentsCfg["list"].([]any)
	return list
}

func buildManagedAgents(list []agents.Agent) []any {
	out := make([]any, 0, len(list))
	for _, agent := range list {
		entry := map[string]any{
			"id":   agent.OpenClawAgentID,
			"name": agent.Name,
			"identity": map[string]any{
				"name": agent.Name,
			},
		}
		if agent.OpenClawAgentID == define.OpenClawMainAgentID {
			entry["default"] = true
		}
		out = append(out, entry)
	}
	return out
}

func filterUnmanagedAgents(existing []any) []any {
	out := make([]any, 0, len(existing))
	for _, item := range existing {
		obj, ok := item.(map[string]any)
		if !ok {
			out = append(out, item)
			continue
		}
		id, _ := obj["id"].(string)
		if isManagedAgentID(strings.TrimSpace(id)) {
			continue
		}
		out = append(out, item)
	}
	return out
}

func isManagedAgentID(id string) bool {
	return id == define.OpenClawMainAgentID || strings.HasPrefix(id, define.OpenClawManagedAgentIDPrefix)
}

// normalizeForCompare strips Gateway-added fields from managed agents so that
// comparison only considers the fields ChatClaw actually writes (id, name,
// identity, default). Unmanaged agents are kept as-is.
func normalizeForCompare(list []any) []any {
	out := make([]any, 0, len(list))
	for _, item := range list {
		obj, ok := item.(map[string]any)
		if !ok {
			out = append(out, item)
			continue
		}
		id, _ := obj["id"].(string)
		if !isManagedAgentID(strings.TrimSpace(id)) {
			out = append(out, item)
			continue
		}
		normalized := map[string]any{"id": id, "name": obj["name"]}
		if identity, ok := obj["identity"]; ok {
			if idMap, ok := identity.(map[string]any); ok {
				normalized["identity"] = map[string]any{"name": idMap["name"]}
			}
		}
		if def, ok := obj["default"]; ok {
			normalized["default"] = def
		}
		out = append(out, normalized)
	}
	return out
}

func agentListsEqual(a, b []any) bool {
	left, err := json.Marshal(a)
	if err != nil {
		return false
	}
	right, err := json.Marshal(b)
	if err != nil {
		return false
	}
	return bytes.Equal(left, right)
}

func (s *AgentSyncer) snapshot() (uint64, uint64, time.Time, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.generation, s.syncedGeneration, s.lastDirtyAt, !s.closed
}

func (s *AgentSyncer) markSynced(gen uint64) {
	s.mu.Lock()
	if gen > s.syncedGeneration {
		s.syncedGeneration = gen
	}
	s.mu.Unlock()
}

func (s *AgentSyncer) writeWindowWait() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.writeAttempts) == 0 {
		return 0
	}
	cutoff := time.Now().Add(-agentSyncWriteWindow)
	trimmed := s.writeAttempts[:0]
	for _, ts := range s.writeAttempts {
		if ts.After(cutoff) {
			trimmed = append(trimmed, ts)
		}
	}
	s.writeAttempts = trimmed
	if len(s.writeAttempts) < agentSyncWriteLimit {
		return 0
	}
	return time.Until(s.writeAttempts[0].Add(agentSyncWriteWindow))
}

func (s *AgentSyncer) recordWriteAttempt() {
	s.mu.Lock()
	s.writeAttempts = append(s.writeAttempts, time.Now())
	s.mu.Unlock()
}

func (s *AgentSyncer) sleep(d time.Duration) bool {
	if d <= 0 {
		return true
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-s.stopCh:
		return false
	case <-timer.C:
		return true
	case <-s.wakeCh:
		return true
	}
}

func (s *AgentSyncer) logWarn(msg string, args ...any) {
	if s.app != nil {
		s.app.Logger.Warn(msg, args...)
	}
}

func retryDelay(err error) time.Duration {
	var reqErr *GatewayRequestError
	if errors.As(err, &reqErr) {
		if strings.EqualFold(reqErr.Code, "UNAVAILABLE") {
			if m := retryAfterPattern.FindStringSubmatch(reqErr.Message); len(m) == 2 {
				if ms, convErr := time.ParseDuration(m[1] + "ms"); convErr == nil && ms > 0 {
					return ms
				}
			}
		}
	}
	return agentSyncRetryBackoff
}
