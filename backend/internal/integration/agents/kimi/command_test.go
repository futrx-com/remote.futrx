package kimi

import (
	"slices"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

func TestArgsUseNativePlanModeWhenSelected(t *testing.T) {
	provider := &Provider{}
	plan := provider.args(agent.RunRequest{Prompt: "inspect", Mode: agent.RunModePlan})
	if !slices.Contains(plan, "--plan") {
		t.Fatalf("native Plan mode missing: %#v", plan)
	}

	defaults := provider.args(agent.RunRequest{Prompt: "implement", Mode: agent.RunModeDefault})
	if slices.Contains(defaults, "--plan") {
		t.Fatalf("default mode unexpectedly enabled Plan: %#v", defaults)
	}
}

func TestArgsPreserveExactConfiguredModelAlias(t *testing.T) {
	provider := &Provider{}
	args := provider.args(agent.RunRequest{Prompt: "inspect", Model: "moonshot/kimi-k2[1m]"})
	for index := 0; index+1 < len(args); index++ {
		if args[index] == "--model" && args[index+1] == "moonshot/kimi-k2[1m]" {
			return
		}
	}
	t.Fatalf("exact model alias missing: %#v", args)
}
