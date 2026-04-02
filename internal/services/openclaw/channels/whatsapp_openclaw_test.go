package openclawchannels

import (
	"testing"
	"time"
)

func TestEnsureWhatsappAccountConfigEntry(t *testing.T) {
	tests := []struct {
		name    string
		entry   map[string]any
		changed bool
	}{
		{
			name:    "creates required flags for empty entry",
			entry:   nil,
			changed: true,
		},
		{
			name: "adds self chat mode for legacy enabled entry",
			entry: map[string]any{
				"enabled": true,
			},
			changed: true,
		},
		{
			name: "fixes disabled self chat mode",
			entry: map[string]any{
				"enabled":      true,
				"selfChatMode": false,
			},
			changed: true,
		},
		{
			name: "no change when both flags already enabled",
			entry: map[string]any{
				"enabled":      true,
				"selfChatMode": true,
				"agentId":      "main",
			},
			changed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed := ensureWhatsappAccountConfigEntry(tt.entry)
			if changed != tt.changed {
				t.Fatalf("ensureWhatsappAccountConfigEntry(%v) changed = %v, want %v", tt.entry, changed, tt.changed)
			}
			if enabled, _ := got["enabled"].(bool); !enabled {
				t.Fatalf("enabled = %#v, want true", got["enabled"])
			}
			if selfChatMode, _ := got["selfChatMode"].(bool); !selfChatMode {
				t.Fatalf("selfChatMode = %#v, want true", got["selfChatMode"])
			}
			if tt.changed == false {
				if agentID, _ := got["agentId"].(string); agentID != "main" {
					t.Fatalf("agentId = %#v, want %q", got["agentId"], "main")
				}
			}
		})
	}
}

func TestEnsureWhatsappChannelConfigEntry(t *testing.T) {
	tests := []struct {
		name    string
		entry   map[string]any
		changed bool
	}{
		{
			name:    "creates required flags for empty channel config",
			entry:   nil,
			changed: true,
		},
		{
			name: "adds self chat mode for legacy enabled channel config",
			entry: map[string]any{
				"enabled": true,
			},
			changed: true,
		},
		{
			name: "preserves unrelated channel config",
			entry: map[string]any{
				"enabled":      true,
				"selfChatMode": true,
				"dmPolicy":     "pairing",
			},
			changed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed := ensureWhatsappChannelConfigEntry(tt.entry)
			if changed != tt.changed {
				t.Fatalf("ensureWhatsappChannelConfigEntry(%v) changed = %v, want %v", tt.entry, changed, tt.changed)
			}
			if enabled, _ := got["enabled"].(bool); !enabled {
				t.Fatalf("enabled = %#v, want true", got["enabled"])
			}
			if selfChatMode, _ := got["selfChatMode"].(bool); !selfChatMode {
				t.Fatalf("selfChatMode = %#v, want true", got["selfChatMode"])
			}
			if tt.changed == false {
				if dmPolicy, _ := got["dmPolicy"].(string); dmPolicy != "pairing" {
					t.Fatalf("dmPolicy = %#v, want %q", got["dmPolicy"], "pairing")
				}
			}
		})
	}
}

func TestIsWhatsappConfigEnabledForAccount(t *testing.T) {
	tests := []struct {
		name      string
		channel   map[string]any
		accountID string
		want      bool
	}{
		{
			name: "accepts channel level self chat mode for default account",
			channel: map[string]any{
				"enabled":      true,
				"selfChatMode": true,
			},
			accountID: "default",
			want:      true,
		},
		{
			name: "inherits channel level self chat mode into account",
			channel: map[string]any{
				"enabled":      true,
				"selfChatMode": true,
				"accounts": map[string]any{
					"default": map[string]any{
						"enabled": true,
					},
				},
			},
			accountID: "default",
			want:      true,
		},
		{
			name: "account override can disable self chat mode",
			channel: map[string]any{
				"enabled":      true,
				"selfChatMode": true,
				"accounts": map[string]any{
					"default": map[string]any{
						"enabled":      true,
						"selfChatMode": false,
					},
				},
			},
			accountID: "default",
			want:      false,
		},
		{
			name: "missing self chat mode stays unconfigured",
			channel: map[string]any{
				"enabled": true,
			},
			accountID: "default",
			want:      false,
		},
		{
			name: "enabled defaults to true when self chat mode is configured",
			channel: map[string]any{
				"selfChatMode": true,
			},
			accountID: "default",
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isWhatsappConfigEnabledForAccount(tt.channel, tt.accountID); got != tt.want {
				t.Fatalf("isWhatsappConfigEnabledForAccount(%v, %q) = %v, want %v", tt.channel, tt.accountID, got, tt.want)
			}
		})
	}
}

func TestNextWhatsappAutoChannelName(t *testing.T) {
	tests := []struct {
		name     string
		existing []string
		want     string
	}{
		{
			name:     "first whatsapp connection starts at 1",
			existing: nil,
			want:     "WhatsApp1",
		},
		{
			name:     "legacy default name advances by current count",
			existing: []string{"WhatsApp"},
			want:     "WhatsApp2",
		},
		{
			name:     "custom whatsapp names still advance by connection count",
			existing: []string{"Sales WA", "Support WA"},
			want:     "WhatsApp3",
		},
		{
			name:     "collision falls through to next available suffix",
			existing: []string{"Sales WA", "Support WA", "Billing WA", "whatsapp5"},
			want:     "WhatsApp6",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nextWhatsappAutoChannelName(tt.existing); got != tt.want {
				t.Fatalf("nextWhatsappAutoChannelName(%v) = %q, want %q", tt.existing, got, tt.want)
			}
		})
	}
}

func TestFirstConfiguredWhatsappAccountID(t *testing.T) {
	tests := []struct {
		name    string
		channel map[string]any
		want    string
	}{
		{
			name:    "defaults when accounts are missing",
			channel: nil,
			want:    whatsappDefaultAccountID,
		},
		{
			name: "prefers explicit default account",
			channel: map[string]any{
				"accounts": map[string]any{
					"secondary": map[string]any{},
					"default":   map[string]any{},
				},
			},
			want: whatsappDefaultAccountID,
		},
		{
			name: "falls back to first non-empty configured account",
			channel: map[string]any{
				"accounts": map[string]any{
					" custom ": map[string]any{},
				},
			},
			want: "custom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := firstConfiguredWhatsappAccountID(tt.channel); got != tt.want {
				t.Fatalf("firstConfiguredWhatsappAccountID(%v) = %q, want %q", tt.channel, got, tt.want)
			}
		})
	}
}

func TestWhatsappConfiguredAgentIDFromConfig(t *testing.T) {
	tests := []struct {
		name      string
		cfg       map[string]any
		accountID string
		want      string
	}{
		{
			name: "uses account level agent binding first",
			cfg: map[string]any{
				"channels": map[string]any{
					"whatsapp": map[string]any{
						"accounts": map[string]any{
							"default": map[string]any{
								"agentId": "main",
							},
						},
					},
				},
				"bindings": []any{
					map[string]any{
						"type":    "route",
						"agentId": "fallback",
						"match": map[string]any{
							"channel":   "whatsapp",
							"accountId": "default",
						},
					},
				},
			},
			accountID: "default",
			want:      "main",
		},
		{
			name: "falls back to route binding",
			cfg: map[string]any{
				"bindings": []any{
					map[string]any{
						"type":    "route",
						"agentId": "fallback",
						"match": map[string]any{
							"channel":   "whatsapp",
							"accountId": "custom",
						},
					},
				},
			},
			accountID: "custom",
			want:      "fallback",
		},
		{
			name: "returns empty when nothing configured",
			cfg: map[string]any{
				"channels": map[string]any{
					"whatsapp": map[string]any{},
				},
			},
			accountID: "default",
			want:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := whatsappConfiguredAgentIDFromConfig(tt.cfg, tt.accountID); got != tt.want {
				t.Fatalf("whatsappConfiguredAgentIDFromConfig(%v, %q) = %q, want %q", tt.cfg, tt.accountID, got, tt.want)
			}
		})
	}
}

func TestBuildWhatsappWebLoginStartParams(t *testing.T) {
	params := buildWhatsappWebLoginStartParams("default", 25*time.Second, true)

	if got, ok := params["timeoutMs"].(int); !ok || got != 25000 {
		t.Fatalf("timeoutMs = %#v, want 25000", params["timeoutMs"])
	}
	if got, ok := params["accountId"].(string); !ok || got != "default" {
		t.Fatalf("accountId = %#v, want %q", params["accountId"], "default")
	}
	if got, ok := params["force"].(bool); !ok || !got {
		t.Fatalf("force = %#v, want true", params["force"])
	}
	if got, ok := params["verbose"].(bool); !ok || !got {
		t.Fatalf("verbose = %#v, want true", params["verbose"])
	}
}

func TestBuildWhatsappWebLoginStartParamsWithoutForce(t *testing.T) {
	params := buildWhatsappWebLoginStartParams("default", whatsappQRFastStartTimeout, false)

	if got, ok := params["timeoutMs"].(int); !ok || got != 8000 {
		t.Fatalf("timeoutMs = %#v, want 8000", params["timeoutMs"])
	}
	if _, ok := params["force"]; ok {
		t.Fatalf("force unexpectedly present in start params: %#v", params["force"])
	}
}

func TestBuildWhatsappWebLoginWaitParams(t *testing.T) {
	params := buildWhatsappWebLoginWaitParams("default", 8*time.Minute)

	if got, ok := params["timeoutMs"].(int); !ok || got != 480000 {
		t.Fatalf("timeoutMs = %#v, want 480000", params["timeoutMs"])
	}
	if got, ok := params["accountId"].(string); !ok || got != "default" {
		t.Fatalf("accountId = %#v, want %q", params["accountId"], "default")
	}
	if _, ok := params["force"]; ok {
		t.Fatalf("force unexpectedly present in wait params: %#v", params["force"])
	}
	if _, ok := params["verbose"]; ok {
		t.Fatalf("verbose unexpectedly present in wait params: %#v", params["verbose"])
	}
}

func TestWhatsappLoginWaitMessageSuggestsRetry(t *testing.T) {
	tests := []struct {
		name string
		msg  string
		want bool
	}{
		{
			name: "restart required",
			msg:  "WhatsApp login failed: status=515 Unknown Stream Errored (restart required)",
			want: true,
		},
		{
			name: "login ended without connection",
			msg:  "Login ended without a connection.",
			want: true,
		},
		{
			name: "still waiting for qr scan",
			msg:  "Still waiting for the QR scan. Let me know when you’ve scanned it.",
			want: true,
		},
		{
			name: "logged out is terminal",
			msg:  "WhatsApp reported the session is logged out. Cleared cached web session; please scan a new QR.",
			want: false,
		},
		{
			name: "empty",
			msg:  "",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := whatsappLoginWaitMessageSuggestsRetry(tt.msg); got != tt.want {
				t.Fatalf("whatsappLoginWaitMessageSuggestsRetry(%q) = %v, want %v", tt.msg, got, tt.want)
			}
		})
	}
}

func TestWhatsappQRStartMessageSuggestsRetry(t *testing.T) {
	tests := []struct {
		name string
		msg  string
		want bool
	}{
		{
			name: "qr timeout",
			msg:  "Timed out waiting for WhatsApp QR from gateway",
			want: true,
		},
		{
			name: "restart required",
			msg:  "WhatsApp login failed: status=515 Unknown Stream Errored (restart required)",
			want: true,
		},
		{
			name: "empty message gets one retry",
			msg:  "",
			want: true,
		},
		{
			name: "logged out is terminal",
			msg:  "WhatsApp reported the session is logged out. Cleared cached web session; please scan a new QR.",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := whatsappQRStartMessageSuggestsRetry(tt.msg); got != tt.want {
				t.Fatalf("whatsappQRStartMessageSuggestsRetry(%q) = %v, want %v", tt.msg, got, tt.want)
			}
		})
	}
}
