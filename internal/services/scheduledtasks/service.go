package scheduledtasks

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"chatclaw/internal/errs"
	"chatclaw/internal/services/chat"
	"chatclaw/internal/services/conversations"
	"chatclaw/internal/sqlite"

	"github.com/uptrace/bun"
	"github.com/wailsapp/wails/v3/pkg/application"
)

type ScheduledTasksService struct {
	app        *application.App
	db         *bun.DB
	scheduler  *scheduler
	runnerDeps runDependencies
}

func NewScheduledTasksService(app *application.App, convSvc *conversations.ConversationsService, chatSvc *chat.ChatService) *ScheduledTasksService {
	svc := &ScheduledTasksService{
		app:       app,
		scheduler: newScheduler(),
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
		app:       app,
		db:        db,
		scheduler: newScheduler(),
		runnerDeps: runDependencies{
			conversations: convSvc,
			chat:          chatSvc,
		},
	}
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
	return s.reloadEnabledTasks()
}

func (s *ScheduledTasksService) Stop() {
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
		out = append(out, models[i].toDTO())
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
	dto := model.toDTO()
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
		if !task.Enabled {
			summary.Paused++
		}
		switch task.LastStatus {
		case TaskStatusRunning:
			summary.Running++
		case TaskStatusFailed:
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
	db, err := s.dbOrGlobal()
	if err != nil {
		return nil, err
	}
	model, err := s.buildCreateModel(input)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.ensureAgentExists(ctx, db, model.AgentID); err != nil {
		return nil, err
	}
	if _, err := db.NewInsert().Model(model).Exec(ctx); err != nil {
		return nil, errs.Wrap("error.scheduled_task_create_failed", err)
	}

	dto := model.toDTO()
	if err := s.scheduler.register(dto, func() { s.runScheduledTask(dto.ID, RunTriggerSchedule) }); err != nil {
		return nil, errs.Wrap("error.scheduled_task_create_failed", err)
	}
	return &dto, nil
}

func (s *ScheduledTasksService) UpdateScheduledTask(id int64, input UpdateScheduledTaskInput) (*ScheduledTask, error) {
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
	if err := s.applyUpdateInput(&model, input); err != nil {
		return nil, err
	}
	if err := s.ensureAgentExists(ctx, db, model.AgentID); err != nil {
		return nil, err
	}
	if _, err := db.NewUpdate().
		Model((*scheduledTaskModel)(nil)).
		Set("name = ?", model.Name).
		Set("prompt = ?", model.Prompt).
		Set("agent_id = ?", model.AgentID).
		Set("schedule_type = ?", model.ScheduleType).
		Set("schedule_value = ?", model.ScheduleValue).
		Set("cron_expr = ?", model.CronExpr).
		Set("timezone = ?", model.Timezone).
		Set("enabled = ?", model.Enabled).
		Set("next_run_at = ?", model.NextRunAt).
		Where("id = ?", id).
		Exec(ctx); err != nil {
		return nil, errs.Wrap("error.scheduled_task_update_failed", err)
	}

	dto := model.toDTO()
	if err := s.scheduler.register(dto, func() { s.runScheduledTask(dto.ID, RunTriggerSchedule) }); err != nil {
		return nil, errs.Wrap("error.scheduled_task_update_failed", err)
	}
	return &dto, nil
}

func (s *ScheduledTasksService) DeleteScheduledTask(id int64) error {
	if id <= 0 {
		return errs.New("error.scheduled_task_id_required")
	}
	db, err := s.dbOrGlobal()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

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
	return nil
}

func (s *ScheduledTasksService) SetScheduledTaskEnabled(id int64, enabled bool) (*ScheduledTask, error) {
	return s.UpdateScheduledTask(id, UpdateScheduledTaskInput{Enabled: &enabled})
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

	schedule, err := parseSchedule(input.ScheduleType, input.ScheduleValue, input.CronExpr, time.Now())
	if err != nil {
		return nil, errs.Wrap("error.scheduled_task_schedule_invalid", err)
	}

	model := &scheduledTaskModel{
		Name:          name,
		Prompt:        prompt,
		AgentID:       input.AgentID,
		ScheduleType:  schedule.ScheduleType,
		ScheduleValue: schedule.ScheduleValue,
		CronExpr:      schedule.CronExpr,
		Timezone:      schedule.Timezone,
		Enabled:       input.Enabled,
		LastStatus:    TaskStatusPending,
		LastError:     "",
	}
	if input.Enabled {
		model.NextRunAt = schedule.NextRunAt
	}
	return model, nil
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
		schedule, err := parseSchedule(scheduleType, scheduleValue, cronExpr, time.Now())
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
			nextRunAt, err := s.scheduler.next(model.CronExpr, time.Now())
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

	var models []scheduledTaskModel
	if err := db.NewSelect().
		Model(&models).
		Where("deleted_at IS NULL").
		Where("enabled = ?", true).
		Scan(ctx); err != nil {
		return errs.Wrap("error.scheduled_task_list_failed", err)
	}

	for i := range models {
		task := models[i].toDTO()
		if err := s.scheduler.register(task, func() { s.runScheduledTask(task.ID, RunTriggerSchedule) }); err != nil {
			return err
		}
	}
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
	_, _ = s.executeTask(context.Background(), task, triggerType)
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

func taskModelsToDTOs(models []scheduledTaskModel) []ScheduledTask {
	out := make([]ScheduledTask, 0, len(models))
	for i := range models {
		out = append(out, models[i].toDTO())
	}
	return out
}
