package openclawcron

import (
	"encoding/json"
	"time"
)

// OpenClawCronAgentOption is the lightweight agent option for cron forms.
// OpenClaw Cron 助手选项，供前端新增/编辑弹窗使用。
type OpenClawCronAgentOption struct {
	ID              int64  `json:"id"`
	Name            string `json:"name"`
	OpenClawAgentID string `json:"openclaw_agent_id"`
}

// OpenClawCronSummary is the page-level summary card model.
// OpenClaw Cron 页面统计卡片模型。
type OpenClawCronSummary struct {
	Total     int    `json:"total"`
	Enabled   int    `json:"enabled"`
	Disabled  int    `json:"disabled"`
	Failed    int    `json:"failed"`
	StorePath string `json:"store_path"`
}

// OpenClawCronJob is the flattened job DTO returned to the frontend.
// OpenClaw Cron 任务 DTO，已把嵌套 schedule/payload/delivery 展平成表格友好字段。
type OpenClawCronJob struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`

	CreatedAtMs int64 `json:"created_at_ms"`
	UpdatedAtMs int64 `json:"updated_at_ms"`

	AgentID   string `json:"agent_id"`
	AgentName string `json:"agent_name"`

	SessionTarget string `json:"session_target"`
	SessionKey    string `json:"session_key"`
	WakeMode      string `json:"wake_mode"`

	ScheduleKind string `json:"schedule_kind"`
	CronExpr     string `json:"cron_expr"`
	EveryMs      int64  `json:"every_ms"`
	AtISO        string `json:"at_iso"`
	Timezone     string `json:"timezone"`
	Exact        bool   `json:"exact"`

	PayloadKind  string `json:"payload_kind"`
	Message      string `json:"message"`
	SystemEvent  string `json:"system_event"`
	Model        string `json:"model"`
	Thinking     string `json:"thinking"`
	ExpectFinal  bool   `json:"expect_final"`
	LightContext bool   `json:"light_context"`
	TimeoutMs    int64  `json:"timeout_ms"`

	DeliveryMode       string `json:"delivery_mode"`
	DeliveryChannel    string `json:"delivery_channel"`
	DeliveryTo         string `json:"delivery_to"`
	DeliveryAccountID  string `json:"delivery_account_id"`
	Announce           bool   `json:"announce"`
	BestEffortDeliver  bool   `json:"best_effort_deliver"`
	DeleteAfterRun     bool   `json:"delete_after_run"`
	KeepAfterRun       bool   `json:"keep_after_run"`
	LastRunAtMs        int64  `json:"last_run_at_ms"`
	NextRunAtMs        int64  `json:"next_run_at_ms"`
	LastStatus         string `json:"last_status"`
	LastError          string `json:"last_error"`
	LastDurationMs     int64  `json:"last_duration_ms"`
	LastDeliveryStatus string `json:"last_delivery_status"`
}

// CreateOpenClawCronJobInput matches the UI form but preserves OpenClaw-native schedule semantics.
// 新建任务输入保持 OpenClaw 原生时间语义，不做 ChatClaw 风格预设转换。
type CreateOpenClawCronJobInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	AgentID     string `json:"agent_id"`

	ScheduleKind string `json:"schedule_kind"`
	CronExpr     string `json:"cron_expr"`
	Every        string `json:"every"`
	At           string `json:"at"`
	Timezone     string `json:"timezone"`
	Exact        bool   `json:"exact"`

	Message      string `json:"message"`
	SystemEvent  string `json:"system_event"`
	Model        string `json:"model"`
	Thinking     string `json:"thinking"`
	ExpectFinal  bool   `json:"expect_final"`
	LightContext bool   `json:"light_context"`
	TimeoutMs    int64  `json:"timeout_ms"`

	SessionTarget string `json:"session_target"`
	SessionKey    string `json:"session_key"`
	WakeMode      string `json:"wake_mode"`

	Announce          bool   `json:"announce"`
	DeliveryMode      string `json:"delivery_mode"`
	DeliveryChannel   string `json:"delivery_channel"`
	DeliveryTo        string `json:"delivery_to"`
	DeliveryAccountID string `json:"delivery_account_id"`
	BestEffortDeliver bool   `json:"best_effort_deliver"`

	DeleteAfterRun bool `json:"delete_after_run"`
	KeepAfterRun   bool `json:"keep_after_run"`
	Enabled        bool `json:"enabled"`
}

// UpdateOpenClawCronJobInput is a patch-style input for edit flows.
// 编辑输入使用 patch 语义，避免把未修改字段错误覆盖到 CLI。
type UpdateOpenClawCronJobInput struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	AgentID     *string `json:"agent_id"`
	ClearAgent  bool    `json:"clear_agent"`

	ScheduleKind *string `json:"schedule_kind"`
	CronExpr     *string `json:"cron_expr"`
	Every        *string `json:"every"`
	At           *string `json:"at"`
	Timezone     *string `json:"timezone"`
	Exact        *bool   `json:"exact"`

	Message      *string `json:"message"`
	SystemEvent  *string `json:"system_event"`
	Model        *string `json:"model"`
	Thinking     *string `json:"thinking"`
	ExpectFinal  *bool   `json:"expect_final"`
	LightContext *bool   `json:"light_context"`
	TimeoutMs    *int64  `json:"timeout_ms"`

	SessionTarget   *string `json:"session_target"`
	SessionKey      *string `json:"session_key"`
	ClearSessionKey bool    `json:"clear_session_key"`
	WakeMode        *string `json:"wake_mode"`

	Announce          *bool   `json:"announce"`
	DeliveryMode      *string `json:"delivery_mode"`
	DeliveryChannel   *string `json:"delivery_channel"`
	DeliveryTo        *string `json:"delivery_to"`
	DeliveryAccountID *string `json:"delivery_account_id"`
	BestEffortDeliver *bool   `json:"best_effort_deliver"`

	DeleteAfterRun *bool `json:"delete_after_run"`
	KeepAfterRun   *bool `json:"keep_after_run"`
	Enabled        *bool `json:"enabled"`
}

// OpenClawCronRunEntry is the line-level run history entry from cron/runs/*.jsonl.
// 历史运行记录直接来自 OpenClaw 的 run 日志文件或 cron runs 命令。
type OpenClawCronRunEntry struct {
	TimestampMs    int64  `json:"ts"`
	JobID          string `json:"job_id"`
	Action         string `json:"action"`
	Status         string `json:"status"`
	Error          string `json:"error"`
	Summary        string `json:"summary"`
	RunAtMs        int64  `json:"run_at_ms"`
	DurationMs     int64  `json:"duration_ms"`
	NextRunAtMs    int64  `json:"next_run_at_ms"`
	Model          string `json:"model"`
	Provider       string `json:"provider"`
	DeliveryStatus string `json:"delivery_status"`
	SessionID      string `json:"session_id"`
	SessionKey     string `json:"session_key"`
}

// OpenClawCronMessageBlock is the frontend-friendly block model for transcript rendering.
// Transcript 消息块模型，保留 OpenClaw 原始 thinking/text/tool 序列信息。
type OpenClawCronMessageBlock struct {
	Type       string          `json:"type"`
	Text       string          `json:"text,omitempty"`
	Thinking   string          `json:"thinking,omitempty"`
	Name       string          `json:"name,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	ArgsJSON   string          `json:"args_json,omitempty"`
	ResultJSON string          `json:"result_json,omitempty"`
	Raw        json.RawMessage `json:"raw,omitempty"`
}

// OpenClawCronTranscriptMessage is the transcript message shown in the run detail panel.
// 运行详情右侧消息流中的一条 transcript 消息。
type OpenClawCronTranscriptMessage struct {
	ID          string                     `json:"id"`
	Role        string                     `json:"role"`
	Timestamp   time.Time                  `json:"timestamp"`
	Provider    string                     `json:"provider"`
	Model       string                     `json:"model"`
	StopReason  string                     `json:"stop_reason"`
	ContentText string                     `json:"content_text"`
	Blocks      []OpenClawCronMessageBlock `json:"blocks"`
}

// OpenClawCronRunDetail aggregates run metadata with transcript messages.
// 运行详情同时包含 run 汇总信息和 session transcript。
type OpenClawCronRunDetail struct {
	Run                 OpenClawCronRunEntry            `json:"run"`
	SessionFilePath     string                          `json:"session_file_path"`
	RunFilePath         string                          `json:"run_file_path"`
	ConversationID      int64                           `json:"conversation_id"`
	ConversationAgentID int64                           `json:"conversation_agent_id"`
	Messages            []OpenClawCronTranscriptMessage `json:"messages"`
	IsLive              bool                            `json:"is_live"`
}
