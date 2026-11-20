package kimi

import (
	"context"
	"os"
	"os/exec"

	harnessruntime "github.com/futrx-com/remote.futrx.com/internal/integration/harnesses/runtime"
	"github.com/futrx-com/remote.futrx.com/internal/service/harness"
)

// containerKimiHome is the KIMI_CODE_HOME inside a project container — where
// kimi-code reads its OAuth credentials, config, and sessions.
const containerKimiHome = "/root/.kimi-code"

func (p *Provider) args(req harness.RunRequest) []string {
	// kimi-code takes the prompt as a positional argument (NOT stdin). Print
	// mode (`-p`) supplies the provider's normal non-interactive behavior.
	args := []string{"-p", req.Prompt, "--output-format", "stream-json"}
	if req.Mode == harness.RunModePlan {
		args = append(args, "--plan")
	}
	if model := normalizeKimiModel(req.Model); model != "" {
		args = append(args, "--model", model)
	}
	if req.ResumeID != "" {
		args = append(args, "--session", req.ResumeID)
	}
	return args
}

func (p *Provider) buildCmd(
	ctx context.Context,
	req harness.RunRequest,
	args []string,
	emit func(harness.Event),
) (*exec.Cmd, string, error) {
	cwd := req.Cwd
	if cwd == "" {
		cwd = os.Getenv("HOME")
		if cwd == "" {
			cwd = "/root"
		}
	}

	if req.ProjectID == "" || p.projectPreparer == nil {
		cmd := exec.CommandContext(ctx, "kimi", args...)
		cmd.Dir = cwd
		cmd.Env = append(os.Environ(), "KIMI_CODE_HOME="+hostKimiHome())
		cmd.Env = harness.WithRuntimeEnvironment(cmd.Env, req.RuntimeEnv)
		return cmd, "", nil
	}

	project, err := p.projectPreparer.Prepare(ctx, harness.ProjectPreparationRequest{
		ProjectID:      harness.ProjectID(req.ProjectID),
		ConversationID: req.ConversationID,
		EnableBrowser:  req.EnableBrowser,
	}, emit)
	if err != nil {
		return nil, "", err
	}
	cmd := harnessruntime.BuildContainerCommand(ctx, harnessruntime.ContainerCommandSpec{
		ContainerName:      project.ContainerName,
		PrefixEnvironment:  []string{"HOME=/root", "KIMI_CODE_HOME=" + containerKimiHome},
		Secrets:            project.Secrets,
		RuntimeEnvironment: req.RuntimeEnv,
		Binary:             p.profile.CLI.Binary,
		Arguments:          args,
	})
	return cmd, project.ContainerName, nil
}

func hostKimiHome() string {
	if v := os.Getenv("KIMI_CODE_HOME"); v != "" {
		return v
	}
	if home := os.Getenv("HOME"); home != "" {
		return home + "/.kimi-code"
	}
	return "/root/.kimi-code"
}
