package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"chatclaw/internal/services/i18n"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

const (
	// scheduledTaskExpiresAtDescEN keeps the AI tool parameter description consistent
	// across create/update preview and confirm schemas.
	scheduledTaskExpiresAtDescEN = "Optional expiration time in RFC3339 format, for example '2026-04-01T23:59:59Z'."
	// scheduledTaskExpiresAtDescZH mirrors the expiration parameter description for Chinese locales.
	scheduledTaskExpiresAtDescZH = "鍙€夌殑鍒版湡鏃堕棿锛岃浣跨敤 RFC3339 鏍煎紡锛屼緥濡?2026-04-01T23:59:59Z'銆?"
	// scheduledTaskExpiresAtFormatError keeps the expires_at validation message stable for tool callers.
	scheduledTaskExpiresAtFormatError = "expires_at must use RFC3339 format, for example 2026-04-01T23:59:59Z"
)

type ScheduledTaskAgent struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type ScheduledTaskRecord struct {
	ID                     int64      `json:"id"`
	Name                   string     `json:"name"`
	Prompt                 string     `json:"prompt"`
	AgentID                int64      `json:"agent_id"`
	AgentName              string     `json:"agent_name,omitempty"`
	NotificationPlatform   string     `json:"notification_platform,omitempty"`
	NotificationChannelIDs []int64    `json:"notification_channel_ids,omitempty"`
	ScheduleType           string     `json:"schedule_type"`
	ScheduleValue          string     `json:"schedule_value"`
	CronExpr               string     `json:"cron_expr"`
	Timezone               string     `json:"timezone,omitempty"`
	Enabled                bool       `json:"enabled"`
	ExpiresAt              *time.Time `json:"expires_at,omitempty"`
	LastRunAt              *time.Time `json:"last_run_at,omitempty"`
	NextRunAt              *time.Time `json:"next_run_at,omitempty"`
	LastStatus             string     `json:"last_status,omitempty"`
	LastError              string     `json:"last_error,omitempty"`
	LastRunID              *int64     `json:"last_run_id,omitempty"`
	CreatedAt              time.Time  `json:"created_at,omitempty"`
	UpdatedAt              time.Time  `json:"updated_at,omitempty"`
}

type ScheduledTaskCreateInput struct {
	Name                   string  `json:"name"`
	Prompt                 string  `json:"prompt"`
	AgentID                int64   `json:"agent_id"`
	NotificationPlatform   string  `json:"notification_platform"`
	NotificationChannelIDs []int64 `json:"notification_channel_ids"`
	ScheduleType           string  `json:"schedule_type"`
	ScheduleValue          string  `json:"schedule_value"`
	CronExpr               string  `json:"cron_expr"`
	Enabled                bool    `json:"enabled"`
	ExpiresAt              *time.Time `json:"expires_at"`
}

type ScheduledTaskUpdateInput struct {
	Name                   *string  `json:"name"`
	Prompt                 *string  `json:"prompt"`
	AgentID                *int64   `json:"agent_id"`
	NotificationPlatform   *string  `json:"notification_platform"`
	NotificationChannelIDs *[]int64 `json:"notification_channel_ids"`
	ScheduleType           *string  `json:"schedule_type"`
	ScheduleValue          *string  `json:"schedule_value"`
	CronExpr               *string  `json:"cron_expr"`
	Enabled                *bool    `json:"enabled"`
	ExpiresAt              *time.Time `json:"expires_at"`
}

type ScheduledTaskRunRecord struct {
	ID                 int64      `json:"id"`
	TaskID             int64      `json:"task_id"`
	TriggerType        string     `json:"trigger_type"`
	Status             string     `json:"status"`
	ErrorMessage       string     `json:"error_message"`
	ConversationID     *int64     `json:"conversation_id,omitempty"`
	UserMessageID      *int64     `json:"user_message_id,omitempty"`
	AssistantMessageID *int64     `json:"assistant_message_id,omitempty"`
	SnapshotTaskName   string     `json:"snapshot_task_name"`
	SnapshotPrompt     string     `json:"snapshot_prompt"`
	SnapshotAgentID    int64      `json:"snapshot_agent_id"`
	StartedAt          time.Time  `json:"started_at"`
	FinishedAt         *time.Time `json:"finished_at,omitempty"`
	DurationMS         int64      `json:"duration_ms"`
	CreatedAt          time.Time  `json:"created_at,omitempty"`
	UpdatedAt          time.Time  `json:"updated_at,omitempty"`
}

type ScheduledTaskRunDetailRecord struct {
	Run          ScheduledTaskRunRecord `json:"run"`
	Conversation any                    `json:"conversation"`
	Messages     any                    `json:"messages"`
}

type ScheduledTaskValidationResult struct {
	ScheduleType  string     `json:"schedule_type"`
	ScheduleValue string     `json:"schedule_value"`
	CronExpr      string     `json:"cron_expr"`
	Timezone      string     `json:"timezone"`
	NextRunAt     *time.Time `json:"next_run_at,omitempty"`
}

type ScheduledTaskManagementConfig struct {
	ListAgentsForMatchingFn     func() ([]ScheduledTaskAgent, error)
	MatchAgentsByNameFn         func(string) ([]ScheduledTaskAgent, string, error)
	ListScheduledTasksFn        func() ([]ScheduledTaskRecord, error)
	GetScheduledTaskByIDFn      func(int64) (*ScheduledTaskRecord, error)
	FindScheduledTasksFn        func(string) ([]ScheduledTaskRecord, error)
	ListScheduledTaskRunsFn     func(int64, int, int) ([]ScheduledTaskRunRecord, error)
	GetScheduledTaskRunDetailFn func(int64) (*ScheduledTaskRunDetailRecord, error)
	ValidateScheduleFn          func(string, string, string) (*ScheduledTaskValidationResult, error)
	CreateScheduledTaskFn       func(ScheduledTaskCreateInput) (*ScheduledTaskRecord, error)
	UpdateScheduledTaskFn       func(int64, ScheduledTaskUpdateInput) (*ScheduledTaskRecord, error)
	DeleteScheduledTaskFn       func(int64) error
	SetScheduledTaskFn          func(int64, bool) (*ScheduledTaskRecord, error)
}

func NewScheduledTaskManagementTools(config *ScheduledTaskManagementConfig) ([]tool.BaseTool, error) {
	return []tool.BaseTool{
		&scheduledTaskListTool{config: config},
		&agentMatchByNameTool{config: config},
		&scheduledTaskCreatePreviewTool{config: config},
		&scheduledTaskCreateConfirmTool{config: config},
		&scheduledTaskUpdatePreviewTool{config: config},
		&scheduledTaskUpdateConfirmTool{config: config},
		&scheduledTaskDeleteTool{config: config},
		&scheduledTaskEnableTool{config: config},
		&scheduledTaskDisableTool{config: config},
		&scheduledTaskHistoryListTool{config: config},
		&scheduledTaskHistoryDetailTool{config: config},
	}, nil
}

type scheduledTaskListTool struct {
	config *ScheduledTaskManagementConfig
}

type agentMatchByNameTool struct {
	config *ScheduledTaskManagementConfig
}

type scheduledTaskCreatePreviewTool struct {
	config *ScheduledTaskManagementConfig
}

type scheduledTaskCreateConfirmTool struct {
	config *ScheduledTaskManagementConfig
}

type scheduledTaskUpdatePreviewTool struct {
	config *ScheduledTaskManagementConfig
}

type scheduledTaskUpdateConfirmTool struct {
	config *ScheduledTaskManagementConfig
}

type scheduledTaskDeleteTool struct {
	config *ScheduledTaskManagementConfig
}

type scheduledTaskEnableTool struct {
	config *ScheduledTaskManagementConfig
}

type scheduledTaskDisableTool struct {
	config *ScheduledTaskManagementConfig
}

type scheduledTaskHistoryListTool struct {
	config *ScheduledTaskManagementConfig
}

type scheduledTaskHistoryDetailTool struct {
	config *ScheduledTaskManagementConfig
}

type scheduledTaskListInput struct {
	Keyword string `json:"keyword"`
	Status  string `json:"status"`
	Limit   int    `json:"limit"`
}

type agentMatchByNameInput struct {
	Query string `json:"query"`
}

type scheduledTaskCreatePreviewInput struct {
	Name          string `json:"name"`
	Prompt        string `json:"prompt"`
	AgentName     string `json:"agent_name"`
	ScheduleType  string `json:"schedule_type"`
	ScheduleValue string `json:"schedule_value"`
	CronExpr      string `json:"cron_expr"`
	Enabled       *bool  `json:"enabled"`
	ExpiresAt     *string `json:"expires_at"`
}

type scheduledTaskCreateConfirmInput struct {
	Name          string `json:"name"`
	Prompt        string `json:"prompt"`
	AgentID       int64  `json:"agent_id"`
	ScheduleType  string `json:"schedule_type"`
	ScheduleValue string `json:"schedule_value"`
	CronExpr      string `json:"cron_expr"`
	Enabled       *bool  `json:"enabled"`
	ExpiresAt     *string `json:"expires_at"`
}

type scheduledTaskUpdatePreviewInput struct {
	TaskID        int64  `json:"task_id"`
	TaskName      string `json:"task_name"`
	Name          string `json:"name"`
	Prompt        string `json:"prompt"`
	AgentName     string `json:"agent_name"`
	ScheduleType  string `json:"schedule_type"`
	ScheduleValue string `json:"schedule_value"`
	CronExpr      string `json:"cron_expr"`
	Enabled       *bool  `json:"enabled"`
	ExpiresAt     *string `json:"expires_at"`
}

type scheduledTaskUpdateConfirmInput struct {
	TaskID        int64   `json:"task_id"`
	Name          *string `json:"name"`
	Prompt        *string `json:"prompt"`
	AgentID       *int64  `json:"agent_id"`
	ScheduleType  *string `json:"schedule_type"`
	ScheduleValue *string `json:"schedule_value"`
	CronExpr      *string `json:"cron_expr"`
	Enabled       *bool   `json:"enabled"`
	ExpiresAt     *string `json:"expires_at"`
}

type scheduledTaskMutationInput struct {
	TaskID   int64  `json:"task_id"`
	TaskName string `json:"task_name"`
	Confirm  bool   `json:"confirm"`
}

type scheduledTaskHistoryListInput struct {
	TaskID   int64  `json:"task_id"`
	TaskName string `json:"task_name"`
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
}

type scheduledTaskHistoryDetailInput struct {
	RunID int64 `json:"run_id"`
}

func (t *scheduledTaskListTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "scheduled_task_list",
		Desc: selectDesc(
			"List scheduled tasks for search and lookup.",
			"查询计划任务列表，供检索和定位任务。",
		),
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"keyword": {Type: schema.String, Desc: selectDesc("Optional task keyword", "可选的任务关键词"), Required: false},
			"status": {
				Type:     schema.String,
				Desc:     selectDesc("Task status filter", "任务状态过滤"),
				Enum:     []string{"enabled", "disabled", "all"},
				Required: false,
			},
			"limit": {Type: schema.Integer, Desc: selectDesc("Max tasks to return", "最多返回任务数"), Required: false},
		}),
	}, nil
}

func (t *agentMatchByNameTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "agent_match_by_name",
		Desc: selectDesc(
			"Match an AI assistant by name before creating a scheduled task.",
			"按名称匹配 AI 助手，在创建计划任务前定位助手。",
		),
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"query": {Type: schema.String, Desc: selectDesc("Assistant name query", "助手名称查询"), Required: true},
		}),
	}, nil
}

func (t *scheduledTaskCreatePreviewTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "scheduled_task_create_preview",
		Desc: selectDesc(
			"Validate a scheduled task draft and return a confirmation preview without creating it.",
			"校验计划任务草案并返回确认预览，不会真正创建任务。",
		),
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"name":           {Type: schema.String, Desc: selectDesc("Task name", "任务名称"), Required: true},
			"prompt":         {Type: schema.String, Desc: selectDesc("Task prompt", "任务提示词"), Required: true},
			"agent_name":     {Type: schema.String, Desc: selectDesc("Assistant name", "助手名称"), Required: true},
			"schedule_type":  {Type: schema.String, Desc: selectDesc("Schedule type: 'preset', 'custom', or 'cron'. Prefer 'custom' for minute intervals like every 15 minutes instead of cron.", "调度类型: 'preset'、'custom' 或 'cron'。像“每隔5分钟”这类分钟间隔需求，优先使用 'custom'，不要优先使用 cron。"), Required: true},
			"schedule_value": {Type: schema.String, Desc: selectDesc("Schedule value for preset/custom. Example preset: 'every_day_0900'. Example custom interval: '{\"interval_minutes\":15}'. Empty for cron type.", "preset/custom 的调度值。preset 示例: 'every_day_0900'；分钟间隔优先写成 custom，例如 '{\"interval_minutes\":15}'。cron 类型留空。"), Required: true},
			"cron_expr":      {Type: schema.String, Desc: selectDesc("Cron expression for cron type (e.g. '20 15 * * *' for 15:20 daily). Format: minute hour day month weekday.", "Cron表达式，用于cron类型(如'20 15 * * *'表示每天15:20)。格式: 分 时 日 月 周。"), Required: true},
			"enabled":        {Type: schema.Boolean, Desc: selectDesc("Whether enabled after creation", "创建后是否启用"), Required: false},
			"expires_at":     {Type: schema.String, Desc: selectDesc(scheduledTaskExpiresAtDescEN, scheduledTaskExpiresAtDescZH), Required: false},
		}),
	}, nil
}

func (t *scheduledTaskCreateConfirmTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "scheduled_task_create_confirm",
		Desc: selectDesc(
			"Create a scheduled task after the user has confirmed the preview.",
			"在用户确认后真正创建计划任务。",
		),
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"name":           {Type: schema.String, Desc: selectDesc("Task name", "任务名称"), Required: true},
			"prompt":         {Type: schema.String, Desc: selectDesc("Task prompt", "任务提示词"), Required: true},
			"agent_id":       {Type: schema.Integer, Desc: selectDesc("Assistant ID", "助手 ID"), Required: true},
			"schedule_type":  {Type: schema.String, Desc: selectDesc("Schedule type: 'preset', 'custom', or 'cron'. Prefer 'custom' for minute intervals like every 15 minutes instead of cron.", "调度类型: 'preset'、'custom' 或 'cron'。像“每隔5分钟”这类分钟间隔需求，优先使用 'custom'，不要优先使用 cron。"), Required: true},
			"schedule_value": {Type: schema.String, Desc: selectDesc("Schedule value for preset/custom. Example custom interval: '{\"interval_minutes\":15}'.", "preset/custom 的调度值。分钟间隔优先写成 custom，例如 '{\"interval_minutes\":15}'。"), Required: true},
			"cron_expr":      {Type: schema.String, Desc: selectDesc("Cron expression for cron type", "Cron表达式(用于cron类型)"), Required: true},
			"enabled":        {Type: schema.Boolean, Desc: selectDesc("Whether enabled after creation", "创建后是否启用"), Required: false},
			"expires_at":     {Type: schema.String, Desc: selectDesc(scheduledTaskExpiresAtDescEN, scheduledTaskExpiresAtDescZH), Required: false},
		}),
	}, nil
}

func (t *scheduledTaskUpdatePreviewTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "scheduled_task_update_preview",
		Desc: selectDesc(
			"Preview a scheduled task update, validate parameters, and return a confirmation summary without saving.",
			"预览计划任务更新，校验参数并返回确认摘要，不会真正保存。",
		),
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"task_id":        {Type: schema.Integer, Desc: selectDesc("Target task ID", "目标任务 ID"), Required: false},
			"task_name":      {Type: schema.String, Desc: selectDesc("Target task name", "目标任务名称"), Required: false},
			"name":           {Type: schema.String, Desc: selectDesc("New task name", "新的任务名称"), Required: false},
			"prompt":         {Type: schema.String, Desc: selectDesc("New task prompt", "新的任务提示词"), Required: false},
			"agent_name":     {Type: schema.String, Desc: selectDesc("New assistant name", "新的助手名称"), Required: false},
			"schedule_type":  {Type: schema.String, Desc: selectDesc("Schedule type: prefer 'custom' for minute intervals, otherwise use 'preset' or 'cron' as needed", "调度类型：分钟间隔优先用 'custom'，不要优先转成 cron；其他场景按需使用 'preset' 或 'cron'"), Required: false},
			"schedule_value": {Type: schema.String, Desc: selectDesc("Schedule value for preset/custom. Example custom interval: '{\"interval_minutes\":15}'.", "preset/custom 的调度值。分钟间隔优先写成 custom，例如 '{\"interval_minutes\":15}'。"), Required: false},
			"cron_expr":      {Type: schema.String, Desc: selectDesc("Cron expression for cron type", "cron 类型使用的表达式"), Required: false},
			"enabled":        {Type: schema.Boolean, Desc: selectDesc("Whether the task remains enabled", "任务更新后是否启用"), Required: false},
			"expires_at":     {Type: schema.String, Desc: selectDesc(scheduledTaskExpiresAtDescEN, scheduledTaskExpiresAtDescZH), Required: false},
		}),
	}, nil
}

func (t *scheduledTaskUpdateConfirmTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "scheduled_task_update_confirm",
		Desc: selectDesc(
			"Update a scheduled task after the user has confirmed the preview.",
			"在用户确认后真正更新计划任务。",
		),
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"task_id":        {Type: schema.Integer, Desc: selectDesc("Target task ID", "目标任务 ID"), Required: true},
			"name":           {Type: schema.String, Desc: selectDesc("New task name", "新的任务名称"), Required: false},
			"prompt":         {Type: schema.String, Desc: selectDesc("New task prompt", "新的任务提示词"), Required: false},
			"agent_id":       {Type: schema.Integer, Desc: selectDesc("New assistant ID", "新的助手 ID"), Required: false},
			"schedule_type":  {Type: schema.String, Desc: selectDesc("Schedule type: prefer 'custom' for minute intervals, otherwise use 'preset' or 'cron' as needed", "调度类型：分钟间隔优先用 'custom'，不要优先转成 cron；其他场景按需使用 'preset' 或 'cron'"), Required: false},
			"schedule_value": {Type: schema.String, Desc: selectDesc("Schedule value for preset/custom. Example custom interval: '{\"interval_minutes\":15}'.", "preset/custom 的调度值。分钟间隔优先写成 custom，例如 '{\"interval_minutes\":15}'。"), Required: false},
			"cron_expr":      {Type: schema.String, Desc: selectDesc("Cron expression for cron type", "cron 类型使用的表达式"), Required: false},
			"expires_at":     {Type: schema.String, Desc: selectDesc(scheduledTaskExpiresAtDescEN, scheduledTaskExpiresAtDescZH), Required: false},
			"enabled":        {Type: schema.Boolean, Desc: selectDesc("Whether the task remains enabled", "任务更新后是否启用"), Required: false},
		}),
	}, nil
}

func (t *scheduledTaskDeleteTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return mutationToolInfo("scheduled_task_delete", "Preview or delete a scheduled task after user confirmation.", "预检查或删除计划任务，删除前需要用户确认。")
}

func (t *scheduledTaskEnableTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return mutationToolInfo("scheduled_task_enable", "Preview or enable a scheduled task after user confirmation.", "预检查或启用计划任务，启用前需要用户确认。")
}

func (t *scheduledTaskDisableTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return mutationToolInfo("scheduled_task_disable", "Preview or disable a scheduled task after user confirmation.", "预检查或停用计划任务，停用前需要用户确认。")
}

func (t *scheduledTaskHistoryListTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "scheduled_task_history_list",
		Desc: selectDesc(
			"List run history for a scheduled task by task ID or task name.",
			"按任务 ID 或任务名称查询计划任务运行历史。",
		),
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"task_id":   {Type: schema.Integer, Desc: selectDesc("Task ID", "任务 ID"), Required: false},
			"task_name": {Type: schema.String, Desc: selectDesc("Task name", "任务名称"), Required: false},
			"page":      {Type: schema.Integer, Desc: selectDesc("Page number", "页码"), Required: false},
			"page_size": {Type: schema.Integer, Desc: selectDesc("Page size", "每页数量"), Required: false},
		}),
	}, nil
}

func (t *scheduledTaskHistoryDetailTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "scheduled_task_history_detail",
		Desc: selectDesc(
			"Read the full detail for a scheduled task run by run ID.",
			"按运行记录 ID 查询某次计划任务执行详情。",
		),
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"run_id": {Type: schema.Integer, Desc: selectDesc("Run ID", "运行记录 ID"), Required: true},
		}),
	}, nil
}

func mutationToolInfo(name, enDesc, zhDesc string) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: name,
		Desc: selectDesc(enDesc, zhDesc),
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"task_id":   {Type: schema.Integer, Desc: selectDesc("Task ID", "任务 ID"), Required: false},
			"task_name": {Type: schema.String, Desc: selectDesc("Task name", "任务名称"), Required: false},
			"confirm":   {Type: schema.Boolean, Desc: selectDesc("Whether to execute the action", "是否执行操作"), Required: true},
		}),
	}, nil
}

func (t *scheduledTaskListTool) InvokableRun(_ context.Context, arguments string, _ ...tool.Option) (string, error) {
	if t.config == nil || t.config.ListScheduledTasksFn == nil {
		return "", fmt.Errorf("scheduled task list is unavailable")
	}

	var input scheduledTaskListInput
	if err := decodeJSONArguments(arguments, &input); err != nil {
		return "", err
	}

	tasks, err := t.config.ListScheduledTasksFn()
	if err != nil {
		return "", err
	}
	if t.config.ListAgentsForMatchingFn != nil {
		if agents, agentErr := t.config.ListAgentsForMatchingFn(); agentErr == nil {
			applyAgentNames(tasks, agents)
		}
	}

	keyword := strings.TrimSpace(input.Keyword)
	status := strings.TrimSpace(input.Status)
	if status == "" {
		status = "all"
	}

	filtered := make([]ScheduledTaskRecord, 0, len(tasks))
	for _, task := range tasks {
		if keyword != "" && !strings.Contains(strings.ToLower(task.Name), strings.ToLower(keyword)) && !strings.Contains(strings.ToLower(task.Prompt), strings.ToLower(keyword)) {
			continue
		}
		if status == "enabled" && !task.Enabled {
			continue
		}
		if status == "disabled" && task.Enabled {
			continue
		}
		filtered = append(filtered, task)
	}
	if input.Limit > 0 && len(filtered) > input.Limit {
		filtered = filtered[:input.Limit]
	}

	return marshalJSON(map[string]any{
		"tasks": filtered,
		"count": len(filtered),
	}), nil
}

func (t *agentMatchByNameTool) InvokableRun(_ context.Context, arguments string, _ ...tool.Option) (string, error) {
	if t.config == nil || t.config.MatchAgentsByNameFn == nil {
		return "", fmt.Errorf("agent matching is unavailable")
	}

	var input agentMatchByNameInput
	if err := decodeJSONArguments(arguments, &input); err != nil {
		return "", err
	}

	matches, status, err := t.config.MatchAgentsByNameFn(input.Query)
	if err != nil {
		return "", err
	}

	result := map[string]any{
		"query":        strings.TrimSpace(input.Query),
		"match_status": status,
		"matches":      matches,
	}
	if (status == "exact" || status == "single") && len(matches) == 1 {
		result["recommended_agent_id"] = matches[0].ID
	}
	return marshalJSON(result), nil
}

func (t *scheduledTaskCreatePreviewTool) InvokableRun(_ context.Context, arguments string, _ ...tool.Option) (string, error) {
	if t.config == nil || t.config.MatchAgentsByNameFn == nil || t.config.ValidateScheduleFn == nil {
		return "", fmt.Errorf("scheduled task preview is unavailable")
	}

	var input scheduledTaskCreatePreviewInput
	if err := decodeJSONArguments(arguments, &input); err != nil {
		return "", err
	}

	issues := make([]string, 0)
	if strings.TrimSpace(input.Name) == "" {
		issues = append(issues, i18n.T("scheduled_task.issue.name_required"))
	}
	if strings.TrimSpace(input.Prompt) == "" {
		issues = append(issues, i18n.T("scheduled_task.issue.prompt_required"))
	}

	matches, status, err := t.config.MatchAgentsByNameFn(input.AgentName)
	if err != nil {
		return "", err
	}
	if status == "none" {
		issues = append(issues, i18n.T("scheduled_task.issue.agent_not_found"))
	}
	if status == "multiple" {
		issues = append(issues, i18n.T("scheduled_task.issue.agent_ambiguous"))
	}

	schedule, err := t.config.ValidateScheduleFn(input.ScheduleType, input.ScheduleValue, input.CronExpr)
	if err != nil {
		issues = append(issues, i18n.Tf("scheduled_task.issue.schedule_invalid", map[string]any{
			"Error": err.Error(),
		}))
	}

	result := map[string]any{
		"needs_confirmation": false,
		"issues":             issues,
		"agent_matches":      matches,
	}

	if len(issues) > 0 || schedule == nil || len(matches) != 1 || (status != "exact" && status != "single") {
		return marshalJSON(result), nil
	}
	expiresAt, err := parseScheduledTaskExpirationInput(input.ExpiresAt)
	if err != nil {
		result["issues"] = append(issues, err.Error())
		return marshalJSON(result), nil
	}

	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}

	parsedTask := ScheduledTaskRecord{
		Name:          strings.TrimSpace(input.Name),
		Prompt:        strings.TrimSpace(input.Prompt),
		AgentID:       matches[0].ID,
		AgentName:     matches[0].Name,
		ScheduleType:  schedule.ScheduleType,
		ScheduleValue: schedule.ScheduleValue,
		CronExpr:      schedule.CronExpr,
		Timezone:      schedule.Timezone,
		Enabled:       enabled,
		ExpiresAt:     expiresAt,
		NextRunAt:     schedule.NextRunAt,
	}
	result["needs_confirmation"] = true
	result["parsed_task"] = parsedTask
	result["confirmation_message"] = buildCreateConfirmationMessage(parsedTask)

	return marshalJSON(result), nil
}

func (t *scheduledTaskCreateConfirmTool) InvokableRun(_ context.Context, arguments string, _ ...tool.Option) (string, error) {
	if t.config == nil || t.config.CreateScheduledTaskFn == nil || t.config.ValidateScheduleFn == nil {
		return "", fmt.Errorf("scheduled task creation is unavailable")
	}

	var input scheduledTaskCreateConfirmInput
	if err := decodeJSONArguments(arguments, &input); err != nil {
		return "", err
	}
	if strings.TrimSpace(input.Name) == "" {
		return "", fmt.Errorf("task name is required")
	}
	if strings.TrimSpace(input.Prompt) == "" {
		return "", fmt.Errorf("task prompt is required")
	}
	if input.AgentID <= 0 {
		return "", fmt.Errorf("agent_id is required")
	}

	schedule, err := t.config.ValidateScheduleFn(input.ScheduleType, input.ScheduleValue, input.CronExpr)
	if err != nil {
		return "", err
	}
	expiresAt, err := parseScheduledTaskExpirationInput(input.ExpiresAt)
	if err != nil {
		return "", err
	}

	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}

	created, err := t.config.CreateScheduledTaskFn(ScheduledTaskCreateInput{
		Name:          strings.TrimSpace(input.Name),
		Prompt:        strings.TrimSpace(input.Prompt),
		AgentID:       input.AgentID,
		ScheduleType:  schedule.ScheduleType,
		ScheduleValue: schedule.ScheduleValue,
		CronExpr:      schedule.CronExpr,
		Enabled:       enabled,
		ExpiresAt:     expiresAt,
	})
	if err != nil {
		return "", err
	}
	if created != nil && created.Timezone == "" {
		created.Timezone = schedule.Timezone
	}
	return marshalJSON(map[string]any{
		"action": "created",
		"task":   created,
	}), nil
}

func (t *scheduledTaskUpdatePreviewTool) InvokableRun(_ context.Context, arguments string, _ ...tool.Option) (string, error) {
	if t.config == nil || t.config.ValidateScheduleFn == nil {
		return "", fmt.Errorf("scheduled task update preview is unavailable")
	}

	var input scheduledTaskUpdatePreviewInput
	if err := decodeJSONArguments(arguments, &input); err != nil {
		return "", err
	}

	task, candidates, err := resolveTaskForLookup(t.config, input.TaskID, input.TaskName)
	if err != nil {
		return "", err
	}

	issues := make([]string, 0)
	result := map[string]any{
		"needs_confirmation": false,
		"issues":             issues,
	}
	if task == nil {
		if len(candidates) == 0 {
			issues = append(issues, i18n.T("scheduled_task.issue.task_not_found"))
		} else {
			issues = append(issues, i18n.T("scheduled_task.issue.task_ambiguous"))
		}
		result["issues"] = issues
		result["matched_tasks"] = candidates
		return marshalJSON(result), nil
	}

	parsedTask := *task
	agentMatches := make([]ScheduledTaskAgent, 0)
	if strings.TrimSpace(input.Name) != "" {
		parsedTask.Name = strings.TrimSpace(input.Name)
	}
	if strings.TrimSpace(input.Prompt) != "" {
		parsedTask.Prompt = strings.TrimSpace(input.Prompt)
	}
	if input.Enabled != nil {
		parsedTask.Enabled = *input.Enabled
	}
	if input.ExpiresAt != nil {
		expiresAt, err := parseScheduledTaskExpirationInput(input.ExpiresAt)
		if err != nil {
			issues = append(issues, err.Error())
		} else {
			parsedTask.ExpiresAt = expiresAt
		}
	}

	if strings.TrimSpace(input.AgentName) != "" {
		if t.config.MatchAgentsByNameFn == nil {
			return "", fmt.Errorf("agent matching is unavailable")
		}
		matches, status, matchErr := t.config.MatchAgentsByNameFn(input.AgentName)
		if matchErr != nil {
			return "", matchErr
		}
		agentMatches = matches
		if status == "none" {
			issues = append(issues, i18n.T("scheduled_task.issue.agent_not_found"))
		}
		if status == "multiple" {
			issues = append(issues, i18n.T("scheduled_task.issue.agent_ambiguous"))
		}
		if len(matches) == 1 && (status == "exact" || status == "single") {
			parsedTask.AgentID = matches[0].ID
			parsedTask.AgentName = matches[0].Name
		}
	}

	scheduleType := strings.TrimSpace(input.ScheduleType)
	scheduleValue := strings.TrimSpace(input.ScheduleValue)
	cronExpr := strings.TrimSpace(input.CronExpr)
	if scheduleType != "" || scheduleValue != "" || cronExpr != "" {
		schedule, scheduleErr := t.config.ValidateScheduleFn(scheduleType, scheduleValue, cronExpr)
		if scheduleErr != nil {
			issues = append(issues, i18n.Tf("scheduled_task.issue.schedule_invalid", map[string]any{
				"Error": scheduleErr.Error(),
			}))
		} else {
			parsedTask.ScheduleType = schedule.ScheduleType
			parsedTask.ScheduleValue = schedule.ScheduleValue
			parsedTask.CronExpr = schedule.CronExpr
			parsedTask.Timezone = schedule.Timezone
			parsedTask.NextRunAt = schedule.NextRunAt
		}
	}

	result["issues"] = issues
	result["matched_tasks"] = []ScheduledTaskRecord{*task}
	result["agent_matches"] = agentMatches
	if len(issues) > 0 {
		return marshalJSON(result), nil
	}
	result["needs_confirmation"] = true
	result["parsed_task"] = parsedTask
	result["confirmation_message"] = buildUpdateConfirmationMessage(parsedTask)
	return marshalJSON(result), nil
}

func (t *scheduledTaskUpdateConfirmTool) InvokableRun(_ context.Context, arguments string, _ ...tool.Option) (string, error) {
	if t.config == nil || t.config.UpdateScheduledTaskFn == nil {
		return "", fmt.Errorf("scheduled task update is unavailable")
	}

	var input scheduledTaskUpdateConfirmInput
	if err := decodeJSONArguments(arguments, &input); err != nil {
		return "", err
	}
	if input.TaskID <= 0 {
		return "", fmt.Errorf("task_id is required")
	}
	expiresAt, err := parseScheduledTaskExpirationInput(input.ExpiresAt)
	if err != nil {
		return "", err
	}

	updated, err := t.config.UpdateScheduledTaskFn(input.TaskID, ScheduledTaskUpdateInput{
		Name:          input.Name,
		Prompt:        input.Prompt,
		AgentID:       input.AgentID,
		ScheduleType:  input.ScheduleType,
		ScheduleValue: input.ScheduleValue,
		CronExpr:      input.CronExpr,
		Enabled:       input.Enabled,
		ExpiresAt:     expiresAt,
	})
	if err != nil {
		return "", err
	}
	return marshalJSON(map[string]any{
		"action": "updated",
		"task":   updated,
	}), nil
}

func (t *scheduledTaskDeleteTool) InvokableRun(_ context.Context, arguments string, _ ...tool.Option) (string, error) {
	return t.runMutation(arguments, "delete", false)
}

func (t *scheduledTaskEnableTool) InvokableRun(_ context.Context, arguments string, _ ...tool.Option) (string, error) {
	return t.runMutation(arguments, "enable", true)
}

func (t *scheduledTaskDisableTool) InvokableRun(_ context.Context, arguments string, _ ...tool.Option) (string, error) {
	return t.runMutation(arguments, "disable", false)
}

func (t *scheduledTaskHistoryListTool) InvokableRun(_ context.Context, arguments string, _ ...tool.Option) (string, error) {
	if t.config == nil || t.config.ListScheduledTaskRunsFn == nil {
		return "", fmt.Errorf("scheduled task history list is unavailable")
	}

	var input scheduledTaskHistoryListInput
	if err := decodeJSONArguments(arguments, &input); err != nil {
		return "", err
	}

	task, candidates, err := resolveTaskForLookup(t.config, input.TaskID, input.TaskName)
	if err != nil {
		return "", err
	}
	if task == nil {
		issues := []string{i18n.T("scheduled_task.issue.task_ambiguous")}
		if len(candidates) == 0 {
			issues = []string{i18n.T("scheduled_task.issue.task_not_found")}
		}
		return marshalJSON(map[string]any{
			"issues":        issues,
			"matched_tasks": candidates,
		}), nil
	}

	page := input.Page
	if page <= 0 {
		page = 1
	}
	pageSize := input.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}

	runs, err := t.config.ListScheduledTaskRunsFn(task.ID, page, pageSize)
	if err != nil {
		return "", err
	}
	return marshalJSON(map[string]any{
		"task":      task,
		"runs":      runs,
		"count":     len(runs),
		"page":      page,
		"page_size": pageSize,
	}), nil
}

func (t *scheduledTaskHistoryDetailTool) InvokableRun(_ context.Context, arguments string, _ ...tool.Option) (string, error) {
	if t.config == nil || t.config.GetScheduledTaskRunDetailFn == nil {
		return "", fmt.Errorf("scheduled task history detail is unavailable")
	}

	var input scheduledTaskHistoryDetailInput
	if err := decodeJSONArguments(arguments, &input); err != nil {
		return "", err
	}
	if input.RunID <= 0 {
		return "", fmt.Errorf("run_id is required")
	}

	detail, err := t.config.GetScheduledTaskRunDetailFn(input.RunID)
	if err != nil {
		return "", err
	}
	return marshalJSON(detail), nil
}

func (t *scheduledTaskDeleteTool) runMutation(arguments, action string, enabled bool) (string, error) {
	return runTaskMutation(t.config, arguments, action, enabled)
}

func (t *scheduledTaskEnableTool) runMutation(arguments, action string, enabled bool) (string, error) {
	return runTaskMutation(t.config, arguments, action, enabled)
}

func (t *scheduledTaskDisableTool) runMutation(arguments, action string, enabled bool) (string, error) {
	return runTaskMutation(t.config, arguments, action, enabled)
}

func runTaskMutation(config *ScheduledTaskManagementConfig, arguments, action string, enabled bool) (string, error) {
	if config == nil {
		return "", fmt.Errorf("scheduled task mutation is unavailable")
	}

	var input scheduledTaskMutationInput
	if err := decodeJSONArguments(arguments, &input); err != nil {
		return "", err
	}

	task, candidates, err := resolveTaskForMutation(config, input)
	if err != nil {
		return "", err
	}

	issues := make([]string, 0)
	if task == nil {
		if len(candidates) == 0 {
			issues = append(issues, i18n.T("scheduled_task.issue.task_not_found"))
		} else {
			issues = append(issues, i18n.T("scheduled_task.issue.task_ambiguous"))
		}
		return marshalJSON(map[string]any{
			"action":        "preview_" + action,
			"issues":        issues,
			"matched_tasks": candidates,
		}), nil
	}

	if action != "delete" {
		if action == "enable" && task.Enabled {
			return marshalJSON(map[string]any{
				"action": "already_enabled",
				"task":   task,
			}), nil
		}
		if action == "disable" && !task.Enabled {
			return marshalJSON(map[string]any{
				"action": "already_disabled",
				"task":   task,
			}), nil
		}
	}

	if !input.Confirm {
		return marshalJSON(map[string]any{
			"action":               "preview_" + action,
			"needs_confirmation":   true,
			"matched_tasks":        []ScheduledTaskRecord{*task},
			"confirmation_message": buildMutationConfirmationMessage(action, *task),
		}), nil
	}

	switch action {
	case "delete":
		if config.DeleteScheduledTaskFn == nil {
			return "", fmt.Errorf("scheduled task deletion is unavailable")
		}
		if err := config.DeleteScheduledTaskFn(task.ID); err != nil {
			return "", err
		}
		return marshalJSON(map[string]any{
			"action":            "deleted",
			"deleted_task_id":   task.ID,
			"deleted_task_name": task.Name,
		}), nil
	case "enable", "disable":
		if config.SetScheduledTaskFn == nil {
			return "", fmt.Errorf("scheduled task enable/disable is unavailable")
		}
		updated, err := config.SetScheduledTaskFn(task.ID, enabled)
		if err != nil {
			return "", err
		}
		return marshalJSON(map[string]any{
			"action": action + "d",
			"task":   updated,
		}), nil
	default:
		return "", fmt.Errorf("unsupported mutation action: %s", action)
	}
}

func resolveTaskForMutation(config *ScheduledTaskManagementConfig, input scheduledTaskMutationInput) (*ScheduledTaskRecord, []ScheduledTaskRecord, error) {
	return resolveTaskForLookup(config, input.TaskID, input.TaskName)
}

func resolveTaskForLookup(config *ScheduledTaskManagementConfig, taskID int64, taskName string) (*ScheduledTaskRecord, []ScheduledTaskRecord, error) {
	if taskID > 0 {
		if config.GetScheduledTaskByIDFn == nil {
			return nil, nil, fmt.Errorf("scheduled task lookup is unavailable")
		}
		task, err := config.GetScheduledTaskByIDFn(taskID)
		if err != nil {
			return nil, nil, err
		}
		if task == nil {
			return nil, nil, nil
		}
		return task, []ScheduledTaskRecord{*task}, nil
	}

	name := strings.TrimSpace(taskName)
	if name == "" {
		return nil, nil, fmt.Errorf("task_id or task_name is required")
	}
	if config.FindScheduledTasksFn == nil {
		return nil, nil, fmt.Errorf("scheduled task lookup is unavailable")
	}
	tasks, err := config.FindScheduledTasksFn(name)
	if err != nil {
		return nil, nil, err
	}
	if len(tasks) == 1 {
		return &tasks[0], tasks, nil
	}
	return nil, tasks, nil
}

func decodeJSONArguments(arguments string, target any) error {
	if strings.TrimSpace(arguments) == "" {
		arguments = "{}"
	}
	decoder := json.NewDecoder(bytes.NewBufferString(arguments))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("parse arguments: %w", err)
	}
	return nil
}

func marshalJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return `{"error":"marshal_failed"}`
	}
	return string(b)
}

func applyAgentNames(tasks []ScheduledTaskRecord, agents []ScheduledTaskAgent) {
	nameMap := make(map[int64]string, len(agents))
	for _, agent := range agents {
		nameMap[agent.ID] = agent.Name
	}
	for i := range tasks {
		if tasks[i].AgentName == "" {
			tasks[i].AgentName = nameMap[tasks[i].AgentID]
		}
	}
}

func buildCreateConfirmationMessage(task ScheduledTaskRecord) string {
	return i18n.Tf("scheduled_task.confirm.create", map[string]any{
		"Name":      task.Name,
		"AgentName": task.AgentName,
		"CronExpr":  task.CronExpr,
		"Enabled":   task.Enabled,
	})
}

func buildUpdateConfirmationMessage(task ScheduledTaskRecord) string {
	return i18n.Tf("scheduled_task.confirm.mutation", map[string]any{
		"Action": i18n.T("scheduled_task.action.update"),
		"Name":   task.Name,
		"ID":     task.ID,
	})
}

func buildMutationConfirmationMessage(action string, task ScheduledTaskRecord) string {
	return i18n.Tf("scheduled_task.confirm.mutation", map[string]any{
		"Action": scheduledTaskMutationActionLabel(action),
		"Name":   task.Name,
		"ID":     task.ID,
	})
}

func scheduledTaskMutationActionLabel(action string) string {
	switch action {
	case "delete":
		return i18n.T("scheduled_task.action.delete")
	case "enable":
		return i18n.T("scheduled_task.action.enable")
	case "disable":
		return i18n.T("scheduled_task.action.disable")
	case "update":
		return i18n.T("scheduled_task.action.update")
	default:
		return action
	}
}

// parseScheduledTaskExpirationInput keeps AI tool date parsing explicit so
// the tool only accepts unambiguous RFC3339 timestamps.
func parseScheduledTaskExpirationInput(raw *string) (*time.Time, error) {
	if raw == nil {
		return nil, nil
	}

	value := strings.TrimSpace(*raw)
	if value == "" {
		return nil, nil
	}

	expiresAt, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, fmt.Errorf(scheduledTaskExpiresAtFormatError)
	}
	return &expiresAt, nil
}
