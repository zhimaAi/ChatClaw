package scheduledtasks

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/uptrace/bun"
)

func TestCreateScheduledTaskWritesManualOperationLog(t *testing.T) {
	db := newScheduledTasksTestDB(t)
	insertScheduledTasksAgent(t, db, 7)

	svc := NewScheduledTasksServiceForTest(nil, db, nil, nil)

	created, err := svc.CreateScheduledTask(CreateScheduledTaskInput{
		Name:                   "Morning digest",
		Prompt:                 "Send today's digest",
		AgentID:                7,
		ScheduleType:           ScheduleTypePreset,
		ScheduleValue:          "every_day_0900",
		CronExpr:               "",
		Enabled:                true,
		NotificationPlatform:   "feishu",
		NotificationChannelIDs: []int64{11, 12},
	})
	if err != nil {
		t.Fatalf("CreateScheduledTask returned error: %v", err)
	}

	logs := readScheduledTaskOperationLogs(t, db)
	if len(logs) != 1 {
		t.Fatalf("expected 1 operation log, got %d", len(logs))
	}
	if logs[0].TaskID != created.ID {
		t.Fatalf("expected log task id %d, got %d", created.ID, logs[0].TaskID)
	}
	if logs[0].OperationType != OperationTypeCreate {
		t.Fatalf("expected operation type %q, got %q", OperationTypeCreate, logs[0].OperationType)
	}
	if logs[0].OperationSource != OperationSourceManual {
		t.Fatalf("expected operation source %q, got %q", OperationSourceManual, logs[0].OperationSource)
	}

	changed := decodeChangedFields(t, logs[0].ChangedFieldsJSON)
	if len(changed) == 0 {
		t.Fatal("expected create log to include changed fields")
	}

	snapshot := decodeOperationSnapshot(t, logs[0].TaskSnapshotJSON)
	if snapshot.Name != "Morning digest" {
		t.Fatalf("expected snapshot name to be persisted, got %q", snapshot.Name)
	}
	if snapshot.ScheduleType != ScheduleTypePreset {
		t.Fatalf("expected snapshot schedule type %q, got %q", ScheduleTypePreset, snapshot.ScheduleType)
	}
}

func TestUpdateScheduledTaskWritesSingleLogWithMultipleChangedFields(t *testing.T) {
	db := newScheduledTasksTestDB(t)
	insertScheduledTasksAgent(t, db, 7)
	insertScheduledTasksAgent(t, db, 8)

	svc := NewScheduledTasksServiceForTest(nil, db, nil, nil)
	created, err := svc.CreateScheduledTask(CreateScheduledTaskInput{
		Name:                   "Morning digest",
		Prompt:                 "Send today's digest",
		AgentID:                7,
		ScheduleType:           ScheduleTypePreset,
		ScheduleValue:          "every_day_0900",
		CronExpr:               "",
		Enabled:                true,
		NotificationPlatform:   "feishu",
		NotificationChannelIDs: []int64{11, 12},
	})
	if err != nil {
		t.Fatalf("CreateScheduledTask returned error: %v", err)
	}

	name := "Updated digest"
	prompt := "Send the final digest"
	agentID := int64(8)
	scheduleType := ScheduleTypeCron
	scheduleValue := "0 10 * * *"
	cronExpr := "0 10 * * *"
	platform := "wecom"
	channelIDs := []int64{22}

	_, err = svc.UpdateScheduledTask(created.ID, UpdateScheduledTaskInput{
		Name:                   &name,
		Prompt:                 &prompt,
		AgentID:                &agentID,
		ScheduleType:           &scheduleType,
		ScheduleValue:          &scheduleValue,
		CronExpr:               &cronExpr,
		NotificationPlatform:   &platform,
		NotificationChannelIDs: &channelIDs,
	})
	if err != nil {
		t.Fatalf("UpdateScheduledTask returned error: %v", err)
	}

	logs := readScheduledTaskOperationLogs(t, db)
	if len(logs) != 2 {
		t.Fatalf("expected 2 operation logs, got %d", len(logs))
	}
	last := logs[1]
	if last.OperationType != OperationTypeUpdate {
		t.Fatalf("expected operation type %q, got %q", OperationTypeUpdate, last.OperationType)
	}

	changed := decodeChangedFields(t, last.ChangedFieldsJSON)
	if len(changed) < 4 {
		t.Fatalf("expected multiple changed fields, got %d", len(changed))
	}
	assertChangedFieldPresent(t, changed, "name")
	assertChangedFieldPresent(t, changed, "prompt")
	assertChangedFieldPresent(t, changed, "agent")
	assertChangedFieldPresent(t, changed, "schedule_time")
	assertChangedFieldPresent(t, changed, "notification_channels")
}

func TestUpdateScheduledTaskWritesExpirationChangedField(t *testing.T) {
	db := newScheduledTasksTestDB(t)
	insertScheduledTasksAgent(t, db, 7)

	svc := NewScheduledTasksServiceForTest(nil, db, nil, nil)
	created, err := svc.CreateScheduledTask(CreateScheduledTaskInput{
		Name:          "Morning digest",
		Prompt:        "Send today's digest",
		AgentID:       7,
		ScheduleType:  ScheduleTypePreset,
		ScheduleValue: "every_day_0900",
		Enabled:       true,
	})
	if err != nil {
		t.Fatalf("CreateScheduledTask returned error: %v", err)
	}

	expiresAt := time.Date(2026, 4, 1, 23, 59, 59, 0, time.UTC)
	updated, err := svc.UpdateScheduledTask(created.ID, UpdateScheduledTaskInput{
		ExpiresAt: &expiresAt,
	})
	if err != nil {
		t.Fatalf("UpdateScheduledTask returned error: %v", err)
	}
	if updated.ExpiresAt == nil || !updated.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("expected updated expires_at %v, got %v", expiresAt, updated.ExpiresAt)
	}

	logs := readScheduledTaskOperationLogs(t, db)
	if len(logs) != 2 {
		t.Fatalf("expected 2 operation logs, got %d", len(logs))
	}

	changed := decodeChangedFields(t, logs[1].ChangedFieldsJSON)
	if len(changed) != 1 {
		t.Fatalf("expected only expiration changed field, got %d", len(changed))
	}
	if changed[0].FieldKey != "expires_at" {
		t.Fatalf("expected changed field key expires_at, got %q", changed[0].FieldKey)
	}
	if changed[0].FieldLabel != "到期时间" {
		t.Fatalf("expected changed field label 到期时间, got %q", changed[0].FieldLabel)
	}
	if changed[0].Before != "-" {
		t.Fatalf("expected before value -, got %q", changed[0].Before)
	}
	if changed[0].After != "2026-04-01 23:59:59" {
		t.Fatalf("expected after value 2026-04-01 23:59:59, got %q", changed[0].After)
	}
}

func TestSetScheduledTaskEnabledWritesStatusOperationLog(t *testing.T) {
	db := newScheduledTasksTestDB(t)
	insertScheduledTasksAgent(t, db, 7)

	svc := NewScheduledTasksServiceForTest(nil, db, nil, nil)
	created, err := svc.CreateScheduledTask(CreateScheduledTaskInput{
		Name:          "Morning digest",
		Prompt:        "Send today's digest",
		AgentID:       7,
		ScheduleType:  ScheduleTypePreset,
		ScheduleValue: "every_day_0900",
		Enabled:       true,
	})
	if err != nil {
		t.Fatalf("CreateScheduledTask returned error: %v", err)
	}

	_, err = svc.SetScheduledTaskEnabled(created.ID, false)
	if err != nil {
		t.Fatalf("SetScheduledTaskEnabled returned error: %v", err)
	}

	logs := readScheduledTaskOperationLogs(t, db)
	if len(logs) != 2 {
		t.Fatalf("expected 2 operation logs, got %d", len(logs))
	}

	changed := decodeChangedFields(t, logs[1].ChangedFieldsJSON)
	if len(changed) != 1 {
		t.Fatalf("expected 1 changed field for enable toggle, got %d", len(changed))
	}
	if changed[0].FieldKey != "status" {
		t.Fatalf("expected changed field key status, got %q", changed[0].FieldKey)
	}
}

func TestDeleteScheduledTaskWritesDeleteOperationLogWithPreDeleteSnapshot(t *testing.T) {
	db := newScheduledTasksTestDB(t)
	insertScheduledTasksAgent(t, db, 7)

	svc := NewScheduledTasksServiceForTest(nil, db, nil, nil)
	created, err := svc.CreateScheduledTask(CreateScheduledTaskInput{
		Name:          "Morning digest",
		Prompt:        "Send today's digest",
		AgentID:       7,
		ScheduleType:  ScheduleTypePreset,
		ScheduleValue: "every_day_0900",
		Enabled:       true,
	})
	if err != nil {
		t.Fatalf("CreateScheduledTask returned error: %v", err)
	}

	if err := svc.DeleteScheduledTask(created.ID); err != nil {
		t.Fatalf("DeleteScheduledTask returned error: %v", err)
	}

	logs := readScheduledTaskOperationLogs(t, db)
	if len(logs) != 2 {
		t.Fatalf("expected 2 operation logs, got %d", len(logs))
	}

	last := logs[1]
	if last.OperationType != OperationTypeDelete {
		t.Fatalf("expected operation type %q, got %q", OperationTypeDelete, last.OperationType)
	}

	snapshot := decodeOperationSnapshot(t, last.TaskSnapshotJSON)
	if snapshot.Name != "Morning digest" {
		t.Fatalf("expected delete snapshot to keep task name, got %q", snapshot.Name)
	}
}

func TestCreateScheduledTaskWithAISourceWritesAIOperationLog(t *testing.T) {
	db := newScheduledTasksTestDB(t)
	insertScheduledTasksAgent(t, db, 7)

	svc := NewScheduledTasksServiceForTest(nil, db, nil, nil)

	_, err := svc.CreateScheduledTaskWithSource(CreateScheduledTaskInput{
		Name:          "Morning digest",
		Prompt:        "Send today's digest",
		AgentID:       7,
		ScheduleType:  ScheduleTypePreset,
		ScheduleValue: "every_day_0900",
		Enabled:       true,
	}, OperationSourceAI)
	if err != nil {
		t.Fatalf("CreateScheduledTaskWithSource returned error: %v", err)
	}

	logs := readScheduledTaskOperationLogs(t, db)
	if len(logs) != 1 {
		t.Fatalf("expected 1 operation log, got %d", len(logs))
	}
	if logs[0].OperationSource != OperationSourceAI {
		t.Fatalf("expected operation source %q, got %q", OperationSourceAI, logs[0].OperationSource)
	}
}

func TestOperationLogDetailReturnsStoredSnapshot(t *testing.T) {
	db := newScheduledTasksTestDB(t)
	insertScheduledTasksAgent(t, db, 7)

	svc := NewScheduledTasksServiceForTest(nil, db, nil, nil)
	created, err := svc.CreateScheduledTask(CreateScheduledTaskInput{
		Name:          "Morning digest",
		Prompt:        "Send today's digest",
		AgentID:       7,
		ScheduleType:  ScheduleTypePreset,
		ScheduleValue: "every_day_0900",
		Enabled:       true,
	})
	if err != nil {
		t.Fatalf("CreateScheduledTask returned error: %v", err)
	}

	logs, err := svc.ListScheduledTaskOperationLogs(created.ID, 1, 20)
	if err != nil {
		t.Fatalf("ListScheduledTaskOperationLogs returned error: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 operation log in service list, got %d", len(logs))
	}

	detail, err := svc.GetScheduledTaskOperationLogDetail(logs[0].ID)
	if err != nil {
		t.Fatalf("GetScheduledTaskOperationLogDetail returned error: %v", err)
	}
	if detail.TaskSnapshot.Name != "Morning digest" {
		t.Fatalf("expected detail snapshot name to be persisted, got %q", detail.TaskSnapshot.Name)
	}
}

func readScheduledTaskOperationLogs(t *testing.T, db interface {
	NewSelect() *bun.SelectQuery
}) []scheduledTaskOperationLogModel {
	t.Helper()

	var logs []scheduledTaskOperationLogModel
	if err := db.NewSelect().
		Model(&logs).
		OrderExpr("id ASC").
		Scan(context.Background()); err != nil {
		t.Fatalf("read operation logs failed: %v", err)
	}
	return logs
}

func decodeChangedFields(t *testing.T, raw string) []ScheduledTaskOperationChangedField {
	t.Helper()
	var items []ScheduledTaskOperationChangedField
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		t.Fatalf("decode changed fields: %v", err)
	}
	return items
}

func decodeOperationSnapshot(t *testing.T, raw string) ScheduledTaskOperationSnapshot {
	t.Helper()
	var snapshot ScheduledTaskOperationSnapshot
	if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
		t.Fatalf("decode operation snapshot: %v", err)
	}
	return snapshot
}

func assertChangedFieldPresent(t *testing.T, items []ScheduledTaskOperationChangedField, key string) {
	t.Helper()
	for _, item := range items {
		if item.FieldKey == key {
			return
		}
	}
	t.Fatalf("expected changed field %q to be present in %#v", key, items)
}
