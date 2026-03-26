package conversations

import "testing"

func TestNormalizeConversationSource_DefaultsToManual(t *testing.T) {
	got := NormalizeConversationSource("")
	if got != ConversationSourceManual {
		t.Fatalf("expected default conversation source %q, got %q", ConversationSourceManual, got)
	}
}

func TestNormalizeConversationSource_RecognizesOpenClawCron(t *testing.T) {
	got := NormalizeConversationSource(" openclaw_cron ")
	if got != ConversationSourceOpenClawCron {
		t.Fatalf("expected %q, got %q", ConversationSourceOpenClawCron, got)
	}
}

func TestNormalizeConversationSource_RecognizesOpenClawCronJobScopedSource(t *testing.T) {
	got := NormalizeConversationSource(" openclaw_cron:job-123 ")
	if got != "openclaw_cron:job-123" {
		t.Fatalf("expected scoped cron source to be preserved, got %q", got)
	}
}
