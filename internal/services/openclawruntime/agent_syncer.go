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
	DefaultID string            `json:"defaultId"`
	Agents    []agentsListEntry `json:"agents"`
}

type agentsListEntry struct {
	ID       string              `json:"id"`
	Name     string              `json:"name"`
	Identity *agentsListIdentity `json:"identity,omitempty"`
}

type agentsListIdentity struct {
	Name string `json:"name,omitempty"`
}

// configGetResult holds the response from config.get for optimistic locking.
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
			// agents.create initialises workspace dirs, agentDir, sessions
			// store, and AGENTS.md/SOUL.md scaffolding that config.patch alone
			// does not provide.
			if err := s.createAgentDirs(ctx, agent); err != nil {
				return fmt.Errorf("agents.create %s: %w", agentID, err)
			}
			// Atomically set the correct name/identity/workspace/agentDir via
			// config.patch (replaces the placeholder name written by
			// agents.create with the real display name).
			if err := s.upsertAgentConfig(ctx, agent); err != nil {
				return fmt.Errorf("config.patch upsert %s: %w", agentID, err)
			}
			changed = true
			continue
		}

		if needsUpdate(existing, agent) {
			if err := s.upsertAgentConfig(ctx, agent); err != nil {
				return fmt.Errorf("config.patch update %s: %w", agentID, err)
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

// createAgentDirs calls agents.create to initialise workspace dirs, agentDir,
// session store, and scaffold files (AGENTS.md, SOUL.md, etc.).  The config
// entry written by this call uses openclaw_agent_id as both name and agentId;
// the caller must follow up with upsertAgentConfig to set the real name and
// identity atomically.
func (s *AgentSyncer) createAgentDirs(ctx context.Context, agent openclawagents.OpenClawAgent) error {
	var resp map[string]any
	return s.manager.request(ctx, "agents.create", map[string]any{
		"name":      agent.OpenClawAgentID,
		"workspace": s.resolveAgentWorkspace(agent),
	}, &resp)
}

// upsertAgentConfig does a single config.patch to set (or overwrite) the
// agent's name, identity.name, workspace and agentDir in agents.list.
// For the main agent it also sets default:true.  This is atomic — there is
// no intermediate state where name/identity shows a random string.
func (s *AgentSyncer) upsertAgentConfig(ctx context.Context, agent openclawagents.OpenClawAgent) error {
	var cfg configGetResult
	if err := s.manager.request(ctx, "config.get", map[string]any{}, &cfg); err != nil {
		return fmt.Errorf("config.get: %w", err)
	}

	existingList := extractAgentList(cfg.Config)

	entry := map[string]any{
		"id":        agent.OpenClawAgentID,
		"name":      agent.Name,
		"workspace": s.resolveAgentWorkspace(agent),
		"agentDir":  s.resolveAgentDir(agent),
		"identity":  map[string]any{"name": agent.Name},
	}
	if agent.OpenClawAgentID == define.OpenClawMainAgentID {
		entry["default"] = true
	}

	newList := make([]any, 0, len(existingList)+1)
	found := false
	for _, item := range existingList {
		obj, ok := item.(map[string]any)
		if !ok {
			newList = append(newList, item)
			continue
		}
		id, _ := obj["id"].(string)
		if strings.EqualFold(id, agent.OpenClawAgentID) {
			newList = append(newList, entry)
			found = true
		} else {
			newList = append(newList, item)
		}
	}
	if !found {
		newList = append(newList, entry)
	}

	patchRaw, err := json.Marshal(map[string]any{
		"agents": map[string]any{"list": newList},
	})
	if err != nil {
		return fmt.Errorf("marshal config patch: %w", err)
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
