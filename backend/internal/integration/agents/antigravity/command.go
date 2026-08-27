package antigravity

import (
	"context"
	"os"
	"os/exec"
	"strings"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	agentruntime "github.com/futrx-com/remote.futrx.com/internal/integration/agents/runtime"
)

// printTimeout bounds one agy print-mode run. The CLI default (5m) is far too
// small for real agent work; runs are cancelled by the app's own run lifecycle
// well before this backstop.
const printTimeout = "240m"

func (p *Provider) args(req agent.RunRequest) []string {
	// agy takes the prompt via --print (positional value). Default mode disables
	// interactive permission prompts for headless execution. Plan uses agy's
	// native mode and deliberately omits that bypass.
	args := []string{"--print", req.Prompt, "--print-timeout", printTimeout}
	if req.Mode == agent.RunModePlan {
		args = append(args, "--mode", string(agent.RunModePlan))
	} else {
		args = append(args, "--dangerously-skip-permissions")
	}
	if model := strings.TrimSpace(req.Model); model != "" {
		args = append(args, "--model", model)
	}
	if effort := effortFlag(req.Preferences.ReasoningEffort); effort != "" {
		args = append(args, "--effort", effort)
	}
	if req.ResumeID != "" {
		args = append(args, "--conversation", req.ResumeID)
	}
	return args
}

// effortFlag preserves the legacy mappings for Remote's older shared effort
// ladder, then forwards any other syntactically safe value.
func effortFlag(effort agent.ReasoningEffort) string {
	switch strings.ToLower(strings.TrimSpace(string(effort))) {
	case "none", "minimal", "low":
		return "low"
	case "medium":
		return "medium"
	case "high", "xhigh":
		return "high"
	default:
		return agent.NormalizeCapabilityValue(string(effort))
	}
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
		cmd := exec.CommandContext(ctx, "agy", args...)
		cmd.Dir = cwd
		cmd.Env = agent.WithRuntimeEnvironment(os.Environ(), req.RuntimeEnv)
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
		PrefixEnvironment:  []string{"HOME=" + containerAgentHome},
		Secrets:            project.Secrets,
		RuntimeEnvironment: req.RuntimeEnv,
		Binary:             p.binary,
		Arguments:          args,
	})
	return cmd, project.ContainerName, nil
}
