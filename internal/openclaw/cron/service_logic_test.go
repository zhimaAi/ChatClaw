package openclawcron

import (
	"encoding/json"
	"testing"
	"time"

	chatservice "chatclaw/internal/services/chat"
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

func TestMergeHistoryItems_PrefersConversationOverRunLog(t *testing.T) {
	runItems := []OpenClawCronHistoryListItem{{
		JobID:          "job-1",
		RunID:          "run-1",
		SessionID:      "session-1",
		SessionKey:     "agent:main:cron:job-1",
		Status:         "ok",
		RunAtMs:        200,
		DurationMs:     2500,
		TriggerType:    "manual",
		Source:         OpenClawCronHistorySourceRunLog,
		IsPendingLocal: false,
	}}
	conversationItems := []OpenClawCronHistoryListItem{{
		JobID:          "job-1",
		SessionKey:     "agent:main:cron:job-1",
		ConversationID: 42,
		Name:           "job-1 / 2026-03-26 12:00:00",
		Status:         "running",
		RunAtMs:        100,
		Source:         OpenClawCronHistorySourceConversation,
	}}

	merged := mergeHistoryItems(runItems, conversationItems, nil)
	if len(merged) != 1 {
		t.Fatalf("expected 1 merged item, got %d", len(merged))
	}
	if merged[0].ConversationID != 42 {
		t.Fatalf("expected conversation id to be retained, got %d", merged[0].ConversationID)
	}
	if merged[0].Source != OpenClawCronHistorySourceConversation {
		t.Fatalf("expected conversation item to win, got %q", merged[0].Source)
	}
	if merged[0].SessionID != "session-1" {
		t.Fatalf("expected session id from run log to be merged in, got %q", merged[0].SessionID)
	}
	if merged[0].DurationMs != 2500 {
		t.Fatalf("expected duration from run log to be retained, got %d", merged[0].DurationMs)
	}
	if merged[0].TriggerType != "manual" {
		t.Fatalf("expected trigger type from conversation to be retained, got %q", merged[0].TriggerType)
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
		{name: "conversation defaults to manual", action: "", source: OpenClawCronHistorySourceConversation, want: "manual"},
	}

	for _, tc := range tests {
		if got := normalizeHistoryTriggerType(tc.action, tc.source); got != tc.want {
			t.Fatalf("%s: expected %q, got %q", tc.name, tc.want, got)
		}
	}
}

func TestEnrichConversationHistoryItemsWithRunEntries_FillsDurationForManualConversation(t *testing.T) {
	conversationItems := []OpenClawCronHistoryListItem{{
		JobID:          "job-1",
		ConversationID: 42,
		RunAtMs:        1774496838251,
		Status:         "running",
		TriggerType:    "manual",
	}}
	runEntries := []OpenClawCronRunEntry{{
		RunAtMs:    1774496838251,
		DurationMs: 35506,
		SessionKey: "agent:main:cron:job-1:run:session-1",
		SessionID:  "session-1",
		Status:     "ok",
		Action:     "finished",
	}}

	enrichConversationHistoryItemsWithRunEntries(conversationItems, runEntries)

	if conversationItems[0].DurationMs != 35506 {
		t.Fatalf("expected duration to be backfilled, got %d", conversationItems[0].DurationMs)
	}
	if conversationItems[0].SessionKey != "agent:main:cron:job-1:run:session-1" {
		t.Fatalf("expected session key to be backfilled, got %q", conversationItems[0].SessionKey)
	}
	if conversationItems[0].SessionID != "session-1" {
		t.Fatalf("expected session id to be backfilled, got %q", conversationItems[0].SessionID)
	}
}

func TestEnrichConversationHistoryItemsWithRunEntries_UpdatesStatusWhenConversationAlreadyHasSessionAndDuration(t *testing.T) {
	conversationItems := []OpenClawCronHistoryListItem{{
		JobID:          "job-1",
		ConversationID: 42,
		RunAtMs:        1774496838251,
		SessionKey:     "agent:main:cron:job-1:run:session-1",
		SessionID:      "session-1",
		Status:         "running",
		DurationMs:     35506,
		TriggerType:    "manual",
	}}
	runEntries := []OpenClawCronRunEntry{{
		RunAtMs:    1774496838251,
		DurationMs: 35506,
		SessionKey: "agent:main:cron:job-1:run:session-1",
		SessionID:  "session-1",
		Status:     "ok",
		Action:     "finished",
	}}

	enrichConversationHistoryItemsWithRunEntries(conversationItems, runEntries)

	if conversationItems[0].Status != "ok" {
		t.Fatalf("expected status to be updated from run log, got %q", conversationItems[0].Status)
	}
}

func TestDeriveManualConversationHistoryState(t *testing.T) {
	tests := []struct {
		name           string
		runAtMs        int64
		updatedAtMs    int64
		sessionKey     string
		active         bool
		wantStatus     string
		wantDurationMs int64
	}{
		{
			name:           "active manual run stays running",
			runAtMs:        1000,
			updatedAtMs:    5000,
			sessionKey:     "agent:main:conv_1",
			active:         true,
			wantStatus:     "running",
			wantDurationMs: 0,
		},
		{
			name:           "completed manual run shows success and duration",
			runAtMs:        1000,
			updatedAtMs:    5600,
			sessionKey:     "agent:main:conv_1",
			active:         false,
			wantStatus:     "success",
			wantDurationMs: 4600,
		},
		{
			name:           "missing session key keeps pending state",
			runAtMs:        1000,
			updatedAtMs:    5600,
			sessionKey:     "",
			active:         false,
			wantStatus:     "success",
			wantDurationMs: 4600,
		},
	}

	for _, tc := range tests {
		status, durationMs := deriveManualConversationHistoryState(
			tc.runAtMs,
			tc.updatedAtMs,
			tc.sessionKey,
			tc.active,
		)
		if status != tc.wantStatus {
			t.Fatalf("%s: expected status %q, got %q", tc.name, tc.wantStatus, status)
		}
		if durationMs != tc.wantDurationMs {
			t.Fatalf("%s: expected duration %d, got %d", tc.name, tc.wantDurationMs, durationMs)
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

func TestBuildCronConversationSource(t *testing.T) {
	got := buildCronConversationSource(" job-123 ")
	if got != "openclaw_cron:job-123" {
		t.Fatalf("expected scoped cron conversation source, got %q", got)
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
