package claude

import (
	"context"
	"os"
	"os/exec"
	"strings"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	agentruntime "github.com/futrx-com/remote.futrx.com/internal/integration/agents/runtime"
)

func (p *Provider) args(req agent.RunRequest) []string {
	args := []string{
		"-p",
		"--output-format", "stream-json",
		"--include-partial-messages",
		"--verbose",
	}
	if req.Mode == agent.RunModePlan {
		args = append(args, "--permission-mode", string(agent.RunModePlan))
	} else {
		args = append(args, "--dangerously-skip-permissions")
	}
	if model := normalizeModelSelection(req.Model); model != "" {
		args = append(args, "--model", model)
	}
	if effort := reasoningEffortArg(req.Preferences.ReasoningEffort); effort != "" {
		args = append(args, "--effort", effort)
	}
	if req.Preferences.ServiceTier == agent.ServiceTier(fastServiceTier) {
		args = append(args, "--settings", `{"fastMode":true}`)
	}
	if req.ResumeID != "" {
		args = append(args, "--resume", req.ResumeID)
		if req.Fork {
			args = append(args, "--fork-session")
		}
	}
	if req.EnableBrowser {
		args = append(args, "--mcp-config", browserMCPConfigPath)
	}
	return args
}

// reasoningEffortArg syntax-checks the selected or saved value. Empty or
// malformed values omit the flag so the CLI picks a default.
func reasoningEffortArg(effort agent.ReasoningEffort) string {
	return agent.NormalizeCapabilityValue(string(effort))
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
		cmd := exec.CommandContext(ctx, "claude", args...)
		cmd.Dir = cwd
		// IS_SANDBOX=1 lets `claude --dangerously-skip-permissions` run under
		// uid 0. The box is single-user and the UI is auto-approve.
		cmd.Env = append(os.Environ(), "IS_SANDBOX=1")
		cmd.Env = agent.WithRuntimeEnvironment(cmd.Env, req.RuntimeEnv)
		cmd.Stdin = strings.NewReader(req.Prompt)
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
		PrefixEnvironment:  []string{"IS_SANDBOX=1", "HOME=/root"},
		Secrets:            project.Secrets,
		RuntimeEnvironment: req.RuntimeEnv,
		Binary:             p.profile.CLI.Binary,
		Arguments:          args,
	})
	cmd.Stdin = strings.NewReader(req.Prompt)
	return cmd, project.ContainerName, nil
}
