package scheduledtasks

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"chatclaw/internal/errs"
	"chatclaw/internal/services/channels"
	"chatclaw/internal/services/chat"
	"chatclaw/internal/services/conversations"
	"chatclaw/internal/sqlite"

	"github.com/uptrace/bun"
	"github.com/wailsapp/wails/v3/pkg/application"
)

type ScheduledTasksService struct {
	app                  *application.App
	db                   *bun.DB
	scheduler            *scheduler
	runnerDeps           runDependencies
	notificationGateway  *channels.Gateway
	notificationSender   taskNotificationSender
	createMu             sync.Mutex
	createRuns           map[string]*createCall
	recentCreates        map[string]recentCreate
	now                  func() time.Time
	duplicateWindow      time.Duration
	expirationSweepEvery time.Duration
	expirationSweepStop  chan struct{}
	expirationSweepDone  chan struct{}
}

const (
	// snapshotDisplayEmpty is the unified placeholder used when snapshot display data is unavailable.
	snapshotDisplayEmpty = "-"
	// notificationChannelsDisplayPrefixSeparator separates platform and channel labels in snapshot display text.
	notificationChannelsDisplayPrefixSeparator = ": "
	// notificationChannelsDisplayNameSeparator separates multiple channel labels in snapshot display text.
	notificationChannelsDisplayNameSeparator = ", "
)

type createCall struct {
	done chan struct{}
	task *ScheduledTask
	err  error
}

type recentCreate struct {
	task      ScheduledTask
	expiresAt time.Time
}

type taskNotificationSender interface {
	SendTaskResult(ctx context.Context, task ScheduledTask, content string) error
}

func NewScheduledTasksService(app *application.App, convSvc *conversations.ConversationsService, chatSvc *chat.ChatService) *ScheduledTasksService {
	svc := &ScheduledTasksService{
		app:                  app,
		scheduler:            newScheduler(),
		now:                  time.Now,
		duplicateWindow:      time.Second,
		expirationSweepEvery: time.Minute,
	}
	if convSvc != nil {
		svc.runnerDeps.conversations = convSvc
	}
	if chatSvc != nil {
		svc.runnerDeps.chat = chatSvc
	}
	return svc
}

func NewScheduledTasksServiceForTest(app *application.App, db *bun.DB, convSvc conversationService, chatSvc chatService) *ScheduledTasksService {
	return &ScheduledTasksService{
		app:                  app,
		db:                   db,
		scheduler:            newScheduler(),
		now:                  time.Now,
		duplicateWindow:      time.Second,
		expirationSweepEvery: time.Minute,
		runnerDeps: runDependencies{
			conversations: convSvc,
			chat:          chatSvc,
		},
	}
}

func (s *ScheduledTasksService) SetNotificationGateway(gw *channels.Gateway) {
	s.notificationGateway = gw
}

func (s *ScheduledTasksService) dbOrGlobal() (*bun.DB, error) {
	if s.db != nil {
		return s.db, nil
	}
	db := sqlite.DB()
	if db == nil {
		return nil, errs.New("error.sqlite_not_initialized")
	}
	return db, nil
}

func (s *ScheduledTasksService) Start() error {
	s.scheduler.start()
	if err := s.reloadEnabledTasks(); err != nil {
		return err
	}
	s.startExpirationSweeper()
	return nil
}

func (s *ScheduledTasksService) Stop() {
	s.stopExpirationSweeper()
	s.scheduler.stop()
}

func (s *ScheduledTasksService) ListScheduledTasks() ([]ScheduledTask, error) {
	db, err := s.dbOrGlobal()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var models []scheduledTaskModel
	if err := db.NewSelect().
		Model(&models).
		Where("deleted_at IS NULL").
		OrderExpr("created_at DESC, id DESC").
		Scan(ctx); err != nil {
		return nil, errs.Wrap("error.scheduled_task_list_failed", err)
	}

	out := make([]ScheduledTask, 0, len(models))
	for i := range models {
		if err := s.markTaskExpiredIfNeeded(ctx, db, &models[i]); err != nil {
			return nil, err
		}
		if err := s.ensureTaskNextRunAt(ctx, db, &models[i]); err != nil {
			return nil, err
		}
		out = append(out, s.toScheduledTaskDTO(models[i]))
	}
	return out, nil
}

func (s *ScheduledTasksService) GetScheduledTaskByID(id int64) (*ScheduledTask, error) {
	if id <= 0 {
		return nil, errs.New("error.scheduled_task_id_required")
	}
	db, err := s.dbOrGlobal()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	model, err := s.getTaskModel(ctx, db, id)
	if err != nil {
		return nil, err
	}
	if err := s.markTaskExpiredIfNeeded(ctx, db, &model); err != nil {
		return nil, err
	}
	dto := s.toScheduledTaskDTO(model)
	return &dto, nil
}

func (s *ScheduledTasksService) FindScheduledTasksByName(name string) ([]ScheduledTask, error) {
	db, err := s.dbOrGlobal()
	if err != nil {
		return nil, err
	}

	query := strings.TrimSpace(name)
	if query == "" {
		return []ScheduledTask{}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	exactModels := make([]scheduledTaskModel, 0)
	if err := db.NewSelect().
		Model(&exactModels).
		Where("deleted_at IS NULL").
		Where("name = ?", query).
		OrderExpr("created_at DESC, id DESC").
		Scan(ctx); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, errs.Wrap("error.scheduled_task_list_failed", err)
	}
	if len(exactModels) > 0 {
		return taskModelsToDTOs(exactModels), nil
	}

	containsModels := make([]scheduledTaskModel, 0)
	if err := db.NewSelect().
		Model(&containsModels).
		Where("deleted_at IS NULL").
		Where("name LIKE ?", "%"+query+"%").
		OrderExpr("created_at DESC, id DESC").
		Scan(ctx); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, errs.Wrap("error.scheduled_task_list_failed", err)
	}
	return taskModelsToDTOs(containsModels), nil
}

func (s *ScheduledTasksService) GetScheduledTaskSummary() (*ScheduledTaskSummary, error) {
	tasks, err := s.ListScheduledTasks()
	if err != nil {
		return nil, err
	}
	summary := &ScheduledTaskSummary{Total: len(tasks)}
	for _, task := range tasks {
		if task.LastStatus == TaskStatusExpired {
			summary.Paused++
		} else if task.Enabled {
			summary.Running++
		} else {
			summary.Paused++
		}
		if task.LastStatus == TaskStatusFailed {
			summary.Failed++
		}
	}
	return summary, nil
}

func (s *ScheduledTasksService) ValidateSchedule(scheduleType, scheduleValue, cronExpr string) (*ScheduleValidationResult, error) {
	schedule, err := parseSchedule(scheduleType, scheduleValue, cronExpr, time.Now())
	if err != nil {
		return nil, errs.Wrap("error.scheduled_task_schedule_invalid", err)
	}
	return &ScheduleValidationResult{
		ScheduleType:  schedule.ScheduleType,
		ScheduleValue: schedule.ScheduleValue,
		CronExpr:      schedule.CronExpr,
		Timezone:      schedule.Timezone,
		NextRunAt:     schedule.NextRunAt,
	}, nil
}

func (s *ScheduledTasksService) CreateScheduledTask(input CreateScheduledTaskInput) (*ScheduledTask, error) {
	return s.CreateScheduledTaskWithSource(input, OperationSourceManual)
}

func (s *ScheduledTasksService) CreateScheduledTaskWithSource(input CreateScheduledTaskInput, source string) (*ScheduledTask, error) {
	db, err := s.dbOrGlobal()
	if err != nil {
		return nil, err
	}
	model, err := s.buildCreateModel(input)
	if err != nil {
		return nil, err
	}
	if model.Enabled && s.isTaskExpired(model.ExpiresAt) {
		return nil, errs.New("error.scheduled_task_expired")
	}
	fingerprint := createTaskFingerprint(model)
	cachedTask, call, isLeader := s.beginCreateCall(fingerprint)
	if cachedTask != nil {
		return cachedTask, nil
	}
	if !isLeader {
		<-call.done
		if call.task == nil {
			return nil, call.err
		}
		taskCopy := *call.task
		return &taskCopy, call.err
	}
	defer s.finishCreateCall(fingerprint, call)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.ensureAgentExists(ctx, db, model.AgentID); err != nil {
		call.err = err
		return nil, err
	}
	if _, err := db.NewInsert().Model(model).Exec(ctx); err != nil {
		call.err = errs.Wrap("error.scheduled_task_create_failed", err)
		return nil, call.err
	}

	dto := s.toScheduledTaskDTO(*model)
	if err := s.scheduler.register(dto, func() { s.runScheduledTask(dto.ID, RunTriggerSchedule) }); err != nil {
		call.err = errs.Wrap("error.scheduled_task_create_failed", err)
		return nil, call.err
	}
	call.task = &dto
	return &dto, nil
}

func (s *ScheduledTasksService) UpdateScheduledTask(id int64, input UpdateScheduledTaskInput) (*ScheduledTask, error) {
	return s.UpdateScheduledTaskWithSource(id, input, OperationSourceManual)
}

func (s *ScheduledTasksService) UpdateScheduledTaskWithSource(id int64, input UpdateScheduledTaskInput, source string) (*ScheduledTask, error) {
	if id <= 0 {
		return nil, errs.New("error.scheduled_task_id_required")
	}
	db, err := s.dbOrGlobal()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	model, err := s.getTaskModel(ctx, db, id)
	if err != nil {
		return nil, err
	}
	beforeModel := model
	if err := s.applyUpdateInput(&model, input); err != nil {
		return nil, err
	}
	if s.isTaskExpired(model.ExpiresAt) {
		if shouldRejectExpiredEnable(beforeModel.Enabled, input.Enabled) {
			return nil, errs.New("error.scheduled_task_expired")
		}
		model.LastStatus = TaskStatusExpired
		model.NextRunAt = nil
	} else {
		if beforeModel.LastStatus == TaskStatusExpired {
			model.LastStatus = TaskStatusPending
			model.LastError = ""
		}
		if model.Enabled && model.NextRunAt == nil {
			nextRunAt, nextErr := s.scheduler.next(model.CronExpr, s.currentTime())
			if nextErr != nil {
				return nil, errs.Wrap("error.scheduled_task_schedule_invalid", nextErr)
			}
			model.NextRunAt = nextRunAt
		}
	}
	if err := s.ensureAgentExists(ctx, db, model.AgentID); err != nil {
		return nil, err
	}
	if _, err := db.NewUpdate().
		Model((*scheduledTaskModel)(nil)).
		Set("name = ?", model.Name).
		Set("prompt = ?", model.Prompt).
		Set("agent_id = ?", model.AgentID).
		Set("notification_platform = ?", model.NotificationPlatform).
		Set("notification_channel_ids = ?", model.NotificationChannelIDs).
		Set("schedule_type = ?", model.ScheduleType).
		Set("schedule_value = ?", model.ScheduleValue).
		Set("cron_expr = ?", model.CronExpr).
		Set("timezone = ?", model.Timezone).
		Set("enabled = ?", model.Enabled).
		Set("expires_at = ?", model.ExpiresAt).
		Set("last_status = ?", model.LastStatus).
		Set("last_error = ?", model.LastError).
		Set("next_run_at = ?", model.NextRunAt).
		Where("id = ?", id).
		Exec(ctx); err != nil {
		return nil, errs.Wrap("error.scheduled_task_update_failed", err)
	}

	dto := s.toScheduledTaskDTO(model)
	if err := s.scheduler.register(dto, func() { s.runScheduledTask(dto.ID, RunTriggerSchedule) }); err != nil {
		return nil, errs.Wrap("error.scheduled_task_update_failed", err)
	}
	if err := s.insertOperationLog(ctx, db, model.ID, model.Name, OperationTypeUpdate, source, s.buildUpdateChangedFields(ctx, db, beforeModel, model), s.buildTaskSnapshot(ctx, db, model)); err != nil {
		return nil, errs.Wrap("error.scheduled_task_update_failed", err)
	}
	return &dto, nil
}

func (s *ScheduledTasksService) DeleteScheduledTask(id int64) error {
	return s.DeleteScheduledTaskWithSource(id, OperationSourceManual)
}

func (s *ScheduledTasksService) DeleteScheduledTaskWithSource(id int64, source string) error {
	if id <= 0 {
		return errs.New("error.scheduled_task_id_required")
	}
	db, err := s.dbOrGlobal()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	model, err := s.getTaskModel(ctx, db, id)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	res, err := db.NewUpdate().
		Model((*scheduledTaskModel)(nil)).
		Set("deleted_at = ?", now).
		Set("enabled = ?", false).
		Where("id = ?", id).
		Where("deleted_at IS NULL").
		Exec(ctx)
	if err != nil {
		return errs.Wrap("error.scheduled_task_delete_failed", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return errs.New("error.scheduled_task_not_found")
	}

	s.scheduler.unregister(id)
	if err := s.insertOperationLog(ctx, db, model.ID, model.Name, OperationTypeDelete, source, s.buildDeleteChangedFields(model), s.buildTaskSnapshot(ctx, db, model)); err != nil {
		return errs.Wrap("error.scheduled_task_delete_failed", err)
	}
	return nil
}

func (s *ScheduledTasksService) SetScheduledTaskEnabled(id int64, enabled bool) (*ScheduledTask, error) {
	return s.SetScheduledTaskEnabledWithSource(id, enabled, OperationSourceManual)
}

func (s *ScheduledTasksService) SetScheduledTaskEnabledWithSource(id int64, enabled bool, source string) (*ScheduledTask, error) {
	return s.UpdateScheduledTaskWithSource(id, UpdateScheduledTaskInput{Enabled: &enabled}, source)
}

func (s *ScheduledTasksService) RunScheduledTaskNow(id int64) (*ScheduledTaskRun, error) {
	if id <= 0 {
		return nil, errs.New("error.scheduled_task_id_required")
	}
	db, err := s.dbOrGlobal()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	task, err := s.getTaskModel(ctx, db, id)
	if err != nil {
		return nil, err
	}
	if err := s.markTaskExpiredIfNeeded(ctx, db, &task); err != nil {
		return nil, err
	}
	if task.LastStatus == TaskStatusExpired {
		return nil, errs.New("error.scheduled_task_expired")
	}
	run, err := s.executeTask(ctx, task, RunTriggerManual)
	if err != nil {
		return nil, errs.Wrap("error.scheduled_task_run_failed", err)
	}
	dto := run.toDTO()
	return &dto, nil
}

func (s *ScheduledTasksService) ListScheduledTaskRuns(taskID int64, page, pageSize int) ([]ScheduledTaskRun, error) {
	if taskID <= 0 {
		return nil, errs.New("error.scheduled_task_id_required")
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	db, err := s.dbOrGlobal()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var models []scheduledTaskRunModel
	if err := db.NewSelect().
		Model(&models).
		Where("task_id = ?", taskID).
		OrderExpr("started_at DESC, id DESC").
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Scan(ctx); err != nil {
		return nil, errs.Wrap("error.scheduled_task_run_list_failed", err)
	}

	out := make([]ScheduledTaskRun, 0, len(models))
	for i := range models {
		out = append(out, models[i].toDTO())
	}
	return out, nil
}

func (s *ScheduledTasksService) GetScheduledTaskRunDetail(runID int64) (*ScheduledTaskRunDetail, error) {
	if runID <= 0 {
		return nil, errs.New("error.scheduled_task_run_id_required")
	}
	db, err := s.dbOrGlobal()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var run scheduledTaskRunModel
	if err := db.NewSelect().Model(&run).Where("id = ?", runID).Limit(1).Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.New("error.scheduled_task_run_not_found")
		}
		return nil, errs.Wrap("error.scheduled_task_run_read_failed", err)
	}

	detail := &ScheduledTaskRunDetail{
		Run:      run.toDTO(),
		Messages: []chat.Message{},
	}
	if run.ConversationID == nil {
		return detail, nil
	}
	if s.runnerDeps.conversations != nil {
		if conv, err := s.runnerDeps.conversations.GetConversation(*run.ConversationID); err == nil {
			detail.Conversation = conv
		}
	}
	if s.runnerDeps.chat != nil {
		if messages, err := s.runnerDeps.chat.GetMessages(*run.ConversationID); err == nil {
			detail.Messages = messages
		}
	}
	return detail, nil
}

func (s *ScheduledTasksService) ListScheduledTaskOperationLogs(taskID int64, page, pageSize int) ([]ScheduledTaskOperationLog, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	db, err := s.dbOrGlobal()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := db.NewSelect().
		Model((*scheduledTaskOperationLogModel)(nil)).
		OrderExpr("created_at DESC, id DESC").
		Limit(pageSize).
		Offset((page - 1) * pageSize)
	if taskID > 0 {
		query = query.Where("task_id = ?", taskID)
	}

	var models []scheduledTaskOperationLogModel
	if err := query.Scan(ctx, &models); err != nil {
		return nil, errs.Wrap("error.scheduled_task_operation_log_list_failed", err)
	}

	out := make([]ScheduledTaskOperationLog, 0, len(models))
	for i := range models {
		changedFields := decodeScheduledTaskOperationChangedFields(models[i].ChangedFieldsJSON)
		out = append(out, models[i].toDTO(changedFields))
	}
	return out, nil
}

func (s *ScheduledTasksService) GetScheduledTaskOperationLogDetail(logID int64) (*ScheduledTaskOperationLogDetail, error) {
	if logID <= 0 {
		return nil, errs.New("error.scheduled_task_operation_log_id_required")
	}
	db, err := s.dbOrGlobal()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var model scheduledTaskOperationLogModel
	if err := db.NewSelect().
		Model(&model).
		Where("id = ?", logID).
		Limit(1).
		Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.New("error.scheduled_task_operation_log_not_found")
		}
		return nil, errs.Wrap("error.scheduled_task_operation_log_read_failed", err)
	}

	changedFields := decodeScheduledTaskOperationChangedFields(model.ChangedFieldsJSON)
	snapshot := decodeScheduledTaskOperationSnapshot(model.TaskSnapshotJSON)
	return &ScheduledTaskOperationLogDetail{
		Log:          model.toDTO(changedFields),
		TaskSnapshot: snapshot,
	}, nil
}

func (s *ScheduledTasksService) buildCreateModel(input CreateScheduledTaskInput) (*scheduledTaskModel, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, errs.New("error.scheduled_task_name_required")
	}
	prompt := strings.TrimSpace(input.Prompt)
	if prompt == "" {
		return nil, errs.New("error.scheduled_task_prompt_required")
	}
	if input.AgentID <= 0 {
		return nil, errs.New("error.agent_id_required")
	}
	expiresAt := normalizeScheduledTaskExpiration(input.ExpiresAt)

	schedule, err := parseSchedule(input.ScheduleType, input.ScheduleValue, input.CronExpr, s.currentTime())
	if err != nil {
		return nil, errs.Wrap("error.scheduled_task_schedule_invalid", err)
	}

	model := &scheduledTaskModel{
		Name:                   name,
		Prompt:                 prompt,
		AgentID:                input.AgentID,
		NotificationPlatform:   normalizeNotificationPlatform(input.NotificationPlatform),
		NotificationChannelIDs: mustMarshalNotificationChannelIDs(input.NotificationChannelIDs),
		ScheduleType:           schedule.ScheduleType,
		ScheduleValue:          schedule.ScheduleValue,
		CronExpr:               schedule.CronExpr,
		Timezone:               schedule.Timezone,
		Enabled:                input.Enabled,
		ExpiresAt:              expiresAt,
		LastStatus:             TaskStatusPending,
		LastError:              "",
	}
	if input.Enabled {
		model.NextRunAt = schedule.NextRunAt
	}
	return model, nil
}

func normalizeNotificationPlatform(platform string) string {
	return strings.TrimSpace(strings.ToLower(platform))
}

func normalizeNotificationChannelIDs(channelIDs []int64) []int64 {
	if len(channelIDs) == 0 {
		return []int64{}
	}

	seen := make(map[int64]struct{}, len(channelIDs))
	out := make([]int64, 0, len(channelIDs))
	for _, channelID := range channelIDs {
		if channelID <= 0 {
			continue
		}
		if _, exists := seen[channelID]; exists {
			continue
		}
		seen[channelID] = struct{}{}
		out = append(out, channelID)
	}
	return out
}

func mustMarshalNotificationChannelIDs(channelIDs []int64) string {
	normalized := normalizeNotificationChannelIDs(channelIDs)
	data, err := json.Marshal(normalized)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func parseNotificationChannelIDs(raw string) []int64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []int64{}
	}

	var ids []int64
	if err := json.Unmarshal([]byte(raw), &ids); err != nil {
		return []int64{}
	}
	return normalizeNotificationChannelIDs(ids)
}

func createTaskFingerprint(model *scheduledTaskModel) string {
	return strings.Join([]string{
		model.Name,
		model.Prompt,
		strconv.FormatInt(model.AgentID, 10),
		model.NotificationPlatform,
		model.NotificationChannelIDs,
		model.ScheduleType,
		model.ScheduleValue,
		model.CronExpr,
		model.Timezone,
		strconv.FormatBool(model.Enabled),
		formatTimeFingerprint(model.ExpiresAt),
	}, "\x00")
}

func (s *ScheduledTasksService) beginCreateCall(fingerprint string) (*ScheduledTask, *createCall, bool) {
	s.createMu.Lock()
	defer s.createMu.Unlock()

	now := s.currentTime()
	for key, entry := range s.recentCreates {
		if !entry.expiresAt.After(now) {
			delete(s.recentCreates, key)
		}
	}
	if entry, ok := s.recentCreates[fingerprint]; ok {
		taskCopy := entry.task
		return &taskCopy, nil, false
	}
	if s.createRuns == nil {
		s.createRuns = make(map[string]*createCall)
	}
	if s.recentCreates == nil {
		s.recentCreates = make(map[string]recentCreate)
	}
	if existing, ok := s.createRuns[fingerprint]; ok {
		return nil, existing, false
	}

	call := &createCall{done: make(chan struct{})}
	s.createRuns[fingerprint] = call
	return nil, call, true
}

func (s *ScheduledTasksService) finishCreateCall(fingerprint string, call *createCall) {
	s.createMu.Lock()
	delete(s.createRuns, fingerprint)
	if call.task != nil && call.err == nil {
		if s.recentCreates == nil {
			s.recentCreates = make(map[string]recentCreate)
		}
		s.recentCreates[fingerprint] = recentCreate{
			task:      *call.task,
			expiresAt: s.currentTime().Add(s.duplicateWindow),
		}
	}
	s.createMu.Unlock()
	close(call.done)
}

func (s *ScheduledTasksService) currentTime() time.Time {
	if s.now == nil {
		return time.Now().UTC()
	}
	return s.now().UTC()
}

func (s *ScheduledTasksService) toScheduledTaskDTO(model scheduledTaskModel) ScheduledTask {
	dto := model.toDTO()
	dto.IsExpired = s.isTaskExpired(model.ExpiresAt)
	return dto
}

func normalizeScheduledTaskExpiration(expiresAt *time.Time) *time.Time {
	if expiresAt == nil {
		return nil
	}
	value := expiresAt.UTC()
	return &value
}

func formatTimeFingerprint(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func shouldRejectExpiredEnable(beforeEnabled bool, requestedEnabled *bool) bool {
	return requestedEnabled != nil && *requestedEnabled && !beforeEnabled
}

func (s *ScheduledTasksService) isTaskExpired(expiresAt *time.Time) bool {
	return isScheduledTaskExpired(expiresAt, s.currentTime())
}

func isScheduledTaskExpired(expiresAt *time.Time, now time.Time) bool {
	if expiresAt == nil {
		return false
	}
	return !expiresAt.After(now)
}

func (s *ScheduledTasksService) applyUpdateInput(model *scheduledTaskModel, input UpdateScheduledTaskInput) error {
	if input.Name != nil {
		value := strings.TrimSpace(*input.Name)
		if value == "" {
			return errs.New("error.scheduled_task_name_required")
		}
		model.Name = value
	}
	if input.Prompt != nil {
		value := strings.TrimSpace(*input.Prompt)
		if value == "" {
			return errs.New("error.scheduled_task_prompt_required")
		}
		model.Prompt = value
	}
	if input.AgentID != nil {
		if *input.AgentID <= 0 {
			return errs.New("error.agent_id_required")
		}
		model.AgentID = *input.AgentID
	}
	if input.Enabled != nil {
		model.Enabled = *input.Enabled
	}
	if input.ExpiresAt != nil {
		model.ExpiresAt = normalizeScheduledTaskExpiration(input.ExpiresAt)
	}
	if input.NotificationPlatform != nil {
		model.NotificationPlatform = normalizeNotificationPlatform(*input.NotificationPlatform)
	}
	if input.NotificationChannelIDs != nil {
		model.NotificationChannelIDs = mustMarshalNotificationChannelIDs(*input.NotificationChannelIDs)
	}

	needsScheduleRebuild := input.ScheduleType != nil || input.ScheduleValue != nil || input.CronExpr != nil
	if needsScheduleRebuild {
		scheduleType := model.ScheduleType
		scheduleValue := model.ScheduleValue
		cronExpr := model.CronExpr
		if input.ScheduleType != nil {
			scheduleType = *input.ScheduleType
		}
		if input.ScheduleValue != nil {
			scheduleValue = *input.ScheduleValue
		}
		if input.CronExpr != nil {
			cronExpr = *input.CronExpr
		}
		schedule, err := parseSchedule(scheduleType, scheduleValue, cronExpr, s.currentTime())
		if err != nil {
			return errs.Wrap("error.scheduled_task_schedule_invalid", err)
		}
		model.ScheduleType = schedule.ScheduleType
		model.ScheduleValue = schedule.ScheduleValue
		model.CronExpr = schedule.CronExpr
		model.Timezone = schedule.Timezone
		if model.Enabled {
			model.NextRunAt = schedule.NextRunAt
		} else {
			model.NextRunAt = nil
		}
	} else if input.Enabled != nil {
		if model.Enabled {
			nextRunAt, err := s.scheduler.next(model.CronExpr, s.currentTime())
			if err != nil {
				return errs.Wrap("error.scheduled_task_schedule_invalid", err)
			}
			model.NextRunAt = nextRunAt
		} else {
			model.NextRunAt = nil
		}
	}
	return nil
}

func (s *ScheduledTasksService) ensureAgentExists(ctx context.Context, db *bun.DB, agentID int64) error {
	var count int
	if err := db.NewSelect().Table("agents").ColumnExpr("COUNT(1)").Where("id = ?", agentID).Scan(ctx, &count); err != nil {
		return errs.Wrap("error.scheduled_task_create_failed", err)
	}
	if count == 0 {
		return errs.Newf("error.agent_not_found", map[string]any{"ID": agentID})
	}
	return nil
}

func (s *ScheduledTasksService) getTaskModel(ctx context.Context, db *bun.DB, id int64) (scheduledTaskModel, error) {
	var model scheduledTaskModel
	if err := db.NewSelect().
		Model(&model).
		Where("id = ?", id).
		Where("deleted_at IS NULL").
		Limit(1).
		Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return scheduledTaskModel{}, errs.New("error.scheduled_task_not_found")
		}
		return scheduledTaskModel{}, errs.Wrap("error.scheduled_task_read_failed", err)
	}
	return model, nil
}

func (s *ScheduledTasksService) getTaskAgentConfig(ctx context.Context, db *bun.DB, agentID int64) (scheduledTaskAgentRow, error) {
	var row scheduledTaskAgentRow
	if err := db.NewSelect().
		Table("agents").
		Column("id", "default_llm_provider_id", "default_llm_model_id").
		Where("id = ?", agentID).
		Limit(1).
		Scan(ctx, &row); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return scheduledTaskAgentRow{}, errs.Newf("error.agent_not_found", map[string]any{"ID": agentID})
		}
		return scheduledTaskAgentRow{}, errs.Wrap("error.scheduled_task_run_failed", err)
	}
	return row, nil
}

func (s *ScheduledTasksService) resolveTaskConversationModel(ctx context.Context, db *bun.DB, agentID int64) (scheduledTaskConversationModelRow, error) {
	agentConfig, err := s.getTaskAgentConfig(ctx, db, agentID)
	if err != nil {
		return scheduledTaskConversationModelRow{}, err
	}
	if agentConfig.DefaultLLMProviderID != "" && agentConfig.DefaultLLMModelID != "" {
		return scheduledTaskConversationModelRow{
			ProviderID: agentConfig.DefaultLLMProviderID,
			ModelID:    agentConfig.DefaultLLMModelID,
		}, nil
	}

	var fallback scheduledTaskConversationModelRow
	if err := db.NewSelect().
		TableExpr("providers AS p").
		Join("JOIN models AS m ON m.provider_id = p.provider_id").
		ColumnExpr("p.provider_id AS provider_id").
		ColumnExpr("m.model_id AS model_id").
		Where("p.enabled = ?", true).
		Where("m.enabled = ?", true).
		Where("m.type = ?", "llm").
		OrderExpr("CASE WHEN p.is_free THEN 1 ELSE 0 END ASC").
		OrderExpr("p.sort_order ASC, p.id ASC").
		OrderExpr("m.sort_order ASC, m.id ASC").
		Limit(1).
		Scan(ctx, &fallback); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return scheduledTaskConversationModelRow{}, errs.New("error.chat_model_not_configured")
		}
		return scheduledTaskConversationModelRow{}, errs.Wrap("error.scheduled_task_run_failed", err)
	}
	return fallback, nil
}

func (s *ScheduledTasksService) reloadEnabledTasks() error {
	db, err := s.dbOrGlobal()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := s.markExpiredTasks(ctx); err != nil {
		return err
	}

	var models []scheduledTaskModel
	if err := db.NewSelect().
		Model(&models).
		Where("deleted_at IS NULL").
		Where("enabled = ?", true).
		Scan(ctx); err != nil {
		return errs.Wrap("error.scheduled_task_list_failed", err)
	}

	for i := range models {
		if err := s.markTaskExpiredIfNeeded(ctx, db, &models[i]); err != nil {
			return err
		}
		if models[i].LastStatus == TaskStatusExpired {
			continue
		}
		if err := s.ensureTaskNextRunAt(ctx, db, &models[i]); err != nil {
			return err
		}
		task := s.toScheduledTaskDTO(models[i])
		if err := s.scheduler.register(task, func() { s.runScheduledTask(task.ID, RunTriggerSchedule) }); err != nil {
			return err
		}
	}
	return nil
}

func (s *ScheduledTasksService) ensureTaskNextRunAt(ctx context.Context, db *bun.DB, model *scheduledTaskModel) error {
	if !model.Enabled || model.NextRunAt != nil || strings.TrimSpace(model.CronExpr) == "" {
		return nil
	}

	nextRunAt, err := s.scheduler.next(model.CronExpr, s.currentTime())
	if err != nil {
		return nil
	}

	if _, err := db.NewUpdate().
		Model((*scheduledTaskModel)(nil)).
		Set("next_run_at = ?", nextRunAt).
		Where("id = ?", model.ID).
		Exec(ctx); err != nil {
		return errs.Wrap("error.scheduled_task_update_failed", err)
	}

	model.NextRunAt = nextRunAt
	return nil
}

func (s *ScheduledTasksService) runScheduledTask(taskID int64, triggerType string) {
	db, err := s.dbOrGlobal()
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	task, err := s.getTaskModel(ctx, db, taskID)
	if err != nil {
		return
	}
	if triggerType == RunTriggerSchedule && s.isTaskExpired(task.ExpiresAt) {
		_, _ = s.markTaskExpired(ctx, db, task.ID)
		return
	}
	_, _ = s.executeTask(context.Background(), task, triggerType)
}

func (s *ScheduledTasksService) startExpirationSweeper() {
	if s.expirationSweepEvery <= 0 || s.expirationSweepStop != nil {
		return
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	s.expirationSweepStop = stop
	s.expirationSweepDone = done

	go func() {
		ticker := time.NewTicker(s.expirationSweepEvery)
		defer ticker.Stop()
		defer close(done)
		for {
			select {
			case <-ticker.C:
				_, _ = s.markExpiredTasks(context.Background())
			case <-stop:
				return
			}
		}
	}()
}

func (s *ScheduledTasksService) stopExpirationSweeper() {
	if s.expirationSweepStop == nil {
		return
	}
	close(s.expirationSweepStop)
	<-s.expirationSweepDone
	s.expirationSweepStop = nil
	s.expirationSweepDone = nil
}

// markExpiredTasks persists the expired runtime state without mutating the user's enable switch.
func (s *ScheduledTasksService) markExpiredTasks(ctx context.Context) (int, error) {
	db, err := s.dbOrGlobal()
	if err != nil {
		return 0, err
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var models []scheduledTaskModel
	if err := db.NewSelect().
		Model(&models).
		Where("deleted_at IS NULL").
		Where("expires_at IS NOT NULL").
		Where("expires_at <= ?", s.currentTime()).
		Where("last_status != ?", TaskStatusExpired).
		Scan(ctx); err != nil {
		return 0, errs.Wrap("error.scheduled_task_list_failed", err)
	}
	for i := range models {
		if _, err := s.markTaskExpired(ctx, db, models[i].ID); err != nil {
			return 0, err
		}
	}
	return len(models), nil
}

// markTaskExpired stores the terminal expired status and clears scheduling metadata.
func (s *ScheduledTasksService) markTaskExpired(ctx context.Context, db *bun.DB, taskID int64) (bool, error) {
	res, err := db.NewUpdate().
		Model((*scheduledTaskModel)(nil)).
		Set("last_status = ?", TaskStatusExpired).
		Set("next_run_at = NULL").
		Where("id = ?", taskID).
		Where("deleted_at IS NULL").
		Where("last_status != ?", TaskStatusExpired).
		Exec(ctx)
	if err != nil {
		return false, errs.Wrap("error.scheduled_task_update_failed", err)
	}
	rows, _ := res.RowsAffected()
	if rows > 0 {
		s.scheduler.unregister(taskID)
		return true, nil
	}
	return false, nil
}

func (s *ScheduledTasksService) markTaskExpiredIfNeeded(ctx context.Context, db *bun.DB, model *scheduledTaskModel) error {
	if model == nil || !s.isTaskExpired(model.ExpiresAt) || model.LastStatus == TaskStatusExpired {
		return nil
	}

	updated, err := s.markTaskExpired(ctx, db, model.ID)
	if err != nil {
		return err
	}
	if updated {
		model.LastStatus = TaskStatusExpired
		model.NextRunAt = nil
	}
	return nil
}

func (s *ScheduledTasksService) createRun(ctx context.Context, task scheduledTaskModel, triggerType string) (*scheduledTaskRunModel, error) {
	db, err := s.dbOrGlobal()
	if err != nil {
		return nil, err
	}

	run := &scheduledTaskRunModel{
		TaskID:           task.ID,
		TriggerType:      triggerType,
		Status:           RunStatusRunning,
		StartedAt:        time.Now().UTC(),
		ErrorMessage:     "",
		SnapshotTaskName: task.Name,
		SnapshotPrompt:   task.Prompt,
		SnapshotAgentID:  task.AgentID,
	}
	if _, err := db.NewInsert().Model(run).Exec(ctx); err != nil {
		return nil, errs.Wrap("error.scheduled_task_run_create_failed", err)
	}

	if _, err := db.NewUpdate().
		Model((*scheduledTaskModel)(nil)).
		Set("last_status = ?", TaskStatusRunning).
		Set("last_error = ''").
		Set("last_run_id = ?", run.ID).
		Where("id = ?", task.ID).
		Exec(ctx); err != nil {
		return nil, errs.Wrap("error.scheduled_task_run_create_failed", err)
	}
	return run, nil
}

func (s *ScheduledTasksService) updateRunConversation(ctx context.Context, runID, conversationID int64) error {
	db, err := s.dbOrGlobal()
	if err != nil {
		return err
	}
	if _, err := db.NewUpdate().
		Model((*scheduledTaskRunModel)(nil)).
		Set("conversation_id = ?", conversationID).
		Where("id = ?", runID).
		Exec(ctx); err != nil {
		return errs.Wrap("error.scheduled_task_run_update_failed", err)
	}
	return nil
}

func (s *ScheduledTasksService) markRunStarted(ctx context.Context, taskID, runID, conversationID, userMessageID int64) error {
	db, err := s.dbOrGlobal()
	if err != nil {
		return err
	}

	cronExpr := s.mustGetCronExpr(ctx, db, taskID)
	nextRunAt, nextErr := s.scheduler.next(cronExpr, time.Now())
	if nextErr != nil {
		nextRunAt = nil
	}

	if _, err := db.NewUpdate().
		Model((*scheduledTaskRunModel)(nil)).
		Set("conversation_id = ?", conversationID).
		Set("user_message_id = ?", userMessageID).
		Where("id = ?", runID).
		Exec(ctx); err != nil {
		return errs.Wrap("error.scheduled_task_run_update_failed", err)
	}
	if _, err := db.NewUpdate().
		Model((*scheduledTaskModel)(nil)).
		Set("last_run_at = ?", time.Now().UTC()).
		Set("next_run_at = ?", nextRunAt).
		Set("last_run_id = ?", runID).
		Set("last_status = ?", TaskStatusRunning).
		Set("last_error = ''").
		Where("id = ?", taskID).
		Exec(ctx); err != nil {
		return errs.Wrap("error.scheduled_task_run_update_failed", err)
	}
	return nil
}

type scheduledTaskRunAssistantRow struct {
	ID     int64  `bun:"id"`
	Status string `bun:"status"`
	Error  string `bun:"error"`
}

func (s *ScheduledTasksService) watchRun(taskID, runID, conversationID int64, startedAt time.Time) {
	db, err := s.dbOrGlobal()
	if err != nil {
		return
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	timeout := time.After(5 * time.Minute)

	for {
		select {
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			var assistant scheduledTaskRunAssistantRow
			err := db.NewSelect().
				Table("messages").
				Column("id", "status", "error").
				Where("conversation_id = ?", conversationID).
				Where("role = ?", chat.RoleAssistant).
				OrderExpr("created_at DESC, id DESC").
				Limit(1).
				Scan(ctx, &assistant)
			cancel()
			if err != nil {
				continue
			}

			switch assistant.Status {
			case chat.StatusStreaming, chat.StatusPending, "":
				continue
			case chat.StatusSuccess:
				_ = s.completeRun(context.Background(), taskID, runID, assistant.ID, startedAt, "")
				return
			default:
				if assistant.ID > 0 {
					_ = s.setRunAssistantMessage(context.Background(), runID, assistant.ID)
				}
				_ = s.failRun(context.Background(), taskID, runID, assistant.Error, startedAt)
				return
			}
		case <-timeout:
			_ = s.failRun(context.Background(), taskID, runID, "scheduled task run timed out", startedAt)
			return
		}
	}
}

func (s *ScheduledTasksService) setRunAssistantMessage(ctx context.Context, runID, assistantMessageID int64) error {
	db, err := s.dbOrGlobal()
	if err != nil {
		return err
	}
	_, err = db.NewUpdate().
		Model((*scheduledTaskRunModel)(nil)).
		Set("assistant_message_id = ?", assistantMessageID).
		Where("id = ?", runID).
		Exec(ctx)
	return err
}

func (s *ScheduledTasksService) completeRun(ctx context.Context, taskID, runID, assistantMessageID int64, startedAt time.Time, errorMessage string) error {
	db, err := s.dbOrGlobal()
	if err != nil {
		return err
	}
	finishedAt := time.Now().UTC()
	durationMS := finishedAt.Sub(startedAt).Milliseconds()
	if durationMS < 0 {
		durationMS = 0
	}
	if assistantMessageID > 0 {
		_ = s.setRunAssistantMessage(ctx, runID, assistantMessageID)
	}
	taskModel, err := s.getTaskModel(ctx, db, taskID)
	if err != nil {
		return err
	}
	if notifyErr := s.notifyTaskResult(ctx, taskModel, assistantMessageID); notifyErr != nil {
		return s.failRun(ctx, taskID, runID, fmt.Sprintf("scheduled task notification failed: %v", notifyErr), startedAt)
	}
	if _, err := db.NewUpdate().
		Model((*scheduledTaskRunModel)(nil)).
		Set("status = ?", RunStatusSuccess).
		Set("finished_at = ?", finishedAt).
		Set("duration_ms = ?", durationMS).
		Set("error_message = ?", errorMessage).
		Where("id = ?", runID).
		Exec(ctx); err != nil {
		return errs.Wrap("error.scheduled_task_run_update_failed", err)
	}
	if _, err := db.NewUpdate().
		Model((*scheduledTaskModel)(nil)).
		Set("last_status = ?", TaskStatusSuccess).
		Set("last_error = ''").
		Set("last_run_at = ?", startedAt.UTC()).
		Where("id = ?", taskID).
		Exec(ctx); err != nil {
		return errs.Wrap("error.scheduled_task_run_update_failed", err)
	}
	return nil
}

func (s *ScheduledTasksService) notifyTaskResult(ctx context.Context, task scheduledTaskModel, assistantMessageID int64) error {
	channelIDs := parseNotificationChannelIDs(task.NotificationChannelIDs)
	if task.NotificationPlatform == "" || len(channelIDs) == 0 || assistantMessageID <= 0 {
		return nil
	}

	content, err := s.getMessageContent(ctx, assistantMessageID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(content) == "" {
		return nil
	}

	dto := task.toDTO()
	if s.notificationSender != nil {
		return s.notificationSender.SendTaskResult(ctx, dto, content)
	}
	return s.sendTaskResultToChannels(ctx, dto, content)
}

func (s *ScheduledTasksService) getMessageContent(ctx context.Context, messageID int64) (string, error) {
	db, err := s.dbOrGlobal()
	if err != nil {
		return "", err
	}

	var content string
	if err := db.NewSelect().
		Table("messages").
		Column("content").
		Where("id = ?", messageID).
		Limit(1).
		Scan(ctx, &content); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", errs.New("error.scheduled_task_run_failed")
		}
		return "", errs.Wrap("error.scheduled_task_run_failed", err)
	}
	return content, nil
}

func (s *ScheduledTasksService) sendTaskResultToChannels(ctx context.Context, task ScheduledTask, content string) error {
	if s.notificationGateway == nil {
		return fmt.Errorf("notification gateway not configured")
	}

	db, err := s.dbOrGlobal()
	if err != nil {
		return err
	}

	for _, channelID := range task.NotificationChannelIDs {
		channel, err := s.getNotificationChannel(ctx, db, channelID)
		if err != nil {
			return err
		}
		if normalizeNotificationPlatform(channel.Platform) != task.NotificationPlatform {
			return fmt.Errorf("channel %d platform mismatch", channelID)
		}
		if !channel.Enabled {
			return fmt.Errorf("channel %d is disabled", channelID)
		}

		targetID, err := s.getLatestChannelTarget(ctx, db, channelID)
		if err != nil {
			return err
		}

		adapter := s.notificationGateway.GetAdapter(channelID)
		if adapter == nil {
			return fmt.Errorf("channel %d is not connected", channelID)
		}
		if err := adapter.SendMessage(ctx, targetID, content); err != nil {
			return err
		}
	}

	return nil
}

type scheduledTaskNotificationChannelRow struct {
	ID       int64  `bun:"id"`
	Platform string `bun:"platform"`
	Enabled  bool   `bun:"enabled"`
}

func (s *ScheduledTasksService) getNotificationChannel(ctx context.Context, db *bun.DB, channelID int64) (scheduledTaskNotificationChannelRow, error) {
	var model scheduledTaskNotificationChannelRow
	if err := db.NewSelect().
		Table("channels").
		Column("id", "platform", "enabled").
		Where("id = ?", channelID).
		Limit(1).
		Scan(ctx, &model); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return scheduledTaskNotificationChannelRow{}, fmt.Errorf("channel %d not found", channelID)
		}
		return scheduledTaskNotificationChannelRow{}, err
	}
	return model, nil
}

func (s *ScheduledTasksService) getLatestChannelTarget(ctx context.Context, db *bun.DB, channelID int64) (string, error) {
	var lastSenderID string
	if err := db.NewSelect().
		Table("channels").
		Column("last_sender_id").
		Where("id = ?", channelID).
		Limit(1).
		Scan(ctx, &lastSenderID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("channel %d not found", channelID)
		}
		return "", err
	}

	lastSenderID = strings.TrimSpace(lastSenderID)
	if lastSenderID == "" {
		return "", fmt.Errorf("channel %d has no last sender id", channelID)
	}
	return lastSenderID, nil
}

func (s *ScheduledTasksService) failRun(ctx context.Context, taskID, runID int64, errorMessage string, startedAt time.Time) error {
	db, err := s.dbOrGlobal()
	if err != nil {
		return err
	}
	finishedAt := time.Now().UTC()
	durationMS := finishedAt.Sub(startedAt).Milliseconds()
	if durationMS < 0 {
		durationMS = 0
	}
	if _, err := db.NewUpdate().
		Model((*scheduledTaskRunModel)(nil)).
		Set("status = ?", RunStatusFailed).
		Set("finished_at = ?", finishedAt).
		Set("duration_ms = ?", durationMS).
		Set("error_message = ?", strings.TrimSpace(errorMessage)).
		Where("id = ?", runID).
		Exec(ctx); err != nil {
		return errs.Wrap("error.scheduled_task_run_update_failed", err)
	}
	if _, err := db.NewUpdate().
		Model((*scheduledTaskModel)(nil)).
		Set("last_status = ?", TaskStatusFailed).
		Set("last_error = ?", strings.TrimSpace(errorMessage)).
		Set("last_run_at = ?", startedAt.UTC()).
		Where("id = ?", taskID).
		Exec(ctx); err != nil {
		return errs.Wrap("error.scheduled_task_run_update_failed", err)
	}
	return nil
}

func (s *ScheduledTasksService) mustGetCronExpr(ctx context.Context, db *bun.DB, taskID int64) string {
	var cronExpr string
	_ = db.NewSelect().Table("scheduled_tasks").Column("cron_expr").Where("id = ?", taskID).Scan(ctx, &cronExpr)
	return cronExpr
}

func (s *ScheduledTasksService) insertOperationLog(
	ctx context.Context,
	db *bun.DB,
	taskID int64,
	taskName string,
	operationType string,
	source string,
	changedFields []ScheduledTaskOperationChangedField,
	snapshot ScheduledTaskOperationSnapshot,
) error {
	changedFieldsJSON, err := json.Marshal(changedFields)
	if err != nil {
		return err
	}
	snapshotJSON, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	model := &scheduledTaskOperationLogModel{
		TaskID:            taskID,
		TaskNameSnapshot:  strings.TrimSpace(taskName),
		OperationType:     normalizeOperationType(operationType),
		OperationSource:   normalizeOperationSource(source),
		ChangedFieldsJSON: string(changedFieldsJSON),
		TaskSnapshotJSON:  string(snapshotJSON),
	}
	_, err = db.NewInsert().Model(model).Exec(ctx)
	return err
}

func (s *ScheduledTasksService) buildCreateChangedFields(ctx context.Context, db *bun.DB, task scheduledTaskModel) []ScheduledTaskOperationChangedField {
	changed := []ScheduledTaskOperationChangedField{
		{FieldKey: "status", FieldLabel: "状态", Before: "", After: formatEnabledDisplay(task.Enabled)},
		{FieldKey: "name", FieldLabel: "名称", Before: "", After: task.Name},
		{FieldKey: "prompt", FieldLabel: "提示词", Before: "", After: task.Prompt},
		{FieldKey: "agent", FieldLabel: "关联助手", Before: "", After: s.resolveOperationLogAgentDisplay(ctx, db, task.AgentID)},
		{
			FieldKey:   "notification_channels",
			FieldLabel: "通知渠道",
			Before:     "",
			After: s.resolveOperationLogNotificationChannelsDisplay(
				ctx,
				db,
				task.NotificationPlatform,
				parseNotificationChannelIDs(task.NotificationChannelIDs),
			),
		},
		{FieldKey: "schedule_time", FieldLabel: "执行时间", Before: "", After: formatScheduleDisplay(task.ScheduleType, task.ScheduleValue, task.CronExpr)},
	}
	if task.ExpiresAt != nil {
		changed = append(changed, ScheduledTaskOperationChangedField{
			FieldKey:   "expires_at",
			FieldLabel: "到期时间",
			Before:     "-",
			After:      formatExpirationDisplay(task.ExpiresAt),
		})
	}
	return changed
}

func (s *ScheduledTasksService) buildUpdateChangedFields(ctx context.Context, db *bun.DB, before, after scheduledTaskModel) []ScheduledTaskOperationChangedField {
	changed := make([]ScheduledTaskOperationChangedField, 0, 6)
	if before.Enabled != after.Enabled {
		changed = append(changed, ScheduledTaskOperationChangedField{
			FieldKey:   "status",
			FieldLabel: "状态",
			Before:     formatEnabledDisplay(before.Enabled),
			After:      formatEnabledDisplay(after.Enabled),
		})
	}
	if before.Name != after.Name {
		changed = append(changed, ScheduledTaskOperationChangedField{
			FieldKey:   "name",
			FieldLabel: "名称",
			Before:     before.Name,
			After:      after.Name,
		})
	}
	if before.Prompt != after.Prompt {
		changed = append(changed, ScheduledTaskOperationChangedField{
			FieldKey:   "prompt",
			FieldLabel: "提示词",
			Before:     before.Prompt,
			After:      after.Prompt,
		})
	}
	if before.AgentID != after.AgentID {
		changed = append(changed, ScheduledTaskOperationChangedField{
			FieldKey:   "agent",
			FieldLabel: "关联助手",
			Before:     s.resolveOperationLogAgentDisplay(ctx, db, before.AgentID),
			After:      s.resolveOperationLogAgentDisplay(ctx, db, after.AgentID),
		})
	}
	if before.NotificationPlatform != after.NotificationPlatform || before.NotificationChannelIDs != after.NotificationChannelIDs {
		changed = append(changed, ScheduledTaskOperationChangedField{
			FieldKey:   "notification_channels",
			FieldLabel: "通知渠道",
			Before: s.resolveOperationLogNotificationChannelsDisplay(
				ctx,
				db,
				before.NotificationPlatform,
				parseNotificationChannelIDs(before.NotificationChannelIDs),
			),
			After: s.resolveOperationLogNotificationChannelsDisplay(
				ctx,
				db,
				after.NotificationPlatform,
				parseNotificationChannelIDs(after.NotificationChannelIDs),
			),
		})
	}
	beforeSchedule := formatScheduleDisplay(before.ScheduleType, before.ScheduleValue, before.CronExpr)
	afterSchedule := formatScheduleDisplay(after.ScheduleType, after.ScheduleValue, after.CronExpr)
	if before.ScheduleType != after.ScheduleType || before.ScheduleValue != after.ScheduleValue || before.CronExpr != after.CronExpr {
		changed = append(changed, ScheduledTaskOperationChangedField{
			FieldKey:   "schedule_time",
			FieldLabel: "执行时间",
			Before:     beforeSchedule,
			After:      afterSchedule,
		})
	}
	if !timePointersEqual(before.ExpiresAt, after.ExpiresAt) {
		changed = append(changed, ScheduledTaskOperationChangedField{
			FieldKey:   "expires_at",
			FieldLabel: "到期时间",
			Before:     formatExpirationDisplay(before.ExpiresAt),
			After:      formatExpirationDisplay(after.ExpiresAt),
		})
	}
	if len(changed) == 0 {
		changed = append(changed, ScheduledTaskOperationChangedField{
			FieldKey:   "status",
			FieldLabel: "状态",
			Before:     formatEnabledDisplay(before.Enabled),
			After:      formatEnabledDisplay(after.Enabled),
		})
	}
	return changed
}

func (s *ScheduledTasksService) buildDeleteChangedFields(task scheduledTaskModel) []ScheduledTaskOperationChangedField {
	return []ScheduledTaskOperationChangedField{
		{
			FieldKey:   "status",
			FieldLabel: "状态",
			Before:     formatEnabledDisplay(task.Enabled),
			After:      "已删除",
		},
	}
}

func (s *ScheduledTasksService) buildTaskSnapshot(ctx context.Context, db *bun.DB, task scheduledTaskModel) ScheduledTaskOperationSnapshot {
	// The operation-log detail dialog reads these snapshot fields directly, so we resolve
	// human-readable names here instead of persisting raw IDs.
	notificationChannelIDs := parseNotificationChannelIDs(task.NotificationChannelIDs)
	return ScheduledTaskOperationSnapshot{
		TaskID:                 task.ID,
		Name:                   task.Name,
		Prompt:                 task.Prompt,
		AgentID:                task.AgentID,
		AgentName:              s.resolveSnapshotAgentName(ctx, db, task.AgentID),
		NotificationPlatform:   task.NotificationPlatform,
		NotificationChannelIDs: notificationChannelIDs,
		NotificationChannels:   s.resolveSnapshotNotificationChannels(ctx, db, task.NotificationPlatform, notificationChannelIDs),
		ScheduleType:           task.ScheduleType,
		ScheduleValue:          task.ScheduleValue,
		CronExpr:               task.CronExpr,
		ScheduleDescription:    formatScheduleDisplay(task.ScheduleType, task.ScheduleValue, task.CronExpr),
		Timezone:               task.Timezone,
		Enabled:                task.Enabled,
		ExpiresAt:              task.ExpiresAt,
		IsExpired:              s.isTaskExpired(task.ExpiresAt),
	}
}

// resolveOperationLogAgentDisplay keeps changed-field output human-readable while
// still falling back to the numeric agent ID after the agent is deleted or missing.
func (s *ScheduledTasksService) resolveOperationLogAgentDisplay(ctx context.Context, db *bun.DB, agentID int64) string {
	if agentID <= 0 {
		return snapshotDisplayEmpty
	}

	var agentName string
	if err := db.NewSelect().
		Table("agents").
		Column("name").
		Where("id = ?", agentID).
		Limit(1).
		Scan(ctx, &agentName); err == nil {
		agentName = strings.TrimSpace(agentName)
		if agentName != "" {
			return agentName
		}
	}

	return formatAgentDisplay(agentID)
}

// resolveOperationLogNotificationChannelsDisplay prefers channel names in changed fields,
// but keeps individual IDs when some channels can no longer be resolved.
func (s *ScheduledTasksService) resolveOperationLogNotificationChannelsDisplay(
	ctx context.Context,
	db *bun.DB,
	platform string,
	channelIDs []int64,
) string {
	normalizedPlatform := strings.TrimSpace(platform)
	if normalizedPlatform == "" || len(channelIDs) == 0 {
		return snapshotDisplayEmpty
	}

	channelLabels := make([]string, 0, len(channelIDs))
	for _, channelID := range channelIDs {
		channelLabels = append(channelLabels, s.resolveSnapshotChannelName(ctx, db, channelID))
	}

	return normalizedPlatform +
		notificationChannelsDisplayPrefixSeparator +
		strings.Join(channelLabels, notificationChannelsDisplayNameSeparator)
}

// resolveSnapshotAgentName prefers the current agent name so operation-log details can
// re-display the selected assistant label instead of falling back to the numeric ID.
func (s *ScheduledTasksService) resolveSnapshotAgentName(ctx context.Context, db *bun.DB, agentID int64) string {
	if agentID <= 0 {
		return snapshotDisplayEmpty
	}

	var agentName string
	if err := db.NewSelect().
		Table("agents").
		Column("name").
		Where("id = ?", agentID).
		Limit(1).
		Scan(ctx, &agentName); err == nil {
		agentName = strings.TrimSpace(agentName)
		if agentName != "" {
			return agentName
		}
	}

	return formatAgentDisplay(agentID)
}

// resolveSnapshotNotificationChannels keeps operation-log snapshots readable by storing
// channel names when available, while still falling back to IDs for deleted/missing channels.
func (s *ScheduledTasksService) resolveSnapshotNotificationChannels(
	ctx context.Context,
	db *bun.DB,
	platform string,
	channelIDs []int64,
) string {
	normalizedPlatform := strings.TrimSpace(platform)
	if normalizedPlatform == "" || len(channelIDs) == 0 {
		return snapshotDisplayEmpty
	}

	channelLabels := make([]string, 0, len(channelIDs))
	for _, channelID := range channelIDs {
		channelLabels = append(channelLabels, s.resolveSnapshotChannelName(ctx, db, channelID))
	}

	return normalizedPlatform +
		notificationChannelsDisplayPrefixSeparator +
		strings.Join(channelLabels, notificationChannelsDisplayNameSeparator)
}

func (s *ScheduledTasksService) resolveSnapshotChannelName(ctx context.Context, db *bun.DB, channelID int64) string {
	if channelID <= 0 {
		return snapshotDisplayEmpty
	}

	var channelName string
	if err := db.NewSelect().
		Table("channels").
		Column("name").
		Where("id = ?", channelID).
		Limit(1).
		Scan(ctx, &channelName); err == nil {
		channelName = strings.TrimSpace(channelName)
		if channelName != "" {
			return channelName
		}
	}

	return strconv.FormatInt(channelID, 10)
}

func normalizeOperationType(operationType string) string {
	switch strings.TrimSpace(strings.ToLower(operationType)) {
	case OperationTypeCreate:
		return OperationTypeCreate
	case OperationTypeDelete:
		return OperationTypeDelete
	default:
		return OperationTypeUpdate
	}
}

func normalizeOperationSource(source string) string {
	if strings.TrimSpace(strings.ToLower(source)) == OperationSourceAI {
		return OperationSourceAI
	}
	return OperationSourceManual
}

func decodeScheduledTaskOperationChangedFields(raw string) []ScheduledTaskOperationChangedField {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []ScheduledTaskOperationChangedField{}
	}
	var items []ScheduledTaskOperationChangedField
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return []ScheduledTaskOperationChangedField{}
	}
	return items
}

func decodeScheduledTaskOperationSnapshot(raw string) ScheduledTaskOperationSnapshot {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ScheduledTaskOperationSnapshot{}
	}
	var snapshot ScheduledTaskOperationSnapshot
	if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
		return ScheduledTaskOperationSnapshot{}
	}
	return snapshot
}

func formatEnabledDisplay(enabled bool) string {
	if enabled {
		return "启用"
	}
	return "停用"
}

func formatExpirationDisplay(expiresAt *time.Time) string {
	if expiresAt == nil {
		return snapshotDisplayEmpty
	}
	return expiresAt.UTC().Format("2006-01-02 15:04:05")
}

func timePointersEqual(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Equal(*right)
}

func formatAgentDisplay(agentID int64) string {
	if agentID <= 0 {
		return snapshotDisplayEmpty
	}
	return strconv.FormatInt(agentID, 10)
}

func formatNotificationChannelsDisplay(platform string, channelIDs []int64) string {
	if strings.TrimSpace(platform) == "" || len(channelIDs) == 0 {
		return snapshotDisplayEmpty
	}
	parts := make([]string, 0, len(channelIDs))
	for _, channelID := range channelIDs {
		parts = append(parts, strconv.FormatInt(channelID, 10))
	}
	return fmt.Sprintf(
		"%s%s%s",
		strings.TrimSpace(platform),
		notificationChannelsDisplayPrefixSeparator,
		strings.Join(parts, notificationChannelsDisplayNameSeparator),
	)
}

func formatScheduleDisplay(scheduleType, scheduleValue, cronExpr string) string {
	switch scheduleType {
	case ScheduleTypePreset:
		switch scheduleValue {
		case "every_minute":
			return "每分钟"
		case "every_5_minutes":
			return "每5分钟"
		case "every_15_minutes":
			return "每15分钟"
		case "every_hour":
			return "每小时"
		case "every_day_0900":
			return "每日9点"
		case "every_day_1800":
			return "每日18点"
		case "weekdays_0900":
			return "工作日9点"
		case "every_monday_0900":
			return "每周一9点"
		case "every_month_1_0900":
			return "每月1日9点"
		default:
			return scheduleValue
		}
	case ScheduleTypeCustom:
		return scheduleValue
	case ScheduleTypeCron:
		if strings.TrimSpace(cronExpr) != "" {
			return strings.TrimSpace(cronExpr)
		}
		return scheduleValue
	default:
		if strings.TrimSpace(cronExpr) != "" {
			return strings.TrimSpace(cronExpr)
		}
		if strings.TrimSpace(scheduleValue) != "" {
			return strings.TrimSpace(scheduleValue)
		}
		return "-"
	}
}

func taskModelsToDTOs(models []scheduledTaskModel) []ScheduledTask {
	out := make([]ScheduledTask, 0, len(models))
	for i := range models {
		out = append(out, models[i].toDTO())
	}
	return out
}
