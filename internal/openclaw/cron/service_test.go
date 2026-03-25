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

func assertContains(t *testing.T, items []string, target string) {
	t.Helper()
	for _, item := range items {
		if item == target {
			return
		}
	}
	t.Fatalf("expected %q in %v", target, items)
}
