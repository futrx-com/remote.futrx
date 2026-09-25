package claude

import (
	"context"
	"os"
	"os/exec"
	"strings"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
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
	return p.buildCmdForAccount(ctx, req, args, emit, nil)
}

func (p *Provider) buildCmdForAccount(
	ctx context.Context,
	req agent.RunRequest,
	args []string,
	emit func(agent.Event),
	run *accountRun,
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
		env := os.Environ()
		if run != nil {
			env = isolatedClaudeAuthEnvFor(env, run.hostHome)
		}
		cmd.Env = append(env, "IS_SANDBOX=1")
		cmd.Env = agent.WithRuntimeEnvironment(cmd.Env, req.RuntimeEnv)
		cmd.Stdin = strings.NewReader(req.Prompt)
		return cmd, "", nil
	}

	var credentials *provisioning.CredentialSpec
	if run != nil {
		value := run.credentials.Clone()
		credentials = &value
	}
	project, err := p.projectPreparer.Prepare(ctx, agent.ProjectPreparationRequest{
		ProjectID:           agent.ProjectID(req.ProjectID),
		ConversationID:      req.ConversationID,
		EnableBrowser:       req.EnableBrowser,
		EnableScheduleTools: req.EnableScheduleTools,
		Credentials:         credentials,
	}, emit)
	if err != nil {
		return nil, "", err
	}
	prefixEnvironment := []string{"IS_SANDBOX=1", "HOME=/root"}
	var excludedSecrets []string
	var suffixEnvironment []string
	binary := p.profile.CLI.Binary
	arguments := args
	if run != nil {
		prefixEnvironment = append(prefixEnvironment, "CLAUDE_CONFIG_DIR="+run.containerHome)
		excludedSecrets = []string{"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "CLAUDE_CODE_OAUTH_TOKEN"}
		suffixEnvironment = []string{"ANTHROPIC_API_KEY=", "ANTHROPIC_AUTH_TOKEN=", "CLAUDE_CODE_OAUTH_TOKEN="}
		binary = "sh"
		arguments = claudeAccountContainerArgs(run.containerHome, p.profile.CLI.Binary, args)
	}
	cmd := agentruntime.BuildContainerCommand(ctx, agentruntime.ContainerCommandSpec{
		ContainerName:      project.ContainerName,
		PrefixEnvironment:  prefixEnvironment,
		Secrets:            project.Secrets,
		ExcludedSecrets:    excludedSecrets,
		SuffixEnvironment:  suffixEnvironment,
		RuntimeEnvironment: req.RuntimeEnv,
		Binary:             binary,
		Arguments:          arguments,
	})
	cmd.Stdin = strings.NewReader(req.Prompt)
	return cmd, project.ContainerName, nil
}
