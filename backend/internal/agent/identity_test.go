package agent

import "testing"

func TestProviderIdentitySupportsFutureAgentsWithoutAcceptingUnsafeKeys(t *testing.T) {
	if got := NormalizeProviderID(" Future-Agent "); got != "future-agent" {
		t.Fatalf("normalized provider = %q", got)
	}
	for _, id := range []ProviderID{"claude", "future-agent", "agent2"} {
		if !ValidProviderID(id) {
			t.Fatalf("ValidProviderID(%q) = false", id)
		}
	}
	for _, id := range []ProviderID{"", "Future", "two words", "../agent", "-agent"} {
		if ValidProviderID(id) {
			t.Fatalf("ValidProviderID(%q) = true", id)
		}
	}
}
