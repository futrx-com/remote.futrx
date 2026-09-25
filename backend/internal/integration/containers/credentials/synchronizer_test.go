package credentials

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
)

type runnerResponse struct {
	out string
	err error
}

type recordingRunner struct {
	responses map[string]runnerResponse
	calls     []string
}

func (r *recordingRunner) Available() bool { return true }

func (r *recordingRunner) Run(_ context.Context, args ...string) (string, error) {
	call := strings.Join(args, " ")
	r.calls = append(r.calls, call)
	response := r.responses[call]
	return response.out, response.err
}

func (r *recordingRunner) RunStdin(ctx context.Context, _ io.Reader, args ...string) (string, error) {
	return r.Run(ctx, args...)
}

func TestEnsureRejectsMissingRequiredHostFileBeforeContainerMutation(t *testing.T) {
	runner := &recordingRunner{}
	missing := filepath.Join(t.TempDir(), "missing.json")
	spec := provisioning.CredentialSpec{
		Name:         "agent",
		ContainerDir: "/root/.agent",
		Files: []provisioning.CredentialFile{{
			HostPath:      missing,
			ContainerPath: "/root/.agent/auth.json",
			PushRequired:  true,
		}},
	}

	err := NewAdapter(runner).EnsureFiles(context.Background(), "c1", spec)
	want := "host file missing (provider not authenticated yet?): " + missing
	if err == nil || err.Error() != want {
		t.Fatalf("Ensure error = %v, want %q", err, want)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("required host-file gate must precede container mutation; calls = %v", runner.calls)
	}
}

func TestEnsurePushesOnlyStrictlyNewerFilesWithDefaultMode(t *testing.T) {
	hostDir := t.TempDir()
	equalPath := filepath.Join(hostDir, "equal.json")
	newerPath := filepath.Join(hostDir, "newer.json")
	missingOptionalPath := filepath.Join(hostDir, "optional.json")
	for _, path := range []string{equalPath, newerPath} {
		if err := os.WriteFile(path, []byte("credentials"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	equalTime := time.Unix(1_700_000_000, 0)
	newerTime := equalTime.Add(time.Minute)
	if err := os.Chtimes(equalPath, equalTime, equalTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(newerPath, newerTime, newerTime); err != nil {
		t.Fatal(err)
	}

	runner := &recordingRunner{responses: map[string]runnerResponse{
		"exec c1 -- stat -c %Y /root/.agent/equal.json": {out: "1700000000\n"},
		"exec c1 -- stat -c %Y /root/.agent/newer.json": {out: "1700000059\n"},
	}}
	spec := provisioning.CredentialSpec{
		Name:         "agent",
		ContainerDir: "/root/.agent",
		Files: []provisioning.CredentialFile{
			{HostPath: equalPath, ContainerPath: "/root/.agent/equal.json"},
			{HostPath: newerPath, ContainerPath: "/root/.agent/newer.json"},
			{HostPath: missingOptionalPath, ContainerPath: "/root/.agent/optional.json"},
		},
	}

	if err := NewAdapter(runner).EnsureFiles(context.Background(), "c1", spec); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	wantCalls := []string{
		"exec c1 -- install -d -m 700 /root/.agent",
		"exec c1 -- stat -c %Y /root/.agent/equal.json",
		"exec c1 -- stat -c %Y /root/.agent/newer.json",
		"file push --mode=600 " + newerPath + " c1/root/.agent/newer.json",
	}
	if !reflect.DeepEqual(runner.calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", runner.calls, wantCalls)
	}
}

func TestEnsureAlwaysPushesHostAuthoritativeFile(t *testing.T) {
	hostPath := filepath.Join(t.TempDir(), "auth.json")
	if err := os.WriteFile(hostPath, []byte("credentials"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &recordingRunner{}
	spec := provisioning.CredentialSpec{
		Name: "agent",
		Files: []provisioning.CredentialFile{{
			HostPath: hostPath, ContainerPath: "/root/.agent/auth.json",
			HostAuthoritative: true,
		}},
	}
	if err := NewAdapter(runner).EnsureFiles(context.Background(), "c1", spec); err != nil {
		t.Fatal(err)
	}
	want := []string{"file push --mode=600 " + hostPath + " c1/root/.agent/auth.json"}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("calls = %#v, want %#v", runner.calls, want)
	}
}

func TestSyncFromContainerSkipsMissingOptionalFileButRejectsMissingRequiredFile(t *testing.T) {
	runner := &recordingRunner{responses: map[string]runnerResponse{
		"exec c1 -- test -f /root/.agent/optional.json": {out: "optional absent", err: errors.New("missing")},
		"exec c1 -- test -f /root/.agent/required.json": {out: "required absent", err: errors.New("missing")},
	}}
	hostDir := filepath.Join(t.TempDir(), "credentials")
	spec := provisioning.CredentialSpec{
		Name:    "agent",
		HostDir: hostDir,
		Files: []provisioning.CredentialFile{
			{HostPath: filepath.Join(hostDir, "optional.json"), ContainerPath: "/root/.agent/optional.json"},
			{HostPath: filepath.Join(hostDir, "required.json"), ContainerPath: "/root/.agent/required.json", PullRequired: true},
		},
	}

	err := NewAdapter(runner).SyncFilesFromContainer(context.Background(), "c1", spec)
	want := "container file missing /root/.agent/required.json: missing; output: required absent"
	if err == nil || err.Error() != want {
		t.Fatalf("SyncFromContainer error = %v, want %q", err, want)
	}
	wantCalls := []string{
		"exec c1 -- test -f /root/.agent/optional.json",
		"exec c1 -- test -f /root/.agent/required.json",
	}
	if !reflect.DeepEqual(runner.calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", runner.calls, wantCalls)
	}
}
