package chat

import (
	"encoding/json"
	"testing"
)

func TestCapabilityValuesRemainProviderDefined(t *testing.T) {
	if got := NormalizeReasoningEffort(" Future.V2 "); got != "Future.V2" {
		t.Fatalf("reasoning effort = %q", got)
	}
	if got := NormalizeServiceTier(" Burst_2 "); got != "Burst_2" {
		t.Fatalf("service tier = %q", got)
	}
	if got := NormalizeServiceTier("unsafe;value"); got != "" {
		t.Fatalf("unsafe service tier = %q", got)
	}
}

func TestNormalizeProviderKeepsFutureSafeIdentifiers(t *testing.T) {
	if got := NormalizeProvider(" Future-Agent "); got != "future-agent" {
		t.Fatalf("future provider = %q", got)
	}
	if got := NormalizeProvider("unsafe provider"); got != ProviderCodex {
		t.Fatalf("unsafe provider fallback = %q, want codex", got)
	}
}

func TestMetaJSONPreservesExplicitAutoSelections(t *testing.T) {
	raw, err := json.Marshal(Meta{ID: "abcd"})
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"model", "mode", "reasoningEffort", "serviceTier"} {
		value, ok := result[field]
		if !ok || value != "" {
			t.Fatalf("%s = %#v, present = %t", field, value, ok)
		}
	}
}

func TestMetaSessionsImportAndMirrorLegacyFields(t *testing.T) {
	meta := Meta{
		ClaudeSessionID:      "claude-session",
		AntigravitySessionID: "agy-session",
	}
	meta.NormalizeSessions()
	if meta.SessionID(ProviderClaude) != "claude-session" || meta.SessionID(ProviderAntigravity) != "agy-session" {
		t.Fatalf("imported sessions = %#v", meta.Sessions)
	}

	meta.SetSessionID("future-agent", "future-session")
	meta.SetSessionID(ProviderClaude, "updated-claude")
	if meta.Sessions["future-agent"] != "future-session" || meta.ClaudeSessionID != "updated-claude" {
		t.Fatalf("updated sessions = %#v, legacy Claude = %q", meta.Sessions, meta.ClaudeSessionID)
	}

	snapshot := meta.SessionSnapshot()
	snapshot[ProviderClaude] = "mutated"
	if meta.SessionID(ProviderClaude) != "updated-claude" {
		t.Fatal("session snapshot mutated the metadata")
	}
}

func TestEventSessionSupportsFutureProvidersAndLegacyFields(t *testing.T) {
	event := Event{Type: "session"}
	event.SetSession("future-agent", "future-session")
	if event.Provider != "future-agent" || event.SessionID != "future-session" {
		t.Fatalf("generic session event = %#v", event)
	}

	legacy := Event{Type: "session", KimiSessionID: "kimi-session"}
	legacy.NormalizeSession()
	if legacy.Provider != ProviderKimi || legacy.SessionID != "kimi-session" || legacy.KimiSessionID != "kimi-session" {
		t.Fatalf("normalized legacy event = %#v", legacy)
	}
}
