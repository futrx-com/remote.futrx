package devin

import (
	"context"
	"os"
	"os/exec"
	"strings"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	agentruntime "github.com/futrx-com/remote.futrx.com/internal/integration/agents/runtime"
)

// args builds the `devin acp` argument list. ACP is a stdio JSON-RPC protocol,
// so the only argument is the subcommand. Model and trust flags are passed via
// the CLI's global flags before the subcommand.
func (p *Provider) args(req agent.RunRequest) []string {
	args := []string{"acp"}
	if model := strings.TrimSpace(req.Model); model != "" {
		args = append([]string{"--model", model}, args...)
	}
	// Headless non-interactive mode: do not prompt for workspace trust.
	args = append([]string{"--respect-workspace-trust", "false"}, args...)
	return args
}

// buildCmd constructs the exec.Cmd for a host or project run. The process must
// outlive request cancellation long enough for the harness to send
// session/cancel and receive the terminal stopReason, so the command context
// uses context.WithoutCancel.
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
		cmd := exec.CommandContext(context.WithoutCancel(ctx), p.profile.CLI.Binary, args...)
		cmd.Dir = cwd
		cmd.Env = agent.WithRuntimeEnvironment(devinEnv(os.Environ()), req.RuntimeEnv)
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
	cmd := agentruntime.BuildContainerCommand(context.WithoutCancel(ctx), agentruntime.ContainerCommandSpec{
		ContainerName:      project.ContainerName,
		PrefixEnvironment:  []string{"HOME=/root", "XDG_DATA_HOME=/root/.local/share"},
		Secrets:            project.Secrets,
		RuntimeEnvironment: req.RuntimeEnv,
		Binary:             p.profile.CLI.Binary,
		Arguments:          args,
	})
	return cmd, project.ContainerName, nil
}

// devinEnv prepares the host environment for `devin acp`. It ensures HOME and
// XDG_DATA_HOME point at the root credential directory so the CLI can locate
// credentials.toml without interactive configuration.
func devinEnv(base []string) []string {
	out := make([]string, 0, len(base)+2)
	hasXDG := false
	home := ""
	for _, env := range base {
		if strings.HasPrefix(env, "XDG_DATA_HOME=") {
			hasXDG = true
		}
		if strings.HasPrefix(env, "HOME=") {
			home = strings.TrimPrefix(env, "HOME=")
		}
		out = append(out, env)
	}
	if !hasXDG {
		if home != "" {
			out = append(out, "XDG_DATA_HOME="+home+"/.local/share")
		} else {
			out = append(out, "XDG_DATA_HOME=/root/.local/share")
		}
	}
	return out
}
