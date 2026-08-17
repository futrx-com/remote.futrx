package templates

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

func TestFileExists(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		err     error
		want    bool
		wantErr bool
	}{
		{name: "present", want: true},
		{
			name: "absent is not an error",
			err:  errors.New("exit status 1"),
			want: false,
		},
		{
			name:   "a container that does not exist holds no marker",
			output: "Error: Instance not found",
			err:    errors.New("exit status 1"),
			want:   false,
		},
		{
			name:    "any other failure is an error",
			output:  "Error: cannot connect to the LXD server",
			err:     errors.New("exit status 1"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &recordingRunner{available: true, output: tt.output, err: tt.err}
			got, err := NewAdapter(runner).FileExists(context.Background(), "project-1", "/var/lib/x")
			if (err != nil) != tt.wantErr {
				t.Fatalf("FileExists() error = %v, wantErr %t", err, tt.wantErr)
			}
			if err == nil && got != tt.want {
				t.Fatalf("FileExists() = %t, want %t", got, tt.want)
			}
			want := []string{"exec", "project-1", "--", "test", "-e", "/var/lib/x"}
			if len(runner.calls) != 1 || !slices.Equal(runner.calls[0], want) {
				t.Fatalf("calls = %q, want [%q]", runner.calls, want)
			}
		})
	}
}

func TestImageExists(t *testing.T) {
	tests := []struct {
		name   string
		alias  string
		output string
		want   bool
	}{
		{
			name:   "published",
			alias:  "futrx-remote-wordpress-base",
			output: "futrx-remote-wordpress-base,abc123,no,CONTAINER,1.2GB,x86_64\n",
			want:   true,
		},
		{name: "absent", alias: "futrx-remote-wordpress-base", output: "\n"},
		{
			name:   "a different alias in the listing does not count",
			alias:  "futrx-remote-wordpress-base",
			output: "futrx-remote-dev-base,abc123,no,CONTAINER,1.2GB,x86_64\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &recordingRunner{available: true, output: tt.output}
			got, err := NewAdapter(runner).ImageExists(context.Background(), tt.alias)
			if err != nil {
				t.Fatalf("ImageExists() = %v", err)
			}
			if got != tt.want {
				t.Fatalf("ImageExists() = %t, want %t", got, tt.want)
			}
			want := []string{"image", "list", "--format", "csv", tt.alias}
			if len(runner.calls) != 1 || !slices.Equal(runner.calls[0], want) {
				t.Fatalf("calls = %q, want [%q]", runner.calls, want)
			}
		})
	}
}

func TestRunScriptAndEnsureDirectoryTranslateToCommands(t *testing.T) {
	tests := []struct {
		name     string
		invoke   func(*Adapter) error
		wantArgs []string
	}{
		{
			name: "run script",
			invoke: func(a *Adapter) error {
				_, err := a.RunScript(context.Background(), "project-1", "echo hi")
				return err
			},
			wantArgs: []string{"exec", "project-1", "--", "bash", "-c", "echo hi"},
		},
		{
			name: "ensure directory",
			invoke: func(a *Adapter) error {
				return a.EnsureDirectory(context.Background(), "project-1", "/workspace/sub")
			},
			wantArgs: []string{"exec", "project-1", "--", "install", "-d", "-m", "755", "/workspace/sub"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &recordingRunner{available: true}
			if err := tt.invoke(NewAdapter(runner)); err != nil {
				t.Fatalf("invoke = %v", err)
			}
			if len(runner.calls) != 1 || !slices.Equal(runner.calls[0], tt.wantArgs) {
				t.Fatalf("calls = %q, want [%q]", runner.calls, tt.wantArgs)
			}
		})
	}
}

func TestPushFilePushesWithTheRequestedMode(t *testing.T) {
	runner := &recordingRunner{available: true}
	adapter := NewAdapter(runner)

	if err := adapter.PushFile(
		context.Background(), "project-1", []byte("hello"), "/workspace/README.md", "644",
	); err != nil {
		t.Fatalf("PushFile() = %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("calls = %q", runner.calls)
	}
	call := runner.calls[0]
	if call[0] != "file" || call[1] != "push" || call[2] != "--mode=644" {
		t.Fatalf("call = %q", call)
	}
	if call[len(call)-1] != "project-1/workspace/README.md" {
		t.Fatalf("destination = %q", call[len(call)-1])
	}
}
