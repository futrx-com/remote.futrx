package codex

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
	"github.com/futrx-com/remote.futrx.com/internal/integration/agents/codexharness"
	agentruntime "github.com/futrx-com/remote.futrx.com/internal/integration/agents/runtime"
)

func (p *Provider) args(req agent.RunRequest) []string {
	return codexharness.AppServerArgs(nil, req.EnableBrowser)
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
		authPath := codexCredentialPath()
		env := codexEnv(os.Environ())
		if run != nil {
			authPath = filepath.Join(run.hostHome, "auth.json")
			env = isolatedCodexAuthEnvFor(os.Environ(), run.hostHome)
		}
		if err := ensureSubscriptionAuth(authPath); err != nil {
			return nil, "", err
		}
		// The app-server process must outlive request cancellation long enough for
		// runAppServer to send turn/interrupt and receive the terminal status.
		cmd := exec.CommandContext(context.WithoutCancel(ctx), "codex", args...)
		cmd.Dir = cwd
		cmd.Env = agent.WithRuntimeEnvironment(env, req.RuntimeEnv)
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
	prefixEnvironment := []string{"HOME=/root", "CODEX_HOME=/root/.codex"}
	binary := p.profile.CLI.Binary
	arguments := args
	if run != nil {
		prefixEnvironment = []string{"HOME=" + filepath.Dir(run.containerHome), "CODEX_HOME=" + run.containerHome}
		binary = "sh"
		arguments = codexAccountContainerArgs(run.containerHome, p.profile.CLI.Binary, args)
	}
	cmd := agentruntime.BuildContainerCommand(context.WithoutCancel(ctx), agentruntime.ContainerCommandSpec{
		ContainerName:      project.ContainerName,
		PrefixEnvironment:  prefixEnvironment,
		Secrets:            project.Secrets,
		ExcludedSecrets:    []string{"OPENAI_API_KEY"},
		SuffixEnvironment:  []string{"OPENAI_API_KEY="},
		RuntimeEnvironment: req.RuntimeEnv,
		Binary:             binary,
		Arguments:          arguments,
	})
	return cmd, project.ContainerName, nil
}

func ensureHostSubscriptionAuth() error {
	return ensureSubscriptionAuth(codexCredentialPath())
}

func ensureSubscriptionAuth(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil
	}
	mode, _ := raw["auth_mode"].(string)
	mode = strings.TrimSpace(strings.ToLower(mode))
	_, hasAPIKey := raw["OPENAI_API_KEY"]
	if mode == "apikey" || (mode == "" && hasAPIKey) {
		return ErrCodexAPIKeyAuth
	}
	return nil
}

func codexEnv(base []string) []string {
	out := make([]string, 0, len(base)+1)
	hasCodexHome := false
	home := ""
	for _, env := range base {
		if strings.HasPrefix(env, "OPENAI_API_KEY=") {
			continue
		}
		if strings.HasPrefix(env, "CODEX_HOME=") {
			hasCodexHome = true
		}
		if strings.HasPrefix(env, "HOME=") {
			home = strings.TrimPrefix(env, "HOME=")
		}
		out = append(out, env)
	}
	if hasCodexHome {
		return out
	}
	if home != "" {
		return append(out, "CODEX_HOME="+home+"/.codex")
	}
	return out
}
