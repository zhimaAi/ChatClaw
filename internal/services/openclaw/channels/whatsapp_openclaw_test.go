package openclawchannels

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"chatclaw/internal/services/channels"
)

func TestWithWhatsappAccountID(t *testing.T) {
	t.Run("creates minimal whatsapp config for empty payload", func(t *testing.T) {
		got, err := withWhatsappAccountID("", "whatsapp_demo123")
		if err != nil {
			t.Fatalf("withWhatsappAccountID() error = %v", err)
		}

		var cfg appCredentialsJSON
		if err := json.Unmarshal([]byte(got), &cfg); err != nil {
			t.Fatalf("unmarshal result: %v", err)
		}
		if cfg.Platform != channels.PlatformWhatsapp {
			t.Fatalf("platform = %q, want %q", cfg.Platform, channels.PlatformWhatsapp)
		}
		if cfg.AccountID != "whatsapp_demo123" {
			t.Fatalf("account_id = %q, want %q", cfg.AccountID, "whatsapp_demo123")
		}
	})

	t.Run("preserves existing fields while updating account id", func(t *testing.T) {
		raw := `{"platform":"whatsapp","app_id":"foo","openclaw_channel_id":"channel_7"}`
		got, err := withWhatsappAccountID(raw, "whatsapp_demo456")
		if err != nil {
			t.Fatalf("withWhatsappAccountID() error = %v", err)
		}

		var cfg appCredentialsJSON
		if err := json.Unmarshal([]byte(got), &cfg); err != nil {
			t.Fatalf("unmarshal result: %v", err)
		}
		if cfg.AppID != "foo" {
			t.Fatalf("app_id = %q, want %q", cfg.AppID, "foo")
		}
		if cfg.OpenClawChannelID != "channel_7" {
			t.Fatalf("openclaw_channel_id = %q, want %q", cfg.OpenClawChannelID, "channel_7")
		}
		if cfg.AccountID != "whatsapp_demo456" {
			t.Fatalf("account_id = %q, want %q", cfg.AccountID, "whatsapp_demo456")
		}
	})
}

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

func TestGenerateWhatsappLoginAccountID(t *testing.T) {
	t.Run("uses whatsapp prefix with seven alnum chars", func(t *testing.T) {
		reader := bytes.NewReader([]byte{15, 1, 1, 10, 15, 1, 13})
		got, err := generateWhatsappLoginAccountID(nil, reader)
		if err != nil {
			t.Fatalf("generateWhatsappLoginAccountID() error = %v", err)
		}
		if got != "whatsapp_f11af1d" {
			t.Fatalf("generateWhatsappLoginAccountID() = %q, want %q", got, "whatsapp_f11af1d")
		}
	})

	t.Run("retries when generated id already exists", func(t *testing.T) {
		reader := bytes.NewReader([]byte{
			15, 1, 1, 10, 15, 1, 13,
			2, 3, 4, 5, 6, 7, 8,
		})
		got, err := generateWhatsappLoginAccountID([]string{"whatsapp_f11af1d"}, reader)
		if err != nil {
			t.Fatalf("generateWhatsappLoginAccountID() error = %v", err)
		}
		if got != "whatsapp_2345678" {
			t.Fatalf("generateWhatsappLoginAccountID() = %q, want %q", got, "whatsapp_2345678")
		}
	})

	t.Run("treats existing ids case insensitively", func(t *testing.T) {
		reader := bytes.NewReader([]byte{
			15, 1, 1, 10, 15, 1, 13,
			2, 3, 4, 5, 6, 7, 8,
		})
		got, err := generateWhatsappLoginAccountID([]string{"  WHATSAPP_F11AF1D  "}, reader)
		if err != nil {
			t.Fatalf("generateWhatsappLoginAccountID() error = %v", err)
		}
		if got != "whatsapp_2345678" {
			t.Fatalf("generateWhatsappLoginAccountID() = %q, want %q", got, "whatsapp_2345678")
		}
	})

	t.Run("fails after repeated collisions", func(t *testing.T) {
		repeated := bytes.Repeat([]byte{15, 1, 1, 10, 15, 1, 13}, whatsappAccountIDMaxAttempts)
		_, err := generateWhatsappLoginAccountID([]string{"whatsapp_f11af1d"}, bytes.NewReader(repeated))
		if err == nil {
			t.Fatal("generateWhatsappLoginAccountID() error = nil, want collision exhaustion")
		}
		if !strings.Contains(err.Error(), "exhausted") {
			t.Fatalf("generateWhatsappLoginAccountID() error = %v, want exhausted attempts", err)
		}
	})
}

func TestCancelWhatsappLoginSession(t *testing.T) {
	t.Run("closing dialog before qr generation cleans prepared account", func(t *testing.T) {
		sessions := map[string]*whatsappLoginSession{}
		accountID, shouldCleanup := cancelWhatsappLoginSession(sessions, "whatsapp_demo123", "")
		if accountID != "whatsapp_demo123" {
			t.Fatalf("accountID = %q, want %q", accountID, "whatsapp_demo123")
		}
		if !shouldCleanup {
			t.Fatal("shouldCleanup = false, want true")
		}
	})

	t.Run("canceling known session cleans account when it is the last one", func(t *testing.T) {
		sessions := map[string]*whatsappLoginSession{
			"sess-1": {accountID: "whatsapp_demo123"},
		}
		accountID, shouldCleanup := cancelWhatsappLoginSession(sessions, "", "sess-1")
		if accountID != "whatsapp_demo123" {
			t.Fatalf("accountID = %q, want %q", accountID, "whatsapp_demo123")
		}
		if !shouldCleanup {
			t.Fatal("shouldCleanup = false, want true")
		}
		if got := len(sessions); got != 0 {
			t.Fatalf("sessions len = %d, want 0", got)
		}
	})

	t.Run("canceling one of multiple sessions for same account keeps config", func(t *testing.T) {
		sessions := map[string]*whatsappLoginSession{
			"sess-1": {accountID: "whatsapp_demo123"},
			"sess-2": {accountID: "whatsapp_demo123"},
		}
		accountID, shouldCleanup := cancelWhatsappLoginSession(sessions, "", "sess-1")
		if accountID != "whatsapp_demo123" {
			t.Fatalf("accountID = %q, want %q", accountID, "whatsapp_demo123")
		}
		if shouldCleanup {
			t.Fatal("shouldCleanup = true, want false")
		}
		if got := len(sessions); got != 1 {
			t.Fatalf("sessions len = %d, want 1", got)
		}
		if _, ok := sessions["sess-2"]; !ok {
			t.Fatal("remaining session was removed unexpectedly")
		}
	})

	t.Run("unknown session key is ignored", func(t *testing.T) {
		sessions := map[string]*whatsappLoginSession{
			"sess-1": {accountID: "whatsapp_demo123"},
		}
		accountID, shouldCleanup := cancelWhatsappLoginSession(sessions, "", "missing")
		if accountID != "" {
			t.Fatalf("accountID = %q, want empty", accountID)
		}
		if shouldCleanup {
			t.Fatal("shouldCleanup = true, want false")
		}
		if got := len(sessions); got != 1 {
			t.Fatalf("sessions len = %d, want 1", got)
		}
	})
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

func TestWhatsappManagedBindingAccountIDs(t *testing.T) {
	cfg := map[string]any{
		"bindings": []any{
			map[string]any{
				"type": "route",
				"match": map[string]any{
					"channel":   "whatsapp",
					"accountId": "default",
				},
			},
			map[string]any{
				"type": "route",
				"match": map[string]any{
					"channel":   "qq",
					"accountId": "channel_1",
				},
			},
			map[string]any{
				"type": "route",
				"match": map[string]any{
					"channel":   "whatsapp",
					"accountId": "account_2",
				},
			},
			map[string]any{
				"type": "route",
				"match": map[string]any{
					"channel":   "whatsapp",
					"accountId": "default",
				},
			},
		},
	}

	got := whatsappManagedBindingAccountIDs(cfg)
	want := []string{"default", "account_2"}
	if len(got) != len(want) {
		t.Fatalf("whatsappManagedBindingAccountIDs() len = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("whatsappManagedBindingAccountIDs()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestPurgeWhatsappChannelFromOpenClawJSON(t *testing.T) {
	t.Run("removes matching binding and account while preserving siblings", func(t *testing.T) {
		cfg := map[string]any{
			"bindings": []any{
				map[string]any{
					"type":    "route",
					"agentId": "agent-a",
					"match": map[string]any{
						"channel":   "whatsapp",
						"accountId": "whatsapp_deadbeef",
					},
				},
				map[string]any{
					"type":    "route",
					"agentId": "agent-b",
					"match": map[string]any{
						"channel":   "whatsapp",
						"accountId": "whatsapp_keepme",
					},
				},
				map[string]any{
					"type":    "route",
					"agentId": "agent-c",
					"match": map[string]any{
						"channel":   "qq",
						"accountId": "channel_9",
					},
				},
			},
			"channels": map[string]any{
				"whatsapp": map[string]any{
					"enabled":      true,
					"selfChatMode": true,
					"accounts": map[string]any{
						"whatsapp_deadbeef": map[string]any{
							"enabled":      true,
							"selfChatMode": true,
						},
						"whatsapp_keepme": map[string]any{
							"enabled":      true,
							"selfChatMode": true,
						},
					},
				},
			},
		}

		purgeWhatsappChannelFromOpenClawJSON(cfg, "whatsapp_deadbeef")

		bindings := configBindings(cfg)
		if len(bindings) != 2 {
			t.Fatalf("bindings len = %d, want 2", len(bindings))
		}
		whatsappSection := whatsappChannelConfigFromRoot(cfg)
		if whatsappSection == nil {
			t.Fatal("whatsapp section removed unexpectedly")
		}
		accounts := whatsappAccountConfigs(whatsappSection)
		if _, ok := accounts["whatsapp_deadbeef"]; ok {
			t.Fatal("deleted whatsapp account still present in config")
		}
		if _, ok := accounts["whatsapp_keepme"]; !ok {
			t.Fatal("sibling whatsapp account was removed unexpectedly")
		}
	})

	t.Run("drops empty whatsapp section when last account is removed", func(t *testing.T) {
		cfg := map[string]any{
			"bindings": []any{
				map[string]any{
					"type":    "route",
					"agentId": "agent-a",
					"match": map[string]any{
						"channel":   "whatsapp",
						"accountId": "default",
					},
				},
			},
			"channels": map[string]any{
				"whatsapp": map[string]any{
					"enabled":      true,
					"selfChatMode": true,
					"accounts": map[string]any{
						"default": map[string]any{
							"enabled":      true,
							"selfChatMode": true,
						},
					},
				},
			},
		}

		purgeWhatsappChannelFromOpenClawJSON(cfg, "")

		if _, ok := cfg["channels"]; ok {
			t.Fatalf("channels section still present after removing last whatsapp account: %#v", cfg["channels"])
		}
		if got := len(configBindings(cfg)); got != 0 {
			t.Fatalf("bindings len = %d, want 0", got)
		}
	})

	t.Run("preserves custom whatsapp section keys after account removal", func(t *testing.T) {
		cfg := map[string]any{
			"channels": map[string]any{
				"whatsapp": map[string]any{
					"enabled":      true,
					"selfChatMode": true,
					"dmPolicy":     "pairing",
					"accounts": map[string]any{
						"default": map[string]any{
							"enabled":      true,
							"selfChatMode": true,
						},
					},
				},
			},
		}

		purgeWhatsappChannelFromOpenClawJSON(cfg, "default")

		whatsappSection := whatsappChannelConfigFromRoot(cfg)
		if whatsappSection == nil {
			t.Fatal("whatsapp section removed despite custom keys")
		}
		if dmPolicy, _ := whatsappSection["dmPolicy"].(string); dmPolicy != "pairing" {
			t.Fatalf("dmPolicy = %#v, want %q", whatsappSection["dmPolicy"], "pairing")
		}
		if _, ok := whatsappSection[whatsappConfigKeyAccounts]; ok {
			t.Fatalf("accounts key still present after removing last account: %#v", whatsappSection[whatsappConfigKeyAccounts])
		}
	})
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

func TestWhatsappQRStartReady(t *testing.T) {
	if whatsappQRStartReady(nil) {
		t.Fatal("whatsappQRStartReady(nil) = true, want false")
	}
	if whatsappQRStartReady(&whatsappGatewayLoginStartResult{}) {
		t.Fatal("whatsappQRStartReady(empty) = true, want false")
	}
	if !whatsappQRStartReady(&whatsappGatewayLoginStartResult{QRDataURL: "data:image/png;base64,abc"}) {
		t.Fatal("whatsappQRStartReady(with qr) = false, want true")
	}
}

func TestWhatsappQRStartNeedsRetry(t *testing.T) {
	tests := []struct {
		name  string
		start *whatsappGatewayLoginStartResult
		err   error
		want  bool
	}{
		{
			name: "plugin missing error is terminal",
			err:  errors.New("whatsapp plugin not installed"),
			want: false,
		},
		{
			name: "timeout error retries",
			err:  errors.New("Failed to get QR: Error: Timed out waiting for WhatsApp QR"),
			want: true,
		},
		{
			name: "nil result without error gets retry",
			want: true,
		},
		{
			name: "qr data present does not retry",
			start: &whatsappGatewayLoginStartResult{
				QRDataURL: "data:image/png;base64,abc",
			},
			want: false,
		},
		{
			name: "transient start message retries",
			start: &whatsappGatewayLoginStartResult{
				Message: "Timed out waiting for WhatsApp QR from gateway",
			},
			want: true,
		},
		{
			name: "terminal start message does not retry",
			start: &whatsappGatewayLoginStartResult{
				Message: "WhatsApp reported the session is logged out. Cleared cached web session; please scan a new QR.",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := whatsappQRStartNeedsRetry(tt.start, tt.err); got != tt.want {
				t.Fatalf("whatsappQRStartNeedsRetry(%+v, %v) = %v, want %v", tt.start, tt.err, got, tt.want)
			}
		})
	}
}
