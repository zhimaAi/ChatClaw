package define

import "testing"

func TestBuiltinProviders_ChatWikiUsesChatClawIcon(t *testing.T) {
	for _, provider := range BuiltinProviders {
		if provider.ProviderID != "chatwiki" {
			continue
		}

		if provider.Icon != "chatclaw" {
			t.Fatalf("expected chatwiki icon to be chatclaw, got %q", provider.Icon)
		}
		return
	}

	t.Fatal("chatwiki provider not found")
}
