package config

import (
	"testing"
	"time"
)

func TestLoadUsesGlobalAgentCapabilityTimeout(t *testing.T) {
	t.Setenv("AGENT_CAPABILITY_TIMEOUT", "42s")
	if got := Load().Agent.CapabilityTimeout; got != 42*time.Second {
		t.Fatalf("capability timeout = %s, want 42s", got)
	}

	t.Setenv("AGENT_CAPABILITY_TIMEOUT", "invalid")
	if got := Load().Agent.CapabilityTimeout; got != 30*time.Second {
		t.Fatalf("invalid capability timeout fallback = %s, want 30s", got)
	}
}

func TestLoadUsesGlobalAgentPolicyDefaults(t *testing.T) {
	options := Load().Agent
	if options.CapabilityCacheTTL != 24*time.Hour {
		t.Fatalf("live capability cache TTL = %s, want 24h", options.CapabilityCacheTTL)
	}
	if options.HostCLIVersionTimeout != 15*time.Second {
		t.Fatalf("host CLI version timeout = %s, want 15s", options.HostCLIVersionTimeout)
	}
	if options.DegradedCapabilityCacheTTL != 2*time.Hour {
		t.Fatalf("degraded capability cache TTL = %s, want 2h", options.DegradedCapabilityCacheTTL)
	}
	if options.CredentialSyncTimeout != 30*time.Second {
		t.Fatalf("credential sync timeout = %s, want 30s", options.CredentialSyncTimeout)
	}
	if options.BrowserIdleTTL != 20*time.Minute {
		t.Fatalf("browser idle TTL = %s, want 20m", options.BrowserIdleTTL)
	}
}

func TestLoadUsesAuthPolicyDefaults(t *testing.T) {
	options := Load().Auth
	if options.PendingLoginTTL != 5*time.Minute {
		t.Fatalf("pending login TTL = %s, want 5m", options.PendingLoginTTL)
	}
	if options.EnrollmentTTL != 10*time.Minute {
		t.Fatalf("enrollment TTL = %s, want 10m", options.EnrollmentTTL)
	}
	if options.RecoveryCodeCount != 10 {
		t.Fatalf("recovery code count = %d, want 10", options.RecoveryCodeCount)
	}
	if options.SessionHistoryLimit != 20 {
		t.Fatalf("session history limit = %d, want 20", options.SessionHistoryLimit)
	}
}

func TestCodeServerBaseURLUsesInstalledDomain(t *testing.T) {
	tests := []struct {
		base string
		want string
	}{
		{base: "https://remote.example.com", want: "https://code.remote.example.com/"},
		{base: "https://app.company.test:8443/path?ignored=yes", want: "https://code.app.company.test:8443/"},
	}
	for _, test := range tests {
		got, err := CodeServerBaseURL(test.base)
		if err != nil {
			t.Fatalf("CodeServerBaseURL(%q): %v", test.base, err)
		}
		if got != test.want {
			t.Fatalf("CodeServerBaseURL(%q) = %q, want %q", test.base, got, test.want)
		}
	}
}

func TestCodeServerBaseURLRejectsInvalidBaseURL(t *testing.T) {
	if _, err := CodeServerBaseURL("remote.example.com"); err == nil {
		t.Fatal("CodeServerBaseURL accepted a URL without scheme")
	}
}

func TestPublicHostnameUsesInstalledDomain(t *testing.T) {
	got, err := PublicHostname("https://remote.example.com:8443/path")
	if err != nil {
		t.Fatalf("PublicHostname: %v", err)
	}
	if got != "remote.example.com" {
		t.Fatalf("PublicHostname = %q, want remote.example.com", got)
	}
}
