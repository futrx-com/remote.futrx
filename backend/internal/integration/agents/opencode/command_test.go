package opencode

import (
	"context"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

func TestArgsUseNativePlanAgentWhenSelected(t *testing.T) {
	provider := &Provider{}
	plan := provider.args(agent.RunRequest{Prompt: "inspect", Mode: agent.RunModePlan}, false)
	if !slices.Contains(plan, "plan") || !slices.Contains(plan, "--agent") {
		t.Fatalf("native Plan mode missing: %#v", plan)
	}

	defaults := provider.args(agent.RunRequest{Prompt: "implement", Mode: agent.RunModeDefault}, false)
	if slices.Contains(defaults, "--agent") {
		t.Fatalf("default mode unexpectedly enabled Plan: %#v", defaults)
	}
}

func TestArgsPreserveExactConfiguredModelAlias(t *testing.T) {
	provider := &Provider{}
	args := provider.args(agent.RunRequest{Prompt: "inspect", Model: "anthropic/claude-sonnet-4-5"}, false)
	index := slices.Index(args, "--model")
	if index < 0 || index+1 >= len(args) || args[index+1] != "anthropic/claude-sonnet-4-5" {
		t.Fatalf("exact model alias missing: %#v", args)
	}
}

func TestArgsResumeAndNativeFork(t *testing.T) {
	provider := &Provider{}
	resume := provider.args(agent.RunRequest{Prompt: "next", ResumeID: "ses_abc"}, false)
	if !slices.Contains(resume, "--session") || slices.Contains(resume, "--fork") {
		t.Fatalf("resume args = %#v", resume)
	}

	fork := provider.args(agent.RunRequest{Prompt: "next", ResumeID: "ses_abc", Fork: true}, false)
	if !slices.Contains(fork, "--fork") {
		t.Fatalf("fork args = %#v", fork)
	}

	if got := provider.args(agent.RunRequest{Prompt: "fresh"}, false); slices.Contains(got, "--session") {
		t.Fatalf("fresh run unexpectedly resumes: %#v", got)
	}
}

func TestArgsOmitPromptAndRequestJSON(t *testing.T) {
	provider := &Provider{}
	prompt := "Say exactly: hello --flag"
	args := provider.args(agent.RunRequest{Prompt: prompt}, false)
	if slices.Contains(args, prompt) {
		t.Fatalf("prompt must not be in args: %#v", args)
	}
	if !slices.Contains(args, "--format") || !slices.Contains(args, "json") {
		t.Fatalf("json format missing: %#v", args)
	}
}

func TestArgsEnableAutoInContainerDefaultMode(t *testing.T) {
	provider := &Provider{}

	containerDefault := provider.args(agent.RunRequest{Mode: agent.RunModeDefault}, true)
	if !slices.Contains(containerDefault, "--auto") {
		t.Fatalf("container default mode must pass --auto: %#v", containerDefault)
	}

	containerPlan := provider.args(agent.RunRequest{Mode: agent.RunModePlan}, true)
	if slices.Contains(containerPlan, "--auto") {
		t.Fatalf("container plan mode must not pass --auto: %#v", containerPlan)
	}

	hostDefault := provider.args(agent.RunRequest{Mode: agent.RunModeDefault}, false)
	if slices.Contains(hostDefault, "--auto") {
		t.Fatalf("host mode must not pass --auto: %#v", hostDefault)
	}
}

func TestBuildCmdPassesPromptViaStdinAndOmitsFromArgs(t *testing.T) {
	provider := &Provider{}
	prompt := "Write a function -v with flags and quotes"
	req := agent.RunRequest{
		Prompt: prompt,
		Cwd:    t.TempDir(),
	}
	cmd, containerName, err := provider.buildCmd(
		context.Background(),
		req,
		provider.args(req, false),
		func(agent.Event) {},
	)
	if err != nil {
		t.Fatal(err)
	}
	if containerName != "" {
		t.Fatalf("container name = %q, want empty for host", containerName)
	}
	for _, arg := range cmd.Args {
		if strings.Contains(arg, prompt) {
			t.Fatalf("prompt found in cmd.Args: %v", cmd.Args)
		}
	}
	if cmd.Stdin == nil {
		t.Fatal("cmd.Stdin must not be nil")
	}
	stdinBytes, err := io.ReadAll(cmd.Stdin)
	if err != nil {
		t.Fatal(err)
	}
	if string(stdinBytes) != prompt {
		t.Fatalf("cmd.Stdin = %q, want %q", string(stdinBytes), prompt)
	}
	hasXDG := false
	for _, env := range cmd.Env {
		if strings.HasPrefix(env, "XDG_DATA_HOME=") {
			hasXDG = true
			break
		}
	}
	if !hasXDG {
		t.Fatalf("cmd.Env missing XDG_DATA_HOME: %v", cmd.Env)
	}
}
