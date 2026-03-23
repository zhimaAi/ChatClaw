package openclawruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"chatclaw/internal/define"
	"chatclaw/internal/services/openclawagents"

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
	agentsSvc *openclawagents.OpenClawAgentsService

	wakeCh chan struct{}
	stopCh chan struct{}

	mu               sync.Mutex
	generation       uint64
	syncedGeneration uint64
	lastDirtyAt      time.Time
	writeAttempts    []time.Time
	closed           bool
}

// agentsListResult mirrors the Gateway agents.list response.
type agentsListResult struct {
	DefaultID string              `json:"defaultId"`
	Agents    []agentsListEntry   `json:"agents"`
}

type agentsListEntry struct {
	ID       string               `json:"id"`
	Name     string               `json:"name"`
	Identity *agentsListIdentity  `json:"identity,omitempty"`
}

type agentsListIdentity struct {
	Name string `json:"name,omitempty"`
}

// configGetResult is used only for main-agent config.patch fallback.
type configGetResult struct {
	Config map[string]any `json:"config"`
	Hash   string         `json:"hash"`
}

func NewAgentSyncer(app *application.App, manager *Manager, agentsSvc *openclawagents.OpenClawAgentsService) *AgentSyncer {
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

	desiredAgents, err := s.agentsSvc.ListAgentsForOpenClawSync()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Fetch current Gateway agent list via agents.list RPC
	var gatewayList agentsListResult
	if err := s.manager.request(ctx, "agents.list", map[string]any{}, &gatewayList); err != nil {
		return fmt.Errorf("agents.list: %w", err)
	}

	// Build lookup maps (keys are lowercased to match Gateway's normalizeAgentId)
	gatewayMap := make(map[string]agentsListEntry, len(gatewayList.Agents))
	for _, entry := range gatewayList.Agents {
		gatewayMap[strings.ToLower(entry.ID)] = entry
	}

	desiredMap := make(map[string]openclawagents.OpenClawAgent, len(desiredAgents))
	for _, a := range desiredAgents {
		desiredMap[strings.ToLower(a.OpenClawAgentID)] = a
	}

	changed := false

	// --- Phase 1: Create or update agents that should exist ---
	for _, agent := range desiredAgents {
		agentID := agent.OpenClawAgentID
		normalizedID := strings.ToLower(agentID)

		existing, existsInGateway := gatewayMap[normalizedID]
		if !existsInGateway {
			if agentID == define.OpenClawMainAgentID {
				// "main" cannot be created via agents.create; use config.patch
				if err := s.ensureMainAgentViaConfigPatch(ctx, agent); err != nil {
					return fmt.Errorf("ensure main agent: %w", err)
				}
			} else {
				if err := s.createAgent(ctx, agent); err != nil {
					return fmt.Errorf("agents.create %s: %w", agentID, err)
				}
			}
			changed = true
			continue
		}

		// Agent exists; check if update is needed
		if needsUpdate(existing, agent) {
			if agentID == define.OpenClawMainAgentID {
				if err := s.ensureMainAgentViaConfigPatch(ctx, agent); err != nil {
					return fmt.Errorf("update main agent: %w", err)
				}
			} else {
				if err := s.updateAgent(ctx, agent); err != nil {
					return fmt.Errorf("agents.update %s: %w", agentID, err)
				}
			}
			changed = true
		}
	}

	// --- Phase 2: Delete managed agents that no longer exist in ChatClaw ---
	for _, entry := range gatewayList.Agents {
		normalizedEntryID := strings.ToLower(entry.ID)
		if !isManagedAgentID(normalizedEntryID) {
			continue
		}
		if _, wanted := desiredMap[normalizedEntryID]; wanted {
			continue
		}
		if entry.ID == define.OpenClawMainAgentID {
			// Never delete the main agent
			continue
		}
		if err := s.deleteAgent(ctx, entry.ID); err != nil {
			return fmt.Errorf("agents.delete %s: %w", entry.ID, err)
		}
		changed = true
	}

	if changed {
		s.recordWriteAttempt()
	}
	s.markSynced(gen)
	return nil
}

// createAgent calls agents.create to set up workspace/agentDir/sessions,
// then patchAgentIdentity to set the real display name and identity
// (agents.create derives agentId from the name param, so we pass the
// openclaw_agent_id as name; identity is not part of the create/update schema).
func (s *AgentSyncer) createAgent(ctx context.Context, agent openclawagents.OpenClawAgent) error {
	workspace := s.resolveAgentWorkspace(agent)

	var createResp map[string]any
	if err := s.manager.request(ctx, "agents.create", map[string]any{
		"name":      agent.OpenClawAgentID,
		"workspace": workspace,
	}, &createResp); err != nil {
		return err
	}

	return s.patchAgentIdentity(ctx, agent)
}

// resolveAgentWorkspace returns the workspace directory for an agent.
// Each agent gets its own isolated workspace under the state dir, following
// OpenClaw's convention: $STATE_DIR/workspace-{agentId}.
func (s *AgentSyncer) resolveAgentWorkspace(agent openclawagents.OpenClawAgent) string {
	return filepath.Join(s.stateDir(), "workspace-"+agent.OpenClawAgentID)
}

// resolveAgentDir returns the agent config directory, following
// OpenClaw's convention: $STATE_DIR/agents/{agentId}/agent.
func (s *AgentSyncer) resolveAgentDir(agent openclawagents.OpenClawAgent) string {
	return filepath.Join(s.stateDir(), "agents", agent.OpenClawAgentID, "agent")
}

func (s *AgentSyncer) stateDir() string {
	dir := s.agentsSvc.GetDefaultWorkDir()
	if dir == "" {
		return "."
	}
	return dir
}

func (s *AgentSyncer) updateAgent(ctx context.Context, agent openclawagents.OpenClawAgent) error {
	return s.patchAgentIdentity(ctx, agent)
}

// patchAgentIdentity updates the name and identity.name for a non-main agent
// via config.patch, since agents.update schema does not include identity.
func (s *AgentSyncer) patchAgentIdentity(ctx context.Context, agent openclawagents.OpenClawAgent) error {
	var cfg configGetResult
	if err := s.manager.request(ctx, "config.get", map[string]any{}, &cfg); err != nil {
		return fmt.Errorf("config.get for identity patch: %w", err)
	}

	existingList := extractAgentList(cfg.Config)
	newList := make([]any, 0, len(existingList))
	found := false
	for _, item := range existingList {
		obj, ok := item.(map[string]any)
		if !ok {
			newList = append(newList, item)
			continue
		}
		id, _ := obj["id"].(string)
		if id == agent.OpenClawAgentID {
			found = true
			obj["name"] = agent.Name
			obj["identity"] = map[string]any{"name": agent.Name}
			newList = append(newList, obj)
		} else {
			newList = append(newList, item)
		}
	}
	if !found {
		return fmt.Errorf("agent %s not found in config after create", agent.OpenClawAgentID)
	}

	patchRaw, err := json.Marshal(map[string]any{
		"agents": map[string]any{"list": newList},
	})
	if err != nil {
		return fmt.Errorf("marshal identity patch: %w", err)
	}

	return s.manager.request(ctx, "config.patch", map[string]any{
		"raw":      string(patchRaw),
		"baseHash": cfg.Hash,
	}, nil)
}

func (s *AgentSyncer) deleteAgent(ctx context.Context, agentID string) error {
	return s.manager.request(ctx, "agents.delete", map[string]any{
		"agentId":     agentID,
		"deleteFiles": false,
	}, nil)
}

// ensureMainAgentViaConfigPatch handles the "main" agent which cannot be
// created/deleted via the agents.create/delete RPCs (they forbid "main").
// Falls back to config.patch to insert it into agents.list.
func (s *AgentSyncer) ensureMainAgentViaConfigPatch(ctx context.Context, agent openclawagents.OpenClawAgent) error {
	var cfg configGetResult
	if err := s.manager.request(ctx, "config.get", map[string]any{}, &cfg); err != nil {
		return err
	}

	existingList := extractAgentList(cfg.Config)

	mainEntry := map[string]any{
		"id":        define.OpenClawMainAgentID,
		"name":      agent.Name,
		"default":   true,
		"workspace": s.resolveAgentWorkspace(agent),
		"agentDir":  s.resolveAgentDir(agent),
		"identity": map[string]any{
			"name": agent.Name,
		},
	}

	// Prepend the main entry, keep everything else
	newList := []any{mainEntry}
	for _, item := range existingList {
		obj, ok := item.(map[string]any)
		if !ok {
			newList = append(newList, item)
			continue
		}
		id, _ := obj["id"].(string)
		if id == define.OpenClawMainAgentID {
			continue
		}
		newList = append(newList, item)
	}

	patchRaw, err := json.Marshal(map[string]any{
		"agents": map[string]any{
			"list": newList,
		},
	})
	if err != nil {
		return fmt.Errorf("marshal main agent config patch: %w", err)
	}

	return s.manager.request(ctx, "config.patch", map[string]any{
		"raw":      string(patchRaw),
		"baseHash": cfg.Hash,
	}, nil)
}

// needsUpdate compares the Gateway entry against the desired ChatClaw agent state.
func needsUpdate(existing agentsListEntry, desired openclawagents.OpenClawAgent) bool {
	existingName := existing.Name
	if existing.Identity != nil && existing.Identity.Name != "" {
		existingName = existing.Identity.Name
	}
	return existingName != desired.Name
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

func isManagedAgentID(id string) bool {
	if define.OpenClawManagedAgentIDPrefix == "" {
		return true
	}
	return id == define.OpenClawMainAgentID || strings.HasPrefix(id, define.OpenClawManagedAgentIDPrefix)
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
