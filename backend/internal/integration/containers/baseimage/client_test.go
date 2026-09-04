package baseimage

import (
	"context"
	"errors"
	"io"
	"slices"
	"testing"
)

type recordingRunner struct {
	available bool
	output    string
	err       error
	calls     [][]string
}

func (r *recordingRunner) Available() bool { return r.available }

func (r *recordingRunner) Run(_ context.Context, args ...string) (string, error) {
	r.calls = append(r.calls, slices.Clone(args))
	return r.output, r.err
}

func (r *recordingRunner) RunStdin(ctx context.Context, _ io.Reader, args ...string) (string, error) {
	return r.Run(ctx, args...)
}

func TestClientTranslatesImageOperations(t *testing.T) {
	tests := []struct {
		name     string
		invoke   func(*Client) (string, error)
		wantArgs []string
	}{
		{
			name: "delete container",
			invoke: func(client *Client) (string, error) {
				return client.DeleteContainer(context.Background(), "builder")
			},
			wantArgs: []string{"delete", "--force", "builder"},
		},
		{
			name: "launch container",
			invoke: func(client *Client) (string, error) {
				return client.LaunchContainer(context.Background(), "ubuntu:24.04", "builder")
			},
			wantArgs: []string{"launch", "ubuntu:24.04", "builder"},
		},
		{
			name: "execute script",
			invoke: func(client *Client) (string, error) {
				return client.ExecuteScript(context.Background(), "builder", "set -e\necho ready")
			},
			wantArgs: []string{"exec", "builder", "--", "bash", "-c", "set -e\necho ready"},
		},
		{
			name: "stop container",
			invoke: func(client *Client) (string, error) {
				return client.StopContainer(context.Background(), "builder")
			},
			wantArgs: []string{"stop", "builder"},
		},
		{
			name: "publish image",
			invoke: func(client *Client) (string, error) {
				return client.PublishImage(context.Background(), "builder", "remote-base", "base description")
			},
			wantArgs: []string{"publish", "builder", "--alias", "remote-base", "--compression", "none", "description=base description"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runnerErr := errors.New("exit 1")
			runner := &recordingRunner{available: true, output: "command output", err: runnerErr}
			client := NewClient(runner)

			if !client.Available() {
				t.Fatal("Available() = false, want true")
			}
			output, err := tt.invoke(client)
			if output != runner.output || !errors.Is(err, runnerErr) {
				t.Fatalf("operation = (%q, %v), want (%q, %v)", output, err, runner.output, runnerErr)
			}
			if len(runner.calls) != 1 || !slices.Equal(runner.calls[0], tt.wantArgs) {
				t.Fatalf("calls = %q, want [%q]", runner.calls, tt.wantArgs)
			}
		})
	}
}
