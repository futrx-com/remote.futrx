package codex

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
	agentruntime "github.com/futrx-com/remote.futrx.com/internal/integration/agents/runtime"
	agentauth "github.com/futrx-com/remote.futrx.com/internal/service/agent/auth"
)

const codexAccountCommand = `set -eu
run_home=$1
shared_home=$2
shift 2
for name in AGENTS.md config.toml skills; do
  target="$shared_home/$name"
  link="$run_home/$name"
  if { [ -e "$target" ] || [ -L "$target" ]; } && [ ! -e "$link" ] && [ ! -L "$link" ]; then
    ln -s "$target" "$link"
  fi
done
exec "$@"`

type accountRun struct {
	saved         agentauth.RunCredential
	hostHome      string
	containerHome string
	credentials   provisioning.CredentialSpec
}

func newAccountRun(req agent.RunRequest, saved agentauth.RunCredential) (*accountRun, error) {
	scope := agentruntime.AccountHomeScope(saved.AccountID, req.ProjectID, req.ConversationID)
	hostHome := filepath.Join(codexHomeDir(), "run-accounts", scope, ".codex")
	containerHome := filepath.Join(containerCodexDir, "run-accounts", scope, ".codex")
	if err := os.MkdirAll(hostHome, 0o700); err != nil {
		return nil, fmt.Errorf("create isolated Codex account home: %w", err)
	}
	if err := os.Chmod(hostHome, 0o700); err != nil {
		return nil, fmt.Errorf("secure isolated Codex account home: %w", err)
	}
	if err := agentauth.WriteCredentialFile(filepath.Join(hostHome, "auth.json"), saved.Credential); err != nil {
		return nil, fmt.Errorf("materialize Codex account: %w", err)
	}
	if err := linkCodexSharedState(hostHome, codexHomeDir()); err != nil {
		return nil, err
	}
	return &accountRun{
		saved: saved, hostHome: hostHome, containerHome: containerHome,
		credentials: provisioning.CredentialSpec{
			Name: "codex-run-account", HostDir: hostHome, ContainerDir: containerHome,
			Files: []provisioning.CredentialFile{{
				HostPath: filepath.Join(hostHome, "auth.json"), ContainerPath: filepath.Join(containerHome, "auth.json"),
				Mode: "600", PushRequired: true, PullRequired: true, HostAuthoritative: true,
			}},
		},
	}, nil
}

func linkCodexSharedState(runHome, sharedHome string) error {
	for _, name := range []string{"AGENTS.md", "config.toml", "skills"} {
		target := filepath.Join(sharedHome, name)
		if _, err := os.Lstat(target); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("inspect shared Codex state %s: %w", name, err)
		}
		link := filepath.Join(runHome, name)
		if _, err := os.Lstat(link); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect isolated Codex state %s: %w", name, err)
		}
		if err := os.Symlink(target, link); err != nil {
			return fmt.Errorf("link shared Codex state %s: %w", name, err)
		}
	}
	return nil
}

func (r *accountRun) hostCredential() ([]byte, error) {
	return os.ReadFile(filepath.Join(r.hostHome, "auth.json"))
}

func codexAccountContainerArgs(runHome, binary string, args []string) []string {
	wrapped := []string{"-c", codexAccountCommand, "remote-codex-account", runHome, containerCodexDir, binary}
	return append(wrapped, args...)
}
