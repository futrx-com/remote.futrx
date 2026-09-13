package codeserver

import (
	"context"
	"io"
	"slices"
	"strings"
	"testing"
)

type recordingRunner struct {
	calls [][]string
}

func (*recordingRunner) Available() bool { return true }

func (r *recordingRunner) Run(_ context.Context, args ...string) (string, error) {
	r.calls = append(r.calls, slices.Clone(args))
	return "", nil
}

func (r *recordingRunner) RunStdin(ctx context.Context, _ io.Reader, args ...string) (string, error) {
	return r.Run(ctx, args...)
}

func TestEnsureConfiguresProjectPreviewTemplate(t *testing.T) {
	runner := &recordingRunner{}
	provisioner := NewProvisioner(runner, "remote.example.test")

	if err := provisioner.Ensure(context.Background(), "project-1", "My Project", "my-project"); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	wantEnv := "CODE_SERVER_PROXY_URI=https://my-project--{{port}}.dev.remote.example.test"
	wantViteEnv := "VITE_ALLOWED_HOST=.dev.remote.example.test"
	foundProxy, foundVite, foundGitEnvironment, foundGitHubAuthSetting := false, false, false, false
	for _, call := range runner.calls {
		for _, arg := range call {
			if arg == wantEnv {
				foundProxy = true
			}
			if arg == wantViteEnv {
				foundVite = true
			}
			if strings.Contains(arg, "Environment=HOME=/root") &&
				strings.Contains(arg, "Environment=GIT_CONFIG_GLOBAL=/root/.gitconfig") {
				foundGitEnvironment = true
			}
			if strings.Contains(arg, `settings["github.gitAuthentication"] = false`) {
				foundGitHubAuthSetting = true
			}
		}
	}
	if !foundProxy {
		t.Fatalf("preview template %q missing from calls: %#v", wantEnv, runner.calls)
	}
	if !foundVite {
		t.Fatalf("Vite allowed host %q missing from calls: %#v", wantViteEnv, runner.calls)
	}
	if !foundGitEnvironment {
		t.Fatalf("code-server Git environment missing from calls: %#v", runner.calls)
	}
	if !foundGitHubAuthSetting {
		t.Fatalf("code-server GitHub authentication setting missing from calls: %#v", runner.calls)
	}
}
