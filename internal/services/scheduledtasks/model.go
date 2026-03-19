package scheduledtasks

import (
	"context"
	"time"

	"chatclaw/internal/sqlite"

	"github.com/uptrace/bun"
)

type scheduledTaskModel struct {
	bun.BaseModel `bun:"table:scheduled_tasks,alias:st"`

	ID        int64      `bun:"id,pk,autoincrement"`
	CreatedAt time.Time  `bun:"created_at,notnull"`
	UpdatedAt time.Time  `bun:"updated_at,notnull"`
	DeletedAt *time.Time `bun:"deleted_at"`

	Name                   string     `bun:"name,notnull"`
	Prompt                 string     `bun:"prompt,notnull"`
	AgentID                int64      `bun:"agent_id,notnull"`
	NotificationPlatform   string     `bun:"notification_platform,notnull"`
	NotificationChannelIDs string     `bun:"notification_channel_ids,notnull"`
	ScheduleType           string     `bun:"schedule_type,notnull"`
	ScheduleValue          string     `bun:"schedule_value,notnull"`
	CronExpr               string     `bun:"cron_expr,notnull"`
	Timezone               string     `bun:"timezone,notnull"`
	Enabled                bool       `bun:"enabled,notnull"`
	LastRunAt              *time.Time `bun:"last_run_at"`
	NextRunAt              *time.Time `bun:"next_run_at"`
	LastStatus             string     `bun:"last_status,notnull"`
	LastError              string     `bun:"last_error,notnull"`
	LastRunID              *int64     `bun:"last_run_id"`
}

type scheduledTaskRunModel struct {
	bun.BaseModel `bun:"table:scheduled_task_runs,alias:str"`

	ID        int64     `bun:"id,pk,autoincrement"`
	CreatedAt time.Time `bun:"created_at,notnull"`
	UpdatedAt time.Time `bun:"updated_at,notnull"`

	TaskID             int64      `bun:"task_id,notnull"`
	TriggerType        string     `bun:"trigger_type,notnull"`
	Status             string     `bun:"status,notnull"`
	StartedAt          time.Time  `bun:"started_at,notnull"`
	FinishedAt         *time.Time `bun:"finished_at"`
	DurationMS         int64      `bun:"duration_ms,notnull"`
	ErrorMessage       string     `bun:"error_message,notnull"`
	ConversationID     *int64     `bun:"conversation_id"`
	UserMessageID      *int64     `bun:"user_message_id"`
	AssistantMessageID *int64     `bun:"assistant_message_id"`
	SnapshotTaskName   string     `bun:"snapshot_task_name,notnull"`
	SnapshotPrompt     string     `bun:"snapshot_prompt,notnull"`
	SnapshotAgentID    int64      `bun:"snapshot_agent_id,notnull"`
}

type scheduledTaskOperationLogModel struct {
	bun.BaseModel `bun:"table:scheduled_task_operation_logs,alias:stol"`

	ID               int64     `bun:"id,pk,autoincrement"`
	CreatedAt        time.Time `bun:"created_at,notnull"`
	TaskID           int64     `bun:"task_id,notnull"`
	TaskNameSnapshot string    `bun:"task_name_snapshot,notnull"`
	OperationType    string    `bun:"operation_type,notnull"`
	OperationSource  string    `bun:"operation_source,notnull"`
	ChangedFieldsJSON string   `bun:"changed_fields_json,notnull"`
	TaskSnapshotJSON string    `bun:"task_snapshot_json,notnull"`
}

type scheduledTaskAgentRow struct {
	ID                   int64  `bun:"id"`
	DefaultLLMProviderID string `bun:"default_llm_provider_id"`
	DefaultLLMModelID    string `bun:"default_llm_model_id"`
}

type scheduledTaskConversationModelRow struct {
	ProviderID string `bun:"provider_id"`
	ModelID    string `bun:"model_id"`
}

var _ bun.BeforeInsertHook = (*scheduledTaskModel)(nil)
var _ bun.BeforeUpdateHook = (*scheduledTaskModel)(nil)
var _ bun.BeforeInsertHook = (*scheduledTaskRunModel)(nil)
var _ bun.BeforeUpdateHook = (*scheduledTaskRunModel)(nil)
var _ bun.BeforeInsertHook = (*scheduledTaskOperationLogModel)(nil)

func (*scheduledTaskModel) BeforeInsert(ctx context.Context, query *bun.InsertQuery) error {
	now := sqlite.NowUTC()
	query.Value("created_at", "?", now)
	query.Value("updated_at", "?", now)
	return nil
}

func (*scheduledTaskModel) BeforeUpdate(ctx context.Context, query *bun.UpdateQuery) error {
	query.Set("updated_at = ?", sqlite.NowUTC())
	return nil
}

func (*scheduledTaskRunModel) BeforeInsert(ctx context.Context, query *bun.InsertQuery) error {
	now := sqlite.NowUTC()
	query.Value("created_at", "?", now)
	query.Value("updated_at", "?", now)
	return nil
}

func (*scheduledTaskRunModel) BeforeUpdate(ctx context.Context, query *bun.UpdateQuery) error {
	query.Set("updated_at = ?", sqlite.NowUTC())
	return nil
}

func (*scheduledTaskOperationLogModel) BeforeInsert(ctx context.Context, query *bun.InsertQuery) error {
	query.Value("created_at", "?", sqlite.NowUTC())
	return nil
}

func (m *scheduledTaskModel) toDTO() ScheduledTask {
	return ScheduledTask{
		ID:                     m.ID,
		Name:                   m.Name,
		Prompt:                 m.Prompt,
		AgentID:                m.AgentID,
		NotificationPlatform:   m.NotificationPlatform,
		NotificationChannelIDs: parseNotificationChannelIDs(m.NotificationChannelIDs),
		ScheduleType:           m.ScheduleType,
		ScheduleValue:          m.ScheduleValue,
		CronExpr:               m.CronExpr,
		Timezone:               m.Timezone,
		Enabled:                m.Enabled,
		LastRunAt:              m.LastRunAt,
		NextRunAt:              m.NextRunAt,
		LastStatus:             m.LastStatus,
		LastError:              m.LastError,
		LastRunID:              m.LastRunID,
		CreatedAt:              m.CreatedAt,
		UpdatedAt:              m.UpdatedAt,
	}
}

func (m *scheduledTaskRunModel) toDTO() ScheduledTaskRun {
	return ScheduledTaskRun{
		ID:                 m.ID,
		TaskID:             m.TaskID,
		TriggerType:        m.TriggerType,
		Status:             m.Status,
		ErrorMessage:       m.ErrorMessage,
		ConversationID:     m.ConversationID,
		UserMessageID:      m.UserMessageID,
		AssistantMessageID: m.AssistantMessageID,
		SnapshotTaskName:   m.SnapshotTaskName,
		SnapshotPrompt:     m.SnapshotPrompt,
		SnapshotAgentID:    m.SnapshotAgentID,
		StartedAt:          m.StartedAt,
		FinishedAt:         m.FinishedAt,
		DurationMS:         m.DurationMS,
		CreatedAt:          m.CreatedAt,
		UpdatedAt:          m.UpdatedAt,
	}
}

func (m *scheduledTaskOperationLogModel) toDTO(changedFields []ScheduledTaskOperationChangedField) ScheduledTaskOperationLog {
	return ScheduledTaskOperationLog{
		ID:               m.ID,
		TaskID:           m.TaskID,
		TaskNameSnapshot: m.TaskNameSnapshot,
		OperationType:    m.OperationType,
		OperationSource:  m.OperationSource,
		ChangedFields:    changedFields,
		CreatedAt:        m.CreatedAt,
	}
}
