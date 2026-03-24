package scheduledtasks

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/uptrace/bun"
)

func TestCreateScheduledTaskPersistsExpiresAt(t *testing.T) {
	db := newScheduledTasksTestDB(t)
	insertScheduledTasksAgent(t, db, 7)

	now := time.Date(2026, 3, 19, 10, 0, 0, 0, time.UTC)
	expiresAt := time.Date(2026, 4, 1, 23, 59, 59, 0, time.UTC)

	svc := NewScheduledTasksServiceForTest(nil, db, nil, nil)
	svc.now = func() time.Time { return now }

	created, err := svc.CreateScheduledTask(CreateScheduledTaskInput{
		Name:          "Morning digest",
		Prompt:        "Send today's digest",
		AgentID:       7,
		ScheduleType:  ScheduleTypePreset,
		ScheduleValue: "every_day_0900",
		Enabled:       true,
		ExpiresAt:     &expiresAt,
	})
	if err != nil {
		t.Fatalf("CreateScheduledTask returned error: %v", err)
	}

	if created.ExpiresAt == nil {
		t.Fatal("expected created task to include expires_at")
	}
	if !created.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("expected expires_at %v, got %v", expiresAt, created.ExpiresAt)
	}
	if created.IsExpired {
		t.Fatal("expected freshly created task not to be expired")
	}
}

func TestSetScheduledTaskEnabledRejectsExpiredTask(t *testing.T) {
	db := newScheduledTasksTestDB(t)
	insertScheduledTasksAgent(t, db, 7)

	now := time.Date(2026, 4, 2, 9, 0, 0, 0, time.UTC)
	expiredAt := time.Date(2026, 4, 1, 23, 59, 59, 0, time.UTC)
	insertScheduledTaskWithExpirationRow(t, db, 3001, 7, false, expiredAt)

	svc := NewScheduledTasksServiceForTest(nil, db, nil, nil)
	svc.now = func() time.Time { return now }

	_, err := svc.SetScheduledTaskEnabled(3001, true)
	if err == nil {
		t.Fatal("expected SetScheduledTaskEnabled to reject expired task")
	}
	if !strings.Contains(err.Error(), "expired") && !strings.Contains(err.Error(), "过期") {
		t.Fatalf("expected expiration error, got %v", err)
	}

	model := readScheduledTaskModelByID(t, db, 3001)
	if model.Enabled {
		t.Fatal("expected expired task to remain disabled")
	}
}

func TestDisableExpiredTasksTurnsOffEnabledExpiredTasks(t *testing.T) {
	db := newScheduledTasksTestDB(t)
	insertScheduledTasksAgent(t, db, 7)

	now := time.Date(2026, 4, 2, 9, 0, 0, 0, time.UTC)
	expiredAt := time.Date(2026, 4, 1, 23, 59, 59, 0, time.UTC)
	insertScheduledTaskWithExpirationRow(t, db, 3002, 7, true, expiredAt)

	svc := NewScheduledTasksServiceForTest(nil, db, nil, nil)
	svc.now = func() time.Time { return now }

	disabledCount, err := svc.markExpiredTasks(context.Background())
	if err != nil {
		t.Fatalf("markExpiredTasks returned error: %v", err)
	}
	if disabledCount != 1 {
		t.Fatalf("expected 1 task to be auto-disabled, got %d", disabledCount)
	}

	model := readScheduledTaskModelByID(t, db, 3002)
	if !model.Enabled {
		t.Fatal("expected expired task to keep enabled switch after sweeper")
	}
	if model.NextRunAt != nil {
		t.Fatalf("expected next_run_at to be cleared, got %v", model.NextRunAt)
	}
	if model.LastStatus != TaskStatusExpired {
		t.Fatalf("expected last_status %q, got %q", TaskStatusExpired, model.LastStatus)
	}
}

func TestReloadEnabledTasksAutoDisablesExpiredTasks(t *testing.T) {
	db := newScheduledTasksTestDB(t)
	insertScheduledTasksAgent(t, db, 7)

	now := time.Date(2026, 4, 2, 9, 0, 0, 0, time.UTC)
	expiredAt := time.Date(2026, 4, 1, 23, 59, 59, 0, time.UTC)
	insertScheduledTaskWithExpirationRow(t, db, 3003, 7, true, expiredAt)

	svc := NewScheduledTasksServiceForTest(nil, db, nil, nil)
	svc.now = func() time.Time { return now }

	if err := svc.reloadEnabledTasks(); err != nil {
		t.Fatalf("reloadEnabledTasks returned error: %v", err)
	}

	model := readScheduledTaskModelByID(t, db, 3003)
	if !model.Enabled {
		t.Fatal("expected reloadEnabledTasks to keep enabled switch for expired task")
	}
	if model.LastStatus != TaskStatusExpired {
		t.Fatalf("expected last_status %q, got %q", TaskStatusExpired, model.LastStatus)
	}
}

func insertScheduledTaskWithExpirationRow(t *testing.T, db *bun.DB, taskID int64, agentID int64, enabled bool, expiresAt time.Time) {
	t.Helper()
	enabledInt := 0
	if enabled {
		enabledInt = 1
	}
	if _, err := db.Exec(`
		insert into scheduled_tasks(
			id, name, prompt, agent_id, schedule_type, schedule_value, cron_expr, timezone, enabled,
			notification_platform, notification_channel_ids, last_status, last_error, expires_at
		) values (?, 'task', 'prompt', ?, 'preset', 'every_day_0900', '0 9 * * *', 'UTC', ?, '', '[]', 'pending', '', ?)
	`, taskID, agentID, enabledInt, expiresAt); err != nil {
		t.Fatalf("insert task with expiration failed: %v", err)
	}
}

func readScheduledTaskModelByID(t *testing.T, db *bun.DB, taskID int64) scheduledTaskModel {
	t.Helper()

	var model scheduledTaskModel
	err := db.NewSelect().
		Model(&model).
		Where("id = ?", taskID).
		Limit(1).
		Scan(context.Background())
	if err != nil {
		t.Fatalf("read scheduled task failed: %v", err)
	}
	return model
}
