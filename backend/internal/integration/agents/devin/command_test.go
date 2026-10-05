package devin

import (
	"slices"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
	"github.com/futrx-com/remote.futrx.com/internal/integration/agents/devinharness"
)

func TestArgsIncludeACPSubcommand(t *testing.T) {
	provider := &Provider{profile: provisioning.Profile{CLI: devinharness.NewCLISpec()}}
	args := provider.args(agent.RunRequest{Prompt: "hello"})
	if !slices.Contains(args, "acp") {
		t.Fatalf("args missing 'acp' subcommand: %#v", args)
	}
}

func TestArgsIncludeRespectWorkspaceTrustFalse(t *testing.T) {
	provider := &Provider{profile: provisioning.Profile{CLI: devinharness.NewCLISpec()}}
	args := provider.args(agent.RunRequest{Prompt: "hello"})
	for i, arg := range args {
		if arg == "--respect-workspace-trust" && i+1 < len(args) && args[i+1] == "false" {
			return
		}
	}
	t.Fatalf("args missing --respect-workspace-trust false: %#v", args)
}

func TestArgsIncludeModelWhenSet(t *testing.T) {
	provider := &Provider{profile: provisioning.Profile{CLI: devinharness.NewCLISpec()}}
	args := provider.args(agent.RunRequest{Prompt: "hello", Model: "devin-pro"})
	for i, arg := range args {
		if arg == "--model" && i+1 < len(args) && args[i+1] == "devin-pro" {
			return
		}
	}
	t.Fatalf("args missing --model devin-pro: %#v", args)
}

func TestArgsOmitModelWhenEmpty(t *testing.T) {
	provider := &Provider{profile: provisioning.Profile{CLI: devinharness.NewCLISpec()}}
	args := provider.args(agent.RunRequest{Prompt: "hello"})
	if slices.Contains(args, "--model") {
		t.Fatalf("args should not contain --model when model is empty: %#v", args)
	}
}

func TestArgsPlaceGlobalFlagsBeforeSubcommand(t *testing.T) {
	provider := &Provider{profile: provisioning.Profile{CLI: devinharness.NewCLISpec()}}
	args := provider.args(agent.RunRequest{Prompt: "hello", Model: "devin-pro"})
	acpIdx := slices.Index(args, "acp")
	modelIdx := slices.Index(args, "--model")
	trustIdx := slices.Index(args, "--respect-workspace-trust")
	if acpIdx < 0 || modelIdx < 0 || trustIdx < 0 {
		t.Fatalf("missing expected args: %#v", args)
	}
	if modelIdx > acpIdx || trustIdx > acpIdx {
		t.Fatalf("global flags must precede 'acp' subcommand: %#v", args)
	}
}

func TestDevinEnvSetsXDGDataHome(t *testing.T) {
	env := devinEnv([]string{"HOME=/root", "PATH=/usr/bin"})
	found := false
	for _, entry := range env {
		if entry == "XDG_DATA_HOME=/root/.local/share" {
			found = true
		}
	}
	if !found {
		t.Fatalf("XDG_DATA_HOME not set: %#v", env)
	}
}

func TestDevinEnvPreservesExistingXDGDataHome(t *testing.T) {
	env := devinEnv([]string{"HOME=/root", "XDG_DATA_HOME=/custom/xdg"})
	for _, entry := range env {
		if entry == "XDG_DATA_HOME=/custom/xdg" {
			return
		}
	}
	t.Fatalf("existing XDG_DATA_HOME not preserved: %#v", env)
}
