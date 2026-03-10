package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestScheduledTaskManagementTools(t *testing.T) {
	tools, err := NewScheduledTaskManagementTools(&ScheduledTaskManagementConfig{})
	if err != nil {
		t.Fatalf("NewScheduledTaskManagementTools returned error: %v", err)
	}
	if len(tools) != 7 {
		t.Fatalf("expected 7 tools, got %d", len(tools))
	}
}

func TestScheduledTaskCreatePreviewToolInfo(t *testing.T) {
	info, err := (&scheduledTaskCreatePreviewTool{}).Info(context.Background())
	if err != nil {
		t.Fatalf("Info returned error: %v", err)
	}
	if info.Name != "scheduled_task_create_preview" {
		t.Fatalf("unexpected tool name: %s", info.Name)
	}

	schema, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatalf("ToJSONSchema returned error: %v", err)
	}

	for _, key := range []string{"agent_name", "schedule_type", "schedule_value", "cron_expr"} {
		if _, ok := schema.Properties.Get(key); !ok {
			t.Fatalf("expected schema property %q", key)
		}
	}
}

func TestScheduledTaskListTool(t *testing.T) {
	tool := &scheduledTaskListTool{config: newTestScheduledTaskConfig()}

	result, err := tool.InvokableRun(context.Background(), `{"status":"enabled","limit":1}`)
	if err != nil {
		t.Fatalf("InvokableRun returned error: %v", err)
	}
	if !strings.Contains(result, `"count":1`) {
		t.Fatalf("unexpected result: %s", result)
	}
	if !strings.Contains(result, `"agent_name":"销售助手"`) {
		t.Fatalf("expected agent name in result: %s", result)
	}
}

func TestAgentMatchByNameTool(t *testing.T) {
	tool := &agentMatchByNameTool{config: newTestScheduledTaskConfig()}

	result, err := tool.InvokableRun(context.Background(), `{"query":"销售助手"}`)
	if err != nil {
		t.Fatalf("InvokableRun returned error: %v", err)
	}
	if !strings.Contains(result, `"match_status":"exact"`) {
		t.Fatalf("unexpected result: %s", result)
	}
	if !strings.Contains(result, `"recommended_agent_id":1`) {
		t.Fatalf("unexpected result: %s", result)
	}
}

func TestScheduledTaskCreatePreviewTool(t *testing.T) {
	tool := &scheduledTaskCreatePreviewTool{config: newTestScheduledTaskConfig()}

	result, err := tool.InvokableRun(context.Background(), `{
		"name":"销售日报",
		"prompt":"总结昨日新增线索",
		"agent_name":"销售助手",
		"schedule_type":"preset",
		"schedule_value":"every_day_0900",
		"cron_expr":"",
		"enabled":true
	}`)
	if err != nil {
		t.Fatalf("InvokableRun returned error: %v", err)
	}
	if !strings.Contains(result, `"needs_confirmation":true`) {
		t.Fatalf("unexpected result: %s", result)
	}

	result, err = tool.InvokableRun(context.Background(), `{
		"name":"销售日报",
		"prompt":"总结昨日新增线索",
		"agent_name":"日报",
		"schedule_type":"preset",
		"schedule_value":"every_day_0900",
		"cron_expr":""
	}`)
	if err != nil {
		t.Fatalf("InvokableRun returned error: %v", err)
	}
	if !strings.Contains(result, `"issues":["助手名称匹配到多个结果，需要用户进一步确认"]`) {
		t.Fatalf("unexpected result: %s", result)
	}

	result, err = tool.InvokableRun(context.Background(), `{
		"name":"销售日报",
		"prompt":"总结昨日新增线索",
		"agent_name":"销售助手",
		"schedule_type":"preset",
		"schedule_value":"bad_value",
		"cron_expr":""
	}`)
	if err != nil {
		t.Fatalf("InvokableRun returned error: %v", err)
	}
	if !strings.Contains(result, `调度参数校验失败`) {
		t.Fatalf("unexpected result: %s", result)
	}
}

func TestScheduledTaskCreateConfirmTool(t *testing.T) {
	config := newTestScheduledTaskConfig()
	tool := &scheduledTaskCreateConfirmTool{config: config}

	result, err := tool.InvokableRun(context.Background(), `{
		"name":"销售日报",
		"prompt":"总结昨日新增线索",
		"agent_id":1,
		"schedule_type":"preset",
		"schedule_value":"every_day_0900",
		"cron_expr":"",
		"enabled":true
	}`)
	if err != nil {
		t.Fatalf("InvokableRun returned error: %v", err)
	}
	if !strings.Contains(result, `"action":"created"`) {
		t.Fatalf("unexpected result: %s", result)
	}

	if _, err := tool.InvokableRun(context.Background(), `{
		"name":"销售日报",
		"prompt":"总结昨日新增线索",
		"schedule_type":"preset",
		"schedule_value":"every_day_0900",
		"cron_expr":""
	}`); err == nil {
		t.Fatalf("expected missing agent_id error")
	}

	if _, err := tool.InvokableRun(context.Background(), `{
		"name":"销售日报",
		"prompt":"总结昨日新增线索",
		"agent_id":1,
		"schedule_type":"preset",
		"schedule_value":"bad_value",
		"cron_expr":""
	}`); err == nil {
		t.Fatalf("expected invalid schedule error")
	}
}

func TestScheduledTaskDeleteEnableDisableTools(t *testing.T) {
	config := newTestScheduledTaskConfig()

	deleteTool := &scheduledTaskDeleteTool{config: config}
	enableTool := &scheduledTaskEnableTool{config: config}
	disableTool := &scheduledTaskDisableTool{config: config}

	preview, err := deleteTool.InvokableRun(context.Background(), `{"task_name":"销售日报","confirm":false}`)
	if err != nil {
		t.Fatalf("delete preview returned error: %v", err)
	}
	if !strings.Contains(preview, `"needs_confirmation":true`) {
		t.Fatalf("unexpected delete preview: %s", preview)
	}

	preview, err = disableTool.InvokableRun(context.Background(), `{"task_name":"日报","confirm":false}`)
	if err != nil {
		t.Fatalf("disable preview returned error: %v", err)
	}
	if !strings.Contains(preview, `匹配到多个任务`) {
		t.Fatalf("unexpected disable preview: %s", preview)
	}

	preview, err = disableTool.InvokableRun(context.Background(), `{"task_name":"市场日报","confirm":false}`)
	if err != nil {
		t.Fatalf("disable preview returned error: %v", err)
	}
	if !strings.Contains(preview, `"needs_confirmation":true`) {
		t.Fatalf("unexpected disable preview: %s", preview)
	}

	deleted, err := deleteTool.InvokableRun(context.Background(), `{"task_name":"销售日报","confirm":true}`)
	if err != nil {
		t.Fatalf("delete returned error: %v", err)
	}
	if !strings.Contains(deleted, `"action":"deleted"`) {
		t.Fatalf("unexpected delete result: %s", deleted)
	}

	disabled, err := disableTool.InvokableRun(context.Background(), `{"task_name":"市场日报","confirm":true}`)
	if err != nil {
		t.Fatalf("disable returned error: %v", err)
	}
	if !strings.Contains(disabled, `"action":"disabled"`) {
		t.Fatalf("unexpected disable result: %s", disabled)
	}

	enabled, err := enableTool.InvokableRun(context.Background(), `{"task_name":"市场日报","confirm":true}`)
	if err != nil {
		t.Fatalf("enable returned error: %v", err)
	}
	if !strings.Contains(enabled, `"action":"enabled"`) {
		t.Fatalf("unexpected enable result: %s", enabled)
	}
}

func newTestScheduledTaskConfig() *ScheduledTaskManagementConfig {
	nextRunAt := time.Date(2026, 3, 11, 9, 0, 0, 0, time.UTC)
	agents := []ScheduledTaskAgent{
		{ID: 1, Name: "销售助手"},
		{ID: 2, Name: "销售日报助手"},
		{ID: 3, Name: "日报助手"},
	}
	tasks := []ScheduledTaskRecord{
		{
			ID:            1,
			Name:          "销售日报",
			Prompt:        "总结昨日新增线索",
			AgentID:       1,
			ScheduleType:  "preset",
			ScheduleValue: "every_day_0900",
			CronExpr:      "0 9 * * *",
			Enabled:       true,
			NextRunAt:     &nextRunAt,
			LastStatus:    "pending",
		},
		{
			ID:            2,
			Name:          "市场日报",
			Prompt:        "总结市场动态",
			AgentID:       2,
			ScheduleType:  "preset",
			ScheduleValue: "every_day_0900",
			CronExpr:      "0 9 * * *",
			Enabled:       true,
			NextRunAt:     &nextRunAt,
			LastStatus:    "pending",
		},
	}

	return &ScheduledTaskManagementConfig{
		ListAgentsForMatchingFn: func() ([]ScheduledTaskAgent, error) {
			return append([]ScheduledTaskAgent(nil), agents...), nil
		},
		MatchAgentsByNameFn: func(query string) ([]ScheduledTaskAgent, string, error) {
			query = strings.TrimSpace(query)
			switch query {
			case "销售助手":
				return []ScheduledTaskAgent{{ID: 1, Name: "销售助手"}}, "exact", nil
			case "日报":
				return []ScheduledTaskAgent{{ID: 2, Name: "销售日报助手"}, {ID: 3, Name: "日报助手"}}, "multiple", nil
			default:
				return []ScheduledTaskAgent{}, "none", nil
			}
		},
		ListScheduledTasksFn: func() ([]ScheduledTaskRecord, error) {
			return cloneTasks(tasks), nil
		},
		GetScheduledTaskByIDFn: func(id int64) (*ScheduledTaskRecord, error) {
			for _, task := range tasks {
				if task.ID == id {
					copied := task
					return &copied, nil
				}
			}
			return nil, nil
		},
		FindScheduledTasksFn: func(name string) ([]ScheduledTaskRecord, error) {
			matches := make([]ScheduledTaskRecord, 0)
			for _, task := range tasks {
				if strings.Contains(task.Name, name) {
					matches = append(matches, task)
				}
			}
			return matches, nil
		},
		ValidateScheduleFn: func(scheduleType, scheduleValue, cronExpr string) (*ScheduledTaskValidationResult, error) {
			if scheduleType != "preset" || scheduleValue != "every_day_0900" {
				return nil, fmt.Errorf("unsupported schedule")
			}
			return &ScheduledTaskValidationResult{
				ScheduleType:  "preset",
				ScheduleValue: "every_day_0900",
				CronExpr:      "0 9 * * *",
				Timezone:      "UTC",
				NextRunAt:     &nextRunAt,
			}, nil
		},
		CreateScheduledTaskFn: func(input ScheduledTaskCreateInput) (*ScheduledTaskRecord, error) {
			task := ScheduledTaskRecord{
				ID:            3,
				Name:          input.Name,
				Prompt:        input.Prompt,
				AgentID:       input.AgentID,
				ScheduleType:  input.ScheduleType,
				ScheduleValue: input.ScheduleValue,
				CronExpr:      input.CronExpr,
				Enabled:       input.Enabled,
			}
			return &task, nil
		},
		DeleteScheduledTaskFn: func(id int64) error {
			filtered := tasks[:0]
			for _, task := range tasks {
				if task.ID != id {
					filtered = append(filtered, task)
				}
			}
			tasks = filtered
			return nil
		},
		SetScheduledTaskFn: func(id int64, enabled bool) (*ScheduledTaskRecord, error) {
			for i := range tasks {
				if tasks[i].ID == id {
					tasks[i].Enabled = enabled
					copied := tasks[i]
					return &copied, nil
				}
			}
			return nil, nil
		},
	}
}

func cloneTasks(tasks []ScheduledTaskRecord) []ScheduledTaskRecord {
	b, _ := json.Marshal(tasks)
	var cloned []ScheduledTaskRecord
	_ = json.Unmarshal(b, &cloned)
	return cloned
}
