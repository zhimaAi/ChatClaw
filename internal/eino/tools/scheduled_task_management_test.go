package tools

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestScheduledTaskCreateConfirmToolPassesExpiresAt(t *testing.T) {
	expectedExpiresAt := time.Date(2026, 4, 1, 23, 59, 59, 0, time.UTC)
	captured := ScheduledTaskCreateInput{}
	tool := &scheduledTaskCreateConfirmTool{
		config: &ScheduledTaskManagementConfig{
			ValidateScheduleFn: func(scheduleType, scheduleValue, cronExpr string) (*ScheduledTaskValidationResult, error) {
				return &ScheduledTaskValidationResult{
					ScheduleType:  scheduleType,
					ScheduleValue: scheduleValue,
					CronExpr:      cronExpr,
					Timezone:      "UTC",
				}, nil
			},
			CreateScheduledTaskFn: func(input ScheduledTaskCreateInput) (*ScheduledTaskRecord, error) {
				captured = input
				return &ScheduledTaskRecord{
					ID:        1,
					Name:      input.Name,
					Prompt:    input.Prompt,
					AgentID:   input.AgentID,
					Enabled:   input.Enabled,
					ExpiresAt: input.ExpiresAt,
				}, nil
			},
		},
	}

	result, err := tool.InvokableRun(context.Background(), `{
		"name":"Morning digest",
		"prompt":"Send today's digest",
		"agent_id":7,
		"schedule_type":"preset",
		"schedule_value":"every_day_0900",
		"cron_expr":"",
		"enabled":true,
		"expires_at":"2026-04-01T23:59:59Z"
	}`)
	if err != nil {
		t.Fatalf("InvokableRun returned error: %v", err)
	}
	if captured.ExpiresAt == nil {
		t.Fatal("expected create input expires_at to be forwarded")
	}
	if !captured.ExpiresAt.Equal(expectedExpiresAt) {
		t.Fatalf("expected expires_at %v, got %v", expectedExpiresAt, captured.ExpiresAt)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if payload["action"] != "created" {
		t.Fatalf("expected action created, got %#v", payload["action"])
	}
}

func TestScheduledTaskUpdateConfirmToolPassesExpiresAt(t *testing.T) {
	expectedExpiresAt := time.Date(2026, 5, 2, 8, 30, 0, 0, time.UTC)
	captured := ScheduledTaskUpdateInput{}
	tool := &scheduledTaskUpdateConfirmTool{
		config: &ScheduledTaskManagementConfig{
			UpdateScheduledTaskFn: func(id int64, input ScheduledTaskUpdateInput) (*ScheduledTaskRecord, error) {
				if id != 9 {
					t.Fatalf("expected task id 9, got %d", id)
				}
				captured = input
				return &ScheduledTaskRecord{
					ID:        id,
					Name:      "Morning digest",
					Enabled:   true,
					ExpiresAt: input.ExpiresAt,
				}, nil
			},
		},
	}

	result, err := tool.InvokableRun(context.Background(), `{
		"task_id":9,
		"expires_at":"2026-05-02T08:30:00Z"
	}`)
	if err != nil {
		t.Fatalf("InvokableRun returned error: %v", err)
	}
	if captured.ExpiresAt == nil {
		t.Fatal("expected update input expires_at to be forwarded")
	}
	if !captured.ExpiresAt.Equal(expectedExpiresAt) {
		t.Fatalf("expected expires_at %v, got %v", expectedExpiresAt, captured.ExpiresAt)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if payload["action"] != "updated" {
		t.Fatalf("expected action updated, got %#v", payload["action"])
	}
}

func TestParseScheduledTaskExpirationInputConvertsDateOnlyToLocalEndOfDay(t *testing.T) {
	value := "2026-03-23"

	expiresAt, err := parseScheduledTaskExpirationInput(&value)
	if err != nil {
		t.Fatalf("parseScheduledTaskExpirationInput returned error: %v", err)
	}
	if expiresAt == nil {
		t.Fatal("expected expires_at to be parsed")
	}
	if expiresAt.Year() != 2026 || expiresAt.Month() != time.March || expiresAt.Day() != 23 {
		t.Fatalf("expected local date 2026-03-23, got %v", expiresAt)
	}
	if expiresAt.Hour() != 23 || expiresAt.Minute() != 59 || expiresAt.Second() != 59 {
		t.Fatalf("expected local end-of-day time, got %v", expiresAt)
	}
}

func TestParseScheduledTaskExpirationInputNormalizesRFC3339ToLocalEndOfDay(t *testing.T) {
	value := "2026-03-23T23:59:59Z"

	expiresAt, err := parseScheduledTaskExpirationInput(&value)
	if err != nil {
		t.Fatalf("parseScheduledTaskExpirationInput returned error: %v", err)
	}
	if expiresAt == nil {
		t.Fatal("expected expires_at to be parsed")
	}
	if expiresAt.Year() != 2026 || expiresAt.Month() != time.March || expiresAt.Day() != 23 {
		t.Fatalf("expected normalized local date 2026-03-23, got %v", expiresAt)
	}
	if expiresAt.Hour() != 23 || expiresAt.Minute() != 59 || expiresAt.Second() != 59 {
		t.Fatalf("expected normalized local end-of-day time, got %v", expiresAt)
	}
}

