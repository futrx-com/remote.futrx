package project

import "testing"

func TestSetAgentStatusesPreservesLegacyInspectFields(t *testing.T) {
	var inspect ContainerInspect
	inspect.SetAgentStatuses([]AgentContainerStatus{
		{
			ID:                    "claude",
			Label:                 "Claude Code",
			Installed:             true,
			Version:               "2.1.206",
			InstructionsInstalled: true,
			InstructionsInSync:    true,
		},
		{ID: "codex", Installed: true, Version: "0.144.1"},
		{ID: "kimi", Installed: true, Version: "0.19.2"},
	})
	if len(inspect.Agents) != 3 || inspect.Agents[2].ID != "kimi" {
		t.Fatalf("generic agent statuses = %#v", inspect.Agents)
	}
	inspect.Agents[0].Version = "changed"
	if inspect.Claude.Version != "2.1.206" {
		t.Fatalf("legacy status aliases generic slice: %#v", inspect.Claude)
	}

	if !inspect.Claude.Installed || inspect.Claude.Version != "2.1.206" ||
		!inspect.Claude.ClaudeMDInstalled || !inspect.Claude.ClaudeMDInSync {
		t.Fatalf("Claude compatibility status = %#v", inspect.Claude)
	}
	if !inspect.Codex.Installed || inspect.Codex.Version != "0.144.1" {
		t.Fatalf("Codex compatibility status = %#v", inspect.Codex)
	}
}

func TestNormalizeContainerLimits(t *testing.T) {
	tests := []struct {
		name    string
		limits  ContainerLimits
		want    ContainerLimits
		wantErr bool
	}{
		{name: "empty inherits defaults"},
		{
			name:   "valid values are trimmed",
			limits: ContainerLimits{CPU: " 4 ", Memory: "8GiB", Disk: "40GiB"},
			want:   ContainerLimits{CPU: "4", Memory: "8GiB", Disk: "40GiB"},
		},
		{name: "zero cpu", limits: ContainerLimits{CPU: "0"}, wantErr: true},
		{name: "fractional cpu", limits: ContainerLimits{CPU: "1.5"}, wantErr: true},
		{name: "cpu too large", limits: ContainerLimits{CPU: "257"}, wantErr: true},
		{name: "unit required", limits: ContainerLimits{Memory: "8192"}, wantErr: true},
		{name: "binary unit required", limits: ContainerLimits{Disk: "40GB"}, wantErr: true},
		{name: "zero size", limits: ContainerLimits{Memory: "0GiB"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeContainerLimits(tt.limits)
			if (err != nil) != tt.wantErr {
				t.Fatalf("normalizeContainerLimits() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Fatalf("normalizeContainerLimits() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
