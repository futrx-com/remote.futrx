package lifecycle

import (
	"context"
	"errors"
	"io"
	"slices"
	"testing"
	"time"

	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
)

type runnerResponse struct {
	out string
	err error
}

type timeoutThenMissingRunner struct {
	calls [][]string
}

func (r *timeoutThenMissingRunner) Available() bool { return true }

func (r *timeoutThenMissingRunner) Run(ctx context.Context, args ...string) (string, error) {
	r.calls = append(r.calls, slices.Clone(args))
	if args[0] == "delete" {
		<-ctx.Done()
		return "", ctx.Err()
	}
	return "Error: Instance not found", errors.New("exit 1")
}

func (r *timeoutThenMissingRunner) RunStdin(ctx context.Context, _ io.Reader, args ...string) (string, error) {
	return r.Run(ctx, args...)
}

type recordingRunner struct {
	available bool
	responses []runnerResponse
	calls     [][]string
}

func (r *recordingRunner) Available() bool { return r.available }

func (r *recordingRunner) Run(_ context.Context, args ...string) (string, error) {
	r.calls = append(r.calls, slices.Clone(args))
	callIndex := len(r.calls) - 1
	if callIndex >= len(r.responses) {
		return "", nil
	}
	return r.responses[callIndex].out, r.responses[callIndex].err
}

func (r *recordingRunner) RunStdin(ctx context.Context, _ io.Reader, args ...string) (string, error) {
	return r.Run(ctx, args...)
}

func TestClientStateMapsLXCOutput(t *testing.T) {
	tests := []struct {
		name      string
		available bool
		response  runnerResponse
		want      serviceproject.ContainerState
		wantErr   string
	}{
		{name: "unavailable", want: serviceproject.ContainerStateUnknown},
		{name: "running", available: true, response: runnerResponse{out: "Name: c1\nStatus: RUNNING\n"}, want: serviceproject.ContainerStateRunning},
		{name: "stopped", available: true, response: runnerResponse{out: "Status: stopped"}, want: serviceproject.ContainerStateStopped},
		{name: "frozen", available: true, response: runnerResponse{out: "Status: Frozen"}, want: serviceproject.ContainerStateFrozen},
		{name: "unrecognized", available: true, response: runnerResponse{out: "Status: EVACUATED"}, want: serviceproject.ContainerStateUnknown},
		{name: "missing status", available: true, response: runnerResponse{out: "Name: c1"}, want: serviceproject.ContainerStateUnknown},
		{name: "not found", available: true, response: runnerResponse{out: "Error: Instance not found", err: errors.New("exit 1")}, want: serviceproject.ContainerStateMissing},
		{name: "does not exist", available: true, response: runnerResponse{out: "Instance doesn't exist", err: errors.New("exit 1")}, want: serviceproject.ContainerStateMissing},
		{
			name:      "runtime error",
			available: true,
			response:  runnerResponse{out: "daemon unavailable", err: errors.New("exit 1")},
			want:      serviceproject.ContainerStateUnknown,
			wantErr:   "lxc info: exit 1; output: daemon unavailable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &recordingRunner{available: tt.available, responses: []runnerResponse{tt.response}}
			got, err := NewClient(runner).State(context.Background(), "c1")

			if got != tt.want {
				t.Fatalf("State() state = %q, want %q", got, tt.want)
			}
			assertError(t, err, tt.wantErr)

			var wantCalls [][]string
			if tt.available {
				wantCalls = [][]string{{"info", "c1"}}
			}
			assertArgv(t, runner.calls, wantCalls)
		})
	}
}

func TestClientReadsLocalDiskDeviceFromLXDQuery(t *testing.T) {
	runner := &recordingRunner{available: true, responses: []runnerResponse{{out: `{
        "devices":{"workspace":{"type":"disk","source":"/host/project","path":"/workspace"}}
    }`}}}
	source, path, exists, err := NewClient(runner).Disk(context.Background(), "project-1", "workspace")
	if err != nil {
		t.Fatal(err)
	}
	if !exists || source != "/host/project" || path != "/workspace" {
		t.Fatalf("Disk() = %q, %q, %t", source, path, exists)
	}
	assertArgv(t, runner.calls, [][]string{{"query", "/1.0/instances/project-1"}})
}

func TestClientPullDirectoryTreatsMissingProviderHomeAsEmpty(t *testing.T) {
	for _, output := range []string{"Error: file does not exist", "Error: Not Found"} {
		t.Run(output, func(t *testing.T) {
			runner := &recordingRunner{available: true, responses: []runnerResponse{{
				out: output, err: errors.New("exit 1"),
			}}}
			pulled, err := NewClient(runner).PullDirectory(context.Background(), "project-1", "/root/.kimi-code", "/host/staging")
			if err != nil || pulled {
				t.Fatalf("PullDirectory() = %t, %v", pulled, err)
			}
			assertArgv(t, runner.calls, [][]string{{
				"file", "pull", "--recursive", "project-1/root/.kimi-code", "/host/staging",
			}})
		})
	}
}

func TestClientMountedDistinguishesAnOrdinaryDirectory(t *testing.T) {
	runner := &recordingRunner{available: true, responses: []runnerResponse{{err: errors.New("exit 1")}}}
	mounted, err := NewClient(runner).Mounted(context.Background(), "project-1", "/workspace")
	if err != nil || mounted {
		t.Fatalf("Mounted() = %t, %v", mounted, err)
	}
	assertArgv(t, runner.calls, [][]string{{"exec", "project-1", "--", "mountpoint", "-q", "/workspace"}})
}

func TestClientLifecycleCommandsAndErrorMappings(t *testing.T) {
	exitError := errors.New("exit 1")
	tests := []struct {
		name      string
		available bool
		responses []runnerResponse
		invoke    func(*Client) error
		wantCalls [][]string
		wantErr   string
	}{
		{
			name:      "init",
			available: true,
			invoke: func(client *Client) error {
				return client.Init(context.Background(), "local:remote-base", "project-1")
			},
			wantCalls: [][]string{{"init", "local:remote-base", "project-1"}},
		},
		{
			name:      "init error",
			available: true,
			responses: []runnerResponse{{out: "launch failed", err: exitError}},
			invoke: func(client *Client) error {
				return client.Init(context.Background(), "local:remote-base", "project-1")
			},
			wantCalls: [][]string{{"init", "local:remote-base", "project-1"}},
			wantErr:   "lxc init: exit 1; output: launch failed",
		},
		{
			name:      "attach read-write disk",
			available: true,
			invoke: func(client *Client) error {
				return client.AttachDisk(context.Background(), "project-1", "workspace", "/host/project-1", "/workspace", false)
			},
			wantCalls: [][]string{{
				"config", "device", "add", "project-1", "workspace", "disk",
				"source=/host/project-1", "path=/workspace",
			}},
		},
		{
			name:      "attach read-only disk",
			available: true,
			invoke: func(client *Client) error {
				return client.AttachDisk(context.Background(), "project-1", "credentials", "/host/credentials", "/credentials", true)
			},
			wantCalls: [][]string{{
				"config", "device", "add", "project-1", "credentials", "disk",
				"source=/host/credentials", "path=/credentials", "readonly=true",
			}},
		},
		{
			name:      "attach disk error",
			available: true,
			responses: []runnerResponse{{out: "device failed", err: exitError}},
			invoke: func(client *Client) error {
				return client.AttachDisk(context.Background(), "project-1", "workspace", "/host/project-1", "/workspace", false)
			},
			wantCalls: [][]string{{
				"config", "device", "add", "project-1", "workspace", "disk",
				"source=/host/project-1", "path=/workspace",
			}},
			wantErr: "lxc config device add workspace: exit 1; output: device failed",
		},
		{
			name:      "autostart already enabled",
			available: true,
			responses: []runnerResponse{{out: " true\n"}},
			invoke: func(client *Client) error {
				return client.EnsureBootAutostart(context.Background(), "project-1")
			},
			wantCalls: [][]string{{"config", "get", "project-1", "boot.autostart"}},
		},
		{
			name:      "enable autostart",
			available: true,
			responses: []runnerResponse{{out: "false"}, {}},
			invoke: func(client *Client) error {
				return client.EnsureBootAutostart(context.Background(), "project-1")
			},
			wantCalls: [][]string{
				{"config", "get", "project-1", "boot.autostart"},
				{"config", "set", "project-1", "boot.autostart", "true"},
			},
		},
		{
			name:      "enable autostart error",
			available: true,
			responses: []runnerResponse{{out: "false"}, {out: "set failed", err: exitError}},
			invoke: func(client *Client) error {
				return client.EnsureBootAutostart(context.Background(), "project-1")
			},
			wantCalls: [][]string{
				{"config", "get", "project-1", "boot.autostart"},
				{"config", "set", "project-1", "boot.autostart", "true"},
			},
			wantErr: "set boot.autostart: exit 1; output: set failed",
		},
		{
			name:      "start",
			available: true,
			invoke: func(client *Client) error {
				return client.Start(context.Background(), "project-1")
			},
			wantCalls: [][]string{{"start", "project-1"}},
		},
		{
			name:      "start already running",
			available: true,
			responses: []runnerResponse{{out: "Instance is already running", err: exitError}},
			invoke: func(client *Client) error {
				return client.Start(context.Background(), "project-1")
			},
			wantCalls: [][]string{{"start", "project-1"}},
		},
		{
			name:      "start error",
			available: true,
			responses: []runnerResponse{{out: "daemon unavailable", err: exitError}},
			invoke: func(client *Client) error {
				return client.Start(context.Background(), "project-1")
			},
			wantCalls: [][]string{{"start", "project-1"}},
			wantErr:   "lxc start: exit 1; output: daemon unavailable",
		},
		{
			name:      "stop",
			available: true,
			invoke: func(client *Client) error {
				return client.Stop(context.Background(), "project-1")
			},
			wantCalls: [][]string{{"stop", "project-1"}},
		},
		{
			name:      "stop missing",
			available: true,
			responses: []runnerResponse{{out: "Instance not found", err: exitError}},
			invoke: func(client *Client) error {
				return client.Stop(context.Background(), "project-1")
			},
			wantCalls: [][]string{{"stop", "project-1"}},
		},
		{
			name:      "stop already stopped",
			available: true,
			responses: []runnerResponse{{out: "Instance is already stopped", err: exitError}},
			invoke: func(client *Client) error {
				return client.Stop(context.Background(), "project-1")
			},
			wantCalls: [][]string{{"stop", "project-1"}},
		},
		{
			name:      "stop escalates to force on graceful failure",
			available: true,
			responses: []runnerResponse{{out: "context deadline exceeded", err: exitError}},
			invoke: func(client *Client) error {
				return client.Stop(context.Background(), "project-1")
			},
			wantCalls: [][]string{{"stop", "project-1"}, {"stop", "--force", "project-1"}},
		},
		{
			name:      "stop error when force also fails",
			available: true,
			responses: []runnerResponse{
				{out: "daemon unavailable", err: exitError},
				{out: "daemon unavailable", err: exitError},
			},
			invoke: func(client *Client) error {
				return client.Stop(context.Background(), "project-1")
			},
			wantCalls: [][]string{{"stop", "project-1"}, {"stop", "--force", "project-1"}},
			wantErr:   "lxc stop: exit 1; output: daemon unavailable (force follow-up: daemon unavailable)",
		},
		{
			name:      "restart forces",
			available: true,
			invoke: func(client *Client) error {
				return client.Restart(context.Background(), "project-1")
			},
			wantCalls: [][]string{{"restart", "--force", "project-1"}},
		},
		{
			name:      "restart error",
			available: true,
			responses: []runnerResponse{{out: "daemon unavailable", err: exitError}},
			invoke: func(client *Client) error {
				return client.Restart(context.Background(), "project-1")
			},
			wantCalls: [][]string{{"restart", "--force", "project-1"}},
			wantErr:   "lxc restart --force: exit 1; output: daemon unavailable",
		},
		{
			name: "delete unavailable",
			invoke: func(client *Client) error {
				return client.Delete(context.Background(), "project-1")
			},
		},
		{
			name:      "delete",
			available: true,
			invoke: func(client *Client) error {
				return client.Delete(context.Background(), "project-1")
			},
			wantCalls: [][]string{{"delete", "--force", "project-1"}},
		},
		{
			name:      "delete missing",
			available: true,
			responses: []runnerResponse{{out: "Instance not found", err: exitError}},
			invoke: func(client *Client) error {
				return client.Delete(context.Background(), "project-1")
			},
			wantCalls: [][]string{{"delete", "--force", "project-1"}},
		},
		{
			name:      "delete error",
			available: true,
			responses: []runnerResponse{{out: "daemon unavailable", err: exitError}},
			invoke: func(client *Client) error {
				return client.Delete(context.Background(), "project-1")
			},
			wantCalls: [][]string{{"delete", "--force", "project-1"}},
			wantErr:   "lxc delete: exit 1; output: daemon unavailable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &recordingRunner{available: tt.available, responses: tt.responses}
			err := tt.invoke(NewClient(runner))
			assertError(t, err, tt.wantErr)
			assertArgv(t, runner.calls, tt.wantCalls)
		})
	}
}

func TestClientDeleteReconcilesACompletedDeleteAfterClientTimeout(t *testing.T) {
	runner := &timeoutThenMissingRunner{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()

	if err := NewClient(runner).Delete(ctx, "project-1"); err != nil {
		t.Fatalf("Delete() error = %v, want completed daemon delete to succeed", err)
	}
	assertArgv(t, runner.calls, [][]string{
		{"delete", "--force", "project-1"},
		{"info", "project-1"},
	})
}

func assertError(t *testing.T, err error, want string) {
	t.Helper()
	if want == "" {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return
	}
	if err == nil || err.Error() != want {
		t.Fatalf("error = %v, want %q", err, want)
	}
}

func assertArgv(t *testing.T, got, want [][]string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("argv call count = %d, want %d\n got: %q\nwant: %q", len(got), len(want), got, want)
	}
	for i := range want {
		if !slices.Equal(got[i], want[i]) {
			t.Fatalf("argv call %d:\n got: %q\nwant: %q", i, got[i], want[i])
		}
	}
}
