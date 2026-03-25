package openclawcron

import "testing"

func TestBuildCreateArgs_UsesNativeScheduleKinds(t *testing.T) {
	args, err := buildCreateArgs(CreateOpenClawCronJobInput{
		Name:         "job",
		ScheduleKind: "cron",
		CronExpr:     "*/5 * * * *",
		Message:      "hello",
		Enabled:      true,
	})
	if err != nil {
		t.Fatalf("buildCreateArgs returned error: %v", err)
	}
	assertContains(t, args, "--cron")
	assertContains(t, args, "*/5 * * * *")
	assertContains(t, args, "--message")
	assertContains(t, args, "hello")
}

func TestBuildUpdateArgs_RespectsPatchSemantics(t *testing.T) {
	name := "renamed"
	enabled := false
	args, err := buildUpdateArgs("job-1", UpdateOpenClawCronJobInput{
		Name:    &name,
		Enabled: &enabled,
	})
	if err != nil {
		t.Fatalf("buildUpdateArgs returned error: %v", err)
	}
	assertContains(t, args, "cron")
	assertContains(t, args, "edit")
	assertContains(t, args, "job-1")
	assertContains(t, args, "--name")
	assertContains(t, args, "renamed")
	assertContains(t, args, "--disable")
}

func TestParseAgentIDFromSessionKey(t *testing.T) {
	got := parseAgentIDFromSessionKey("agent:main:cron:job:run:session")
	if got != "main" {
		t.Fatalf("expected main, got %q", got)
	}
}

func TestBuildCLIArgs_OnlyAddsJSONWhenRequested(t *testing.T) {
	args := buildCLIArgs([]string{"cron", "status"}, "ws://127.0.0.1:9527/ws", "token-1", true)
	assertContains(t, args, "--json")
	assertContains(t, args, "--url")
	assertContains(t, args, "ws://127.0.0.1:9527/ws")
	assertContains(t, args, "--token")
	assertContains(t, args, "token-1")

	withoutJSON := buildCLIArgs([]string{"cron", "edit", "job-1"}, "ws://127.0.0.1:9527/ws", "token-1", false)
	assertNotContains(t, withoutJSON, "--json")
}

func TestBuildRunNowArgs_UsesNativeCronRun(t *testing.T) {
	args, err := buildRunNowArgs("job-1")
	if err != nil {
		t.Fatalf("buildRunNowArgs returned error: %v", err)
	}
	assertContains(t, args, "cron")
	assertContains(t, args, "run")
	assertContains(t, args, "job-1")
	assertOrderedSequence(t, args, []string{"cron", "run", "job-1"})
	assertNotContains(t, args, "--json")
	assertNotContains(t, args, "--id")
}

func TestIsBenignCronRunOutput_RecognizesAlreadyRunning(t *testing.T) {
	output := []byte(`{"ok":true,"ran":false,"reason":"already-running"}`)
	if !isBenignCronRunOutput(output) {
		t.Fatalf("expected already-running output to be treated as benign")
	}

	notBenign := []byte(`{"ok":false,"ran":false,"reason":"permission-denied"}`)
	if isBenignCronRunOutput(notBenign) {
		t.Fatalf("did not expect permission-denied output to be treated as benign")
	}
}

func assertContains(t *testing.T, items []string, target string) {
	t.Helper()
	for _, item := range items {
		if item == target {
			return
		}
	}
	t.Fatalf("expected %q in %v", target, items)
}

func assertNotContains(t *testing.T, items []string, target string) {
	t.Helper()
	for _, item := range items {
		if item == target {
			t.Fatalf("did not expect %q in %v", target, items)
		}
	}
}

func assertOrderedSequence(t *testing.T, items []string, seq []string) {
	t.Helper()
	if len(seq) == 0 {
		return
	}
	matchIndex := 0
	for _, item := range items {
		if item == seq[matchIndex] {
			matchIndex++
			if matchIndex == len(seq) {
				return
			}
		}
	}
	t.Fatalf("expected ordered sequence %v in %v", seq, items)
}
