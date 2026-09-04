package codex

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
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
	cwd := req.Cwd
	if cwd == "" {
		cwd = os.Getenv("HOME")
		if cwd == "" {
			cwd = "/root"
		}
	}

	if req.ProjectID == "" || p.projectPreparer == nil {
		if err := ensureHostSubscriptionAuth(); err != nil {
			return nil, "", err
		}
		// The app-server process must outlive request cancellation long enough for
		// runAppServer to send turn/interrupt and receive the terminal status.
		cmd := exec.CommandContext(context.WithoutCancel(ctx), "codex", args...)
		cmd.Dir = cwd
		cmd.Env = agent.WithRuntimeEnvironment(codexEnv(os.Environ()), req.RuntimeEnv)
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
		PrefixEnvironment:  []string{"HOME=/root", "CODEX_HOME=/root/.codex"},
		Secrets:            project.Secrets,
		ExcludedSecrets:    []string{"OPENAI_API_KEY"},
		SuffixEnvironment:  []string{"OPENAI_API_KEY="},
		RuntimeEnvironment: req.RuntimeEnv,
		Binary:             p.profile.CLI.Binary,
		Arguments:          args,
	})
	return cmd, project.ContainerName, nil
}

func ensureHostSubscriptionAuth() error {
	path := filepath.Join(hostCodexHome(), "auth.json")
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

func hostCodexHome() string {
	if v := os.Getenv("CODEX_HOME"); v != "" {
		return v
	}
	if home := os.Getenv("HOME"); home != "" {
		return filepath.Join(home, ".codex")
	}
	return "/root/.codex"
}
