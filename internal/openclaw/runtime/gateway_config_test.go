package openclawruntime

import (
	"context"
	"encoding/json"
	"testing"
)

func TestSessionMemoryHookSection(t *testing.T) {
	t.Parallel()

	section, err := SessionMemoryHookSection()(context.Background())
	if err != nil {
		t.Fatalf("SessionMemoryHookSection() error = %v", err)
	}

	raw, err := json.Marshal(section)
	if err != nil {
		t.Fatalf("marshal section: %v", err)
	}

	got := string(raw)
	want := `{"hooks":{"internal":{"enabled":true,"entries":{"session-memory":{"enabled":true}}}}}`
	if got != want {
		t.Fatalf("unexpected hook config:\n got: %s\nwant: %s", got, want)
	}
}
