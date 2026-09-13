package workspace

import (
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
)

type gitIdentityRunner struct {
	current map[string]string
	calls   []string
}

func (r *gitIdentityRunner) Available() bool { return true }

func (r *gitIdentityRunner) Run(_ context.Context, args ...string) (string, error) {
	r.calls = append(r.calls, strings.Join(args, " "))
	if len(args) >= 2 && args[len(args)-2] == "--get" {
		value, ok := r.current[args[len(args)-1]]
		if !ok {
			return "", errors.New("not configured")
		}
		return value, nil
	}
	return "", nil
}

func (r *gitIdentityRunner) RunStdin(context.Context, io.Reader, ...string) (string, error) {
	return "", nil
}

func newGitIdentityProvisioner(runner *gitIdentityRunner, identity GitIdentity) *Provisioner {
	return NewProvisioner(
		runner,
		nil,
		nil,
		nil,
		identity,
	)
}

func TestMissingGitIdentityIsConfiguredForRootHome(t *testing.T) {
	runner := &gitIdentityRunner{current: map[string]string{}}
	provisioner := newGitIdentityProvisioner(runner, GitIdentity{
		Name:  "Example Developer",
		Email: "developer@example.com",
	})

	if err := provisioner.EnsureGitIdentity(context.Background(), "project-1"); err != nil {
		t.Fatalf("ensure git identity: %v", err)
	}

	want := []string{
		"exec project-1 --env HOME=/root -- git config --file /root/.gitconfig --get user.name",
		"exec project-1 --env HOME=/root -- git config --file /root/.gitconfig user.name Example Developer",
		"exec project-1 --env HOME=/root -- git config --file /root/.gitconfig --get user.email",
		"exec project-1 --env HOME=/root -- git config --file /root/.gitconfig user.email developer@example.com",
	}
	if !slices.Equal(runner.calls, want) {
		t.Fatalf("git config calls = %q, want %q", runner.calls, want)
	}
}

func TestMatchingGitIdentityDoesNotRewriteGlobalConfig(t *testing.T) {
	runner := &gitIdentityRunner{current: map[string]string{
		"user.name":  "Example Developer\n",
		"user.email": "developer@example.com\n",
	}}
	provisioner := newGitIdentityProvisioner(runner, GitIdentity{
		Name:  "Example Developer",
		Email: "developer@example.com",
	})

	if err := provisioner.EnsureGitIdentity(context.Background(), "project-1"); err != nil {
		t.Fatalf("ensure git identity: %v", err)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("git config call count = %d, want two reads and no writes: %q", len(runner.calls), runner.calls)
	}
}
