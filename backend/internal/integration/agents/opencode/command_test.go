package opencode

import (
	"slices"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

func TestArgsUseNativePlanAgentWhenSelected(t *testing.T) {
	provider := &Provider{}
	plan := provider.args(agent.RunRequest{Prompt: "inspect", Mode: agent.RunModePlan})
	if !slices.Contains(plan, "plan") || !slices.Contains(plan, "--agent") {
		t.Fatalf("native Plan mode missing: %#v", plan)
	}

	defaults := provider.args(agent.RunRequest{Prompt: "implement", Mode: agent.RunModeDefault})
	if slices.Contains(defaults, "--agent") {
		t.Fatalf("default mode unexpectedly enabled Plan: %#v", defaults)
	}
}

func TestArgsPreserveExactConfiguredModelAlias(t *testing.T) {
	provider := &Provider{}
	args := provider.args(agent.RunRequest{Prompt: "inspect", Model: "anthropic/claude-sonnet-4-5"})
	index := slices.Index(args, "--model")
	if index < 0 || index+1 >= len(args) || args[index+1] != "anthropic/claude-sonnet-4-5" {
		t.Fatalf("exact model alias missing: %#v", args)
	}
}

func TestArgsResumeAndNativeFork(t *testing.T) {
	provider := &Provider{}
	resume := provider.args(agent.RunRequest{Prompt: "next", ResumeID: "ses_abc"})
	if !slices.Contains(resume, "--session") || slices.Contains(resume, "--fork") {
		t.Fatalf("resume args = %#v", resume)
	}

	fork := provider.args(agent.RunRequest{Prompt: "next", ResumeID: "ses_abc", Fork: true})
	if !slices.Contains(fork, "--fork") {
		t.Fatalf("fork args = %#v", fork)
	}

	if got := provider.args(agent.RunRequest{Prompt: "fresh"}); slices.Contains(got, "--session") {
		t.Fatalf("fresh run unexpectedly resumes: %#v", got)
	}
}

func TestArgsPutPromptLastAndRequestJSON(t *testing.T) {
	provider := &Provider{}
	args := provider.args(agent.RunRequest{Prompt: "Say exactly: hello"})
	if args[len(args)-1] != "Say exactly: hello" {
		t.Fatalf("prompt must be the trailing positional: %#v", args)
	}
	if !slices.Contains(args, "--format") || !slices.Contains(args, "json") {
		t.Fatalf("json format missing: %#v", args)
	}
}
