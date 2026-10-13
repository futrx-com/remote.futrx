package workspace

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
	"github.com/futrx-com/remote.futrx.com/internal/integration/containers/assets"
	serviceprofiles "github.com/futrx-com/remote.futrx.com/internal/service/container/profiles"
)

func TestInstructionsPublishEachProjectsRoutingSlug(t *testing.T) {
	runner := &instructionRunner{contents: map[string]string{}}
	profiles := serviceprofiles.NewCatalog([]provisioning.Profile{
		{Instructions: &provisioning.InstructionTarget{Path: "/root/.codex/AGENTS.md", HashPath: "/root/.instructions-hash"}},
		{Instructions: &provisioning.InstructionTarget{Path: "/root/.claude/CLAUDE.md", HashPath: "/root/.instructions-hash"}},
	})
	template := provisioning.InstructionsTemplate("remote.example.com")
	provisioner := NewProvisioner(runner, profiles, assets.NewPublisher(runner), template)
	for _, slug := range []string{"first-project", "second-project"} {
		container := "container-" + slug
		if err := provisioner.EnsureAgentInstructions(context.Background(), container, slug); err != nil {
			t.Fatal(err)
		}
		for _, target := range []string{"/root/.codex/AGENTS.md", "/root/.claude/CLAUDE.md"} {
			content := runner.contents[container+target]
			if !strings.Contains(content, "authoritative project slug is `"+slug+"`") || strings.Contains(content, "{{PROJECT_SLUG}}") {
				t.Fatalf("incorrect routing context for %s: %s", container+target, content)
			}
			if slug == "second-project" && strings.Contains(content, "`first-project`") {
				t.Fatal("project context leaked across projects")
			}
		}
	}
	if !strings.Contains(string(template), "{{PROJECT_SLUG}}") {
		t.Fatal("shared template was mutated")
	}
}

type instructionRunner struct {
	skillProvisioningRunner
	contents map[string]string
}

func (r *instructionRunner) Run(ctx context.Context, args ...string) (string, error) {
	if len(args) == 5 && args[0] == "file" && args[1] == "push" {
		content, err := os.ReadFile(args[3])
		if err != nil {
			return "", err
		}
		r.contents[args[4]] = string(content)
	}
	return r.skillProvisioningRunner.Run(ctx, args...)
}
