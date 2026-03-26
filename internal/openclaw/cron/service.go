package openclawcron

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	openclawroot "chatclaw/internal/openclaw"
	openclawagents "chatclaw/internal/openclaw/agents"
	openclawruntime "chatclaw/internal/openclaw/runtime"
	chatservice "chatclaw/internal/services/chat"
	"chatclaw/internal/services/conversations"
	"chatclaw/internal/sqlite"

	"github.com/uptrace/bun"
	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	// defaultOpenClawCronListLimit keeps UI list pagination stable with OpenClaw defaults.
	// defaultOpenClawCronListLimit 与 OpenClaw 默认分页保持一致，避免前后端理解不一致。
	defaultOpenClawCronListLimit = 50
	// conversationHistoryMatchWindowMs keeps local conversation history aligned with
	// transcript/run metadata written a few seconds later by OpenClaw.
	// conversationHistoryMatchWindowMs 用于把本地会话历史与稍后写入的 transcript/run 元数据对齐。
	conversationHistoryMatchWindowMs = int64(30 * time.Second / time.Millisecond)
	// globalCronTrackerListenerKey keeps the background run tracker singleton stable.
	globalCronTrackerListenerKey = "openclaw-cron-global-tracker"
	// openClawCronGatewayTimeout keeps gateway cron RPCs bounded.
	openClawCronGatewayTimeout = 60 * time.Second

	openClawCronMethodAdd    = "cron.add"
	openClawCronMethodUpdate = "cron.update"
	openClawCronMethodRemove = "cron.remove"
	openClawCronMethodRun    = "cron.run"
	openClawCronMethodRuns   = "cron.runs"
	openClawCronMethodStatus = "cron.status"

	openClawCronScheduleAt    = "at"
	openClawCronScheduleEvery = "every"
	openClawCronScheduleCron  = "cron"

	openClawCronPayloadAgentTurn   = "agentTurn"
	openClawCronPayloadSystemEvent = "systemEvent"

	openClawCronSessionMain     = "main"
	openClawCronSessionIsolated = "isolated"
	openClawCronWakeNow         = "now"
	openClawCronRunModeForce    = "force"

	openClawCronDeliveryModeNone     = "none"
	openClawCronDeliveryModeAnnounce = "announce"
	openClawCronDeliveryModeWebhook  = "webhook"
	// openClawCronDeliveryTargetModeLastActive resolves delivery.to from channels.last_sender_id.
	openClawCronDeliveryTargetModeLastActive = "last_active"
	// openClawCronDeliveryTargetModeTargetID sends to the user-provided target directly.
	openClawCronDeliveryTargetModeTargetID = "target_id"
	// openClawCronChannelPlatformFeishu keeps OpenClaw account-key mapping explicit.
	openClawCronChannelPlatformFeishu = "feishu"

	// openClawCronHistoryStatusRunning is the temporary UI state before a durable
	// run log or local manual-run completion proves a terminal result.
	openClawCronHistoryStatusRunning = "running"
	// openClawCronHistoryStatusSuccess is used for manual chat runs after the
	// local conversation has finished and no durable run-log row exists.
	openClawCronHistoryStatusSuccess = "success"
	// openClawCronChannelVisibilitySQL matches the OpenClaw channel list visibility rule.
	openClawCronChannelVisibilitySQL = `(ch.openclaw_scope = 1 OR (ch.agent_id > 0 AND EXISTS (SELECT 1 FROM openclaw_agents AS oa WHERE oa.id = ch.agent_id) AND NOT EXISTS (SELECT 1 FROM agents AS a WHERE a.id = ch.agent_id)))`
)

// OpenClawCronService wraps OpenClaw-native cron management for the Wails frontend.
// OpenClawCronService 为前端提供独立于 ChatClaw 的 OpenClaw Cron 管理能力。
type OpenClawCronService struct {
	app       *application.App
	manager   *openclawruntime.Manager
	agentsSvc *openclawagents.OpenClawAgentsService
	convSvc   *conversations.ConversationsService
	chatSvc   *chatservice.ChatService
	watchMu   sync.Mutex
	watchSeq  atomic.Int64
	watches   map[string]string
	convMu    sync.Mutex
	forwardMu sync.Mutex
	forwards  map[string]*cronForwardState
}

// cronForwardState tracks the synthetic forwarded chat stream for one cron session.
// cronForwardState 记录单个 Cron 会话转发到统一聊天流时的合成状态。
type cronForwardState struct {
	RequestID string
	MessageID int64
	Seq       int
	Started   bool
	Finished  bool
}

// cronForwardedEvent couples a frontend event name with its typed payload.
// cronForwardedEvent 把前端事件名和对应的事件体组合在一起，便于统一发射。
type cronForwardedEvent struct {
	Name    string
	Payload any
}

type cronConversationRecord struct {
	ID                 int64  `bun:"id"`
	AgentID            int64  `bun:"agent_id"`
	OpenClawSessionKey string `bun:"openclaw_session_key"`
}

type manualRunConversationRow struct {
	ID                 int64     `bun:"id"`
	AgentID            int64     `bun:"agent_id"`
	ExternalID         string    `bun:"external_id"`
	LastMessage        string    `bun:"last_message"`
	OpenClawSessionKey string    `bun:"openclaw_session_key"`
	CreatedAt          time.Time `bun:"created_at"`
	UpdatedAt          time.Time `bun:"updated_at"`
	OpenClawAgentID    string    `bun:"openclaw_agent_id"`
}

type manualConversationHistoryState struct {
	Status     string
	DurationMs int64
}

type cronDeliveryChannelOption struct {
	ID           int64  `bun:"id"`
	AgentRowID   int64  `bun:"agent_id"`
	Platform     string `bun:"platform"`
	LastSenderID string `bun:"last_sender_id"`
	ExtraConfig  string `bun:"extra_config"`
}

type cronDeliverySelectionInput struct {
	OpenClawAgentID string
	Platform        string
	TargetID        string
}

func NewOpenClawCronService(
	app *application.App,
	manager *openclawruntime.Manager,
	agentsSvc *openclawagents.OpenClawAgentsService,
	convSvc *conversations.ConversationsService,
	chatSvc *chatservice.ChatService,
) *OpenClawCronService {
	return &OpenClawCronService{
		app:       app,
		manager:   manager,
		agentsSvc: agentsSvc,
		convSvc:   convSvc,
		chatSvc:   chatSvc,
		watches:   make(map[string]string),
		forwards:  make(map[string]*cronForwardState),
	}
}

// ListAgents returns local OpenClaw agents as form options.
// 返回 OpenClaw 助手下拉选项，供前端弹窗选择。
func (s *OpenClawCronService) ListAgents() ([]OpenClawCronAgentOption, error) {
	items, err := s.agentsSvc.ListAgents()
	if err != nil {
		return nil, err
	}
	out := make([]OpenClawCronAgentOption, 0, len(items))
	for _, item := range items {
		out = append(out, OpenClawCronAgentOption{
			ID:              item.ID,
			Name:            item.Name,
			OpenClawAgentID: item.OpenClawAgentID,
		})
	}
	return out, nil
}

// ListDeliveryPlatforms returns distinct configured OpenClaw channel platforms for cron delivery.
// 返回已配置 OpenClaw 频道的平台列表，供 Cron 表单选择。
func (s *OpenClawCronService) ListDeliveryPlatforms() ([]OpenClawCronDeliveryPlatformOption, error) {
	options, err := s.listDeliveryChannelOptions("")
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{})
	out := make([]OpenClawCronDeliveryPlatformOption, 0, len(options))
	for _, option := range options {
		platform := strings.TrimSpace(option.Platform)
		if platform == "" {
			continue
		}
		if _, exists := seen[platform]; exists {
			continue
		}
		seen[platform] = struct{}{}
		out = append(out, OpenClawCronDeliveryPlatformOption{
			Platform: platform,
			Label:    platform,
		})
	}
	slices.SortFunc(out, func(left, right OpenClawCronDeliveryPlatformOption) int {
		return strings.Compare(left.Platform, right.Platform)
	})
	return out, nil
}

// GetLatestDeliveryTarget returns the most recent target id for the given OpenClaw agent and platform.
// 返回指定 OpenClaw 助手与频道类型对应的最近投递目标 ID。
func (s *OpenClawCronService) GetLatestDeliveryTarget(openClawAgentID string, platform string) (string, error) {
	channel, targetID, accountID, err := s.resolveDeliverySelection(cronDeliverySelectionInput{
		OpenClawAgentID: openClawAgentID,
		Platform:        platform,
		TargetID:        "",
	})
	if err != nil {
		return "", err
	}
	return extractLatestDeliveryTarget(channel, targetID, accountID), nil
}

// ListJobs reads the OpenClaw jobs store directly.
// 直接读取 OpenClaw jobs.json，避免 CLI 对 disabled 任务返回不稳定。
func (s *OpenClawCronService) ListJobs() ([]OpenClawCronJob, error) {
	storePath, err := s.jobsStorePath()
	if err != nil {
		return nil, err
	}
	items, err := s.readJobsFromStore(storePath)
	if err != nil {
		return nil, err
	}
	agentNameMap, _ := s.agentNameMap()
	out := make([]OpenClawCronJob, 0, len(items))
	for _, item := range items {
		job := flattenJob(item)
		if job.AgentID != "" {
			job.AgentName = agentNameMap[job.AgentID]
		}
		out = append(out, job)
	}
	return out, nil
}

// GetSummary computes summary cards from the OpenClaw jobs store.
// 从 OpenClaw 原生任务存储计算页面摘要卡片。
func (s *OpenClawCronService) GetSummary() (*OpenClawCronSummary, error) {
	storePath, err := s.jobsStorePath()
	if err != nil {
		return nil, err
	}
	items, err := s.readJobsFromStore(storePath)
	if err != nil {
		return nil, err
	}
	summary := &OpenClawCronSummary{
		Total:     len(items),
		StorePath: storePath,
	}
	for _, item := range items {
		if item.Enabled {
			summary.Enabled++
		} else {
			summary.Disabled++
		}
		if strings.EqualFold(item.State.LastStatus, "error") || strings.EqualFold(item.State.LastRunStatus, "error") {
			summary.Failed++
		}
	}
	return summary, nil
}

// CreateJob creates a cron job through the OpenClaw Gateway RPC.
// 通过 OpenClaw Gateway RPC 新建 Cron 任务，避免每次拉起 CLI 子进程。
func (s *OpenClawCronService) CreateJob(input CreateOpenClawCronJobInput) (*OpenClawCronJob, error) {
	resolvedInput, err := s.resolveCreateDeliveryInput(input)
	if err != nil {
		return nil, err
	}
	payload, err := buildCreateJobPayload(resolvedInput)
	if err != nil {
		return nil, err
	}
	var raw openClawJobStoreItem
	if err := s.gatewayRequest(openClawCronMethodAdd, payload, &raw); err != nil {
		return nil, err
	}
	job := flattenJob(raw)
	agentNameMap, _ := s.agentNameMap()
	job.AgentName = agentNameMap[job.AgentID]
	return &job, nil
}

// UpdateJob patches a cron job through the OpenClaw Gateway RPC.
// 编辑任务时仅传递被修改的字段，避免用默认值误伤原配置。
func (s *OpenClawCronService) UpdateJob(jobID string, input UpdateOpenClawCronJobInput) (*OpenClawCronJob, error) {
	resolvedInput, err := s.resolveUpdateDeliveryInput(input)
	if err != nil {
		return nil, err
	}
	patch, err := buildUpdateJobPatch(resolvedInput)
	if err != nil {
		return nil, err
	}
	if len(patch) == 0 {
		return s.findJobByID(jobID)
	}
	if err := s.gatewayRequest(openClawCronMethodUpdate, map[string]any{
		"id":    strings.TrimSpace(jobID),
		"patch": patch,
	}, nil); err != nil {
		return nil, err
	}
	return s.findJobByID(jobID)
}

func (s *OpenClawCronService) DeleteJob(jobID string) error {
	return s.gatewayRequest(openClawCronMethodRemove, map[string]any{
		"id": strings.TrimSpace(jobID),
	}, nil)
}

func (s *OpenClawCronService) EnableJob(jobID string) (*OpenClawCronJob, error) {
	if err := s.gatewayRequest(openClawCronMethodUpdate, map[string]any{
		"id": strings.TrimSpace(jobID),
		"patch": map[string]any{
			"enabled": true,
		},
	}, nil); err != nil {
		return nil, err
	}
	return s.findJobByID(jobID)
}

func (s *OpenClawCronService) DisableJob(jobID string) (*OpenClawCronJob, error) {
	if err := s.gatewayRequest(openClawCronMethodUpdate, map[string]any{
		"id": strings.TrimSpace(jobID),
		"patch": map[string]any{
			"enabled": false,
		},
	}, nil); err != nil {
		return nil, err
	}
	return s.findJobByID(jobID)
}

// RunJobNow triggers OpenClaw-native cron execution for the target job.
// RunJobNow 点击“立即运行”时直接触发 OpenClaw 原生 cron run，保持与自动调度同一执行链路。
func (s *OpenClawCronService) RunJobNow(jobID string) (*OpenClawCronRunNowResult, error) {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return nil, fmt.Errorf("openclaw cron id is required")
	}

	job, err := s.findJobByID(jobID)
	if err != nil {
		return nil, err
	}
	triggeredAt := time.Now()
	var payload struct {
		Enqueued bool   `json:"enqueued"`
		Ran      bool   `json:"ran"`
		RunID    string `json:"runId"`
	}
	err = s.gatewayRequest(openClawCronMethodRun, map[string]any{
		"id":   jobID,
		"mode": openClawCronRunModeForce,
	}, &payload)
	if err != nil {
		return nil, fmt.Errorf("openclaw cron run %s failed: %w", jobID, err)
	}
	result := &OpenClawCronRunNowResult{
		RunID:       strings.TrimSpace(payload.RunID),
		TriggerAtMs: triggeredAt.UnixMilli(),
		Enqueued:    payload.Enqueued || payload.Ran,
	}
	if result.RunID == "" {
		return result, nil
	}

	if fixedSessionKey := strings.TrimSpace(job.SessionKey); fixedSessionKey != "" {
		if _, _, ensureErr := s.ensureConversationForSession(jobID, *job, fixedSessionKey, result.RunID, result.TriggerAtMs); ensureErr != nil {
			s.app.Logger.Warn("[openclaw-cron] ensure fixed-session conversation failed", "job_id", jobID, "run_id", result.RunID, "error", ensureErr)
		}
		return result, nil
	}

	return result, nil
}

// ListRuns reads job run history via OpenClaw CLI first, then falls back to the JSONL log.
// 先走 OpenClaw CLI 读取历史，必要时回退到本地 JSONL，保证页面始终可见历史记录。
func (s *OpenClawCronService) ListRuns(jobID string, limit int) ([]OpenClawCronRunEntry, error) {
	if limit <= 0 {
		limit = defaultOpenClawCronListLimit
	}
	merged := make([]OpenClawCronRunEntry, 0)
	var result struct {
		Entries []openClawRunEntryCLI `json:"entries"`
	}
	err := s.gatewayQuery(openClawCronMethodRuns, map[string]any{
		"id":    strings.TrimSpace(jobID),
		"limit": limit,
	}, &result)
	if err == nil {
		for _, item := range result.Entries {
			merged = append(merged, item.toDTO())
		}
	} else {
		fileEntries, fileErr := s.readRunEntriesFromFile(jobID, limit)
		if fileErr != nil {
			return nil, fileErr
		}
		merged = append(merged, fileEntries...)
	}
	sortOpenClawRunsDesc(merged)
	if len(merged) > limit {
		merged = merged[:limit]
	}
	return merged, nil
}

// ListHistory loads local cron conversation history directly from conversations.
func (s *OpenClawCronService) ListHistory(jobID string, limit int) ([]OpenClawCronHistoryListItem, error) {
	if limit <= 0 {
		limit = defaultOpenClawCronListLimit
	}

	conversationItems, err := s.listConversationHistoryItems(jobID)
	if err != nil {
		return nil, err
	}

	merged := mergeHistoryItems(nil, conversationItems, nil)
	if len(merged) > limit {
		merged = merged[:limit]
	}
	return merged, nil
}

// GetRunDetail loads both the run record and the transcript file for a cron execution.
// 运行详情同时读取 run JSONL 和 session transcript JSONL，供右侧详情面板展示。
func (s *OpenClawCronService) GetRunDetail(jobID string, sessionID string) (*OpenClawCronRunDetail, error) {
	runs, err := s.readRunEntriesFromFile(jobID, defaultOpenClawCronListLimit)
	if err != nil {
		return nil, err
	}
	sessionID = strings.TrimSpace(sessionID)
	var target *OpenClawCronRunEntry
	for i := range runs {
		if runs[i].SessionID == sessionID {
			runCopy := runs[i]
			target = &runCopy
			break
		}
	}
	if target == nil {
		return nil, fmt.Errorf("openclaw cron run not found")
	}

	sessionPath := s.sessionFilePath(target.SessionKey, target.SessionID)
	messages, err := readTranscriptMessages(sessionPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	conversationID, conversationAgentID, err := s.ensureRunConversation(jobID, *target)
	if err != nil {
		return nil, err
	}

	return &OpenClawCronRunDetail{
		Run:                 *target,
		SessionFilePath:     sessionPath,
		RunFilePath:         s.runFilePath(jobID),
		ConversationID:      conversationID,
		ConversationAgentID: conversationAgentID,
		Messages:            messages,
		IsLive:              strings.EqualFold(target.Status, "running"),
	}, nil
}

// GetSchedulerStatus exposes the raw OpenClaw cron status payload for diagnostics.
// 返回 OpenClaw 原生调度器状态，便于页面展示和问题排查。
func (s *OpenClawCronService) GetSchedulerStatus() (map[string]any, error) {
	var out map[string]any
	if err := s.gatewayQuery(openClawCronMethodStatus, map[string]any{}, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// StartRunStream subscribes to gateway events for a specific cron run session.
// StartRunStream 按 sessionKey 订阅网关事件，并转发给前端做实时详情刷新。
func (s *OpenClawCronService) StartRunStream(jobID string, sessionID string, sessionKey string) (string, error) {
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" {
		return "", fmt.Errorf("openclaw cron session key is required")
	}

	watchID := fmt.Sprintf("openclaw-cron-%d", s.watchSeq.Add(1))
	s.manager.AddEventListener(watchID, func(event string, payload json.RawMessage) {
		if event != "agent" && event != "chat" {
			return
		}

		if event == "agent" {
			var frame struct {
				SessionKey string `json:"sessionKey"`
			}
			if json.Unmarshal(payload, &frame) != nil || !matchSessionKey(frame.SessionKey, sessionKey) {
				return
			}
		}

		if event == "chat" {
			var frame struct {
				SessionKey string `json:"sessionKey"`
			}
			if json.Unmarshal(payload, &frame) != nil || !matchSessionKey(frame.SessionKey, sessionKey) {
				return
			}
		}

		if s.app != nil {
			s.app.Event.Emit("openclaw:cron-run-event", map[string]any{
				"watch_id":    watchID,
				"job_id":      strings.TrimSpace(jobID),
				"session_id":  strings.TrimSpace(sessionID),
				"session_key": sessionKey,
				"event":       event,
				"payload":     json.RawMessage(payload),
			})
		}
	})

	s.watchMu.Lock()
	s.watches[watchID] = sessionKey
	s.watchMu.Unlock()

	return watchID, nil
}

// StopRunStream removes a previously registered run stream watcher.
// StopRunStream 移除前端不再需要的实时订阅，避免累积监听器。
func (s *OpenClawCronService) StopRunStream(watchID string) {
	watchID = strings.TrimSpace(watchID)
	if watchID == "" {
		return
	}
	s.manager.RemoveEventListener(watchID)
	s.watchMu.Lock()
	delete(s.watches, watchID)
	s.watchMu.Unlock()
}

// OnGatewayReady registers the background run tracker after the gateway becomes available.
func (s *OpenClawCronService) OnGatewayReady() {
	s.manager.RemoveEventListener(globalCronTrackerListenerKey)
	s.manager.AddEventListener(globalCronTrackerListenerKey, func(event string, payload json.RawMessage) {
		runID, sessionKey := extractGatewayRunContext(event, payload)
		if runID == "" || sessionKey == "" {
			return
		}
		conversationID := s.bindRunSession(runID, sessionKey)
		if conversationID <= 0 || s.app == nil {
			return
		}

		forwardState := s.ensureCronForwardState(sessionKey, runID, conversationID)
		forwardedEvents := buildCronForwardEvents(conversationID, sessionKey, runID, forwardState, event, payload)
		for _, item := range forwardedEvents {
			s.app.Event.Emit(item.Name, item.Payload)
		}
		if forwardState.Finished {
			s.clearCronForwardState(sessionKey)
		}
	})
}

// ensureCronForwardState returns a stable synthetic streaming state for one cron session.
// ensureCronForwardState 返回单个 Cron 会话稳定的合成流式状态，保证前端能持续复用同一条占位消息。
func (s *OpenClawCronService) ensureCronForwardState(sessionKey string, runID string, conversationID int64) *cronForwardState {
	s.forwardMu.Lock()
	defer s.forwardMu.Unlock()

	key := strings.TrimSpace(sessionKey)
	state := s.forwards[key]
	if state == nil || state.Finished {
		state = &cronForwardState{}
		s.forwards[key] = state
	}

	if strings.TrimSpace(runID) != "" {
		state.RequestID = buildCronForwardRequestID(key, runID)
	}
	if state.RequestID == "" {
		state.RequestID = buildCronForwardRequestID(key, "")
	}
	if state.MessageID == 0 {
		state.MessageID = buildCronForwardMessageID(conversationID, key)
	}
	return state
}

// clearCronForwardState clears finished cron-forward stream state.
// clearCronForwardState 清理已结束的 Cron 转发状态，避免下一轮运行复用旧的 request/message 标识。
func (s *OpenClawCronService) clearCronForwardState(sessionKey string) {
	s.forwardMu.Lock()
	defer s.forwardMu.Unlock()
	delete(s.forwards, strings.TrimSpace(sessionKey))
}

func (s *OpenClawCronService) jobsStorePath() (string, error) {
	if status, err := s.GetSchedulerStatus(); err == nil {
		if value, _ := status["storePath"].(string); strings.TrimSpace(value) != "" {
			return filepath.Clean(value), nil
		}
	}
	root, err := openclawroot.DataRootDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "cron", "jobs.json"), nil
}

func (s *OpenClawCronService) db() (*bun.DB, error) {
	db := sqlite.DB()
	if db == nil {
		return nil, fmt.Errorf("sqlite not initialized")
	}
	return db, nil
}

func (s *OpenClawCronService) listDeliveryChannelOptions(platform string) ([]cronDeliveryChannelOption, error) {
	db, err := s.db()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var options []cronDeliveryChannelOption
	query := db.NewSelect().
		TableExpr("channels AS ch").
		Column("ch.id", "ch.agent_id", "ch.platform", "ch.last_sender_id", "ch.extra_config").
		Where(openClawCronChannelVisibilitySQL)
	if trimmedPlatform := strings.TrimSpace(platform); trimmedPlatform != "" {
		query = query.Where("ch.platform = ?", trimmedPlatform)
	}
	if err := query.OrderExpr("ch.id DESC").Scan(ctx, &options); err != nil {
		return nil, err
	}
	return options, nil
}

func (s *OpenClawCronService) readJobsFromStore(storePath string) ([]openClawJobStoreItem, error) {
	data, err := os.ReadFile(storePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []openClawJobStoreItem{}, nil
		}
		return nil, err
	}
	var payload struct {
		Jobs []openClawJobStoreItem `json:"jobs"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	if payload.Jobs == nil {
		return []openClawJobStoreItem{}, nil
	}
	return payload.Jobs, nil
}

func (s *OpenClawCronService) findJobByID(jobID string) (*OpenClawCronJob, error) {
	items, err := s.ListJobs()
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if item.ID == strings.TrimSpace(jobID) {
			copyItem := item
			return &copyItem, nil
		}
	}
	return nil, fmt.Errorf("openclaw cron job not found")
}

// bindRunSession creates/updates the local conversation mapping directly from the
// gateway event payload instead of relying on in-memory pending state.
// bindRunSession 直接根据网关事件里的 sessionKey 建立本地会话映射，不再依赖 pending。
func (s *OpenClawCronService) bindRunSession(runID string, sessionKey string) int64 {
	runID = strings.TrimSpace(runID)
	sessionKey = strings.TrimSpace(sessionKey)
	if runID == "" || sessionKey == "" {
		return 0
	}

	jobID := parseJobIDFromSessionKey(sessionKey)
	if jobID == "" {
		return 0
	}

	job := OpenClawCronJob{
		ID:         jobID,
		AgentID:    "",
		SessionKey: sessionKey,
	}
	if foundJob, err := s.findJobByID(jobID); err == nil && foundJob != nil {
		job = *foundJob
		job.SessionKey = sessionKey
	} else if s.app != nil && err != nil {
		s.app.Logger.Warn("[openclaw-cron] find job by session key failed", "job_id", jobID, "run_id", runID, "error", err)
	}

	conversationID, _, err := s.ensureConversationForSession(jobID, job, sessionKey, runID, time.Now().UnixMilli())
	if err != nil {
		if s.app != nil {
			s.app.Logger.Warn("[openclaw-cron] ensure conversation from gateway event failed", "job_id", jobID, "run_id", runID, "error", err)
		}
		return 0
	}
	return conversationID
}

func (s *OpenClawCronService) listConversationHistoryItems(jobID string) ([]OpenClawCronHistoryListItem, error) {
	job, err := s.findJobByID(jobID)
	if err != nil {
		return []OpenClawCronHistoryListItem{}, nil
	}

	localAgentID, err := s.resolveManualRunAgentID(job.AgentID)
	if err != nil {
		return nil, err
	}
	if localAgentID <= 0 {
		return []OpenClawCronHistoryListItem{}, nil
	}

	conversationList, err := s.convSvc.ListConversationsBySource(
		localAgentID,
		conversations.AgentTypeOpenClaw,
		buildCronConversationSource(jobID),
	)
	if err != nil {
		return nil, err
	}

	items := make([]OpenClawCronHistoryListItem, 0, len(conversationList))
	for _, conversation := range conversationList {
		sessionKey := strings.TrimSpace(conversation.OpenClawSessionKey)
		runAtMs := conversation.UpdatedAt.UnixMilli()
		triggerType := normalizeHistoryTriggerType("", OpenClawCronHistorySourceConversation)
		if triggerAtMs := parseManualTriggerAtMs(conversation.ExternalID); triggerAtMs > 0 {
			runAtMs = triggerAtMs
			triggerType = "manual"
		}
		historyStatus, historyDurationMs := deriveManualConversationHistoryState(
			runAtMs,
			conversation.UpdatedAt.UnixMilli(),
			sessionKey,
			s.chatSvc != nil && s.chatSvc.HasActiveGeneration(conversation.ID),
		)
		items = append(items, OpenClawCronHistoryListItem{
			JobID:          strings.TrimSpace(jobID),
			SessionKey:     sessionKey,
			ConversationID: conversation.ID,
			Name:           strings.TrimSpace(conversation.Name),
			Status:         historyStatus,
			RunAtMs:        runAtMs,
			DurationMs:     historyDurationMs,
			TriggerType:    triggerType,
			Source:         OpenClawCronHistorySourceConversation,
		})
	}
	return items, nil
}

func enrichConversationHistoryItemsWithRunEntries(conversationItems []OpenClawCronHistoryListItem, runEntries []OpenClawCronRunEntry) {
	for index := range conversationItems {
		item := &conversationItems[index]
		if isTerminalHistoryItem(*item) {
			continue
		}

		for _, entry := range runEntries {
			runAtMs := chooseRunTimestamp(entry.RunAtMs, entry.TimestampMs)
			if !isCloseRunTimestamp(item.RunAtMs, runAtMs) {
				continue
			}
			if item.SessionKey == "" {
				item.SessionKey = strings.TrimSpace(entry.SessionKey)
			}
			if item.SessionID == "" {
				item.SessionID = strings.TrimSpace(entry.SessionID)
			}
			if item.DurationMs <= 0 {
				item.DurationMs = entry.DurationMs
			}
			if item.TriggerType == "" {
				item.TriggerType = normalizeHistoryTriggerType(entry.Action, item.Source)
			}
			if item.Status == "" || item.Status == openClawCronHistoryStatusRunning {
				item.Status = strings.TrimSpace(entry.Status)
			}
			break
		}
	}
}

// buildRunLogHistoryItems converts run-log rows into the shared history DTO.
// buildRunLogHistoryItems 把 run log 记录转换成历史列表统一使用的数据结构。
func buildRunLogHistoryItems(runEntries []OpenClawCronRunEntry) []OpenClawCronHistoryListItem {
	items := make([]OpenClawCronHistoryListItem, 0, len(runEntries))
	for _, entry := range runEntries {
		items = append(items, OpenClawCronHistoryListItem{
			JobID:       strings.TrimSpace(entry.JobID),
			SessionID:   strings.TrimSpace(entry.SessionID),
			SessionKey:  strings.TrimSpace(entry.SessionKey),
			Status:      strings.TrimSpace(entry.Status),
			RunAtMs:     chooseRunTimestamp(entry.RunAtMs, entry.TimestampMs),
			DurationMs:  entry.DurationMs,
			TriggerType: normalizeHistoryTriggerType(entry.Action, OpenClawCronHistorySourceRunLog),
			Source:      OpenClawCronHistorySourceRunLog,
		})
	}
	return items
}

func (s *OpenClawCronService) runFilePath(jobID string) string {
	root, err := openclawroot.DataRootDir()
	if err != nil {
		return ""
	}
	return filepath.Join(root, "cron", "runs", strings.TrimSpace(jobID)+".jsonl")
}

func (s *OpenClawCronService) readRunEntriesFromFile(jobID string, limit int) ([]OpenClawCronRunEntry, error) {
	if limit <= 0 {
		limit = defaultOpenClawCronListLimit
	}
	filePath := s.runFilePath(jobID)
	file, err := os.Open(filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []OpenClawCronRunEntry{}, nil
		}
		return nil, err
	}
	defer file.Close()

	items := make([]OpenClawCronRunEntry, 0)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var raw openClawRunEntryCLI
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}
		items = append(items, raw.toDTO())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	sortOpenClawRunsDesc(items)
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func sortOpenClawRunsDesc(items []OpenClawCronRunEntry) {
	slices.SortFunc(items, func(left, right OpenClawCronRunEntry) int {
		leftTs := left.RunAtMs
		if leftTs <= 0 {
			leftTs = left.TimestampMs
		}
		rightTs := right.RunAtMs
		if rightTs <= 0 {
			rightTs = right.TimestampMs
		}
		switch {
		case leftTs > rightTs:
			return -1
		case leftTs < rightTs:
			return 1
		default:
			return strings.Compare(right.SessionID, left.SessionID)
		}
	})
}

func (s *OpenClawCronService) sessionFilePath(sessionKey, sessionID string) string {
	root, err := openclawroot.DataRootDir()
	if err != nil {
		return ""
	}
	agentID := parseAgentIDFromSessionKey(sessionKey)
	if agentID == "" {
		agentID = "main"
	}
	return filepath.Join(root, "agents", agentID, "sessions", strings.TrimSpace(sessionID)+".jsonl")
}

func (s *OpenClawCronService) agentNameMap() (map[string]string, error) {
	items, err := s.agentsSvc.ListAgents()
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(items))
	for _, item := range items {
		out[item.OpenClawAgentID] = item.Name
	}
	return out, nil
}

// ensureRunConversation finds or creates a local OpenClaw conversation for a cron run session.
// ensureRunConversation 为 Cron run 的真实 session_key 建立本地 conversations 映射，供嵌入聊天页复用。
func (s *OpenClawCronService) ensureRunConversation(jobID string, run OpenClawCronRunEntry) (int64, int64, error) {
	sessionKey := strings.TrimSpace(run.SessionKey)
	if sessionKey == "" {
		return 0, 0, nil
	}

	openClawAgentID := parseAgentIDFromSessionKey(sessionKey)
	if openClawAgentID == "" {
		return 0, 0, nil
	}

	localAgentID, err := s.findLocalOpenClawAgentID(openClawAgentID)
	if err != nil || localAgentID <= 0 {
		return 0, 0, err
	}

	jobName := strings.TrimSpace(jobID)
	if foundJob, findErr := s.findJobByID(jobID); findErr == nil {
		if strings.TrimSpace(foundJob.Name) != "" {
			jobName = strings.TrimSpace(foundJob.Name)
		}
	}
	if jobName == "" {
		jobName = openClawCronDefaultConversationName
	}

	return s.ensureConversationRecord(
		localAgentID,
		buildCronConversationSource(jobID),
		sessionKey,
		buildCronExternalID(jobID, run.SessionID),
		buildCronConversationName(jobName, run.RunAtMs, run.TimestampMs),
	)
}

// findConversationBySessionKey looks up the local conversation mapped to an OpenClaw session key.
// findConversationBySessionKey 按 openclaw_session_key 查找本地会话，避免重复创建历史映射。
func (s *OpenClawCronService) findConversationBySessionKey(sessionKey string) (*cronConversationRecord, error) {
	db, err := s.db()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var record cronConversationRecord
	err = db.NewSelect().
		Table("conversations").
		Column("id", "agent_id", "openclaw_session_key").
		Where("openclaw_session_key = ?", strings.TrimSpace(sessionKey)).
		OrderExpr("id DESC").
		Limit(1).
		Scan(ctx, &record)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &record, nil
}

// findLocalOpenClawAgentID maps an OpenClaw runtime agent id back to the local openclaw_agents row id.
// findLocalOpenClawAgentID 把 runtime 的 openclaw_agent_id 映射回本地 openclaw_agents 主键。
func (s *OpenClawCronService) findLocalOpenClawAgentID(openClawAgentID string) (int64, error) {
	items, err := s.agentsSvc.ListAgents()
	if err != nil {
		return 0, err
	}
	for _, item := range items {
		if strings.TrimSpace(item.OpenClawAgentID) == strings.TrimSpace(openClawAgentID) {
			return item.ID, nil
		}
	}
	return 0, nil
}

// splitCronModelSelection converts the form's "provider/model" shorthand into
// conversation model fields. Bare aliases fall back to the agent's default provider.
func splitCronModelSelection(raw string) (string, string) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", ""
	}
	parts := strings.SplitN(trimmed, "/", 2)
	if len(parts) == 2 && strings.TrimSpace(parts[0]) != "" && strings.TrimSpace(parts[1]) != "" {
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	return "", trimmed
}

// normalizeCronThinking maps cron thinking presets onto the boolean
// conversation flag currently supported by the OpenClaw chat pipeline.
func normalizeCronThinking(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "off", "false", "0", "none":
		return false
	default:
		return true
	}
}

// resolveManualRunAgentID maps the cron job agent to a local OpenClaw agent row and falls back to main.
// resolveManualRunAgentID 把 Cron 任务里的 agent_id 映射到本地 openclaw_agents 记录，未指定时回退到 main。
func (s *OpenClawCronService) resolveManualRunAgentID(openClawAgentID string) (int64, error) {
	localAgentID, err := s.findLocalOpenClawAgentID(strings.TrimSpace(openClawAgentID))
	if err != nil {
		return 0, err
	}
	if localAgentID > 0 {
		return localAgentID, nil
	}
	return s.findLocalOpenClawAgentID("main")
}

// insertConversationRecord creates a read-only history conversation record for a cron run session.
// insertConversationRecord 为 Cron 历史创建本地会话记录，供只读嵌入页读取完整 OpenClaw 消息。
func (s *OpenClawCronService) insertConversationRecord(agentID int64, conversationSource, sessionKey, externalID, name string) (int64, error) {
	created, err := s.convSvc.CreateConversation(conversations.CreateConversationInput{
		AgentID:            agentID,
		AgentType:          conversations.AgentTypeOpenClaw,
		ConversationSource: strings.TrimSpace(conversationSource),
		Name:               strings.TrimSpace(name),
		ExternalID:         strings.TrimSpace(externalID),
		OpenClawSessionKey: strings.TrimSpace(sessionKey),
		ChatMode:           conversations.ChatModeTask,
		TeamType:           conversations.TeamTypePerson,
	})
	if err != nil {
		return 0, err
	}
	return created.ID, nil
}

// ensureConversationRecord serializes session-key based history inserts so background binding
// and detail reads cannot create duplicate local conversations for the same cron session.
func (s *OpenClawCronService) ensureConversationRecord(agentID int64, conversationSource, sessionKey, externalID, name string) (int64, int64, error) {
	s.convMu.Lock()
	defer s.convMu.Unlock()

	existing, err := s.findConversationBySessionKey(sessionKey)
	if err != nil {
		return 0, 0, err
	}
	if existing != nil {
		return existing.ID, existing.AgentID, nil
	}

	createdID, err := s.insertConversationRecord(agentID, conversationSource, sessionKey, externalID, name)
	if err != nil {
		return 0, 0, err
	}
	return createdID, agentID, nil
}

func (s *OpenClawCronService) gatewayRequest(method string, params any, out any) error {
	ctx, cancel := context.WithTimeout(context.Background(), openClawCronGatewayTimeout)
	defer cancel()
	if out == nil {
		var ignored json.RawMessage
		return s.manager.QueryRequest(ctx, method, params, &ignored)
	}
	return s.manager.QueryRequest(ctx, method, params, out)
}

func (s *OpenClawCronService) gatewayQuery(method string, params any, out any) error {
	ctx, cancel := context.WithTimeout(context.Background(), openClawCronGatewayTimeout)
	defer cancel()
	if out == nil {
		var ignored json.RawMessage
		return s.manager.QueryRequest(ctx, method, params, &ignored)
	}
	return s.manager.QueryRequest(ctx, method, params, out)
}

// ensureConversationForSession creates a cron conversation as soon as the session key is known.
func (s *OpenClawCronService) ensureConversationForSession(jobID string, job OpenClawCronJob, sessionKey, runID string, runAtMs int64) (int64, int64, error) {
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" {
		return 0, 0, nil
	}

	var err error
	localAgentID := int64(0)
	if strings.TrimSpace(job.AgentID) != "" {
		localAgentID, err = s.resolveManualRunAgentID(job.AgentID)
		if err != nil {
			return 0, 0, err
		}
	}
	if localAgentID <= 0 {
		localAgentID, err = s.findLocalOpenClawAgentID(parseAgentIDFromSessionKey(sessionKey))
		if err != nil {
			return 0, 0, err
		}
	}
	if localAgentID <= 0 {
		return 0, 0, nil
	}

	jobName := strings.TrimSpace(job.Name)
	if jobName == "" {
		jobName = strings.TrimSpace(jobID)
	}
	if jobName == "" {
		jobName = openClawCronDefaultConversationName
	}

	return s.ensureConversationRecord(
		localAgentID,
		buildCronConversationSource(jobID),
		sessionKey,
		buildCronExternalID(jobID, runID),
		buildCronConversationName(jobName, runAtMs, runAtMs),
	)
}

func (s *OpenClawCronService) runCLIJSON(args []string, out any) error {
	return s.runCLI(args, true, out)
}

// runCLI executes an OpenClaw CLI command against the embedded gateway.
// runCLI 统一处理 OpenClaw CLI 调用，并按需决定是否追加 JSON 输出参数。
func (s *OpenClawCronService) runCLI(args []string, jsonOutput bool, out any) error {
	output, err := s.runCLIOutput(args, jsonOutput)
	if err != nil {
		return fmt.Errorf("openclaw %s failed: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(output, out); err != nil {
		return fmt.Errorf("decode openclaw response: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

// runCLIOutput executes an OpenClaw CLI command and returns raw combined output.
// runCLIOutput 负责执行 OpenClaw CLI，并返回原始输出供调用方做细粒度判断。
func (s *OpenClawCronService) runCLIOutput(args []string, jsonOutput bool) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cliPath, env, err := s.manager.CLICommand()
	if err != nil {
		return nil, fmt.Errorf("resolve openclaw cli command: %w", err)
	}

	gatewayURL := strings.TrimSpace(strings.Replace(s.manager.GatewayURL(), "http://", "ws://", 1)) + "/ws"
	token := strings.TrimSpace(s.manager.GatewayToken())
	fullArgs := buildCLIArgs(args, gatewayURL, token, jsonOutput)

	// Force UTF-8 safe output decoding on Windows and keep the command pinned to the embedded gateway.
	// 在 Windows 下固定 UTF-8 输出，并显式指向内嵌 Gateway，避免误连用户全局 OpenClaw。
	cmd := exec.CommandContext(ctx, cliPath, fullArgs...)
	cmd.Env = append(env, "PYTHONUTF8=1")
	setCmdHideWindow(cmd)
	return cmd.CombinedOutput()
}

// buildCLIArgs appends command-scoped gateway options after the subcommand tokens.
// buildCLIArgs 负责把网关路由参数追加到子命令之后，匹配 OpenClaw CLI 的真实参数语义。
func buildCLIArgs(args []string, gatewayURL string, token string, jsonOutput bool) []string {
	fullArgs := make([]string, 0, len(args)+5)
	fullArgs = append(fullArgs, args...)
	if shouldAppendJSONFlag(args, jsonOutput) {
		fullArgs = append(fullArgs, "--json")
	}
	if strings.TrimSpace(gatewayURL) != "" {
		fullArgs = append(fullArgs, "--url", strings.TrimSpace(gatewayURL))
	}
	if strings.TrimSpace(token) != "" {
		fullArgs = append(fullArgs, "--token", strings.TrimSpace(token))
	}
	return fullArgs
}

const (
	openClawCronCommand                 = "cron"
	openClawRunCommand                  = "run"
	openClawCronDefaultConversationName = "OpenClaw Cron"
)

// shouldAppendJSONFlag guards OpenClaw commands that reject the global --json flag.
func shouldAppendJSONFlag(args []string, jsonOutput bool) bool {
	if !jsonOutput {
		return false
	}
	if len(args) < 2 {
		return true
	}
	if strings.TrimSpace(args[0]) == openClawCronCommand && strings.TrimSpace(args[1]) == openClawRunCommand {
		return false
	}
	return true
}

// buildRunNowArgs keeps manual trigger semantics aligned with OpenClaw-native cron execution.
// buildRunNowArgs 保证“立即运行”走 OpenClaw 原生 cron run，而不是本地拼装手动会话。
func buildRunNowArgs(jobID string) ([]string, error) {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return nil, fmt.Errorf("openclaw cron id is required")
	}
	return []string{"cron", "run", jobID}, nil
}

// isBenignCronRunOutput treats "already-running" as a non-fatal result for manual triggers.
// isBenignCronRunOutput 将 already-running 识别为“已在运行”的可接受结果，避免前端误报失败。
func isBenignCronRunOutput(output []byte) bool {
	var payload struct {
		OK     bool   `json:"ok"`
		Ran    bool   `json:"ran"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(output, &payload); err != nil {
		return false
	}
	return payload.OK && !payload.Ran && strings.EqualFold(strings.TrimSpace(payload.Reason), "already-running")
}

func (s *OpenClawCronService) resolveCreateDeliveryInput(input CreateOpenClawCronJobInput) (CreateOpenClawCronJobInput, error) {
	resolvedChannel, resolvedTarget, resolvedAccountID, err := s.resolveDeliverySelection(
		cronDeliverySelectionInput{
			OpenClawAgentID: input.AgentID,
			Platform:        input.DeliveryChannel,
			TargetID:        input.DeliveryTargetID,
		},
	)
	if err != nil {
		return input, err
	}
	if resolvedChannel == "" {
		return input, nil
	}
	input.DeliveryChannel = resolvedChannel
	input.DeliveryTo = resolvedTarget
	input.DeliveryAccountID = resolvedAccountID
	return input, nil
}

func (s *OpenClawCronService) resolveUpdateDeliveryInput(input UpdateOpenClawCronJobInput) (UpdateOpenClawCronJobInput, error) {
	if input.DeliveryChannel == nil && input.DeliveryTargetID == nil && input.AgentID == nil {
		return input, nil
	}

	resolvedChannel, resolvedTarget, resolvedAccountID, err := s.resolveDeliverySelection(
		cronDeliverySelectionInput{
			OpenClawAgentID: derefTrim(input.AgentID),
			Platform:        derefTrim(input.DeliveryChannel),
			TargetID:        derefTrim(input.DeliveryTargetID),
		},
	)
	if err != nil {
		return input, err
	}

	input.DeliveryChannel = stringPtr(resolvedChannel)
	input.DeliveryTo = stringPtr(resolvedTarget)
	input.DeliveryAccountID = stringPtr(resolvedAccountID)
	return input, nil
}

func (s *OpenClawCronService) resolveDeliverySelection(input cronDeliverySelectionInput) (string, string, string, error) {
	platform := strings.TrimSpace(input.Platform)
	if platform == "" {
		return "", "", "", nil
	}

	options, err := s.listDeliveryChannelOptions(platform)
	if err != nil {
		return "", "", "", err
	}
	localAgentID, err := s.resolveDeliveryAgentRowID(input.OpenClawAgentID)
	if err != nil {
		return "", "", "", err
	}
	matchedChannel, err := resolveCronDeliveryChannelOption(localAgentID, platform, options)
	if err != nil {
		return "", "", "", err
	}

	resolvedTarget, err := resolveCronDeliveryTargetID(input.TargetID, matchedChannel)
	if err != nil {
		return "", "", "", err
	}
	return platform, resolvedTarget, deriveCronDeliveryAccountID(matchedChannel), nil
}

func (s *OpenClawCronService) resolveDeliveryAgentRowID(openClawAgentID string) (int64, error) {
	trimmedAgentID := strings.TrimSpace(openClawAgentID)
	if trimmedAgentID == "" {
		return s.findLocalOpenClawAgentID("main")
	}
	return s.findLocalOpenClawAgentID(trimmedAgentID)
}

func resolveCronDeliveryChannelOption(agentRowID int64, platform string, channels []cronDeliveryChannelOption) (cronDeliveryChannelOption, error) {
	if agentRowID <= 0 {
		return cronDeliveryChannelOption{}, fmt.Errorf("openclaw cron delivery agent is required")
	}
	trimmedPlatform := strings.TrimSpace(platform)
	if trimmedPlatform == "" {
		return cronDeliveryChannelOption{}, fmt.Errorf("openclaw cron delivery platform is required")
	}

	filteredChannels := make([]cronDeliveryChannelOption, 0, len(channels))
	for _, channel := range channels {
		if channel.AgentRowID != agentRowID {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(channel.Platform), trimmedPlatform) {
			filteredChannels = append(filteredChannels, channel)
		}
	}
	if len(filteredChannels) == 0 {
		return cronDeliveryChannelOption{}, fmt.Errorf("openclaw channel platform %s is not bound to the selected agent", trimmedPlatform)
	}
	if len(filteredChannels) > 1 {
		return cronDeliveryChannelOption{}, fmt.Errorf("multiple openclaw channels match agent and platform %s", trimmedPlatform)
	}
	return filteredChannels[0], nil
}

func resolveCronDeliveryTargetID(explicitTargetID string, channel cronDeliveryChannelOption) (string, error) {
	resolvedTargetID := strings.TrimSpace(explicitTargetID)
	if resolvedTargetID != "" {
		return resolvedTargetID, nil
	}

	resolvedTargetID = strings.TrimSpace(channel.LastSenderID)
	if resolvedTargetID == "" {
		return "", fmt.Errorf("openclaw channel platform %s has no last active target", strings.TrimSpace(channel.Platform))
	}
	return resolvedTargetID, nil
}

// extractLatestDeliveryTarget prefers the resolved target id chosen for delivery.
// extractLatestDeliveryTarget 优先返回已经解析出的发送目标，保持历史展示与发送行为一致。
func extractLatestDeliveryTarget(channel string, targetID string, accountID string) string {
	return strings.TrimSpace(targetID)
}

func deriveCronDeliveryAccountID(option cronDeliveryChannelOption) string {
	if openClawChannelID := extractOpenClawChannelIDFromExtraConfig(option.ExtraConfig); openClawChannelID != "" {
		return openClawChannelID
	}
	if option.ID <= 0 {
		return ""
	}
	return fmt.Sprintf("channel_%d", option.ID)
}

func extractOpenClawChannelIDFromExtraConfig(extraConfig string) string {
	if strings.TrimSpace(extraConfig) == "" {
		return ""
	}
	var payload struct {
		OpenClawChannelID string `json:"openclaw_channel_id"`
	}
	if err := json.Unmarshal([]byte(extraConfig), &payload); err != nil {
		return ""
	}
	return strings.TrimSpace(payload.OpenClawChannelID)
}

func buildCreateJobPayload(input CreateOpenClawCronJobInput) (map[string]any, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, fmt.Errorf("openclaw cron name is required")
	}

	schedule, err := buildCreateSchedulePayload(input.ScheduleKind, input.CronExpr, input.Every, input.At, input.Timezone, input.Exact)
	if err != nil {
		return nil, err
	}
	payload, sessionTarget, err := buildCreatePayload(input)
	if err != nil {
		return nil, err
	}
	delivery := buildCreateDeliveryPayload(input, sessionTarget, payload)

	job := map[string]any{
		"name":          name,
		"schedule":      schedule,
		"sessionTarget": sessionTarget,
		"wakeMode":      normalizeWakeModeOrDefault(input.WakeMode),
		"payload":       payload,
		"enabled":       input.Enabled,
	}
	if description := strings.TrimSpace(input.Description); description != "" {
		job["description"] = description
	}
	if agentID := strings.TrimSpace(input.AgentID); agentID != "" {
		job["agentId"] = agentID
	}
	if sessionKey := strings.TrimSpace(input.SessionKey); sessionKey != "" {
		job["sessionKey"] = sessionKey
	}
	if delivery != nil {
		job["delivery"] = delivery
	}
	if deleteAfterRun, ok := normalizeDeleteAfterRun(input.DeleteAfterRun, input.KeepAfterRun); ok {
		job["deleteAfterRun"] = deleteAfterRun
	}
	return job, nil
}

func buildUpdateJobPatch(input UpdateOpenClawCronJobInput) (map[string]any, error) {
	patch := make(map[string]any)
	if input.Name != nil && strings.TrimSpace(*input.Name) != "" {
		patch["name"] = strings.TrimSpace(*input.Name)
	}
	if input.Description != nil {
		patch["description"] = strings.TrimSpace(*input.Description)
	}
	if input.Enabled != nil {
		patch["enabled"] = *input.Enabled
	}
	if input.DeleteAfterRun != nil || input.KeepAfterRun != nil {
		deleteAfterRun, ok := normalizeDeleteAfterRun(boolPtrValue(input.DeleteAfterRun), boolPtrValue(input.KeepAfterRun))
		if ok {
			patch["deleteAfterRun"] = deleteAfterRun
		}
	}
	if input.ClearAgent {
		patch["agentId"] = nil
	} else if input.AgentID != nil && strings.TrimSpace(*input.AgentID) != "" {
		patch["agentId"] = strings.TrimSpace(*input.AgentID)
	}
	if input.ClearSessionKey {
		patch["sessionKey"] = nil
	} else if input.SessionKey != nil && strings.TrimSpace(*input.SessionKey) != "" {
		patch["sessionKey"] = strings.TrimSpace(*input.SessionKey)
	}
	if input.SessionTarget != nil {
		if sessionTarget := normalizeSessionTargetValue(*input.SessionTarget); sessionTarget != "" {
			patch["sessionTarget"] = sessionTarget
		}
	}
	if input.WakeMode != nil {
		if wakeMode := normalizeWakeModeValue(*input.WakeMode); wakeMode != "" {
			patch["wakeMode"] = wakeMode
		}
	}
	schedule, err := buildUpdateSchedulePayload(input)
	if err != nil {
		return nil, err
	}
	if schedule != nil {
		patch["schedule"] = schedule
	}
	payload := buildUpdatePayloadPatch(input)
	if payload != nil {
		patch["payload"] = payload
	}
	delivery := buildUpdateDeliveryPatch(input)
	if delivery != nil {
		patch["delivery"] = delivery
	}
	return patch, nil
}

func buildCreateSchedulePayload(kind, cronExpr, every, at, timezone string, exact bool) (map[string]any, error) {
	switch strings.TrimSpace(kind) {
	case openClawCronScheduleCron:
		expr := strings.TrimSpace(cronExpr)
		if expr == "" {
			return nil, fmt.Errorf("openclaw cron expression is required")
		}
		schedule := map[string]any{
			"kind": openClawCronScheduleCron,
			"expr": expr,
		}
		if tz := strings.TrimSpace(timezone); tz != "" {
			schedule["tz"] = tz
		}
		if exact {
			schedule["staggerMs"] = int64(0)
		}
		return schedule, nil
	case openClawCronScheduleEvery:
		everyMs, err := parseEveryDurationMs(every)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"kind":    openClawCronScheduleEvery,
			"everyMs": everyMs,
		}, nil
	case openClawCronScheduleAt:
		atValue := strings.TrimSpace(at)
		if atValue == "" {
			return nil, fmt.Errorf("openclaw at value is required")
		}
		return map[string]any{
			"kind": openClawCronScheduleAt,
			"at":   atValue,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported openclaw schedule kind")
	}
}

func buildUpdateSchedulePayload(input UpdateOpenClawCronJobInput) (map[string]any, error) {
	if input.ScheduleKind == nil {
		return nil, nil
	}
	switch strings.TrimSpace(*input.ScheduleKind) {
	case openClawCronScheduleCron:
		if input.CronExpr == nil || strings.TrimSpace(*input.CronExpr) == "" {
			return nil, fmt.Errorf("openclaw cron expression is required")
		}
		schedule := map[string]any{
			"kind": openClawCronScheduleCron,
			"expr": strings.TrimSpace(*input.CronExpr),
		}
		if input.Timezone != nil && strings.TrimSpace(*input.Timezone) != "" {
			schedule["tz"] = strings.TrimSpace(*input.Timezone)
		}
		if input.Exact != nil && *input.Exact {
			schedule["staggerMs"] = int64(0)
		}
		return schedule, nil
	case openClawCronScheduleEvery:
		if input.Every == nil {
			return nil, fmt.Errorf("openclaw every duration is required")
		}
		everyMs, err := parseEveryDurationMs(*input.Every)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"kind":    openClawCronScheduleEvery,
			"everyMs": everyMs,
		}, nil
	case openClawCronScheduleAt:
		if input.At == nil || strings.TrimSpace(*input.At) == "" {
			return nil, fmt.Errorf("openclaw at value is required")
		}
		return map[string]any{
			"kind": openClawCronScheduleAt,
			"at":   strings.TrimSpace(*input.At),
		}, nil
	case "":
		return nil, nil
	default:
		return nil, fmt.Errorf("unsupported openclaw schedule kind")
	}
}

func buildCreatePayload(input CreateOpenClawCronJobInput) (map[string]any, string, error) {
	message := strings.TrimSpace(input.Message)
	systemEvent := strings.TrimSpace(input.SystemEvent)
	if (message == "") == (systemEvent == "") {
		return nil, "", fmt.Errorf("choose exactly one payload: message or system event")
	}

	if systemEvent != "" {
		return map[string]any{
			"kind": openClawCronPayloadSystemEvent,
			"text": systemEvent,
		}, normalizeSessionTargetForCreate(strings.TrimSpace(input.SessionTarget), openClawCronPayloadSystemEvent), nil
	}

	payload := map[string]any{
		"kind":    openClawCronPayloadAgentTurn,
		"message": message,
	}
	appendAgentTurnCommonPayload(payload, strings.TrimSpace(input.Model), strings.TrimSpace(input.Thinking), input.TimeoutMs, input.LightContext)
	return payload, normalizeSessionTargetForCreate(strings.TrimSpace(input.SessionTarget), openClawCronPayloadAgentTurn), nil
}

func buildUpdatePayloadPatch(input UpdateOpenClawCronJobInput) map[string]any {
	hasSystemEventPatch := input.SystemEvent != nil
	hasAgentTurnPatch := input.Message != nil || hasAgentTurnCommonPatch(input)
	if hasSystemEventPatch && hasAgentTurnPatch {
		return nil
	}
	if hasSystemEventPatch {
		text := strings.TrimSpace(derefTrim(input.SystemEvent))
		return map[string]any{
			"kind": openClawCronPayloadSystemEvent,
			"text": text,
		}
	}
	if !hasAgentTurnPatch {
		return nil
	}
	payload := map[string]any{
		"kind": openClawCronPayloadAgentTurn,
	}
	if input.Message != nil {
		payload["message"] = strings.TrimSpace(*input.Message)
	}
	appendAgentTurnCommonPatch(payload, input)
	return payload
}

func buildCreateDeliveryPayload(input CreateOpenClawCronJobInput, sessionTarget string, payload map[string]any) map[string]any {
	if sessionTarget != openClawCronSessionIsolated || payload["kind"] != openClawCronPayloadAgentTurn {
		return nil
	}

	mode := normalizeCreateDeliveryMode(input.Announce, input.DeliveryMode)
	delivery := map[string]any{
		"mode": mode,
	}
	if channel := strings.TrimSpace(input.DeliveryChannel); channel != "" {
		delivery["channel"] = strings.ToLower(channel)
	}
	if to := strings.TrimSpace(input.DeliveryTo); to != "" {
		delivery["to"] = to
	}
	if accountID := strings.TrimSpace(input.DeliveryAccountID); accountID != "" {
		delivery["accountId"] = accountID
	}
	if input.BestEffortDeliver {
		delivery["bestEffort"] = true
	}
	return delivery
}

func buildUpdateDeliveryPatch(input UpdateOpenClawCronJobInput) map[string]any {
	delivery := make(map[string]any)
	hasExplicitDeliveryMode := false
	if input.DeliveryMode != nil {
		if mode := normalizeDeliveryModeValue(*input.DeliveryMode); mode != "" {
			delivery["mode"] = mode
			hasExplicitDeliveryMode = true
		}
	}
	// Explicit mode wins / 显式模式优先：when delivery_mode is provided, do not let the legacy announce bool overwrite it.
	if input.Announce != nil && !hasExplicitDeliveryMode {
		if *input.Announce {
			delivery["mode"] = openClawCronDeliveryModeAnnounce
		} else {
			delivery["mode"] = openClawCronDeliveryModeNone
		}
	}
	if input.DeliveryChannel != nil {
		trimmed := strings.TrimSpace(*input.DeliveryChannel)
		if trimmed != "" {
			delivery["channel"] = strings.ToLower(trimmed)
		}
	}
	if input.DeliveryTo != nil {
		trimmed := strings.TrimSpace(*input.DeliveryTo)
		if trimmed != "" {
			delivery["to"] = trimmed
		}
	}
	if input.DeliveryAccountID != nil {
		trimmed := strings.TrimSpace(*input.DeliveryAccountID)
		if trimmed != "" {
			delivery["accountId"] = trimmed
		}
	}
	if input.BestEffortDeliver != nil {
		delivery["bestEffort"] = *input.BestEffortDeliver
		if _, ok := delivery["mode"]; !ok {
			delivery["mode"] = openClawCronDeliveryModeAnnounce
		}
	}
	if len(delivery) == 0 {
		return nil
	}
	return delivery
}

func appendAgentTurnCommonPayload(payload map[string]any, model, thinking string, timeoutMs int64, lightContext bool) {
	if model != "" {
		payload["model"] = model
	}
	if thinking != "" {
		payload["thinking"] = thinking
	}
	if timeoutSeconds, ok := timeoutMsToSeconds(timeoutMs); ok {
		payload["timeoutSeconds"] = timeoutSeconds
	}
	if lightContext {
		payload["lightContext"] = true
	}
}

func appendAgentTurnCommonPatch(payload map[string]any, input UpdateOpenClawCronJobInput) {
	if input.Model != nil {
		trimmedModel := strings.TrimSpace(*input.Model)
		if trimmedModel == "" {
			payload["model"] = nil
		} else {
			payload["model"] = trimmedModel
		}
	}
	if input.Thinking != nil && strings.TrimSpace(*input.Thinking) != "" {
		payload["thinking"] = strings.TrimSpace(*input.Thinking)
	}
	if input.TimeoutMs != nil {
		if timeoutSeconds, ok := timeoutMsToSeconds(*input.TimeoutMs); ok {
			payload["timeoutSeconds"] = timeoutSeconds
		}
	}
	if input.LightContext != nil {
		payload["lightContext"] = *input.LightContext
	}
}

func hasAgentTurnCommonPatch(input UpdateOpenClawCronJobInput) bool {
	return input.Model != nil || input.Thinking != nil || input.TimeoutMs != nil || input.LightContext != nil ||
		input.Announce != nil || input.DeliveryMode != nil || input.DeliveryChannel != nil ||
		input.DeliveryTo != nil || input.DeliveryAccountID != nil || input.BestEffortDeliver != nil
}

func normalizeSessionTargetForCreate(raw string, payloadKind string) string {
	if sessionTarget := normalizeSessionTargetValue(raw); sessionTarget != "" {
		return sessionTarget
	}
	if payloadKind == openClawCronPayloadSystemEvent {
		return openClawCronSessionMain
	}
	return openClawCronSessionIsolated
}

func normalizeSessionTargetValue(raw string) string {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case openClawCronSessionMain:
		return openClawCronSessionMain
	case openClawCronSessionIsolated:
		return openClawCronSessionIsolated
	default:
		return ""
	}
}

func normalizeWakeModeOrDefault(raw string) string {
	if wakeMode := normalizeWakeModeValue(raw); wakeMode != "" {
		return wakeMode
	}
	return openClawCronWakeNow
}

func normalizeWakeModeValue(raw string) string {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case openClawCronWakeNow:
		return openClawCronWakeNow
	case "next-heartbeat":
		return "next-heartbeat"
	default:
		return ""
	}
}

func normalizeCreateDeliveryMode(announce bool, rawMode string) string {
	if mode := normalizeDeliveryModeValue(rawMode); mode != "" {
		return mode
	}
	if announce {
		return openClawCronDeliveryModeAnnounce
	}
	return openClawCronDeliveryModeAnnounce
}

func normalizeDeliveryModeValue(raw string) string {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case openClawCronDeliveryModeNone:
		return openClawCronDeliveryModeNone
	case openClawCronDeliveryModeWebhook:
		return openClawCronDeliveryModeWebhook
	case openClawCronDeliveryModeAnnounce:
		return openClawCronDeliveryModeAnnounce
	default:
		return ""
	}
}

func normalizeDeleteAfterRun(deleteAfterRun bool, keepAfterRun bool) (bool, bool) {
	if deleteAfterRun {
		return true, true
	}
	if keepAfterRun {
		return false, true
	}
	return false, false
}

func timeoutMsToSeconds(timeoutMs int64) (int64, bool) {
	if timeoutMs <= 0 {
		return 0, false
	}
	seconds := timeoutMs / 1000
	if timeoutMs%1000 != 0 {
		seconds++
	}
	if seconds <= 0 {
		return 0, false
	}
	return seconds, true
}

func parseEveryDurationMs(value string) (int64, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, fmt.Errorf("openclaw every duration is required")
	}
	duration, err := time.ParseDuration(trimmed)
	if err != nil {
		return 0, fmt.Errorf("invalid openclaw every duration: %w", err)
	}
	if duration <= 0 {
		return 0, fmt.Errorf("openclaw every duration must be positive")
	}
	return int64(duration / time.Millisecond), nil
}

func boolPtrValue(value *bool) bool {
	return value != nil && *value
}

func buildCreateArgs(input CreateOpenClawCronJobInput) ([]string, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, fmt.Errorf("openclaw cron name is required")
	}
	args := []string{"cron", "add", "--name", name}
	if description := strings.TrimSpace(input.Description); description != "" {
		args = append(args, "--description", description)
	}
	if err := appendScheduleArgs(&args, input.ScheduleKind, input.CronExpr, input.Every, input.At, input.Timezone); err != nil {
		return nil, err
	}
	if agentID := strings.TrimSpace(input.AgentID); agentID != "" {
		args = append(args, "--agent", agentID)
	}
	appendPayloadArgs(&args, strings.TrimSpace(input.Message), strings.TrimSpace(input.SystemEvent))
	appendCommonArgs(&args,
		strings.TrimSpace(input.Model),
		strings.TrimSpace(input.Thinking),
		input.ExpectFinal,
		input.LightContext,
		input.TimeoutMs,
		strings.TrimSpace(input.SessionTarget),
		strings.TrimSpace(input.SessionKey),
		strings.TrimSpace(input.WakeMode),
	)
	appendDeliveryArgs(&args, input.Announce, strings.TrimSpace(input.DeliveryMode), strings.TrimSpace(input.DeliveryChannel), strings.TrimSpace(input.DeliveryTo), strings.TrimSpace(input.DeliveryAccountID), input.BestEffortDeliver)
	if input.Exact {
		args = append(args, "--exact")
	}
	if input.DeleteAfterRun {
		args = append(args, "--delete-after-run")
	}
	if input.KeepAfterRun {
		args = append(args, "--keep-after-run")
	}
	if !input.Enabled {
		args = append(args, "--disabled")
	}
	return args, nil
}

func buildUpdateArgs(jobID string, input UpdateOpenClawCronJobInput) ([]string, error) {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return nil, fmt.Errorf("openclaw cron id is required")
	}
	args := []string{"cron", "edit", jobID}
	if input.Name != nil && strings.TrimSpace(*input.Name) != "" {
		args = append(args, "--name", strings.TrimSpace(*input.Name))
	}
	if input.Description != nil {
		args = append(args, "--description", strings.TrimSpace(*input.Description))
	}
	if input.ClearAgent {
		args = append(args, "--clear-agent")
	} else if input.AgentID != nil && strings.TrimSpace(*input.AgentID) != "" {
		args = append(args, "--agent", strings.TrimSpace(*input.AgentID))
	}
	if input.ScheduleKind != nil {
		if err := appendScheduleArgsForUpdate(&args, strings.TrimSpace(*input.ScheduleKind), input.CronExpr, input.Every, input.At, input.Timezone); err != nil {
			return nil, err
		}
	}
	if input.Message != nil || input.SystemEvent != nil {
		appendPayloadArgs(&args, derefTrim(input.Message), derefTrim(input.SystemEvent))
	}
	appendOptionalStringArg(&args, "--model", input.Model)
	appendOptionalStringArg(&args, "--thinking", input.Thinking)
	appendOptionalBoolArg(&args, "--expect-final", input.ExpectFinal)
	appendOptionalBoolArg(&args, "--light-context", input.LightContext)
	if input.TimeoutMs != nil && *input.TimeoutMs > 0 {
		args = append(args, "--timeout", strconv.FormatInt(*input.TimeoutMs, 10))
	}
	appendOptionalStringArg(&args, "--session", input.SessionTarget)
	if input.ClearSessionKey {
		args = append(args, "--clear-session-key")
	} else {
		appendOptionalStringArg(&args, "--session-key", input.SessionKey)
	}
	appendOptionalStringArg(&args, "--wake", input.WakeMode)
	if input.Announce != nil {
		if *input.Announce {
			args = append(args, "--announce")
		} else {
			args = append(args, "--no-deliver")
		}
	}
	appendOptionalStringArg(&args, "--channel", input.DeliveryChannel)
	appendOptionalStringArg(&args, "--to", input.DeliveryTo)
	appendOptionalStringArg(&args, "--account", input.DeliveryAccountID)
	if input.BestEffortDeliver != nil {
		if *input.BestEffortDeliver {
			args = append(args, "--best-effort-deliver")
		} else {
			args = append(args, "--no-best-effort-deliver")
		}
	}
	if input.Exact != nil && *input.Exact {
		args = append(args, "--exact")
	}
	if input.DeleteAfterRun != nil && *input.DeleteAfterRun {
		args = append(args, "--delete-after-run")
	}
	if input.KeepAfterRun != nil && *input.KeepAfterRun {
		args = append(args, "--keep-after-run")
	}
	if input.Enabled != nil {
		if *input.Enabled {
			args = append(args, "--enable")
		} else {
			args = append(args, "--disable")
		}
	}
	return args, nil
}

func appendScheduleArgs(args *[]string, kind, cronExpr, every, at, timezone string) error {
	switch strings.TrimSpace(kind) {
	case "cron":
		if strings.TrimSpace(cronExpr) == "" {
			return fmt.Errorf("openclaw cron expression is required")
		}
		*args = append(*args, "--cron", strings.TrimSpace(cronExpr))
	case "every":
		if strings.TrimSpace(every) == "" {
			return fmt.Errorf("openclaw every duration is required")
		}
		*args = append(*args, "--every", strings.TrimSpace(every))
	case "at":
		if strings.TrimSpace(at) == "" {
			return fmt.Errorf("openclaw at value is required")
		}
		*args = append(*args, "--at", strings.TrimSpace(at))
	default:
		return fmt.Errorf("unsupported openclaw schedule kind")
	}
	if strings.TrimSpace(timezone) != "" {
		*args = append(*args, "--tz", strings.TrimSpace(timezone))
	}
	return nil
}

func appendScheduleArgsForUpdate(args *[]string, kind string, cronExpr, every, at, timezone *string) error {
	switch kind {
	case "cron":
		appendOptionalStringArg(args, "--cron", cronExpr)
	case "every":
		appendOptionalStringArg(args, "--every", every)
	case "at":
		appendOptionalStringArg(args, "--at", at)
	case "":
	default:
		return fmt.Errorf("unsupported openclaw schedule kind")
	}
	appendOptionalStringArg(args, "--tz", timezone)
	return nil
}

func appendPayloadArgs(args *[]string, message, systemEvent string) {
	if message != "" {
		*args = append(*args, "--message", message)
	}
	if systemEvent != "" {
		*args = append(*args, "--system-event", systemEvent)
	}
}

func appendCommonArgs(
	args *[]string,
	model string,
	thinking string,
	expectFinal bool,
	lightContext bool,
	timeoutMs int64,
	sessionTarget string,
	sessionKey string,
	wakeMode string,
) {
	if model != "" {
		*args = append(*args, "--model", model)
	}
	if thinking != "" {
		*args = append(*args, "--thinking", thinking)
	}
	if expectFinal {
		*args = append(*args, "--expect-final")
	}
	if lightContext {
		*args = append(*args, "--light-context")
	}
	if timeoutMs > 0 {
		*args = append(*args, "--timeout", strconv.FormatInt(timeoutMs, 10))
	}
	if sessionTarget != "" {
		*args = append(*args, "--session", sessionTarget)
	}
	if sessionKey != "" {
		*args = append(*args, "--session-key", sessionKey)
	}
	if wakeMode != "" {
		*args = append(*args, "--wake", wakeMode)
	}
}

func appendDeliveryArgs(args *[]string, announce bool, deliveryMode, channel, to, accountID string, bestEffort bool) {
	if announce || deliveryMode == "announce" || deliveryMode == "" {
		*args = append(*args, "--announce")
	}
	if channel != "" {
		*args = append(*args, "--channel", channel)
	}
	if to != "" {
		*args = append(*args, "--to", to)
	}
	if accountID != "" {
		*args = append(*args, "--account", accountID)
	}
	if bestEffort {
		*args = append(*args, "--best-effort-deliver")
	}
}

func appendOptionalStringArg(args *[]string, flag string, value *string) {
	if value == nil {
		return
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return
	}
	*args = append(*args, flag, trimmed)
}

func appendOptionalBoolArg(args *[]string, flag string, value *bool) {
	if value == nil {
		return
	}
	if *value {
		*args = append(*args, flag)
		return
	}
	switch flag {
	case "--light-context":
		*args = append(*args, "--no-light-context")
	}
}

func stringPtr(value string) *string {
	return &value
}

func derefTrim(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func parseAgentIDFromSessionKey(sessionKey string) string {
	parts := strings.Split(strings.TrimSpace(sessionKey), ":")
	if len(parts) < 2 || parts[0] != "agent" {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

func parseJobIDFromSessionKey(sessionKey string) string {
	parts := strings.Split(strings.TrimSpace(sessionKey), ":")
	if len(parts) < 4 || parts[0] != "agent" || parts[2] != "cron" {
		return ""
	}
	return strings.TrimSpace(parts[3])
}

// buildCronForwardEvents translates cron gateway frames into the standard
// chat:* events already consumed by the shared frontend chat store.
// buildCronForwardEvents 把 Cron 网关帧翻译成前端已经消费的 chat:* 事件，避免页面再维护第二套状态机。
func buildCronForwardEvents(
	conversationID int64,
	sessionKey string,
	runID string,
	state *cronForwardState,
	event string,
	payload json.RawMessage,
) []cronForwardedEvent {
	if conversationID <= 0 || state == nil {
		return nil
	}

	appendStart := func(items []cronForwardedEvent) []cronForwardedEvent {
		// Emit a synthetic start only once per run so all later deltas keep writing
		// into the same assistant placeholder on the page.
		// 每轮运行只发送一次合成 start，保证后续增量都落在同一条助手占位消息上。
		if state.Started {
			return items
		}
		state.Started = true
		return append(items, cronForwardedEvent{
			Name: chatservice.EventChatStart,
			Payload: chatservice.ChatStartEvent{
				ChatEvent: buildCronChatEvent(conversationID, state),
				Status:    "streaming",
			},
		})
	}

	switch strings.TrimSpace(event) {
	case "agent":
		var frame struct {
			RunID      string          `json:"runId"`
			SessionKey string          `json:"sessionKey"`
			Stream     string          `json:"stream"`
			Data       json.RawMessage `json:"data"`
		}
		if json.Unmarshal(payload, &frame) != nil {
			return nil
		}
		if !matchSessionKey(frame.SessionKey, sessionKey) {
			return nil
		}
		if trimmedRunID := strings.TrimSpace(runID); trimmedRunID != "" && strings.TrimSpace(frame.RunID) != "" && strings.TrimSpace(frame.RunID) != trimmedRunID {
			return nil
		}

		switch strings.TrimSpace(frame.Stream) {
		case "assistant":
			var data struct {
				Delta string `json:"delta"`
				Text  string `json:"text"`
			}
			if json.Unmarshal(frame.Data, &data) != nil {
				return nil
			}
			delta := firstNonEmptyString(data.Delta, data.Text)
			if delta == "" {
				return nil
			}
			items := appendStart(nil)
			return append(items, cronForwardedEvent{
				Name: chatservice.EventChatChunk,
				Payload: chatservice.ChatChunkEvent{
					ChatEvent: buildCronChatEvent(conversationID, state),
					Delta:     delta,
				},
			})
		case "thinking":
			var data struct {
				Delta    string `json:"delta"`
				Text     string `json:"text"`
				NewBlock bool   `json:"newBlock"`
			}
			if json.Unmarshal(frame.Data, &data) != nil {
				return nil
			}
			delta := firstNonEmptyString(data.Delta, data.Text)
			if delta == "" {
				return nil
			}
			items := appendStart(nil)
			return append(items, cronForwardedEvent{
				Name: chatservice.EventChatThinking,
				Payload: chatservice.ChatThinkingEvent{
					ChatEvent: buildCronChatEvent(conversationID, state),
					Delta:     delta,
					NewBlock:  data.NewBlock,
				},
			})
		case "retrieval":
			var data struct {
				Items     []map[string]any `json:"items"`
				Results   []map[string]any `json:"results"`
				Documents []map[string]any `json:"documents"`
			}
			if json.Unmarshal(frame.Data, &data) != nil {
				return nil
			}
			rawItems := data.Items
			if len(rawItems) == 0 {
				rawItems = data.Results
			}
			if len(rawItems) == 0 {
				rawItems = data.Documents
			}
			items := make([]chatservice.RetrievalItem, 0, len(rawItems))
			for _, item := range rawItems {
				content := firstNonEmptyString(stringFromAny(item["content"]), stringFromAny(item["text"]), stringFromAny(item["snippet"]))
				if content == "" {
					continue
				}
				source := "knowledge"
				if strings.TrimSpace(stringFromAny(item["source"])) == "memory" {
					source = "memory"
				}
				items = append(items, chatservice.RetrievalItem{
					Source:  source,
					Content: content,
					Score:   float64FromAny(item["score"]),
				})
			}
			if len(items) == 0 {
				return nil
			}
			forwarded := appendStart(nil)
			return append(forwarded, cronForwardedEvent{
				Name: chatservice.EventChatRetrieval,
				Payload: chatservice.ChatRetrievalEvent{
					ChatEvent: buildCronChatEvent(conversationID, state),
					Items:     items,
				},
			})
		case "tool":
			var data struct {
				Phase      string          `json:"phase"`
				ToolCallID string          `json:"toolCallId"`
				Name       string          `json:"name"`
				Args       json.RawMessage `json:"args"`
				Result     json.RawMessage `json:"result"`
				Meta       string          `json:"meta"`
				Error      string          `json:"error"`
				Message    string          `json:"message"`
			}
			if json.Unmarshal(frame.Data, &data) != nil {
				return nil
			}
			phase := strings.TrimSpace(data.Phase)
			if phase != "start" && phase != "result" {
				return nil
			}
			forwarded := appendStart(nil)
			resultJSON := ""
			if len(data.Result) > 0 && string(data.Result) != "null" {
				resultJSON = string(data.Result)
			} else {
				resultJSON = firstNonEmptyString(data.Meta, data.Error, data.Message)
			}
			argsJSON := ""
			if len(data.Args) > 0 && string(data.Args) != "null" {
				argsJSON = string(data.Args)
			}
			return append(forwarded, cronForwardedEvent{
				Name: chatservice.EventChatTool,
				Payload: chatservice.ChatToolEvent{
					ChatEvent:  buildCronChatEvent(conversationID, state),
					Type:       mapToolPhaseToEventType(phase),
					ToolCallID: strings.TrimSpace(data.ToolCallID),
					ToolName:   strings.TrimSpace(data.Name),
					ArgsJSON:   argsJSON,
					ResultJSON: resultJSON,
				},
			})
		case "lifecycle":
			var data struct {
				Phase   string `json:"phase"`
				Error   string `json:"error"`
				Message string `json:"message"`
			}
			if json.Unmarshal(frame.Data, &data) != nil {
				return nil
			}
			switch strings.TrimSpace(data.Phase) {
			case "error":
				state.Finished = true
				return []cronForwardedEvent{{
					Name: chatservice.EventChatError,
					Payload: chatservice.ChatErrorEvent{
						ChatEvent: buildCronChatEvent(conversationID, state),
						Status:    "error",
						ErrorKey:  "error.chat_generation_failed",
						ErrorData: map[string]any{"Error": firstNonEmptyString(data.Error, data.Message)},
					},
				}}
			case "end":
				state.Finished = true
				if !state.Started {
					return nil
				}
				return []cronForwardedEvent{{
					Name: chatservice.EventChatComplete,
					Payload: chatservice.ChatCompleteEvent{
						ChatEvent:    buildCronChatEvent(conversationID, state),
						Status:       "success",
						FinishReason: "stop",
					},
				}}
			}
		}
	case "chat":
		var frame struct {
			RunID        string `json:"runId"`
			SessionKey   string `json:"sessionKey"`
			State        string `json:"state"`
			ErrorMessage string `json:"errorMessage"`
			Message      struct {
				StopReason string `json:"stopReason"`
			} `json:"message"`
		}
		if json.Unmarshal(payload, &frame) != nil {
			return nil
		}
		if !matchSessionKey(frame.SessionKey, sessionKey) {
			return nil
		}
		if trimmedRunID := strings.TrimSpace(runID); trimmedRunID != "" && strings.TrimSpace(frame.RunID) != "" && strings.TrimSpace(frame.RunID) != trimmedRunID {
			return nil
		}

		switch strings.TrimSpace(frame.State) {
		case "error":
			state.Finished = true
			return []cronForwardedEvent{{
				Name: chatservice.EventChatError,
				Payload: chatservice.ChatErrorEvent{
					ChatEvent: buildCronChatEvent(conversationID, state),
					Status:    "error",
					ErrorKey:  "error.chat_generation_failed",
					ErrorData: map[string]any{"Error": frame.ErrorMessage},
				},
			}}
		case "aborted":
			state.Finished = true
			if !state.Started {
				return nil
			}
			return []cronForwardedEvent{{
				Name: chatservice.EventChatStopped,
				Payload: chatservice.ChatStoppedEvent{
					ChatEvent: buildCronChatEvent(conversationID, state),
					Status:    "cancelled",
				},
			}}
		case "final":
			state.Finished = true
			if !state.Started {
				return nil
			}
			return []cronForwardedEvent{{
				Name: chatservice.EventChatComplete,
				Payload: chatservice.ChatCompleteEvent{
					ChatEvent:    buildCronChatEvent(conversationID, state),
					Status:       "success",
					FinishReason: firstNonEmptyString(frame.Message.StopReason, "stop"),
				},
			}}
		}
	}

	return nil
}

// buildCronChatEvent creates the common chat-event metadata for cron forwarding.
// buildCronChatEvent 构造 Cron 转发事件公用的聊天元数据。
func buildCronChatEvent(conversationID int64, state *cronForwardState) chatservice.ChatEvent {
	if state == nil {
		return chatservice.ChatEvent{ConversationID: conversationID}
	}
	state.Seq++
	return chatservice.ChatEvent{
		ConversationID: conversationID,
		RequestID:      state.RequestID,
		Seq:            state.Seq,
		MessageID:      state.MessageID,
		Ts:             time.Now().UnixMilli(),
	}
}

func buildCronForwardRequestID(sessionKey string, runID string) string {
	trimmedSessionKey := strings.TrimSpace(sessionKey)
	trimmedRunID := strings.TrimSpace(runID)
	if trimmedRunID == "" {
		return "openclaw-cron:" + trimmedSessionKey
	}
	return "openclaw-cron:" + trimmedSessionKey + ":" + trimmedRunID
}

func buildCronForwardMessageID(conversationID int64, sessionKey string) int64 {
	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(strings.TrimSpace(sessionKey)))
	return -conversationID*1_000_000 - int64(hasher.Sum32()%1_000_000) - 1
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func mapToolPhaseToEventType(phase string) string {
	if strings.TrimSpace(phase) == "start" {
		return "call"
	}
	return "result"
}

func stringFromAny(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", value))
	}
}

func float64FromAny(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case int32:
		return float64(typed)
	case json.Number:
		parsed, err := typed.Float64()
		if err == nil {
			return parsed
		}
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		if err == nil {
			return parsed
		}
	}
	return 0
}

func matchSessionKey(got string, expected string) bool {
	got = strings.TrimSpace(got)
	expected = strings.TrimSpace(expected)
	if got == "" || expected == "" {
		return false
	}
	return got == expected || strings.HasSuffix(got, ":"+expected)
}

func buildCronExternalID(jobID string, runPart string) string {
	trimmedJobID := strings.TrimSpace(jobID)
	trimmedRunPart := strings.TrimSpace(runPart)
	if trimmedRunPart == "" {
		return fmt.Sprintf("openclaw-cron:%s", trimmedJobID)
	}
	return fmt.Sprintf("openclaw-cron:%s:%s", trimmedJobID, trimmedRunPart)
}

func buildCronConversationSource(jobID string) string {
	trimmedJobID := strings.TrimSpace(jobID)
	if trimmedJobID == "" {
		return conversations.ConversationSourceOpenClawCron
	}
	return conversations.ConversationSourceOpenClawCron + ":" + trimmedJobID
}

func parseManualTriggerAtMs(externalID string) int64 {
	trimmedExternalID := strings.TrimSpace(externalID)
	if trimmedExternalID == "" {
		return 0
	}
	parts := strings.Split(trimmedExternalID, ":")
	if len(parts) < 2 {
		return 0
	}
	if parts[len(parts)-2] != "manual" {
		return 0
	}
	triggerAtMs, err := strconv.ParseInt(strings.TrimSpace(parts[len(parts)-1]), 10, 64)
	if err != nil || triggerAtMs <= 0 {
		return 0
	}
	return triggerAtMs
}

func buildCronConversationName(jobName string, primaryTimeMs int64, fallbackTimeMs int64) string {
	trimmedJobName := strings.TrimSpace(jobName)
	if trimmedJobName == "" {
		trimmedJobName = openClawCronDefaultConversationName
	}

	runTimeMs := primaryTimeMs
	if runTimeMs <= 0 {
		runTimeMs = fallbackTimeMs
	}
	if runTimeMs <= 0 {
		runTimeMs = time.Now().UnixMilli()
	}
	return fmt.Sprintf("%s / %s", trimmedJobName, time.UnixMilli(runTimeMs).Local().Format("2006-01-02 15:04:05"))
}

func chooseRunTimestamp(primaryTimeMs int64, fallbackTimeMs int64) int64 {
	if primaryTimeMs > 0 {
		return primaryTimeMs
	}
	if fallbackTimeMs > 0 {
		return fallbackTimeMs
	}
	return time.Now().UnixMilli()
}

func parseRunNowResult(output []byte, triggeredAt time.Time) (*OpenClawCronRunNowResult, error) {
	var payload struct {
		OK       bool   `json:"ok"`
		Enqueued bool   `json:"enqueued"`
		Ran      bool   `json:"ran"`
		RunID    string `json:"runId"`
	}
	if err := json.Unmarshal(output, &payload); err != nil {
		return nil, fmt.Errorf("decode cron run result: %w", err)
	}
	return &OpenClawCronRunNowResult{
		RunID:       strings.TrimSpace(payload.RunID),
		TriggerAtMs: triggeredAt.UnixMilli(),
		Enqueued:    payload.Enqueued || payload.Ran,
	}, nil
}

func isCloseRunTimestamp(leftMs int64, rightMs int64) bool {
	if leftMs <= 0 || rightMs <= 0 {
		return false
	}
	diff := leftMs - rightMs
	if diff < 0 {
		diff = -diff
	}
	return diff <= conversationHistoryMatchWindowMs
}

func extractGatewayRunContext(event string, payload json.RawMessage) (string, string) {
	switch strings.TrimSpace(event) {
	case "agent", "chat":
	default:
		return "", ""
	}

	var frame struct {
		RunID      string `json:"runId"`
		SessionKey string `json:"sessionKey"`
	}
	if err := json.Unmarshal(payload, &frame); err != nil {
		return "", ""
	}
	return strings.TrimSpace(frame.RunID), strings.TrimSpace(frame.SessionKey)
}

func mergeHistoryItems(
	runItems []OpenClawCronHistoryListItem,
	conversationItems []OpenClawCronHistoryListItem,
	pendingItems []OpenClawCronHistoryListItem,
) []OpenClawCronHistoryListItem {
	merged := make(map[string]OpenClawCronHistoryListItem)
	order := make([]string, 0, len(runItems)+len(conversationItems)+len(pendingItems))

	upsert := func(item OpenClawCronHistoryListItem) {
		key := historyMergeKey(item)
		if key == "" {
			return
		}
		existing, exists := merged[key]
		if !exists {
			merged[key] = item
			order = append(order, key)
			return
		}
		merged[key] = preferHistoryItem(existing, item)
	}

	for _, item := range conversationItems {
		upsert(item)
	}
	for _, item := range pendingItems {
		upsert(item)
	}
	for _, item := range runItems {
		upsert(item)
	}

	out := make([]OpenClawCronHistoryListItem, 0, len(order))
	for _, key := range order {
		out = append(out, merged[key])
	}
	slices.SortFunc(out, func(left, right OpenClawCronHistoryListItem) int {
		switch {
		case left.RunAtMs > right.RunAtMs:
			return -1
		case left.RunAtMs < right.RunAtMs:
			return 1
		default:
			return strings.Compare(right.RunID, left.RunID)
		}
	})
	return out
}

func historyMergeKey(item OpenClawCronHistoryListItem) string {
	if sessionKey := strings.TrimSpace(item.SessionKey); sessionKey != "" {
		return "session:" + sessionKey
	}
	if runID := strings.TrimSpace(item.RunID); runID != "" {
		return "run:" + runID
	}
	if item.ConversationID > 0 {
		return fmt.Sprintf("conversation:%d", item.ConversationID)
	}
	return ""
}

func preferHistoryItem(existing OpenClawCronHistoryListItem, incoming OpenClawCronHistoryListItem) OpenClawCronHistoryListItem {
	preferred := existing
	if historySourceRank(incoming.Source) >= historySourceRank(existing.Source) {
		preferred = incoming
	}

	if preferred.ConversationID == 0 {
		if existing.ConversationID > 0 {
			preferred.ConversationID = existing.ConversationID
		}
		if preferred.ConversationID == 0 && incoming.ConversationID > 0 {
			preferred.ConversationID = incoming.ConversationID
		}
	}
	if preferred.SessionID == "" {
		if existing.SessionID != "" {
			preferred.SessionID = existing.SessionID
		} else {
			preferred.SessionID = incoming.SessionID
		}
	}
	if preferred.SessionKey == "" {
		if existing.SessionKey != "" {
			preferred.SessionKey = existing.SessionKey
		} else {
			preferred.SessionKey = incoming.SessionKey
		}
	}
	if preferred.RunID == "" {
		if existing.RunID != "" {
			preferred.RunID = existing.RunID
		} else {
			preferred.RunID = incoming.RunID
		}
	}
	if preferred.Name == "" {
		if existing.Name != "" {
			preferred.Name = existing.Name
		} else {
			preferred.Name = incoming.Name
		}
	}
	if preferred.RunAtMs <= 0 {
		if existing.RunAtMs > 0 {
			preferred.RunAtMs = existing.RunAtMs
		} else {
			preferred.RunAtMs = incoming.RunAtMs
		}
	}
	if preferred.DurationMs <= 0 {
		if existing.DurationMs > 0 {
			preferred.DurationMs = existing.DurationMs
		} else {
			preferred.DurationMs = incoming.DurationMs
		}
	}
	if preferred.TriggerType == "" {
		if existing.TriggerType != "" {
			preferred.TriggerType = existing.TriggerType
		} else {
			preferred.TriggerType = incoming.TriggerType
		}
	}
	return preferred
}

// deriveManualConversationHistoryState infers a terminal manual-run state from
// the local conversation when the OpenClaw run log is not available.
func deriveManualConversationHistoryState(
	runAtMs int64,
	updatedAtMs int64,
	_ string,
	hasActiveGeneration bool,
) (string, int64) {
	state := manualConversationHistoryState{
		Status:     openClawCronHistoryStatusRunning,
		DurationMs: 0,
	}
	// Manual runs that go through the local chat pipeline can finish before a session key
	// is ever written back. Once generation is no longer active and the conversation moved
	// forward in time, the history list should treat the run as completed.
	if hasActiveGeneration || runAtMs <= 0 || updatedAtMs <= runAtMs {
		return state.Status, state.DurationMs
	}
	state.Status = openClawCronHistoryStatusSuccess
	state.DurationMs = updatedAtMs - runAtMs
	return state.Status, state.DurationMs
}

// isTerminalHistoryItem skips run-log enrichment only when the item already has
// enough durable data for the history list.
func isTerminalHistoryItem(item OpenClawCronHistoryListItem) bool {
	if strings.TrimSpace(item.Status) == "" || strings.TrimSpace(item.Status) == openClawCronHistoryStatusRunning {
		return false
	}
	return strings.TrimSpace(item.SessionKey) != "" && item.DurationMs > 0
}

func normalizeHistoryTriggerType(action string, source string) string {
	normalizedAction := strings.ToLower(strings.TrimSpace(action))
	switch normalizedAction {
	case "manual", "run_now":
		return "manual"
	case "schedule", "scheduled", "cron":
		return "schedule"
	}

	switch strings.TrimSpace(source) {
	case OpenClawCronHistorySourceRunLog:
		return "schedule"
	case OpenClawCronHistorySourcePending, OpenClawCronHistorySourceConversation:
		return "manual"
	default:
		return "manual"
	}
}

func historySourceRank(source string) int {
	switch strings.TrimSpace(source) {
	case OpenClawCronHistorySourceConversation:
		return 3
	case OpenClawCronHistorySourcePending:
		return 2
	case OpenClawCronHistorySourceRunLog:
		return 1
	default:
		return 0
	}
}

func readTranscriptMessages(filePath string) ([]OpenClawCronTranscriptMessage, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	out := make([]OpenClawCronTranscriptMessage, 0)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var envelope map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			continue
		}
		var itemType string
		_ = json.Unmarshal(envelope["type"], &itemType)
		if itemType != "message" {
			continue
		}

		var raw openClawTranscriptLine
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}
		out = append(out, raw.toDTO())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

type openClawJobStoreItem struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	Enabled       bool   `json:"enabled"`
	AgentID       string `json:"agentId"`
	CreatedAtMs   int64  `json:"createdAtMs"`
	UpdatedAtMs   int64  `json:"updatedAtMs"`
	SessionTarget string `json:"sessionTarget"`
	SessionKey    string `json:"sessionKey"`
	WakeMode      string `json:"wakeMode"`
	Schedule      struct {
		Kind     string `json:"kind"`
		CronExpr string `json:"cronExpr"`
		EveryMs  int64  `json:"everyMs"`
		At       string `json:"at"`
		Timezone string `json:"tz"`
		Exact    bool   `json:"exact"`
	} `json:"schedule"`
	Payload struct {
		Kind         string `json:"kind"`
		AgentID      string `json:"agentId"`
		Message      string `json:"message"`
		SystemEvent  string `json:"systemEvent"`
		Model        string `json:"model"`
		Thinking     string `json:"thinking"`
		ExpectFinal  bool   `json:"expectFinal"`
		LightContext bool   `json:"lightContext"`
		TimeoutMs    int64  `json:"timeoutMs"`
	} `json:"payload"`
	Delivery struct {
		Mode      string `json:"mode"`
		Channel   string `json:"channel"`
		To        string `json:"to"`
		AccountID string `json:"accountId"`
		Announce  bool   `json:"announce"`
		// BestEffort reads the native OpenClaw persisted key.
		// BestEffort 读取 OpenClaw jobs.json 的原生字段 bestEffort。
		BestEffort bool `json:"bestEffort"`
		// BestEffortDeliver keeps backward compatibility with earlier bridge payloads.
		// BestEffortDeliver 兼容旧桥接层可能使用的 bestEffortDeliver 字段。
		BestEffortDeliver bool `json:"bestEffortDeliver"`
		DeleteAfterRun    bool `json:"deleteAfterRun"`
		KeepAfterRun      bool `json:"keepAfterRun"`
	} `json:"delivery"`
	State struct {
		NextRunAtMs        int64  `json:"nextRunAtMs"`
		LastRunAtMs        int64  `json:"lastRunAtMs"`
		LastStatus         string `json:"lastStatus"`
		LastRunStatus      string `json:"lastRunStatus"`
		LastError          string `json:"lastError"`
		LastDurationMs     int64  `json:"lastDurationMs"`
		LastDeliveryStatus string `json:"lastDeliveryStatus"`
	} `json:"state"`
}

func flattenJob(item openClawJobStoreItem) OpenClawCronJob {
	return OpenClawCronJob{
		ID:                 item.ID,
		Name:               item.Name,
		Description:        item.Description,
		Enabled:            item.Enabled,
		CreatedAtMs:        item.CreatedAtMs,
		UpdatedAtMs:        item.UpdatedAtMs,
		AgentID:            firstNonEmpty(item.AgentID, item.Payload.AgentID),
		SessionTarget:      item.SessionTarget,
		SessionKey:         item.SessionKey,
		WakeMode:           item.WakeMode,
		ScheduleKind:       item.Schedule.Kind,
		CronExpr:           item.Schedule.CronExpr,
		EveryMs:            item.Schedule.EveryMs,
		AtISO:              item.Schedule.At,
		Timezone:           item.Schedule.Timezone,
		Exact:              item.Schedule.Exact,
		PayloadKind:        item.Payload.Kind,
		Message:            item.Payload.Message,
		SystemEvent:        item.Payload.SystemEvent,
		Model:              item.Payload.Model,
		Thinking:           item.Payload.Thinking,
		ExpectFinal:        item.Payload.ExpectFinal,
		LightContext:       item.Payload.LightContext,
		TimeoutMs:          item.Payload.TimeoutMs,
		DeliveryMode:       firstNonEmpty(item.Delivery.Mode, "announce"),
		DeliveryChannel:    item.Delivery.Channel,
		DeliveryTo:         item.Delivery.To,
		DeliveryAccountID:  item.Delivery.AccountID,
		Announce:           item.Delivery.Mode == "announce" || item.Delivery.Announce,
		DeliveryTargetMode: inferCronDeliveryTargetMode(item.Delivery.To),
		DeliveryTargetID:   item.Delivery.To,
		// Prefer the native persisted key, but keep old snapshots readable as well.
		// 优先读取原生持久化字段，同时兼容旧快照中的历史字段。
		BestEffortDeliver:  item.Delivery.BestEffort || item.Delivery.BestEffortDeliver,
		DeleteAfterRun:     item.Delivery.DeleteAfterRun,
		KeepAfterRun:       item.Delivery.KeepAfterRun,
		NextRunAtMs:        item.State.NextRunAtMs,
		LastRunAtMs:        item.State.LastRunAtMs,
		LastStatus:         firstNonEmpty(item.State.LastStatus, item.State.LastRunStatus),
		LastError:          item.State.LastError,
		LastDurationMs:     item.State.LastDurationMs,
		LastDeliveryStatus: item.State.LastDeliveryStatus,
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func inferCronDeliveryTargetMode(deliveryTo string) string {
	if strings.TrimSpace(deliveryTo) != "" {
		return openClawCronDeliveryTargetModeTargetID
	}
	return openClawCronDeliveryTargetModeLastActive
}

type openClawRunEntryCLI struct {
	TimestampMs    int64  `json:"ts"`
	JobID          string `json:"jobId"`
	Action         string `json:"action"`
	Status         string `json:"status"`
	Error          string `json:"error"`
	Summary        string `json:"summary"`
	RunAtMs        int64  `json:"runAtMs"`
	DurationMs     int64  `json:"durationMs"`
	NextRunAtMs    int64  `json:"nextRunAtMs"`
	Model          string `json:"model"`
	Provider       string `json:"provider"`
	DeliveryStatus string `json:"deliveryStatus"`
	SessionID      string `json:"sessionId"`
	SessionKey     string `json:"sessionKey"`
}

func (r openClawRunEntryCLI) toDTO() OpenClawCronRunEntry {
	return OpenClawCronRunEntry{
		TimestampMs:    r.TimestampMs,
		JobID:          r.JobID,
		Action:         r.Action,
		Status:         r.Status,
		Error:          r.Error,
		Summary:        r.Summary,
		RunAtMs:        r.RunAtMs,
		DurationMs:     r.DurationMs,
		NextRunAtMs:    r.NextRunAtMs,
		Model:          r.Model,
		Provider:       r.Provider,
		DeliveryStatus: r.DeliveryStatus,
		SessionID:      r.SessionID,
		SessionKey:     r.SessionKey,
	}
}

type openClawTranscriptLine struct {
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Message   struct {
		Role       string            `json:"role"`
		Content    []json.RawMessage `json:"content"`
		Provider   string            `json:"provider"`
		Model      string            `json:"model"`
		StopReason string            `json:"stopReason"`
	} `json:"message"`
}

func (l openClawTranscriptLine) toDTO() OpenClawCronTranscriptMessage {
	out := OpenClawCronTranscriptMessage{
		ID:         l.ID,
		Role:       l.Message.Role,
		Timestamp:  l.Timestamp,
		Provider:   l.Message.Provider,
		Model:      l.Message.Model,
		StopReason: l.Message.StopReason,
		Blocks:     make([]OpenClawCronMessageBlock, 0, len(l.Message.Content)),
	}
	var textParts []string
	for _, raw := range l.Message.Content {
		block := parseTranscriptBlock(raw)
		out.Blocks = append(out.Blocks, block)
		switch block.Type {
		case "text":
			if block.Text != "" {
				textParts = append(textParts, block.Text)
			}
		case "thinking":
			if block.Thinking != "" {
				textParts = append(textParts, block.Thinking)
			}
		}
	}
	out.ContentText = strings.TrimSpace(strings.Join(textParts, "\n\n"))
	return out
}

func parseTranscriptBlock(raw json.RawMessage) OpenClawCronMessageBlock {
	out := OpenClawCronMessageBlock{Raw: raw}
	var base struct {
		Type       string          `json:"type"`
		Text       string          `json:"text"`
		Thinking   string          `json:"thinking"`
		Name       string          `json:"name"`
		ToolCallID string          `json:"toolCallId"`
		Args       json.RawMessage `json:"args"`
		Content    json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &base); err != nil {
		return out
	}
	out.Type = base.Type
	out.Text = base.Text
	out.Thinking = base.Thinking
	out.Name = base.Name
	out.ToolCallID = base.ToolCallID
	if len(base.Args) > 0 {
		out.ArgsJSON = string(base.Args)
	}
	if len(base.Content) > 0 {
		out.ResultJSON = string(base.Content)
	}
	return out
}
