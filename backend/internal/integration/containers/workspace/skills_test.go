package workspace

import (
	"context"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
	"github.com/futrx-com/remote.futrx.com/internal/integration/containers/assets"
	serviceprofiles "github.com/futrx-com/remote.futrx.com/internal/service/container/profiles"
)

func TestWorkspaceSkillLinksScriptUsesConfiguredProfiles(t *testing.T) {
	profiles := []provisioning.Profile{
		{WorkspaceSkills: &provisioning.WorkspaceSkills{WorkspaceHome: "/workspace/.alpha"}},
		{WorkspaceSkills: &provisioning.WorkspaceSkills{WorkspaceHome: "/workspace/.beta", HomeSkillsDir: "/root/.beta/skills"}},
	}
	script := workspaceSkillLinksScript(profiles)

	for _, want := range []string{
		"migrate_skills_dir '/workspace/.alpha/skills'",
		"migrate_skills_dir '/workspace/.beta/skills'",
		"link_skills_dir '/workspace/.alpha' '../.agents/skills'",
		"link_skills_dir '/workspace/.beta' '../.agents/skills'",
		"mirror_home_skills '/root/.beta/skills'",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("workspace skills script is missing %q", want)
		}
	}
}

func TestEnsureSkillLinksPublishesEmbeddedDefaultSkill(t *testing.T) {
	runner := &skillProvisioningRunner{}
	provisioner := NewProvisioner(
		runner,
		serviceprofiles.NewCatalog(nil),
		assets.NewPublisher(runner),
		nil,
	)

	if err := provisioner.EnsureSkillLinks(context.Background(), "project-container"); err != nil {
		t.Fatal(err)
	}

	for _, destination := range []string{
		"project-container/workspace/.agents/skills/remote-application/SKILL.md",
		"project-container/workspace/.agents/skills/remote-application/agents/openai.yaml",
		"project-container/workspace/.agents/skills/remote-application/references/capability-selection.md",
	} {
		if !slices.ContainsFunc(runner.calls, func(call string) bool {
			return strings.Contains(call, "file push --mode=644") && strings.Contains(call, destination)
		}) {
			t.Fatalf("default skill destination %q missing from calls %#v", destination, runner.calls)
		}
	}

	topologyCalls := 0
	for _, call := range runner.calls {
		if strings.Contains(call, "exec project-container -- sh -c") {
			topologyCalls++
		}
	}
	if topologyCalls != 2 {
		t.Fatalf("skill topology calls = %d, want 2 around default publication; calls = %#v", topologyCalls, runner.calls)
	}
}

type skillProvisioningRunner struct {
	calls []string
}

func (*skillProvisioningRunner) Available() bool { return true }

func (r *skillProvisioningRunner) Run(_ context.Context, args ...string) (string, error) {
	r.calls = append(r.calls, strings.Join(args, " "))
	return "", nil
}

func (r *skillProvisioningRunner) RunStdin(_ context.Context, _ io.Reader, args ...string) (string, error) {
	r.calls = append(r.calls, strings.Join(args, " "))
	return "", nil
}
