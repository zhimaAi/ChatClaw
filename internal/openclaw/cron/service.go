package openclawcron

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	openclawroot "chatclaw/internal/openclaw"
	openclawagents "chatclaw/internal/openclaw/agents"
	openclawruntime "chatclaw/internal/openclaw/runtime"
	"chatclaw/internal/sqlite"

	"github.com/uptrace/bun"
	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	// openClawCommandName is the CLI entrypoint used to operate the embedded gateway.
	// openClawCommandName 是调用 OpenClaw CLI 的命令名。
	openClawCommandName = "openclaw"
	// defaultOpenClawCronListLimit keeps UI list pagination stable with OpenClaw defaults.
	// defaultOpenClawCronListLimit 与 OpenClaw 默认分页保持一致，避免前后端理解不一致。
	defaultOpenClawCronListLimit = 50
)

// OpenClawCronService wraps OpenClaw-native cron management for the Wails frontend.
// OpenClawCronService 为前端提供独立于 ChatClaw 的 OpenClaw Cron 管理能力。
type OpenClawCronService struct {
	app       *application.App
	manager   *openclawruntime.Manager
	agentsSvc *openclawagents.OpenClawAgentsService
	watchMu   sync.Mutex
	watchSeq  atomic.Int64
	watches   map[string]string
}

type cronConversationRecord struct {
	ID                 int64  `bun:"id"`
	AgentID            int64  `bun:"agent_id"`
	OpenClawSessionKey string `bun:"openclaw_session_key"`
}

func NewOpenClawCronService(
	app *application.App,
	manager *openclawruntime.Manager,
	agentsSvc *openclawagents.OpenClawAgentsService,
) *OpenClawCronService {
	return &OpenClawCronService{
		app:       app,
		manager:   manager,
		agentsSvc: agentsSvc,
		watches:   make(map[string]string),
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

// CreateJob creates a cron job through the OpenClaw CLI.
// 通过 OpenClaw CLI 新建 Cron 任务，确保写入格式完全与原生一致。
func (s *OpenClawCronService) CreateJob(input CreateOpenClawCronJobInput) (*OpenClawCronJob, error) {
	args, err := buildCreateArgs(input)
	if err != nil {
		return nil, err
	}
	var raw openClawJobStoreItem
	if err := s.runCLIJSON(args, &raw); err != nil {
		return nil, err
	}
	job := flattenJob(raw)
	agentNameMap, _ := s.agentNameMap()
	job.AgentName = agentNameMap[job.AgentID]
	return &job, nil
}

// UpdateJob patches a cron job through the OpenClaw CLI.
// 编辑任务时仅传递被修改的字段，避免用默认值误伤原配置。
func (s *OpenClawCronService) UpdateJob(jobID string, input UpdateOpenClawCronJobInput) (*OpenClawCronJob, error) {
	args, err := buildUpdateArgs(jobID, input)
	if err != nil {
		return nil, err
	}
	if len(args) == 3 {
		return s.findJobByID(jobID)
	}
	var raw openClawJobStoreItem
	if err := s.runCLIJSON(args, &raw); err != nil {
		return nil, err
	}
	job := flattenJob(raw)
	agentNameMap, _ := s.agentNameMap()
	job.AgentName = agentNameMap[job.AgentID]
	return &job, nil
}

func (s *OpenClawCronService) DeleteJob(jobID string) error {
	var out map[string]any
	return s.runCLIJSON([]string{"cron", "rm", strings.TrimSpace(jobID)}, &out)
}

func (s *OpenClawCronService) EnableJob(jobID string) (*OpenClawCronJob, error) {
	var raw openClawJobStoreItem
	if err := s.runCLIJSON([]string{"cron", "enable", strings.TrimSpace(jobID)}, &raw); err != nil {
		return nil, err
	}
	job := flattenJob(raw)
	agentNameMap, _ := s.agentNameMap()
	job.AgentName = agentNameMap[job.AgentID]
	return &job, nil
}

func (s *OpenClawCronService) DisableJob(jobID string) (*OpenClawCronJob, error) {
	var raw openClawJobStoreItem
	if err := s.runCLIJSON([]string{"cron", "disable", strings.TrimSpace(jobID)}, &raw); err != nil {
		return nil, err
	}
	job := flattenJob(raw)
	agentNameMap, _ := s.agentNameMap()
	job.AgentName = agentNameMap[job.AgentID]
	return &job, nil
}

// RunJobNow enqueues a manual run and returns the generated run id.
// 手动运行返回 OpenClaw 原生 runId，前端可据此决定是否进入实时刷新。
func (s *OpenClawCronService) RunJobNow(jobID string) (string, error) {
	var out struct {
		RunID string `json:"runId"`
	}
	if err := s.runCLIJSON([]string{"cron", "run", strings.TrimSpace(jobID)}, &out); err != nil {
		return "", err
	}
	return strings.TrimSpace(out.RunID), nil
}

// ListRuns reads job run history via OpenClaw CLI first, then falls back to the JSONL log.
// 先走 OpenClaw CLI 读取历史，必要时回退到本地 JSONL，保证页面始终可见历史记录。
func (s *OpenClawCronService) ListRuns(jobID string, limit int) ([]OpenClawCronRunEntry, error) {
	if limit <= 0 {
		limit = defaultOpenClawCronListLimit
	}
	var result struct {
		Entries []openClawRunEntryCLI `json:"entries"`
	}
	err := s.runCLIJSON([]string{
		"cron", "runs",
		"--id", strings.TrimSpace(jobID),
		"--limit", strconv.Itoa(limit),
	}, &result)
	if err == nil {
		out := make([]OpenClawCronRunEntry, 0, len(result.Entries))
		for _, item := range result.Entries {
			out = append(out, item.toDTO())
		}
		return out, nil
	}
	return s.readRunEntriesFromFile(jobID, limit)
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
	if err := s.runCLIJSON([]string{"cron", "status"}, &out); err != nil {
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
	for left, right := 0, len(items)-1; left < right; left, right = left+1, right-1 {
		items[left], items[right] = items[right], items[left]
	}
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
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

	existing, err := s.findConversationBySessionKey(sessionKey)
	if err != nil {
		return 0, 0, err
	}
	if existing != nil {
		return existing.ID, existing.AgentID, nil
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
	if job, findErr := s.findJobByID(jobID); findErr == nil && strings.TrimSpace(job.Name) != "" {
		jobName = strings.TrimSpace(job.Name)
	}
	if jobName == "" {
		jobName = "OpenClaw Cron"
	}

	runTime := run.RunAtMs
	if runTime <= 0 {
		runTime = run.TimestampMs
	}
	name := fmt.Sprintf("%s / %s", jobName, time.UnixMilli(runTime).Local().Format("2006-01-02 15:04:05"))
	externalID := fmt.Sprintf("openclaw-cron:%s:%s", strings.TrimSpace(jobID), strings.TrimSpace(run.SessionID))

	createdID, err := s.insertConversationRecord(localAgentID, sessionKey, externalID, name)
	if err != nil {
		return 0, 0, err
	}
	return createdID, localAgentID, nil
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

// insertConversationRecord creates a read-only history conversation record for a cron run session.
// insertConversationRecord 为 Cron 历史创建本地会话记录，供只读嵌入页读取完整 OpenClaw 消息。
func (s *OpenClawCronService) insertConversationRecord(agentID int64, sessionKey, externalID, name string) (int64, error) {
	db, err := s.db()
	if err != nil {
		return 0, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	now := time.Now().UTC()
	res, err := db.ExecContext(ctx, `
INSERT INTO conversations (
	created_at,
	updated_at,
	agent_id,
	agent_type,
	name,
	external_id,
	last_message,
	is_pinned,
	llm_provider_id,
	llm_model_id,
	library_ids,
	enable_thinking,
	openclaw_session_key,
	chat_mode,
	team_type,
	dialogue_id,
	team_library_id
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		now,
		now,
		agentID,
		"openclaw",
		strings.TrimSpace(name),
		strings.TrimSpace(externalID),
		"",
		false,
		"",
		"",
		"[]",
		false,
		strings.TrimSpace(sessionKey),
		"task",
		"person",
		0,
		"",
	)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (s *OpenClawCronService) runCLIJSON(args []string, out any) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	fullArgs := append([]string{}, args...)
	fullArgs = append(fullArgs, "--json")
	gatewayURL := strings.TrimSpace(strings.Replace(s.manager.GatewayURL(), "http://", "ws://", 1)) + "/ws"
	token := strings.TrimSpace(s.manager.GatewayToken())
	if gatewayURL != "" {
		fullArgs = append(fullArgs, "--url", gatewayURL)
	}
	if token != "" {
		fullArgs = append(fullArgs, "--token", token)
	}

	// Force UTF-8 safe output decoding on Windows and keep the command pinned to the embedded gateway.
	// 在 Windows 下固定 UTF-8 输出，并显式指向内嵌 Gateway，避免误连用户全局 OpenClaw。
	cmd := exec.CommandContext(ctx, openClawCommandName, fullArgs...)
	cmd.Env = append(os.Environ(), "PYTHONUTF8=1")
	setCmdHideWindow(cmd)
	output, err := cmd.CombinedOutput()
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

func matchSessionKey(got string, expected string) bool {
	got = strings.TrimSpace(got)
	expected = strings.TrimSpace(expected)
	if got == "" || expected == "" {
		return false
	}
	return got == expected || strings.HasSuffix(got, ":"+expected)
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
		Mode              string `json:"mode"`
		Channel           string `json:"channel"`
		To                string `json:"to"`
		AccountID         string `json:"accountId"`
		Announce          bool   `json:"announce"`
		BestEffortDeliver bool   `json:"bestEffortDeliver"`
		DeleteAfterRun    bool   `json:"deleteAfterRun"`
		KeepAfterRun      bool   `json:"keepAfterRun"`
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
		AgentID:            item.Payload.AgentID,
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
		BestEffortDeliver:  item.Delivery.BestEffortDeliver,
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
