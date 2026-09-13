package workspace

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/integration/containers/command"
)

const gitIdentityTimeout = 10 * time.Second
const globalGitConfigPath = "/root/.gitconfig"

type GitIdentity struct {
	Name  string
	Email string
}

// EnsureGitIdentity converges only Git's non-secret author metadata. Provider
// tokens, SSH keys, and credential helpers are deliberately outside its scope.
func (p *Provisioner) EnsureGitIdentity(ctx context.Context, containerName string) error {
	if !p.runner.Available() {
		return command.ErrUnavailable
	}
	identity := GitIdentity{
		Name:  strings.TrimSpace(p.gitIdentity.Name),
		Email: strings.TrimSpace(p.gitIdentity.Email),
	}
	if identity.Name == "" || identity.Email == "" {
		return errors.New("git user name and email are required")
	}
	if strings.ContainsAny(identity.Name+identity.Email, "\r\n") {
		return errors.New("git identity cannot contain line breaks")
	}

	for _, setting := range [][2]string{
		{"user.name", identity.Name},
		{"user.email", identity.Email},
	} {
		current, err := command.RunWithTimeout(
			ctx, p.runner, gitIdentityTimeout,
			"exec", containerName, "--env", "HOME=/root", "--",
			"git", "config", "--file", globalGitConfigPath, "--get", setting[0],
		)
		if err == nil && strings.TrimSpace(current) == setting[1] {
			continue
		}
		if output, err := command.RunWithTimeout(
			ctx, p.runner, gitIdentityTimeout,
			"exec", containerName, "--env", "HOME=/root", "--",
			"git", "config", "--file", globalGitConfigPath, setting[0], setting[1],
		); err != nil {
			return fmt.Errorf("configure git %s: %w; output: %s", setting[0], err, output)
		}
	}
	return nil
}
