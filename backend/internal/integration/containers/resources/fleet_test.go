package resources

import (
	"context"
	"testing"

	serviceresources "github.com/futrx-com/remote.futrx.com/internal/service/resources"
)

const instancesRecursion1 = `[
  {"name":"alpha","status":"Running","config":{"limits.memory":"8GiB"},"expanded_config":{"limits.memory":"8GiB","limits.cpu":"4"}},
  {"name":"beta","status":"Stopped","config":{},"expanded_config":{"limits.memory":"2GiB"}},
  {"name":"gamma","status":"Running","config":{},"expanded_config":{"limits.memory":"2GiB"}},
  {"name":"delta","status":"Running","config":{},"expanded_config":{}}
]`

func TestRunningInstancesReportsEffectiveMemoryCeilings(t *testing.T) {
	runner := &fakeRunner{responses: map[string]fakeResponse{
		"query /1.0/instances?recursion=1": {out: instancesRecursion1},
	}}

	got, err := NewManager(runner).RunningInstances(context.Background())
	if err != nil {
		t.Fatalf("RunningInstances: %v", err)
	}

	want := []serviceresources.Instance{
		{Name: "alpha", Running: true, Memory: "8GiB"},
		{Name: "beta", Running: false, Memory: "2GiB"},
		{Name: "gamma", Running: true, Memory: "2GiB"},
		{Name: "delta", Running: true, Memory: ""},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d instances, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("instance %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestApplyDefaultsWritesOperatorEnvelopeToProfile(t *testing.T) {
	runner := &fakeRunner{responses: map[string]fakeResponse{
		"profile show " + ProfileName: {out: "name: " + ProfileName},
	}}
	manager := NewManager(runner)

	err := manager.ApplyDefaults(context.Background(), serviceresources.Limits{
		Memory:    "2GiB",
		CPU:       1.5,
		Processes: 1500,
		Disk:      "20GiB",
	})
	if err != nil {
		t.Fatalf("ApplyDefaults: %v", err)
	}

	// A fractional policy CPU rounds up so a workspace never gets zero cores.
	want := map[string]string{
		"limits.memory":    "2GiB",
		"limits.cpu":       "2",
		"limits.processes": "1500",
		"security.nesting": "true",
	}
	for _, entry := range manager.profileEntries() {
		if want[entry[0]] != entry[1] {
			t.Fatalf("profile %s = %q, want %q", entry[0], entry[1], want[entry[0]])
		}
		delete(want, entry[0])
	}
	if len(want) != 0 {
		t.Fatalf("profile is missing entries: %v", want)
	}
	// The disk quota is a per-container device property, never a profile key.
	for _, call := range runner.called("profile set") {
		if call == "profile set "+ProfileName+" limits.disk 20GiB" {
			t.Fatalf("disk quota must not be written to the profile: %q", call)
		}
	}
}

func TestFormatCores(t *testing.T) {
	tests := []struct {
		cpu  float64
		want string
	}{
		{cpu: 0, want: ""},
		{cpu: -1, want: ""},
		{cpu: 0.5, want: "1"},
		{cpu: 1, want: "1"},
		{cpu: 2, want: "2"},
		{cpu: 2.1, want: "3"},
		{cpu: 6, want: "6"},
	}
	for _, test := range tests {
		if got := formatCores(test.cpu); got != test.want {
			t.Fatalf("formatCores(%g) = %q, want %q", test.cpu, got, test.want)
		}
	}
}
