package agent

import (
	"slices"
	"testing"
)

func TestRuntimeEnvironmentIsSortedAndRejectsInvalidNames(t *testing.T) {
	got := RuntimeEnvironment(map[string]string{
		"Z_TOKEN": "z",
		"A_URL":   "a",
		"bad-key": "ignored",
		"9BAD":    "ignored",
	})
	want := []string{"A_URL=a", "Z_TOKEN=z"}
	if !slices.Equal(got, want) {
		t.Fatalf("RuntimeEnvironment() = %#v, want %#v", got, want)
	}
}

func TestWithRuntimeEnvironmentReplacesExistingValues(t *testing.T) {
	got := WithRuntimeEnvironment(
		[]string{"PATH=/bin", "REMOTE_APPLICATION_GRANT=stale", "KEEP=yes"},
		map[string]string{"REMOTE_APPLICATION_GRANT": "fresh"},
	)
	want := []string{"PATH=/bin", "KEEP=yes", "REMOTE_APPLICATION_GRANT=fresh"}
	if !slices.Equal(got, want) {
		t.Fatalf("WithRuntimeEnvironment() = %#v, want %#v", got, want)
	}
}
