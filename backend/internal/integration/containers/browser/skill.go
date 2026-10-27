package browser

// Browser-skill provisioning: ships the `browser` SKILL.md into the workspace
// so it shows up in the skill picker and is available to the agent. The skill
// holds the browser playbook (how to use the browser_* tools, the login
// handoff, the write-approval policy). Provider factories declare whether
// shared preparation should also wire MCP/core when the skill is selected.
// Provisioned at container launch like the browser script, so every project
// has it without bloating AGENTS.md.

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/futrx-com/remote.futrx.com/internal/integration/containers/command"
)

//go:embed assets/skills/browser/SKILL.md
var browserSkillTemplate []byte

const (
	containerBrowserSkillDir  = "/workspace/.agents/skills/browser"
	containerBrowserSkillMD   = containerBrowserSkillDir + "/SKILL.md"
	containerBrowserSkillHash = containerBrowserSkillDir + "/.skill.sha256"
)

// EnsureBrowserSkill provisions the `browser` skill into the workspace skills
// directory. Idempotent: re-pushed only when the embedded SKILL.md changes.
func (a *Adapter) EnsureSkill(ctx context.Context, containerName string) error {
	if !a.runner.Available() {
		return command.ErrUnavailable
	}
	out, err := command.RunWithTimeout(ctx, a.runner, queryTimeout, "exec", containerName, "--", "install", "-d", "-m", "755", containerBrowserSkillDir)
	if err != nil {
		return fmt.Errorf("mkdir %s: %w; output: %s", containerBrowserSkillDir, err, out)
	}
	return a.publisher.Push(ctx, containerName, browserSkillTemplate, containerBrowserSkillHash, "644", containerBrowserSkillMD)
}
