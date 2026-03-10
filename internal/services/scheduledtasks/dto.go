package scheduledtasks

import (
	"time"

	"chatclaw/internal/services/chat"
	"chatclaw/internal/services/conversations"
)

const (
	ScheduleTypePreset = "preset"
	ScheduleTypeCustom = "custom"
	ScheduleTypeCron   = "cron"

	TaskStatusPending = "pending"
	TaskStatusRunning = "running"
	TaskStatusSuccess = "success"
	TaskStatusFailed  = "failed"

	RunTriggerSchedule = "schedule"
	RunTriggerManual   = "manual"

	RunStatusQueued  = "queued"
	RunStatusRunning = "running"
	RunStatusSuccess = "success"
	RunStatusFailed  = "failed"
)

type ScheduledTask struct {
	ID int64 `json:"id"`

	Name    string `json:"name"`
	Prompt  string `json:"prompt"`
	AgentID int64  `json:"agent_id"`

	ScheduleType  string     `json:"schedule_type"`
	ScheduleValue string     `json:"schedule_value"`
	CronExpr      string     `json:"cron_expr"`
	Timezone      string     `json:"timezone"`
	Enabled       bool       `json:"enabled"`
	LastRunAt     *time.Time `json:"last_run_at"`
	NextRunAt     *time.Time `json:"next_run_at"`
	LastStatus    string     `json:"last_status"`
	LastError     string     `json:"last_error"`
	LastRunID     *int64     `json:"last_run_id"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ScheduledTaskRun struct {
	ID int64 `json:"id"`

	TaskID             int64      `json:"task_id"`
	TriggerType        string     `json:"trigger_type"`
	Status             string     `json:"status"`
	ErrorMessage       string     `json:"error_message"`
	ConversationID     *int64     `json:"conversation_id"`
	UserMessageID      *int64     `json:"user_message_id"`
	AssistantMessageID *int64     `json:"assistant_message_id"`
	SnapshotTaskName   string     `json:"snapshot_task_name"`
	SnapshotPrompt     string     `json:"snapshot_prompt"`
	SnapshotAgentID    int64      `json:"snapshot_agent_id"`
	StartedAt          time.Time  `json:"started_at"`
	FinishedAt         *time.Time `json:"finished_at"`
	DurationMS         int64      `json:"duration_ms"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

type ScheduledTaskSummary struct {
	Total   int `json:"total"`
	Running int `json:"running"`
	Paused  int `json:"paused"`
	Failed  int `json:"failed"`
}

type CreateScheduledTaskInput struct {
	Name          string `json:"name"`
	Prompt        string `json:"prompt"`
	AgentID       int64  `json:"agent_id"`
	ScheduleType  string `json:"schedule_type"`
	ScheduleValue string `json:"schedule_value"`
	CronExpr      string `json:"cron_expr"`
	Enabled       bool   `json:"enabled"`
}

type ScheduleValidationResult struct {
	ScheduleType  string     `json:"schedule_type"`
	ScheduleValue string     `json:"schedule_value"`
	CronExpr      string     `json:"cron_expr"`
	Timezone      string     `json:"timezone"`
	NextRunAt     *time.Time `json:"next_run_at,omitempty"`
}

type UpdateScheduledTaskInput struct {
	Name          *string `json:"name"`
	Prompt        *string `json:"prompt"`
	AgentID       *int64  `json:"agent_id"`
	ScheduleType  *string `json:"schedule_type"`
	ScheduleValue *string `json:"schedule_value"`
	CronExpr      *string `json:"cron_expr"`
	Enabled       *bool   `json:"enabled"`
}

type ScheduledTaskRunDetail struct {
	Run          ScheduledTaskRun            `json:"run"`
	Conversation *conversations.Conversation `json:"conversation"`
	Messages     []chat.Message              `json:"messages"`
}
