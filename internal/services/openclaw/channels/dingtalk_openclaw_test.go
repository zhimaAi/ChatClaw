package openclawchannels

import "testing"

func TestContainsDingTalkPluginMarker(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want bool
	}{
		{
			name: "matches package marker",
			out:  "installed plugin: @dingtalk-real-ai/dingtalk-connector",
			want: true,
		},
		{
			name: "matches channel id",
			out:  "channel dingtalk-connector registered",
			want: true,
		},
		{
			name: "rejects unrelated output",
			out:  "installed plugin: @example/other-plugin",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := containsDingTalkPluginMarker(tt.out); got != tt.want {
				t.Fatalf("containsDingTalkPluginMarker(%q) = %v, want %v", tt.out, got, tt.want)
			}
		})
	}
}
