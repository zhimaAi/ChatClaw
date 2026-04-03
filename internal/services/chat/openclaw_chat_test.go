package chat

import (
	"reflect"
	"testing"

	"chatclaw/internal/define"
	"chatclaw/internal/services/channels"
)

func TestOpenClawSessionCandidateAgentIDs(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "empty falls back to main only",
			input: "",
			want:  []string{define.OpenClawMainAgentID},
		},
		{
			name:  "main stays single",
			input: define.OpenClawMainAgentID,
			want:  []string{define.OpenClawMainAgentID},
		},
		{
			name:  "custom agent includes main fallback",
			input: "assistant-a",
			want:  []string{"assistant-a", define.OpenClawMainAgentID},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := openClawSessionCandidateAgentIDs(tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("openClawSessionCandidateAgentIDs(%q) = %#v, want %#v", tt.input, got, tt.want)
			}
		})
	}
}

func TestAppendOpenClawChannelSessionCandidatesWhatsappDMIncludesMainFallback(t *testing.T) {
	got := appendOpenClawChannelSessionCandidates(
		nil,
		[]string{"assistant-a", define.OpenClawMainAgentID},
		channels.PlatformWhatsapp,
		channels.ChannelConversationScopeDM,
		"+8613545220341",
	)

	want := []string{
		"agent:assistant-a:whatsapp:dm:+8613545220341",
		"agent:assistant-a:whatsapp:direct:+8613545220341",
		"agent:main:whatsapp:dm:+8613545220341",
		"agent:main:whatsapp:direct:+8613545220341",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("appendOpenClawChannelSessionCandidates(...) = %#v, want %#v", got, want)
	}
}
