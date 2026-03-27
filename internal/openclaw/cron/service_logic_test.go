package openclawcron

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	chatservice "chatclaw/internal/services/chat"
	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestParseRunNowResult_ExtractsRunID(t *testing.T) {
	result, err := parseRunNowResult([]byte(`{"ok":true,"enqueued":true,"runId":"run-123"}`), time.UnixMilli(1234))
	if err != nil {
		t.Fatalf("parseRunNowResult returned error: %v", err)
	}
	if result.RunID != "run-123" {
		t.Fatalf("expected run id run-123, got %q", result.RunID)
	}
	if !result.Enqueued {
		t.Fatalf("expected enqueued result")
	}
}

func TestExtractGatewayRunContext_ReadsAgentEvent(t *testing.T) {
	payload := json.RawMessage(`{"runId":"run-1","sessionKey":"agent:main:cron:job-1","stream":"assistant","data":{"delta":"hi"}}`)
	runID, sessionKey := extractGatewayRunContext("agent", payload)
	if runID != "run-1" || sessionKey != "agent:main:cron:job-1" {
		t.Fatalf("expected run/session context from agent event, got %q / %q", runID, sessionKey)
	}
}

func TestParseJobIDFromSessionKey_ReadsCronSessionKey(t *testing.T) {
	jobID := parseJobIDFromSessionKey("agent:main:cron:job-1")
	if jobID != "job-1" {
		t.Fatalf("expected job id job-1, got %q", jobID)
	}
}

func TestBuildCronForwardMessageID_DiffersAcrossRuns(t *testing.T) {
	left := buildCronForwardMessageID(29, "agent:main:cron:job-1", "run-1")
	right := buildCronForwardMessageID(29, "agent:main:cron:job-1", "run-2")

	if left == right {
		t.Fatalf("expected different message ids for different runs, got %d and %d", left, right)
	}
}

func TestBuildRunLogHistoryItems_MapsRunEntriesToHistoryItems(t *testing.T) {
	runItems := buildRunLogHistoryItems([]OpenClawCronRunEntry{{
		JobID:      "job-1",
		Action:     "manual",
		Status:     "success",
		RunAtMs:    1234,
		DurationMs: 5678,
		SessionID:  "session-1",
		SessionKey: "agent:main:cron:job-1:run:session-1",
	}})

	if len(runItems) != 1 {
		t.Fatalf("expected 1 run history item, got %d", len(runItems))
	}
	if runItems[0].JobID != "job-1" {
		t.Fatalf("expected job id to be preserved, got %q", runItems[0].JobID)
	}
	if runItems[0].TriggerType != "manual" {
		t.Fatalf("expected manual trigger type, got %q", runItems[0].TriggerType)
	}
	if runItems[0].Source != OpenClawCronHistorySourceRunLog {
		t.Fatalf("expected run log source, got %q", runItems[0].Source)
	}
	if runItems[0].SessionKey != "agent:main:cron:job-1:run:session-1" {
		t.Fatalf("expected session key to be preserved, got %q", runItems[0].SessionKey)
	}
}

func TestBuildCronForwardEvents_StartsAndStreamsAssistantDelta(t *testing.T) {
	state := &cronForwardState{}
	events := buildCronForwardEvents(
		29,
		"agent:main:cron:job-1",
		"run-1",
		state,
		"agent",
		json.RawMessage(`{"runId":"run-1","sessionKey":"agent:main:cron:job-1","stream":"assistant","data":{"delta":"hello"}}`),
	)

	if len(events) != 2 {
		t.Fatalf("expected start + chunk events, got %d", len(events))
	}
	if events[0].Name != chatservice.EventChatStart {
		t.Fatalf("expected first event chat:start, got %q", events[0].Name)
	}
	if events[1].Name != chatservice.EventChatChunk {
		t.Fatalf("expected second event chat:chunk, got %q", events[1].Name)
	}
	chunk, ok := events[1].Payload.(chatservice.ChatChunkEvent)
	if !ok {
		t.Fatalf("expected chunk payload type, got %T", events[1].Payload)
	}
	if chunk.ConversationID != 29 {
		t.Fatalf("expected conversation id 29, got %d", chunk.ConversationID)
	}
	if chunk.Delta != "hello" {
		t.Fatalf("expected delta hello, got %q", chunk.Delta)
	}
	if !state.Started {
		t.Fatalf("expected forward state to be marked started")
	}
}

func TestBuildCronForwardEvents_UsesTextFallbackForAssistantStream(t *testing.T) {
	state := &cronForwardState{}
	events := buildCronForwardEvents(
		29,
		"agent:main:cron:job-1",
		"run-1",
		state,
		"agent",
		json.RawMessage(`{"runId":"run-1","sessionKey":"agent:main:cron:job-1","stream":"assistant","data":{"text":"hello"}}`),
	)

	if len(events) != 2 {
		t.Fatalf("expected start + chunk events, got %d", len(events))
	}
	chunk, ok := events[1].Payload.(chatservice.ChatChunkEvent)
	if !ok {
		t.Fatalf("expected chunk payload type, got %T", events[1].Payload)
	}
	if chunk.Delta != "hello" {
		t.Fatalf("expected text fallback hello, got %q", chunk.Delta)
	}
}

func TestBuildCronForwardEvents_CompletesOnLifecycleEnd(t *testing.T) {
	state := &cronForwardState{
		RequestID: "openclaw-cron:agent:main:cron:job-1:run-1",
		MessageID: -29001,
		Started:   true,
	}
	events := buildCronForwardEvents(
		29,
		"agent:main:cron:job-1",
		"run-1",
		state,
		"agent",
		json.RawMessage(`{"runId":"run-1","sessionKey":"agent:main:cron:job-1","stream":"lifecycle","data":{"phase":"end"}}`),
	)

	if len(events) != 1 {
		t.Fatalf("expected complete event only, got %d", len(events))
	}
	if events[0].Name != chatservice.EventChatComplete {
		t.Fatalf("expected chat:complete event, got %q", events[0].Name)
	}
	complete, ok := events[0].Payload.(chatservice.ChatCompleteEvent)
	if !ok {
		t.Fatalf("expected complete payload type, got %T", events[0].Payload)
	}
	if complete.Status != "success" {
		t.Fatalf("expected success status, got %q", complete.Status)
	}
	if !state.Finished {
		t.Fatalf("expected forward state to be marked finished")
	}
}

func TestBuildCronForwardEvents_TranslatesChatDeltaBlocks(t *testing.T) {
	state := &cronForwardState{
		RequestID: "openclaw-cron:agent:main:cron:job-1:run-1",
		MessageID: -29001,
	}
	events := buildCronForwardEvents(
		29,
		"agent:main:cron:job-1",
		"run-1",
		state,
		"chat",
		json.RawMessage(`{
			"runId":"run-1",
			"sessionKey":"agent:main:cron:job-1",
			"state":"delta",
			"message":{
				"role":"assistant",
				"content":[
					{"type":"thinking","thinking":"step one"},
					{"type":"toolCall","toolCallId":"call-1","name":"weather","args":{"city":"Shanghai"}},
					{"type":"toolResult","toolCallId":"call-1","content":{"temp":12}},
					{"type":"text","text":"hello"}
				]
			}
		}`),
	)

	if len(events) != 5 {
		t.Fatalf("expected start + tool call + tool result + thinking + chunk, got %d", len(events))
	}
	if events[0].Name != chatservice.EventChatStart {
		t.Fatalf("expected first event chat:start, got %q", events[0].Name)
	}
	if events[1].Name != chatservice.EventChatTool || events[2].Name != chatservice.EventChatTool {
		t.Fatalf("expected tool events at positions 2 and 3, got %q / %q", events[1].Name, events[2].Name)
	}
	if events[3].Name != chatservice.EventChatThinking {
		t.Fatalf("expected fourth event chat:thinking, got %q", events[3].Name)
	}
	if events[4].Name != chatservice.EventChatChunk {
		t.Fatalf("expected final event chat:chunk, got %q", events[4].Name)
	}

	toolCall, ok := events[1].Payload.(chatservice.ChatToolEvent)
	if !ok || toolCall.Type != "call" || toolCall.ToolCallID != "call-1" {
		t.Fatalf("unexpected tool call payload: %#v", events[1].Payload)
	}
	toolResult, ok := events[2].Payload.(chatservice.ChatToolEvent)
	if !ok || toolResult.Type != "result" || toolResult.ToolCallID != "call-1" {
		t.Fatalf("unexpected tool result payload: %#v", events[2].Payload)
	}
	thinking, ok := events[3].Payload.(chatservice.ChatThinkingEvent)
	if !ok || thinking.Delta != "step one" {
		t.Fatalf("unexpected thinking payload: %#v", events[3].Payload)
	}
	chunk, ok := events[4].Payload.(chatservice.ChatChunkEvent)
	if !ok || chunk.Delta != "hello" {
		t.Fatalf("unexpected chunk payload: %#v", events[4].Payload)
	}
}

func TestListHistory_ReadsOpenClawRunLogWithoutConversationFallback(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("USERPROFILE", tempHome)
	t.Setenv("HOMEDRIVE", "")
	t.Setenv("HOMEPATH", "")

	runFile := filepath.Join(tempHome, ".chatclaw", "openclaw", "cron", "runs", "job-1.jsonl")
	if err := os.MkdirAll(filepath.Dir(runFile), 0o755); err != nil {
		t.Fatalf("create run log dir: %v", err)
	}
	if err := os.WriteFile(runFile, []byte(`{"ts":1234,"jobId":"job-1","action":"manual","status":"success","runAtMs":1234,"durationMs":5678,"sessionId":"session-1","sessionKey":"agent:main:cron:job-1:run:session-1"}`+"\n"), 0o644); err != nil {
		t.Fatalf("write run log: %v", err)
	}

	service := &OpenClawCronService{}
	items, err := service.ListHistory("job-1", 10)
	if err != nil {
		t.Fatalf("ListHistory returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 history item from run log, got %d", len(items))
	}
	if items[0].Source != OpenClawCronHistorySourceRunLog {
		t.Fatalf("expected run log source, got %q", items[0].Source)
	}
	if items[0].SessionID != "session-1" {
		t.Fatalf("expected session id from run log, got %q", items[0].SessionID)
	}
}

func TestGetRunDetail_ReadsTranscriptWithoutCreatingConversation(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("USERPROFILE", tempHome)
	t.Setenv("HOMEDRIVE", "")
	t.Setenv("HOMEPATH", "")

	rootDir := filepath.Join(tempHome, ".chatclaw", "openclaw")
	runFile := filepath.Join(rootDir, "cron", "runs", "job-1.jsonl")
	sessionFile := filepath.Join(rootDir, "agents", "main", "sessions", "session-1.jsonl")
	if err := os.MkdirAll(filepath.Dir(runFile), 0o755); err != nil {
		t.Fatalf("create run log dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(sessionFile), 0o755); err != nil {
		t.Fatalf("create session dir: %v", err)
	}
	runLine := `{"ts":1234,"jobId":"job-1","action":"manual","status":"success","runAtMs":1234,"durationMs":5678,"sessionId":"session-1","sessionKey":"agent:main:cron:job-1:run:session-1"}`
	if err := os.WriteFile(runFile, []byte(runLine+"\n"), 0o644); err != nil {
		t.Fatalf("write run log: %v", err)
	}
	transcriptLine := `{"type":"message","id":"msg-1","timestamp":"2026-03-27T10:00:00Z","message":{"role":"assistant","provider":"openai","model":"gpt-5","stopReason":"end_turn","content":[{"type":"thinking","thinking":"thinking text"},{"type":"text","text":"final answer"}]}}`
	if err := os.WriteFile(sessionFile, []byte(transcriptLine+"\n"), 0o644); err != nil {
		t.Fatalf("write session transcript: %v", err)
	}

	service := &OpenClawCronService{}
	detail, err := service.GetRunDetail("job-1", "session-1")
	if err != nil {
		t.Fatalf("GetRunDetail returned error: %v", err)
	}
	if detail == nil {
		t.Fatalf("expected detail to be returned")
	}
	if detail.ConversationID != 0 {
		t.Fatalf("expected history detail to avoid local conversation mapping, got %d", detail.ConversationID)
	}
	if len(detail.Messages) != 1 {
		t.Fatalf("expected 1 transcript message, got %d", len(detail.Messages))
	}
	if detail.Messages[0].Role != "assistant" {
		t.Fatalf("expected assistant transcript message, got %q", detail.Messages[0].Role)
	}
	if detail.Messages[0].ContentText != "thinking text\n\nfinal answer" {
		t.Fatalf("expected transcript text to be preserved, got %q", detail.Messages[0].ContentText)
	}
}

func TestGetSummary_CountsFailedJobs(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("USERPROFILE", tempHome)
	t.Setenv("HOMEDRIVE", "")
	t.Setenv("HOMEPATH", "")

	rootDir := filepath.Join(tempHome, ".chatclaw", "openclaw")
	jobsFile := filepath.Join(rootDir, "cron", "jobs.json")
	firstRunFile := filepath.Join(rootDir, "cron", "runs", "job-1.jsonl")
	secondRunFile := filepath.Join(rootDir, "cron", "runs", "job-2.jsonl")
	if err := os.MkdirAll(filepath.Dir(jobsFile), 0o755); err != nil {
		t.Fatalf("create cron dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(firstRunFile), 0o755); err != nil {
		t.Fatalf("create run dir: %v", err)
	}

	jobsPayload := `{
		"jobs":[
			{"id":"job-1","name":"job-1","enabled":true,"state":{"lastStatus":"error"}},
			{"id":"job-2","name":"job-2","enabled":false,"state":{"lastStatus":"success"}}
		]
	}`
	if err := os.WriteFile(jobsFile, []byte(jobsPayload), 0o644); err != nil {
		t.Fatalf("write jobs store: %v", err)
	}
	if err := os.WriteFile(
		firstRunFile,
		[]byte(
			`{"ts":1000,"job_id":"job-1","status":"error"}`+"\n"+
				`{"ts":2000,"job_id":"job-1","status":"success"}`+"\n"+
				`{"ts":3000,"job_id":"job-1","status":"failed"}`+"\n",
		),
		0o644,
	); err != nil {
		t.Fatalf("write first run log: %v", err)
	}
	if err := os.WriteFile(
		secondRunFile,
		[]byte(
			`{"ts":4000,"job_id":"job-2","status":"failed"}`+"\n"+
				`{"ts":5000,"job_id":"job-2","status":"failed"}`+"\n",
		),
		0o644,
	); err != nil {
		t.Fatalf("write second run log: %v", err)
	}

	service := &OpenClawCronService{}
	summary, err := service.GetSummary()
	if err != nil {
		t.Fatalf("GetSummary returned error: %v", err)
	}
	if summary == nil {
		t.Fatalf("expected summary to be returned")
	}
	if summary.Total != 2 {
		t.Fatalf("expected total 2, got %d", summary.Total)
	}
	if summary.Failed != 1 {
		t.Fatalf("expected failed job count 1, got %d", summary.Failed)
	}
	if summary.FailedRuns != 4 {
		t.Fatalf("expected failed run count 4, got %d", summary.FailedRuns)
	}
}

func TestListJobs_PrefersLatestRunLogStatusOverStoreSnapshot(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("USERPROFILE", tempHome)
	t.Setenv("HOMEDRIVE", "")
	t.Setenv("HOMEPATH", "")

	rootDir := filepath.Join(tempHome, ".chatclaw", "openclaw")
	jobsFile := filepath.Join(rootDir, "cron", "jobs.json")
	runFile := filepath.Join(rootDir, "cron", "runs", "job-1.jsonl")
	if err := os.MkdirAll(filepath.Dir(runFile), 0o755); err != nil {
		t.Fatalf("create run dir: %v", err)
	}

	jobsPayload := `{
		"jobs":[
			{"id":"job-1","name":"job-1","enabled":true,"state":{"lastRunAtMs":1000,"lastStatus":"success","lastError":""}}
		]
	}`
	if err := os.WriteFile(jobsFile, []byte(jobsPayload), 0o644); err != nil {
		t.Fatalf("write jobs store: %v", err)
	}
	runPayload := "" +
		`{"ts":2000,"jobId":"job-1","status":"success","runAtMs":2000,"sessionId":"session-old"}` + "\n" +
		`{"ts":3000,"jobId":"job-1","status":"failed","error":"gateway timeout","runAtMs":3000,"sessionId":"session-new"}` + "\n"
	if err := os.WriteFile(runFile, []byte(runPayload), 0o644); err != nil {
		t.Fatalf("write run log: %v", err)
	}

	service := &OpenClawCronService{}
	items, err := service.ListJobs()
	if err != nil {
		t.Fatalf("ListJobs returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 job, got %d", len(items))
	}
	if items[0].LastRunAtMs != 3000 {
		t.Fatalf("expected latest run timestamp 3000 from run log, got %d", items[0].LastRunAtMs)
	}
	if items[0].LastStatus != "failed" {
		t.Fatalf("expected latest run status failed from run log, got %q", items[0].LastStatus)
	}
	if items[0].LastError != "gateway timeout" {
		t.Fatalf("expected latest run error from run log, got %q", items[0].LastError)
	}
}

func TestListJobs_UsesStoreSnapshotWhenNoRunLogExists(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("USERPROFILE", tempHome)
	t.Setenv("HOMEDRIVE", "")
	t.Setenv("HOMEPATH", "")

	rootDir := filepath.Join(tempHome, ".chatclaw", "openclaw")
	jobsFile := filepath.Join(rootDir, "cron", "jobs.json")
	if err := os.MkdirAll(filepath.Dir(jobsFile), 0o755); err != nil {
		t.Fatalf("create cron dir: %v", err)
	}

	jobsPayload := `{
		"jobs":[
			{"id":"job-1","name":"job-1","enabled":true,"state":{"lastRunAtMs":1000,"lastStatus":"success","lastError":"stale snapshot error"}}
		]
	}`
	if err := os.WriteFile(jobsFile, []byte(jobsPayload), 0o644); err != nil {
		t.Fatalf("write jobs store: %v", err)
	}

	service := &OpenClawCronService{}
	items, err := service.ListJobs()
	if err != nil {
		t.Fatalf("ListJobs returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 job, got %d", len(items))
	}
	if items[0].LastRunAtMs != 1000 {
		t.Fatalf("expected store snapshot timestamp 1000, got %d", items[0].LastRunAtMs)
	}
	if items[0].LastStatus != "success" {
		t.Fatalf("expected store snapshot status success, got %q", items[0].LastStatus)
	}
	if items[0].LastError != "stale snapshot error" {
		t.Fatalf("expected store snapshot error to remain, got %q", items[0].LastError)
	}
}

func TestEmitConversationChanged_EmitsConversationRefreshEvent(t *testing.T) {
	const (
		testOpenClawCronConversationChangedEvent = "conversations:changed"
		testOpenClawCronAgentID                  = int64(136)
		testOpenClawCronEventWaitTimeout         = time.Second
	)

	app := application.New(application.Options{})
	service := &OpenClawCronService{app: app}

	eventCh := make(chan map[string]any, 1)
	unsubscribe := app.Event.On(testOpenClawCronConversationChangedEvent, func(event *application.CustomEvent) {
		payload, ok := event.Data.(map[string]any)
		if !ok {
			return
		}
		eventCh <- payload
	})
	defer unsubscribe()

	service.emitConversationChanged(testOpenClawCronAgentID)

	select {
	case payload := <-eventCh:
		agentID, ok := payload["agent_id"].(int64)
		if ok && agentID == testOpenClawCronAgentID {
			return
		}
		floatID, ok := payload["agent_id"].(float64)
		if !ok || int64(floatID) != testOpenClawCronAgentID {
			t.Fatalf("unexpected event payload: %#v", payload)
		}
	case <-time.After(testOpenClawCronEventWaitTimeout):
		t.Fatalf("expected %q event", testOpenClawCronConversationChangedEvent)
	}
}

func TestEmitConversationActivated_EmitsConversationSelectionEvent(t *testing.T) {
	const (
		testOpenClawCronConversationActivatedEvent = "channel:conversation-activated"
		testOpenClawCronAgentID                    = int64(136)
		testOpenClawCronConversationID             = int64(62)
		testOpenClawCronEventWaitTimeout           = time.Second
	)

	app := application.New(application.Options{})
	service := &OpenClawCronService{app: app}

	eventCh := make(chan map[string]any, 1)
	unsubscribe := app.Event.On(testOpenClawCronConversationActivatedEvent, func(event *application.CustomEvent) {
		payload, ok := event.Data.(map[string]any)
		if !ok {
			return
		}
		eventCh <- payload
	})
	defer unsubscribe()

	service.emitConversationActivated(testOpenClawCronAgentID, testOpenClawCronConversationID)

	select {
	case payload := <-eventCh:
		if payload["agent_type"] != "openclaw" {
			t.Fatalf("unexpected agent type payload: %#v", payload)
		}
		agentID, ok := payload["agent_id"].(int64)
		if !ok || agentID != testOpenClawCronAgentID {
			t.Fatalf("unexpected agent id payload: %#v", payload)
		}
		conversationID, ok := payload["conversation_id"].(int64)
		if !ok || conversationID != testOpenClawCronConversationID {
			t.Fatalf("unexpected conversation id payload: %#v", payload)
		}
	case <-time.After(testOpenClawCronEventWaitTimeout):
		t.Fatalf("expected %q event", testOpenClawCronConversationActivatedEvent)
	}
}

func TestBuildCronForwardEvents_MapsToolStartToChatToolCall(t *testing.T) {
	state := &cronForwardState{}
	events := buildCronForwardEvents(
		29,
		"agent:main:cron:job-1",
		"run-1",
		state,
		"agent",
		json.RawMessage(`{"runId":"run-1","sessionKey":"agent:main:cron:job-1","stream":"tool","data":{"phase":"start","toolCallId":"call-1","name":"weather","args":{"city":"Shanghai"}}}`),
	)

	if len(events) != 2 {
		t.Fatalf("expected start + tool events, got %d", len(events))
	}
	if events[1].Name != chatservice.EventChatTool {
		t.Fatalf("expected chat:tool event, got %q", events[1].Name)
	}
	toolEvent, ok := events[1].Payload.(chatservice.ChatToolEvent)
	if !ok {
		t.Fatalf("expected tool payload type, got %T", events[1].Payload)
	}
	if toolEvent.Type != "call" {
		t.Fatalf("expected tool call type, got %q", toolEvent.Type)
	}
	if toolEvent.ToolCallID != "call-1" {
		t.Fatalf("expected tool call id call-1, got %q", toolEvent.ToolCallID)
	}
	if toolEvent.ToolName != "weather" {
		t.Fatalf("expected tool name weather, got %q", toolEvent.ToolName)
	}
	if toolEvent.ArgsJSON != `{"city":"Shanghai"}` {
		t.Fatalf("expected args json to be preserved, got %q", toolEvent.ArgsJSON)
	}
}

func TestBuildCronForwardEvents_MapsToolResultToChatToolResult(t *testing.T) {
	state := &cronForwardState{Started: true, RequestID: "req-1", MessageID: -1}
	events := buildCronForwardEvents(
		29,
		"agent:main:cron:job-1",
		"run-1",
		state,
		"agent",
		json.RawMessage(`{"runId":"run-1","sessionKey":"agent:main:cron:job-1","stream":"tool","data":{"phase":"result","toolCallId":"call-1","name":"weather","result":{"temp":12}}}`),
	)

	if len(events) != 1 {
		t.Fatalf("expected tool result event only, got %d", len(events))
	}
	toolEvent, ok := events[0].Payload.(chatservice.ChatToolEvent)
	if !ok {
		t.Fatalf("expected tool payload type, got %T", events[0].Payload)
	}
	if toolEvent.Type != "result" {
		t.Fatalf("expected tool result type, got %q", toolEvent.Type)
	}
	if toolEvent.ResultJSON != `{"temp":12}` {
		t.Fatalf("expected result json to be preserved, got %q", toolEvent.ResultJSON)
	}
}

func TestBuildCronForwardEvents_MapsRetrievalItems(t *testing.T) {
	state := &cronForwardState{}
	events := buildCronForwardEvents(
		29,
		"agent:main:cron:job-1",
		"run-1",
		state,
		"agent",
		json.RawMessage(`{"runId":"run-1","sessionKey":"agent:main:cron:job-1","stream":"retrieval","data":{"items":[{"source":"memory","content":"memo","score":0.9},{"source":"knowledge","text":"doc","score":0.7}]}}`),
	)

	if len(events) != 2 {
		t.Fatalf("expected start + retrieval events, got %d", len(events))
	}
	if events[1].Name != chatservice.EventChatRetrieval {
		t.Fatalf("expected chat:retrieval event, got %q", events[1].Name)
	}
	retrievalEvent, ok := events[1].Payload.(chatservice.ChatRetrievalEvent)
	if !ok {
		t.Fatalf("expected retrieval payload type, got %T", events[1].Payload)
	}
	if len(retrievalEvent.Items) != 2 {
		t.Fatalf("expected 2 retrieval items, got %d", len(retrievalEvent.Items))
	}
	if retrievalEvent.Items[0].Source != "memory" || retrievalEvent.Items[0].Content != "memo" {
		t.Fatalf("expected first retrieval item to preserve memory content, got %+v", retrievalEvent.Items[0])
	}
	if retrievalEvent.Items[1].Source != "knowledge" || retrievalEvent.Items[1].Content != "doc" {
		t.Fatalf("expected second retrieval item to normalize text into content, got %+v", retrievalEvent.Items[1])
	}
}

func TestNormalizeHistoryTriggerType(t *testing.T) {
	tests := []struct {
		name   string
		action string
		source string
		want   string
	}{
		{name: "manual action wins", action: "manual", source: OpenClawCronHistorySourceRunLog, want: "manual"},
		{name: "run log defaults to schedule", action: "", source: OpenClawCronHistorySourceRunLog, want: "schedule"},
		{name: "pending defaults to manual", action: "", source: OpenClawCronHistorySourcePending, want: "manual"},
	}

	for _, tc := range tests {
		if got := normalizeHistoryTriggerType(tc.action, tc.source); got != tc.want {
			t.Fatalf("%s: expected %q, got %q", tc.name, tc.want, got)
		}
	}
}

func TestBuildCLIArgs_AppendsGatewayFlagsToSubcommand(t *testing.T) {
	args := buildCLIArgs([]string{"cron", "run", "job-1"}, "ws://127.0.0.1:39960/ws", "token-1", true)
	expected := []string{
		"cron", "run", "job-1",
		"--url", "ws://127.0.0.1:39960/ws",
		"--token", "token-1",
	}
	if len(args) != len(expected) {
		t.Fatalf("expected %d args, got %d: %#v", len(expected), len(args), args)
	}
	for i := range expected {
		if args[i] != expected[i] {
			t.Fatalf("expected arg[%d] = %q, got %q", i, expected[i], args[i])
		}
	}
}

func TestSplitCronModelSelection(t *testing.T) {
	providerID, modelID := splitCronModelSelection("openai/gpt-5")
	if providerID != "openai" || modelID != "gpt-5" {
		t.Fatalf("expected openai/gpt-5, got %q / %q", providerID, modelID)
	}

	providerID, modelID = splitCronModelSelection("gpt-5")
	if providerID != "" || modelID != "gpt-5" {
		t.Fatalf("expected alias-only model, got %q / %q", providerID, modelID)
	}
}

func TestNormalizeCronThinking(t *testing.T) {
	if normalizeCronThinking("off") {
		t.Fatalf("expected off to disable thinking")
	}
	if !normalizeCronThinking("medium") {
		t.Fatalf("expected medium to enable thinking")
	}
}

func TestBuildCreateJobPayload_MapsAgentTurnFields(t *testing.T) {
	payload, err := buildCreateJobPayload(CreateOpenClawCronJobInput{
		Name:              "job",
		Description:       "desc",
		AgentID:           "agent-1",
		ScheduleKind:      "every",
		Every:             "5m",
		Message:           "hello",
		Model:             "gpt-5",
		Thinking:          "high",
		ExpectFinal:       true,
		LightContext:      true,
		TimeoutMs:         1500,
		SessionTarget:     "isolated",
		SessionKey:        "agent:main",
		WakeMode:          "now",
		Announce:          true,
		DeliveryChannel:   "telegram",
		DeliveryTo:        "123",
		DeliveryAccountID: "acct-1",
		BestEffortDeliver: true,
		KeepAfterRun:      true,
		Enabled:           true,
	})
	if err != nil {
		t.Fatalf("buildCreateJobPayload returned error: %v", err)
	}

	schedule := payload["schedule"].(map[string]any)
	if schedule["kind"] != "every" || schedule["everyMs"] != int64(5*time.Minute/time.Millisecond) {
		t.Fatalf("unexpected schedule payload: %#v", schedule)
	}
	runPayload := payload["payload"].(map[string]any)
	if runPayload["kind"] != "agentTurn" || runPayload["message"] != "hello" {
		t.Fatalf("unexpected run payload: %#v", runPayload)
	}
	if runPayload["timeoutSeconds"] != int64(2) {
		t.Fatalf("expected timeoutSeconds 2, got %#v", runPayload["timeoutSeconds"])
	}
	delivery := payload["delivery"].(map[string]any)
	if delivery["mode"] != "announce" || delivery["bestEffort"] != true {
		t.Fatalf("unexpected delivery payload: %#v", delivery)
	}
	if payload["deleteAfterRun"] != false {
		t.Fatalf("expected keep-after-run to map deleteAfterRun=false, got %#v", payload["deleteAfterRun"])
	}
}

func TestBuildUpdateJobPatch_MapsDisableAndClearFlags(t *testing.T) {
	enabled := false
	announce := false
	timeoutMs := int64(999)
	sessionKey := "  "
	patch, err := buildUpdateJobPatch(UpdateOpenClawCronJobInput{
		Enabled:         &enabled,
		Announce:        &announce,
		TimeoutMs:       &timeoutMs,
		ClearAgent:      true,
		ClearSessionKey: true,
		SessionKey:      &sessionKey,
		KeepAfterRun:    &[]bool{true}[0],
	})
	if err != nil {
		t.Fatalf("buildUpdateJobPatch returned error: %v", err)
	}
	if patch["enabled"] != false {
		t.Fatalf("expected enabled=false, got %#v", patch["enabled"])
	}
	if patch["agentId"] != nil {
		t.Fatalf("expected agentId=nil, got %#v", patch["agentId"])
	}
	if patch["sessionKey"] != nil {
		t.Fatalf("expected sessionKey=nil, got %#v", patch["sessionKey"])
	}
	runPayload := patch["payload"].(map[string]any)
	if runPayload["timeoutSeconds"] != int64(1) {
		t.Fatalf("expected timeoutSeconds 1, got %#v", runPayload["timeoutSeconds"])
	}
	delivery := patch["delivery"].(map[string]any)
	if delivery["mode"] != "none" {
		t.Fatalf("expected delivery mode none, got %#v", delivery["mode"])
	}
	if patch["deleteAfterRun"] != false {
		t.Fatalf("expected keep-after-run to map deleteAfterRun=false, got %#v", patch["deleteAfterRun"])
	}
}

func TestFlattenJob_ReadsBestEffortFromPersistedDelivery(t *testing.T) {
	raw := []byte(`{
		"id":"job-1",
		"name":"job",
		"delivery":{
			"mode":"announce",
			"channel":"feishu",
			"to":"oc_xxx",
			"bestEffort":true
		}
	}`)
	var item openClawJobStoreItem
	if err := json.Unmarshal(raw, &item); err != nil {
		t.Fatalf("expected store item json to unmarshal: %v", err)
	}

	job := flattenJob(item)
	if !job.BestEffortDeliver {
		t.Fatalf("expected persisted delivery.bestEffort to map back to BestEffortDeliver")
	}
}

func TestDeriveCronDeliveryAccountID(t *testing.T) {
	tests := []struct {
		name   string
		option cronDeliveryChannelOption
		want   string
	}{
		{
			name: "uses explicit openclaw channel id for any platform",
			option: cronDeliveryChannelOption{
				ID:          7,
				Platform:    "dingtalk",
				ExtraConfig: `{"openclaw_channel_id":"shared_account"}`,
			},
			want: "shared_account",
		},
		{
			name: "falls back to channel id when extra config has no account key",
			option: cronDeliveryChannelOption{
				ID:       9,
				Platform: "wecom",
			},
			want: "channel_9",
		},
		{
			name: "returns empty when channel id is invalid",
			option: cronDeliveryChannelOption{
				ID:       0,
				Platform: "qq",
			},
			want: "",
		},
	}

	for _, tc := range tests {
		if got := deriveCronDeliveryAccountID(tc.option); got != tc.want {
			t.Fatalf("%s: expected %q, got %q", tc.name, tc.want, got)
		}
	}
}

func TestResolveCronDeliveryChannelOption(t *testing.T) {
	tests := []struct {
		name        string
		agentRowID  int64
		platform    string
		channels    []cronDeliveryChannelOption
		wantChannel cronDeliveryChannelOption
		wantOK      bool
	}{
		{
			name:       "matches bound channel by agent and platform",
			agentRowID: 11,
			platform:   "feishu",
			channels: []cronDeliveryChannelOption{
				{ID: 1, AgentRowID: 10, Platform: "feishu", LastSenderID: "ou_old"},
				{ID: 2, AgentRowID: 11, Platform: "feishu", LastSenderID: "ou_latest"},
			},
			wantChannel: cronDeliveryChannelOption{ID: 2, AgentRowID: 11, Platform: "feishu", LastSenderID: "ou_latest"},
			wantOK:      true,
		},
		{
			name:       "rejects multiple bound channels for same agent and platform",
			agentRowID: 11,
			platform:   "feishu",
			channels: []cronDeliveryChannelOption{
				{ID: 2, AgentRowID: 11, Platform: "feishu", LastSenderID: "ou_a"},
				{ID: 3, AgentRowID: 11, Platform: "feishu", LastSenderID: "ou_b"},
			},
			wantOK: false,
		},
		{
			name:       "rejects missing bound channel",
			agentRowID: 11,
			platform:   "wecom",
			channels: []cronDeliveryChannelOption{
				{ID: 2, AgentRowID: 11, Platform: "feishu", LastSenderID: "ou_a"},
			},
			wantOK: false,
		},
	}

	for _, tc := range tests {
		got, err := resolveCronDeliveryChannelOption(tc.agentRowID, tc.platform, tc.channels)
		if tc.wantOK {
			if err != nil {
				t.Fatalf("%s: unexpected error: %v", tc.name, err)
			}
			if got.ID != tc.wantChannel.ID || got.LastSenderID != tc.wantChannel.LastSenderID {
				t.Fatalf("%s: expected %#v, got %#v", tc.name, tc.wantChannel, got)
			}
			continue
		}
		if err == nil {
			t.Fatalf("%s: expected error, got nil", tc.name)
		}
	}
}

func TestResolveCronDeliveryTargetID(t *testing.T) {
	tests := []struct {
		name         string
		explicit     string
		channel      cronDeliveryChannelOption
		wantTargetID string
		wantErr      bool
	}{
		{
			name:     "uses explicit target when provided",
			explicit: "ou_manual",
			channel: cronDeliveryChannelOption{
				LastSenderID: "ou_last",
			},
			wantTargetID: "ou_manual",
		},
		{
			name: "falls back to channel last sender id",
			channel: cronDeliveryChannelOption{
				LastSenderID: "ou_last",
			},
			wantTargetID: "ou_last",
		},
		{
			name: "rejects missing explicit target and last sender id",
			channel: cronDeliveryChannelOption{
				Platform: "feishu",
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		got, err := resolveCronDeliveryTargetID(tc.explicit, tc.channel)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("%s: expected error, got nil", tc.name)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", tc.name, err)
		}
		if got != tc.wantTargetID {
			t.Fatalf("%s: expected %q, got %q", tc.name, tc.wantTargetID, got)
		}
	}
}

func TestBuildUpdateJobPatch_PrefersExplicitDeliveryModeOverLegacyAnnounceFlag(t *testing.T) {
	deliveryMode := "announce"
	announce := false
	channel := "feishu"
	targetID := "oc_target_1"
	patch, err := buildUpdateJobPatch(UpdateOpenClawCronJobInput{
		DeliveryMode:    &deliveryMode,
		Announce:        &announce,
		DeliveryChannel: &channel,
		DeliveryTo:      &targetID,
	})
	if err != nil {
		t.Fatalf("buildUpdateJobPatch returned error: %v", err)
	}

	delivery := patch["delivery"].(map[string]any)
	if delivery["mode"] != "announce" {
		t.Fatalf("expected explicit delivery mode announce to win, got %#v", delivery["mode"])
	}
	if delivery["channel"] != "feishu" {
		t.Fatalf("expected delivery channel feishu, got %#v", delivery["channel"])
	}
	if delivery["to"] != "oc_target_1" {
		t.Fatalf("expected delivery target preserved, got %#v", delivery["to"])
	}
}

func TestExtractLatestDeliveryTarget_ReturnsResolvedTargetID(t *testing.T) {
	channel, targetID, accountID := "feishu", "ou_latest_sender", "channel_7"
	if got := extractLatestDeliveryTarget(channel, targetID, accountID); got != targetID {
		t.Fatalf("expected latest delivery target %q, got %q", targetID, got)
	}
}
