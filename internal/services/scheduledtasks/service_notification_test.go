package scheduledtasks

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
)

type fakeNotificationSender struct {
	calls []fakeNotificationCall
	err   error
}

type fakeNotificationCall struct {
	task    ScheduledTask
	content string
}

func (f *fakeNotificationSender) SendTaskResult(_ context.Context, task ScheduledTask, content string) error {
	f.calls = append(f.calls, fakeNotificationCall{task: task, content: content})
	return f.err
}

func TestCreateAndUpdateScheduledTaskPersistsNotificationFields(t *testing.T) {
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

	if created.NotificationPlatform != "feishu" {
		t.Fatalf("expected notification platform feishu, got %q", created.NotificationPlatform)
	}
	if len(created.NotificationChannelIDs) != 2 || created.NotificationChannelIDs[0] != 11 || created.NotificationChannelIDs[1] != 12 {
		t.Fatalf("unexpected notification channel ids: %#v", created.NotificationChannelIDs)
	}

	updatedPlatform := "wecom"
	updatedChannelIDs := []int64{21}
	updated, err := svc.UpdateScheduledTask(created.ID, UpdateScheduledTaskInput{
		NotificationPlatform:   &updatedPlatform,
		NotificationChannelIDs: &updatedChannelIDs,
	})
	if err != nil {
		t.Fatalf("UpdateScheduledTask returned error: %v", err)
	}

	if updated.NotificationPlatform != "wecom" {
		t.Fatalf("expected updated notification platform wecom, got %q", updated.NotificationPlatform)
	}
	if len(updated.NotificationChannelIDs) != 1 || updated.NotificationChannelIDs[0] != 21 {
		t.Fatalf("unexpected updated notification channel ids: %#v", updated.NotificationChannelIDs)
	}

	items, err := svc.ListScheduledTasks()
	if err != nil {
		t.Fatalf("ListScheduledTasks returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 task, got %d", len(items))
	}
	if items[0].NotificationPlatform != "wecom" {
		t.Fatalf("expected persisted notification platform wecom, got %q", items[0].NotificationPlatform)
	}
	if len(items[0].NotificationChannelIDs) != 1 || items[0].NotificationChannelIDs[0] != 21 {
		t.Fatalf("unexpected persisted notification channel ids: %#v", items[0].NotificationChannelIDs)
	}
}

func TestCompleteRunSendsNotificationWithAssistantMessage(t *testing.T) {
	db := newScheduledTasksTestDB(t)
	now := time.Date(2026, 3, 19, 9, 0, 0, 0, time.UTC)
	insertScheduledTasksAgent(t, db, 9)
	insertScheduledTaskRow(t, db, 1001, 9, "feishu", `[101,102]`)
	insertScheduledTaskRunRow(t, db, 2001, 1001, now)
	insertAssistantMessageRow(t, db, 3001, 4001, "Daily summary is ready.")

	sender := &fakeNotificationSender{}
	svc := NewScheduledTasksServiceForTest(nil, db, nil, nil)
	svc.notificationSender = sender

	if err := svc.completeRun(context.Background(), 1001, 2001, 3001, now, ""); err != nil {
		t.Fatalf("completeRun returned error: %v", err)
	}

	if len(sender.calls) != 1 {
		t.Fatalf("expected 1 notification call, got %d", len(sender.calls))
	}
	if sender.calls[0].content != "Daily summary is ready." {
		t.Fatalf("unexpected notification content: %q", sender.calls[0].content)
	}
	if sender.calls[0].task.NotificationPlatform != "feishu" {
		t.Fatalf("unexpected notification platform: %q", sender.calls[0].task.NotificationPlatform)
	}
	if len(sender.calls[0].task.NotificationChannelIDs) != 2 {
		t.Fatalf("unexpected notification channel count: %#v", sender.calls[0].task.NotificationChannelIDs)
	}

	runStatus, taskStatus, taskError := readRunAndTaskStatus(t, db, 2001, 1001)
	if runStatus != RunStatusSuccess {
		t.Fatalf("expected run status success, got %q", runStatus)
	}
	if taskStatus != TaskStatusSuccess {
		t.Fatalf("expected task status success, got %q", taskStatus)
	}
	if taskError != "" {
		t.Fatalf("expected empty task error, got %q", taskError)
	}
}

func TestCompleteRunFailsWhenNotificationDeliveryFails(t *testing.T) {
	db := newScheduledTasksTestDB(t)
	now := time.Date(2026, 3, 19, 9, 0, 0, 0, time.UTC)
	insertScheduledTasksAgent(t, db, 9)
	insertScheduledTaskRow(t, db, 1002, 9, "feishu", `[101]`)
	insertScheduledTaskRunRow(t, db, 2002, 1002, now)
	insertAssistantMessageRow(t, db, 3002, 4002, "A notification should fail.")

	sender := &fakeNotificationSender{err: fmt.Errorf("channel unavailable")}
	svc := NewScheduledTasksServiceForTest(nil, db, nil, nil)
	svc.notificationSender = sender

	if err := svc.completeRun(context.Background(), 1002, 2002, 3002, now, ""); err != nil {
		t.Fatalf("completeRun returned error: %v", err)
	}

	runStatus, taskStatus, taskError := readRunAndTaskStatus(t, db, 2002, 1002)
	if runStatus != RunStatusFailed {
		t.Fatalf("expected run status failed, got %q", runStatus)
	}
	if taskStatus != TaskStatusFailed {
		t.Fatalf("expected task status failed, got %q", taskStatus)
	}
	if !strings.Contains(taskError, "channel unavailable") {
		t.Fatalf("expected task error to contain notification failure, got %q", taskError)
	}
}

func TestGetLatestChannelTargetUsesChannelLastSenderID(t *testing.T) {
	db := newScheduledTasksTestDB(t)
	svc := NewScheduledTasksServiceForTest(nil, db, nil, nil)

	insertNotificationChannelRow(t, db, 501, "feishu", true, "ou_latest_sender")

	targetID, err := svc.getLatestChannelTarget(context.Background(), db, 501)
	if err != nil {
		t.Fatalf("getLatestChannelTarget returned error: %v", err)
	}
	if targetID != "ou_latest_sender" {
		t.Fatalf("expected target id ou_latest_sender, got %q", targetID)
	}
}

func TestGetLatestChannelTargetRejectsEmptyChannelLastSenderID(t *testing.T) {
	db := newScheduledTasksTestDB(t)
	svc := NewScheduledTasksServiceForTest(nil, db, nil, nil)

	insertNotificationChannelRow(t, db, 502, "feishu", true, "")

	_, err := svc.getLatestChannelTarget(context.Background(), db, 502)
	if err == nil {
		t.Fatal("expected getLatestChannelTarget to fail for empty last_sender_id")
	}
	if !strings.Contains(err.Error(), "last sender") {
		t.Fatalf("expected error to mention last sender, got %v", err)
	}
}

func newScheduledTasksTestDB(t *testing.T) *bun.DB {
	t.Helper()

	sqlDB, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	db := bun.NewDB(sqlDB, sqlitedialect.New())
	t.Cleanup(func() {
		_ = db.Close()
	})

	schema := []string{
		`create table agents (
			id integer primary key,
			default_llm_provider_id text not null default '',
			default_llm_model_id text not null default ''
		);`,
		`create table scheduled_tasks (
			id integer primary key,
			created_at datetime not null default current_timestamp,
			updated_at datetime not null default current_timestamp,
			deleted_at datetime,
			name text not null,
			prompt text not null,
			agent_id integer not null,
			schedule_type text not null,
			schedule_value text not null default '',
			cron_expr text not null,
			timezone text not null default 'Local',
			enabled boolean not null default true,
			expires_at datetime,
			notification_platform text not null default '',
			notification_channel_ids text not null default '[]',
			last_run_at datetime,
			next_run_at datetime,
			last_status text not null default 'pending',
			last_error text not null default '',
			last_run_id integer
		);`,
		`create table scheduled_task_runs (
			id integer primary key,
			created_at datetime not null default current_timestamp,
			updated_at datetime not null default current_timestamp,
			task_id integer not null,
			trigger_type text not null,
			status text not null,
			started_at datetime not null,
			finished_at datetime,
			duration_ms integer not null default 0,
			error_message text not null default '',
			conversation_id integer,
			user_message_id integer,
			assistant_message_id integer,
			snapshot_task_name text not null,
			snapshot_prompt text not null,
			snapshot_agent_id integer not null
		);`,
		`create table scheduled_task_operation_logs (
			id integer primary key,
			created_at datetime not null default current_timestamp,
			task_id integer not null,
			task_name_snapshot text not null default '',
			operation_type text not null,
			operation_source text not null,
			changed_fields_json text not null default '[]',
			task_snapshot_json text not null default '{}'
		);`,
		`create table messages (
			id integer primary key,
			conversation_id integer not null,
			role text not null,
			content text not null,
			status text not null default 'success',
			error text not null default ''
		);`,
		`create table channels (
			id integer primary key,
			platform text not null,
			enabled boolean not null default true,
			last_sender_id text not null default ''
		);`,
	}

	for _, stmt := range schema {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("create schema failed: %v", err)
		}
	}

	return db
}

func insertScheduledTasksAgent(t *testing.T, db *bun.DB, agentID int64) {
	t.Helper()
	if _, err := db.Exec(`insert into agents(id, default_llm_provider_id, default_llm_model_id) values (?, '', '')`, agentID); err != nil {
		t.Fatalf("insert agent failed: %v", err)
	}
}

func insertScheduledTaskRow(t *testing.T, db *bun.DB, taskID int64, agentID int64, platform string, channelIDsJSON string) {
	t.Helper()
	if _, err := db.Exec(`
		insert into scheduled_tasks(
			id, name, prompt, agent_id, schedule_type, schedule_value, cron_expr, timezone, enabled,
			notification_platform, notification_channel_ids, last_status, last_error
		) values (?, 'task', 'prompt', ?, 'preset', 'every_day_0900', '0 9 * * *', 'UTC', 1, ?, ?, 'running', '')
	`, taskID, agentID, platform, channelIDsJSON); err != nil {
		t.Fatalf("insert task failed: %v", err)
	}
}

func insertScheduledTaskRunRow(t *testing.T, db *bun.DB, runID int64, taskID int64, startedAt time.Time) {
	t.Helper()
	if _, err := db.Exec(`
		insert into scheduled_task_runs(
			id, task_id, trigger_type, status, started_at, error_message, snapshot_task_name, snapshot_prompt, snapshot_agent_id
		) values (?, ?, 'schedule', 'running', ?, '', 'task', 'prompt', 9)
	`, runID, taskID, startedAt); err != nil {
		t.Fatalf("insert run failed: %v", err)
	}
}

func insertAssistantMessageRow(t *testing.T, db *bun.DB, messageID int64, conversationID int64, content string) {
	t.Helper()
	if _, err := db.Exec(`
		insert into messages(id, conversation_id, role, content, status, error)
		values (?, ?, 'assistant', ?, 'success', '')
	`, messageID, conversationID, content); err != nil {
		t.Fatalf("insert assistant message failed: %v", err)
	}
}

func insertNotificationChannelRow(t *testing.T, db *bun.DB, channelID int64, platform string, enabled bool, lastSenderID string) {
	t.Helper()
	if _, err := db.Exec(`
		insert into channels(id, platform, enabled, last_sender_id)
		values (?, ?, ?, ?)
	`, channelID, platform, enabled, lastSenderID); err != nil {
		t.Fatalf("insert channel failed: %v", err)
	}
}

func readRunAndTaskStatus(t *testing.T, db *bun.DB, runID int64, taskID int64) (string, string, string) {
	t.Helper()

	var runStatus string
	if err := db.NewSelect().
		Table("scheduled_task_runs").
		Column("status").
		Where("id = ?", runID).
		Scan(context.Background(), &runStatus); err != nil {
		t.Fatalf("read run status failed: %v", err)
	}

	var taskStatus string
	var taskError string
	if err := db.NewSelect().
		Table("scheduled_tasks").
		Column("last_status", "last_error").
		Where("id = ?", taskID).
		Scan(context.Background(), &taskStatus, &taskError); err != nil {
		t.Fatalf("read task status failed: %v", err)
	}

	return runStatus, taskStatus, taskError
}
