package opencode

import (
	"context"
	"os"
	"os/exec"
	"strings"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	agentruntime "github.com/futrx-com/remote.futrx.com/internal/integration/agents/runtime"
)

func (p *Provider) args(req agent.RunRequest, isContainer bool) []string {
	// `opencode run --format json` streams one JSON event per line on stdout.
	// The prompt is passed via stdin (NOT as a positional argument) so opencode
	// does not mangle whitespace, escape quotes, or interpret leading flags.
	args := []string{"run", "--format", "json"}
	if req.Mode == agent.RunModePlan {
		// OpenCode ships a built-in read-only `plan` agent.
		args = append(args, "--agent", "plan")
	} else if isContainer {
		// In project containers, auto-approve permissions so tool execution
		// is not rejected in non-interactive headless mode.
		args = append(args, "--auto")
	}
	if model := strings.TrimSpace(req.Model); model != "" {
		args = append(args, "--model", model)
	}
	if req.ResumeID != "" {
		args = append(args, "--session", req.ResumeID)
		// OpenCode forks natively when continuing with --fork.
		if req.Fork {
			args = append(args, "--fork")
		}
	}
	return args
}

func (p *Provider) buildCmd(
	ctx context.Context,
	req agent.RunRequest,
	args []string,
	emit func(agent.Event),
) (*exec.Cmd, string, error) {
	cwd := req.Cwd
	if cwd == "" {
		cwd = os.Getenv("HOME")
		if cwd == "" {
			cwd = "/root"
		}
	}

	if req.ProjectID == "" || p.projectPreparer == nil {
		cmd := exec.CommandContext(ctx, "opencode", args...)
		cmd.Dir = cwd
		cmd.Stdin = strings.NewReader(req.Prompt)
		cmd.Env = agent.WithRuntimeEnvironment(opencodeEnv(os.Environ()), req.RuntimeEnv)
		return cmd, "", nil
	}

	project, err := p.projectPreparer.Prepare(ctx, agent.ProjectPreparationRequest{
		ProjectID:           agent.ProjectID(req.ProjectID),
		ConversationID:      req.ConversationID,
		EnableBrowser:       req.EnableBrowser,
		EnableScheduleTools: req.EnableScheduleTools,
	}, emit)
	if err != nil {
		return nil, "", err
	}
	cmd := agentruntime.BuildContainerCommand(ctx, agentruntime.ContainerCommandSpec{
		ContainerName:      project.ContainerName,
		PrefixEnvironment:  []string{"HOME=/root", "XDG_DATA_HOME=/root/.local/share"},
		Secrets:            project.Secrets,
		RuntimeEnvironment: req.RuntimeEnv,
		Binary:             p.profile.CLI.Binary,
		Arguments:          args,
	})
	cmd.Stdin = strings.NewReader(req.Prompt)
	return cmd, project.ContainerName, nil
}
