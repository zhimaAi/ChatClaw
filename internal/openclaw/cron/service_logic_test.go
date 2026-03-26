package openclawcron

import (
	"encoding/json"
	"testing"
	"time"
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

func TestMergeHistoryItems_PrefersRunLogAndRetainsConversation(t *testing.T) {
	runItems := []OpenClawCronHistoryListItem{{
		JobID:          "job-1",
		RunID:          "run-1",
		SessionID:      "session-1",
		SessionKey:     "agent:main:cron:job-1",
		Status:         "ok",
		RunAtMs:        200,
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
	if merged[0].Source != OpenClawCronHistorySourceRunLog {
		t.Fatalf("expected run log item to win, got %q", merged[0].Source)
	}
	if merged[0].SessionID != "session-1" {
		t.Fatalf("expected session id from run log, got %q", merged[0].SessionID)
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

func TestIsPendingRunAwaitingBinding(t *testing.T) {
	tests := []struct {
		name    string
		pending *pendingCronRun
		want    bool
	}{
		{
			name: "waits before session is correlated",
			pending: &pendingCronRun{
				RunID: "run-1",
			},
			want: true,
		},
		{
			name: "stops waiting after session key is known",
			pending: &pendingCronRun{
				RunID:      "run-1",
				SessionKey: "agent:main:cron:job-1:run:session-1",
			},
			want: false,
		},
		{
			name: "stops waiting after local conversation is created",
			pending: &pendingCronRun{
				RunID:          "run-1",
				ConversationID: 22,
			},
			want: false,
		},
	}

	for _, tc := range tests {
		if got := isPendingRunAwaitingBinding(tc.pending); got != tc.want {
			t.Fatalf("%s: expected %v, got %v", tc.name, tc.want, got)
		}
	}
}

func TestShouldRunManualCronViaChat(t *testing.T) {
	tests := []struct {
		name string
		job  OpenClawCronJob
		want bool
	}{
		{
			name: "message agent turn uses chat pipeline",
			job: OpenClawCronJob{
				PayloadKind: openClawCronPayloadAgentTurn,
				Message:     "hello",
			},
			want: true,
		},
		{
			name: "system event keeps native cron path",
			job: OpenClawCronJob{
				PayloadKind: openClawCronPayloadSystemEvent,
				Message:     "hello",
			},
			want: false,
		},
		{
			name: "empty message keeps native cron path",
			job: OpenClawCronJob{
				PayloadKind: openClawCronPayloadAgentTurn,
				Message:     "",
			},
			want: false,
		},
	}

	for _, tc := range tests {
		if got := shouldRunManualCronViaChat(tc.job); got != tc.want {
			t.Fatalf("%s: expected %v, got %v", tc.name, tc.want, got)
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

func TestHasMatchingRunHistory_MatchesNearbyRunLogTimestamp(t *testing.T) {
	pending := &pendingCronRun{
		RunID:       "manual:job-1:1",
		TriggerAtMs: 1774496122760,
	}
	runEntries := []OpenClawCronRunEntry{
		{
			RunAtMs: 1774496122705,
		},
	}
	if !hasMatchingRunHistory(pending, runEntries, nil) {
		t.Fatalf("expected nearby run log to reconcile pending item")
	}
}

func TestHasMatchingRunHistory_DoesNotMatchFarTimestamp(t *testing.T) {
	pending := &pendingCronRun{
		RunID:       "manual:job-1:1",
		TriggerAtMs: 1774496122760,
	}
	runEntries := []OpenClawCronRunEntry{
		{
			RunAtMs: 1774496022705,
		},
	}
	if hasMatchingRunHistory(pending, runEntries, nil) {
		t.Fatalf("did not expect distant run log to reconcile pending item")
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
