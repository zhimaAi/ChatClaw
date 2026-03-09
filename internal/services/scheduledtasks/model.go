package scheduledtasks

import (
	"context"
	"encoding/json"
	"log/slog"
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

	Name           string     `bun:"name,notnull"`
	Prompt         string     `bun:"prompt,notnull"`
	AgentID        int64      `bun:"agent_id,notnull"`
	LLMProviderID  string     `bun:"llm_provider_id,notnull"`
	LLMModelID     string     `bun:"llm_model_id,notnull"`
	LibraryIDs     string     `bun:"library_ids,notnull"`
	EnableThinking bool       `bun:"enable_thinking,notnull"`
	ChatMode       string     `bun:"chat_mode,notnull"`
	ScheduleType   string     `bun:"schedule_type,notnull"`
	ScheduleValue  string     `bun:"schedule_value,notnull"`
	CronExpr       string     `bun:"cron_expr,notnull"`
	Timezone       string     `bun:"timezone,notnull"`
	Enabled        bool       `bun:"enabled,notnull"`
	LastRunAt      *time.Time `bun:"last_run_at"`
	NextRunAt      *time.Time `bun:"next_run_at"`
	LastStatus     string     `bun:"last_status,notnull"`
	LastError      string     `bun:"last_error,notnull"`
	LastRunID      *int64     `bun:"last_run_id"`
}

type scheduledTaskRunModel struct {
	bun.BaseModel `bun:"table:scheduled_task_runs,alias:str"`

	ID        int64     `bun:"id,pk,autoincrement"`
	CreatedAt time.Time `bun:"created_at,notnull"`
	UpdatedAt time.Time `bun:"updated_at,notnull"`

	TaskID                 int64      `bun:"task_id,notnull"`
	TriggerType            string     `bun:"trigger_type,notnull"`
	Status                 string     `bun:"status,notnull"`
	StartedAt              time.Time  `bun:"started_at,notnull"`
	FinishedAt             *time.Time `bun:"finished_at"`
	DurationMS             int64      `bun:"duration_ms,notnull"`
	ErrorMessage           string     `bun:"error_message,notnull"`
	ConversationID         *int64     `bun:"conversation_id"`
	UserMessageID          *int64     `bun:"user_message_id"`
	AssistantMessageID     *int64     `bun:"assistant_message_id"`
	SnapshotTaskName       string     `bun:"snapshot_task_name,notnull"`
	SnapshotPrompt         string     `bun:"snapshot_prompt,notnull"`
	SnapshotAgentID        int64      `bun:"snapshot_agent_id,notnull"`
	SnapshotProviderID     string     `bun:"snapshot_provider_id,notnull"`
	SnapshotModelID        string     `bun:"snapshot_model_id,notnull"`
	SnapshotLibraryIDs     string     `bun:"snapshot_library_ids,notnull"`
	SnapshotEnableThinking bool       `bun:"snapshot_enable_thinking,notnull"`
	SnapshotChatMode       string     `bun:"snapshot_chat_mode,notnull"`
}

var _ bun.BeforeInsertHook = (*scheduledTaskModel)(nil)
var _ bun.BeforeUpdateHook = (*scheduledTaskModel)(nil)
var _ bun.BeforeInsertHook = (*scheduledTaskRunModel)(nil)
var _ bun.BeforeUpdateHook = (*scheduledTaskRunModel)(nil)

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

func (m *scheduledTaskModel) toDTO() ScheduledTask {
	return ScheduledTask{
		ID:             m.ID,
		Name:           m.Name,
		Prompt:         m.Prompt,
		AgentID:        m.AgentID,
		LLMProviderID:  m.LLMProviderID,
		LLMModelID:     m.LLMModelID,
		LibraryIDs:     parseInt64Array(m.LibraryIDs),
		EnableThinking: m.EnableThinking,
		ChatMode:       m.ChatMode,
		ScheduleType:   m.ScheduleType,
		ScheduleValue:  m.ScheduleValue,
		CronExpr:       m.CronExpr,
		Timezone:       m.Timezone,
		Enabled:        m.Enabled,
		LastRunAt:      m.LastRunAt,
		NextRunAt:      m.NextRunAt,
		LastStatus:     m.LastStatus,
		LastError:      m.LastError,
		LastRunID:      m.LastRunID,
		CreatedAt:      m.CreatedAt,
		UpdatedAt:      m.UpdatedAt,
	}
}

func (m *scheduledTaskRunModel) toDTO() ScheduledTaskRun {
	return ScheduledTaskRun{
		ID:                     m.ID,
		TaskID:                 m.TaskID,
		TriggerType:            m.TriggerType,
		Status:                 m.Status,
		ErrorMessage:           m.ErrorMessage,
		ConversationID:         m.ConversationID,
		UserMessageID:          m.UserMessageID,
		AssistantMessageID:     m.AssistantMessageID,
		SnapshotTaskName:       m.SnapshotTaskName,
		SnapshotPrompt:         m.SnapshotPrompt,
		SnapshotAgentID:        m.SnapshotAgentID,
		SnapshotProviderID:     m.SnapshotProviderID,
		SnapshotModelID:        m.SnapshotModelID,
		SnapshotLibraryIDs:     parseInt64Array(m.SnapshotLibraryIDs),
		SnapshotEnableThinking: m.SnapshotEnableThinking,
		SnapshotChatMode:       m.SnapshotChatMode,
		StartedAt:              m.StartedAt,
		FinishedAt:             m.FinishedAt,
		DurationMS:             m.DurationMS,
		CreatedAt:              m.CreatedAt,
		UpdatedAt:              m.UpdatedAt,
	}
}

func parseInt64Array(raw string) []int64 {
	if raw == "" || raw == "[]" {
		return []int64{}
	}
	var ids []int64
	if err := json.Unmarshal([]byte(raw), &ids); err != nil {
		slog.Warn("[scheduledtasks] failed to parse int64 array", "raw", raw, "error", err)
		return []int64{}
	}
	if ids == nil {
		return []int64{}
	}
	return ids
}

func encodeInt64Array(ids []int64) string {
	if len(ids) == 0 {
		return "[]"
	}
	data, err := json.Marshal(ids)
	if err != nil {
		return "[]"
	}
	return string(data)
}
