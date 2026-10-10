package workspace

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
	"github.com/futrx-com/remote.futrx.com/internal/integration/containers/assets"
	serviceprofiles "github.com/futrx-com/remote.futrx.com/internal/service/container/profiles"
)

type instructionRunner struct {
	files   map[string]string
	markers map[string]string
}

func (*instructionRunner) Available() bool { return true }
func (r *instructionRunner) Run(_ context.Context, args ...string) (string, error) {
	if args[0] == "file" && args[1] == "push" {
		content, err := os.ReadFile(args[3])
		if err != nil {
			return "", err
		}
		r.files[args[4]] = string(content)
	}
	if args[0] == "exec" && args[3] == "cat" {
		return r.markers[args[4]], nil
	}
	return "", nil
}
func (r *instructionRunner) RunStdin(_ context.Context, input io.Reader, args ...string) (string, error) {
	content, err := io.ReadAll(input)
	if err != nil {
		return "", err
	}
	r.markers[args[4]] = string(content)
	return "", nil
}

func TestInstructionsPublishProviderContentAndUpdateExistingContainers(t *testing.T) {
	runner := &instructionRunner{files: map[string]string{}, markers: map[string]string{}}
	profiles := []provisioning.Profile{
		{ID: "claude", Instructions: &provisioning.InstructionTarget{Path: "/root/.claude/CLAUDE.md", HashPath: "/root/.claude/hash", Content: []byte("global\nclaude")}},
		{ID: "codex", Instructions: &provisioning.InstructionTarget{Path: "/root/.codex/AGENTS.md", HashPath: "/root/.codex/hash"}},
	}
	provision := func() {
		t.Helper()
		p := NewProvisioner(runner, serviceprofiles.NewCatalog(profiles), assets.NewPublisher(runner), []byte("global"))
		if err := p.EnsureAgentInstructions(context.Background(), "c1"); err != nil {
			t.Fatal(err)
		}
	}
	provision()
	if runner.files["c1/root/.claude/CLAUDE.md"] != "global\nclaude" || runner.files["c1/root/.codex/AGENTS.md"] != "global" {
		t.Fatal(runner.files)
	}
	if runner.markers["/root/.claude/hash"] != assets.Hash([]byte("global\nclaude")) {
		t.Fatal("incorrect provider marker")
	}
	profiles[0].Instructions.Content = []byte("updated")
	provision()
	if runner.files["c1/root/.claude/CLAUDE.md"] != "updated" {
		t.Fatal("did not replace stale instructions")
	}
	for destination := range runner.files {
		if strings.Contains(destination, "/workspace/") {
			t.Fatal("overwrote project-owned instructions")
		}
	}
}

func TestInstructionsRejectConflictingSharedMarkers(t *testing.T) {
	runner := &instructionRunner{files: map[string]string{}, markers: map[string]string{}}
	profiles := []provisioning.Profile{
		{Instructions: &provisioning.InstructionTarget{Path: "/root/a", HashPath: "/root/hash", Content: []byte("a")}},
		{Instructions: &provisioning.InstructionTarget{Path: "/root/b", HashPath: "/root/hash", Content: []byte("b")}},
	}
	p := NewProvisioner(runner, serviceprofiles.NewCatalog(profiles), assets.NewPublisher(runner), []byte("global"))
	if err := p.EnsureAgentInstructions(context.Background(), "c1"); err == nil {
		t.Fatal("accepted conflicting marker")
	}
	if len(runner.files) != 0 {
		t.Fatal("partially published conflicting content")
	}
}
