package scheduledtasks

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"chatclaw/internal/services/i18n"
	"chatclaw/internal/services/chat"
	"chatclaw/internal/services/conversations"

	_ "github.com/mattn/go-sqlite3"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
)

func TestParseSchedule(t *testing.T) {
	now := time.Date(2026, 3, 9, 8, 30, 0, 0, time.FixedZone("CST", 8*3600))

	t.Run("preset weekdays", func(t *testing.T) {
		schedule, err := parseSchedule(ScheduleTypePreset, "weekdays_0900", "", now)
		if err != nil {
			t.Fatalf("parseSchedule returned error: %v", err)
		}
		if schedule.CronExpr != "0 9 * * 1-5" {
			t.Fatalf("unexpected cron expr: %s", schedule.CronExpr)
		}
		if schedule.NextRunAt == nil || schedule.NextRunAt.Before(now) {
			t.Fatalf("expected future next run")
		}
	})

	t.Run("preset every 5 minutes", func(t *testing.T) {
		schedule, err := parseSchedule(ScheduleTypePreset, "every_5_minutes", "", now)
		if err != nil {
			t.Fatalf("parseSchedule returned error: %v", err)
		}
		if schedule.CronExpr != "*/5 * * * *" {
			t.Fatalf("unexpected cron expr: %s", schedule.CronExpr)
		}
		if schedule.NextRunAt == nil || schedule.NextRunAt.Format(time.RFC3339) != "2026-03-09T08:35:00+08:00" {
			t.Fatalf("unexpected next run: %v", schedule.NextRunAt)
		}
	})

	t.Run("preset every minute", func(t *testing.T) {
		schedule, err := parseSchedule(ScheduleTypePreset, "every_minute", "", now)
		if err != nil {
			t.Fatalf("parseSchedule returned error: %v", err)
		}
		if schedule.CronExpr != "* * * * *" {
			t.Fatalf("unexpected cron expr: %s", schedule.CronExpr)
		}
	})

	t.Run("preset every 15 minutes", func(t *testing.T) {
		schedule, err := parseSchedule(ScheduleTypePreset, "every_15_minutes", "", now)
		if err != nil {
			t.Fatalf("parseSchedule returned error: %v", err)
		}
		if schedule.CronExpr != "*/15 * * * *" {
			t.Fatalf("unexpected cron expr: %s", schedule.CronExpr)
		}
	})

	t.Run("preset every day 6pm", func(t *testing.T) {
		schedule, err := parseSchedule(ScheduleTypePreset, "every_day_1800", "", now)
		if err != nil {
			t.Fatalf("parseSchedule returned error: %v", err)
		}
		if schedule.CronExpr != "0 18 * * *" {
			t.Fatalf("unexpected cron expr: %s", schedule.CronExpr)
		}
	})

	t.Run("preset first day of month 9am", func(t *testing.T) {
		schedule, err := parseSchedule(ScheduleTypePreset, "every_month_1_0900", "", now)
		if err != nil {
			t.Fatalf("parseSchedule returned error: %v", err)
		}
		if schedule.CronExpr != "0 9 1 * *" {
			t.Fatalf("unexpected cron expr: %s", schedule.CronExpr)
		}
	})

	t.Run("custom weekdays", func(t *testing.T) {
		schedule, err := parseSchedule(ScheduleTypeCustom, `{"minute":15,"hour":10,"weekdays":[1,3,5]}`, "", now)
		if err != nil {
			t.Fatalf("parseSchedule returned error: %v", err)
		}
		if schedule.CronExpr != "15 10 * * 1,3,5" {
			t.Fatalf("unexpected cron expr: %s", schedule.CronExpr)
		}
	})

	t.Run("invalid preset", func(t *testing.T) {
		if _, err := parseSchedule(ScheduleTypePreset, "unknown", "", now); err == nil {
			t.Fatalf("expected invalid preset error")
		}
	})
}

func TestScheduledTaskCRUD(t *testing.T) {
	db := newTestDB(t)
	seedAgent(t, db, 1, "openai", "gpt-5")

	svc := NewScheduledTasksServiceForTest(nil, db, &stubConversationService{}, stubChatService{})

	task, err := svc.CreateScheduledTask(CreateScheduledTaskInput{
		Name:          "日报",
		Prompt:        "生成日报",
		AgentID:       1,
		ScheduleType:  ScheduleTypePreset,
		ScheduleValue: "every_day_0900",
		Enabled:       true,
	})
	if err != nil {
		t.Fatalf("CreateScheduledTask returned error: %v", err)
	}
	if task.ID <= 0 {
		t.Fatalf("expected persisted task ID")
	}
	if task.NextRunAt == nil {
		t.Fatalf("expected next run")
	}

	newName := "工作日报"
	updated, err := svc.UpdateScheduledTask(task.ID, UpdateScheduledTaskInput{
		Name:    &newName,
		Enabled: boolPtr(false),
	})
	if err != nil {
		t.Fatalf("UpdateScheduledTask returned error: %v", err)
	}
	if updated.Name != newName {
		t.Fatalf("unexpected updated name: %s", updated.Name)
	}
	if updated.NextRunAt != nil {
		t.Fatalf("expected next run cleared when disabled")
	}

	summary, err := svc.GetScheduledTaskSummary()
	if err != nil {
		t.Fatalf("GetScheduledTaskSummary returned error: %v", err)
	}
	if summary.Total != 1 || summary.Paused != 1 {
		t.Fatalf("unexpected summary: %+v", summary)
	}

	if err := svc.DeleteScheduledTask(task.ID); err != nil {
		t.Fatalf("DeleteScheduledTask returned error: %v", err)
	}
	tasks, err := svc.ListScheduledTasks()
	if err != nil {
		t.Fatalf("ListScheduledTasks returned error: %v", err)
	}
	if len(tasks) != 0 {
		t.Fatalf("expected deleted task hidden from list")
	}
}

func TestGetScheduledTaskByID(t *testing.T) {
	db := newTestDB(t)
	seedAgent(t, db, 1, "openai", "gpt-5")

	svc := NewScheduledTasksServiceForTest(nil, db, &stubConversationService{}, stubChatService{})
	created, err := svc.CreateScheduledTask(CreateScheduledTaskInput{
		Name:          "日报",
		Prompt:        "生成日报",
		AgentID:       1,
		ScheduleType:  ScheduleTypePreset,
		ScheduleValue: "every_day_0900",
		Enabled:       true,
	})
	if err != nil {
		t.Fatalf("CreateScheduledTask returned error: %v", err)
	}

	task, err := svc.GetScheduledTaskByID(created.ID)
	if err != nil {
		t.Fatalf("GetScheduledTaskByID returned error: %v", err)
	}
	if task.ID != created.ID || task.Name != created.Name {
		t.Fatalf("unexpected task: %+v", task)
	}
}

func TestFindScheduledTasksByName(t *testing.T) {
	db := newTestDB(t)
	seedAgent(t, db, 1, "openai", "gpt-5")

	svc := NewScheduledTasksServiceForTest(nil, db, &stubConversationService{}, stubChatService{})
	first, err := svc.CreateScheduledTask(CreateScheduledTaskInput{
		Name:          "销售日报",
		Prompt:        "生成销售日报",
		AgentID:       1,
		ScheduleType:  ScheduleTypePreset,
		ScheduleValue: "every_day_0900",
		Enabled:       true,
	})
	if err != nil {
		t.Fatalf("CreateScheduledTask returned error: %v", err)
	}
	second, err := svc.CreateScheduledTask(CreateScheduledTaskInput{
		Name:          "市场日报",
		Prompt:        "生成市场日报",
		AgentID:       1,
		ScheduleType:  ScheduleTypePreset,
		ScheduleValue: "every_day_0900",
		Enabled:       true,
	})
	if err != nil {
		t.Fatalf("CreateScheduledTask returned error: %v", err)
	}

	tasks, err := svc.FindScheduledTasksByName("日报")
	if err != nil {
		t.Fatalf("FindScheduledTasksByName returned error: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("unexpected tasks: %+v", tasks)
	}

	if err := svc.DeleteScheduledTask(first.ID); err != nil {
		t.Fatalf("DeleteScheduledTask returned error: %v", err)
	}

	tasks, err = svc.FindScheduledTasksByName("日报")
	if err != nil {
		t.Fatalf("FindScheduledTasksByName returned error: %v", err)
	}
	if len(tasks) != 1 || tasks[0].ID != second.ID {
		t.Fatalf("unexpected remaining tasks: %+v", tasks)
	}
}

func TestRunScheduledTaskNow(t *testing.T) {
	db := newTestDB(t)
	seedAgent(t, db, 1, "openai", "gpt-5")

	convSvc := &stubConversationService{
		conversation: &conversations.Conversation{ID: 42, AgentID: 1, Name: "scheduled"},
	}
	svc := NewScheduledTasksServiceForTest(nil, db, convSvc, stubChatService{
		sendResult: &chat.SendMessageResult{MessageID: 88, RequestID: "req-1"},
	})

	task, err := svc.CreateScheduledTask(CreateScheduledTaskInput{
		Name:          "日报",
		Prompt:        "生成日报",
		AgentID:       1,
		ScheduleType:  ScheduleTypePreset,
		ScheduleValue: "every_day_0900",
		Enabled:       true,
	})
	if err != nil {
		t.Fatalf("CreateScheduledTask returned error: %v", err)
	}

	run, err := svc.RunScheduledTaskNow(task.ID)
	if err != nil {
		t.Fatalf("RunScheduledTaskNow returned error: %v", err)
	}
	if run.TaskID != task.ID {
		t.Fatalf("unexpected task id: %d", run.TaskID)
	}
	if run.ConversationID == nil || *run.ConversationID != 42 {
		t.Fatalf("expected conversation id to be stored")
	}
	if run.UserMessageID == nil || *run.UserMessageID != 88 {
		t.Fatalf("expected user message id to be stored")
	}
	if convSvc.lastCreateInput == nil {
		t.Fatalf("expected conversation create input to be captured")
	}
	if convSvc.lastCreateInput.LLMProviderID != "openai" {
		t.Fatalf("expected provider from agent defaults, got %q", convSvc.lastCreateInput.LLMProviderID)
	}
	if convSvc.lastCreateInput.LLMModelID != "gpt-5" {
		t.Fatalf("expected model from agent defaults, got %q", convSvc.lastCreateInput.LLMModelID)
	}
	if len(convSvc.lastCreateInput.LibraryIDs) != 0 {
		t.Fatalf("expected no libraries for scheduled task conversation, got %+v", convSvc.lastCreateInput.LibraryIDs)
	}
	if convSvc.lastCreateInput.EnableThinking {
		t.Fatalf("expected thinking disabled by default")
	}
	if convSvc.lastCreateInput.ChatMode != conversations.ChatModeTask {
		t.Fatalf("expected task mode by default, got %q", convSvc.lastCreateInput.ChatMode)
	}
}

func TestRunScheduledTaskNowConversationNameFollowsLocale(t *testing.T) {
	db := newTestDB(t)
	seedAgent(t, db, 1, "openai", "gpt-5")

	makeService := func() (*ScheduledTasksService, *stubConversationService) {
		convSvc := &stubConversationService{
			conversation: &conversations.Conversation{ID: 42, AgentID: 1, Name: "scheduled"},
		}
		svc := NewScheduledTasksServiceForTest(nil, db, convSvc, stubChatService{
			sendResult: &chat.SendMessageResult{MessageID: 88, RequestID: "req-1"},
		})
		return svc, convSvc
	}

	i18n.SetLocale(i18n.LocaleZhCN)
	svc, convSvc := makeService()
	task, err := svc.CreateScheduledTask(CreateScheduledTaskInput{
		Name:          "日报",
		Prompt:        "生成日报",
		AgentID:       1,
		ScheduleType:  ScheduleTypePreset,
		ScheduleValue: "every_day_0900",
		Enabled:       true,
	})
	if err != nil {
		t.Fatalf("CreateScheduledTask returned error: %v", err)
	}
	if _, err := svc.RunScheduledTaskNow(task.ID); err != nil {
		t.Fatalf("RunScheduledTaskNow returned error: %v", err)
	}
	if convSvc.lastCreateInput == nil || convSvc.lastCreateInput.Name == "" {
		t.Fatalf("expected conversation create input name")
	}
	if !strings.HasPrefix(convSvc.lastCreateInput.Name, "(定时) ") {
		t.Fatalf("expected zh-CN conversation name, got %q", convSvc.lastCreateInput.Name)
	}

	i18n.SetLocale(i18n.LocaleEnUS)
	svc, convSvc = makeService()
	englishTask, err := svc.CreateScheduledTask(CreateScheduledTaskInput{
		Name:          "Daily Report",
		Prompt:        "Generate daily report",
		AgentID:       1,
		ScheduleType:  ScheduleTypePreset,
		ScheduleValue: "every_day_0900",
		Enabled:       true,
	})
	if err != nil {
		t.Fatalf("CreateScheduledTask returned error: %v", err)
	}
	if _, err := svc.RunScheduledTaskNow(englishTask.ID); err != nil {
		t.Fatalf("RunScheduledTaskNow returned error: %v", err)
	}
	if convSvc.lastCreateInput == nil || convSvc.lastCreateInput.Name == "" {
		t.Fatalf("expected conversation create input name")
	}
	if !strings.HasPrefix(convSvc.lastCreateInput.Name, "(Scheduled) ") {
		t.Fatalf("expected en-US conversation name, got %q", convSvc.lastCreateInput.Name)
	}
}

func newTestDB(t *testing.T) *bun.DB {
	t.Helper()

	sqlDB, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	db := bun.NewDB(sqlDB, sqlitedialect.New())

	schema := `
create table agents (
	id integer primary key,
	name text not null default '',
	prompt text not null default '',
	icon text not null default '',
	default_llm_provider_id text not null default '',
	default_llm_model_id text not null default '',
	llm_temperature real not null default 0,
	llm_top_p real not null default 0,
	llm_max_context_count integer not null default 0,
	llm_max_tokens integer not null default 0,
	enable_llm_temperature boolean not null default false,
	enable_llm_top_p boolean not null default false,
	enable_llm_max_tokens boolean not null default false,
	retrieval_match_threshold real not null default 0,
	retrieval_top_k integer not null default 0,
	sandbox_mode text not null default '',
	sandbox_network boolean not null default false,
	work_dir text not null default '',
	created_at datetime not null default current_timestamp,
	updated_at datetime not null default current_timestamp
);
create table scheduled_tasks (
	id integer primary key autoincrement,
	created_at datetime not null default current_timestamp,
	updated_at datetime not null default current_timestamp,
	deleted_at datetime,
	name text not null,
	prompt text not null,
	agent_id integer not null,
	schedule_type text not null,
	schedule_value text not null,
	cron_expr text not null,
	timezone text not null,
	enabled boolean not null default true,
	last_run_at datetime,
	next_run_at datetime,
	last_status text not null default 'pending',
	last_error text not null default '',
	last_run_id integer
);
create table scheduled_task_runs (
	id integer primary key autoincrement,
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
);
create table messages (
	id integer primary key autoincrement,
	conversation_id integer not null,
	role text not null,
	status text not null default '',
	error text not null default '',
	created_at datetime not null default current_timestamp
);`
	if _, err := db.ExecContext(context.Background(), schema); err != nil {
		t.Fatalf("schema exec failed: %v", err)
	}

	t.Cleanup(func() {
		_ = db.Close()
	})
	return db
}

func seedAgent(t *testing.T, db *bun.DB, id int64, providerID, modelID string) {
	t.Helper()
	query := `insert into agents(id, name, prompt, default_llm_provider_id, default_llm_model_id) values(?, ?, ?, ?, ?)`
	if _, err := db.ExecContext(context.Background(), query, id, "assistant", "prompt", providerID, modelID); err != nil {
		t.Fatalf("seed agent failed: %v", err)
	}
}

type stubConversationService struct {
	conversation    *conversations.Conversation
	err             error
	lastCreateInput *conversations.CreateConversationInput
}

func (s *stubConversationService) CreateConversation(input conversations.CreateConversationInput) (*conversations.Conversation, error) {
	s.lastCreateInput = &input
	if s.err != nil {
		return nil, s.err
	}
	if s.conversation != nil {
		return s.conversation, nil
	}
	return &conversations.Conversation{ID: 1, AgentID: input.AgentID, Name: input.Name}, nil
}

func (s *stubConversationService) GetConversation(id int64) (*conversations.Conversation, error) {
	if s.conversation != nil {
		return s.conversation, nil
	}
	return &conversations.Conversation{ID: id}, nil
}

type stubChatService struct {
	sendResult *chat.SendMessageResult
	sendErr    error
	messages   []chat.Message
}

func (s stubChatService) SendMessage(input chat.SendMessageInput) (*chat.SendMessageResult, error) {
	if s.sendErr != nil {
		return nil, s.sendErr
	}
	if s.sendResult != nil {
		return s.sendResult, nil
	}
	return &chat.SendMessageResult{MessageID: 1, RequestID: "req"}, nil
}

func (s stubChatService) GetMessages(conversationID int64) ([]chat.Message, error) {
	return s.messages, nil
}

func boolPtr(v bool) *bool {
	return &v
}
