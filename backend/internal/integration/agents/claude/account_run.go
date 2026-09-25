package claude

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
	agentruntime "github.com/futrx-com/remote.futrx.com/internal/integration/agents/runtime"
	agentauth "github.com/futrx-com/remote.futrx.com/internal/service/agent/auth"
)

const (
	containerClaudeHome  = "/root/.claude"
	claudeAccountCommand = `set -eu
run_home=$1
shared_home=$2
shift 2
for name in CLAUDE.md settings.json settings.local.json commands skills plugins; do
  target="$shared_home/$name"
  link="$run_home/$name"
  if { [ -e "$target" ] || [ -L "$target" ]; } && [ ! -e "$link" ] && [ ! -L "$link" ]; then
    ln -s "$target" "$link"
  fi
done
exec "$@"`
)

type accountRun struct {
	saved         agentauth.RunCredential
	hostHome      string
	containerHome string
	credentials   provisioning.CredentialSpec
}

func newAccountRun(req agent.RunRequest, saved agentauth.RunCredential) (*accountRun, error) {
	scope := agentruntime.AccountHomeScope(saved.AccountID, req.ProjectID, req.ConversationID)
	hostHome := filepath.Join(claudeHomeDir(), "run-accounts", scope)
	containerHome := filepath.Join(containerClaudeHome, "run-accounts", scope)
	if err := os.MkdirAll(hostHome, 0o700); err != nil {
		return nil, fmt.Errorf("create isolated Claude account home: %w", err)
	}
	if err := os.Chmod(hostHome, 0o700); err != nil {
		return nil, fmt.Errorf("secure isolated Claude account home: %w", err)
	}
	if err := writeAccountCredential(
		filepath.Join(hostHome, ".credentials.json"),
		filepath.Join(hostHome, ".claude.json"),
		saved.Credential,
	); err != nil {
		return nil, fmt.Errorf("materialize Claude account: %w", err)
	}
	if err := linkClaudeSharedState(hostHome, claudeHomeDir()); err != nil {
		return nil, err
	}
	return &accountRun{
		saved: saved, hostHome: hostHome, containerHome: containerHome,
		credentials: provisioning.CredentialSpec{
			Name: "claude-run-account", HostDir: hostHome, ContainerDir: containerHome,
			Files: []provisioning.CredentialFile{
				{
					HostPath: filepath.Join(hostHome, ".claude.json"), ContainerPath: filepath.Join(containerHome, ".claude.json"),
					Mode: "600", PushRequired: true, PullRequired: true, HostAuthoritative: true,
				},
				{
					HostPath: filepath.Join(hostHome, ".credentials.json"), ContainerPath: filepath.Join(containerHome, ".credentials.json"),
					Mode: "600", PushRequired: true, PullRequired: true, HostAuthoritative: true,
				},
			},
		},
	}, nil
}

func linkClaudeSharedState(runHome, sharedHome string) error {
	for _, name := range []string{"CLAUDE.md", "settings.json", "settings.local.json", "commands", "skills", "plugins"} {
		target := filepath.Join(sharedHome, name)
		if _, err := os.Lstat(target); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("inspect shared Claude state %s: %w", name, err)
		}
		link := filepath.Join(runHome, name)
		if _, err := os.Lstat(link); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect isolated Claude state %s: %w", name, err)
		}
		if err := os.Symlink(target, link); err != nil {
			return fmt.Errorf("link shared Claude state %s: %w", name, err)
		}
	}
	return nil
}

func (r *accountRun) hostCredential() ([]byte, error) {
	return readAccountCredential(
		filepath.Join(r.hostHome, ".credentials.json"),
		filepath.Join(r.hostHome, ".claude.json"),
	)
}

func claudeAccountContainerArgs(runHome, binary string, args []string) []string {
	wrapped := []string{"-c", claudeAccountCommand, "remote-claude-account", runHome, containerClaudeHome, binary}
	return append(wrapped, args...)
}
