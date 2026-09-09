package launch

import (
	"context"
	"errors"
	"slices"
	"testing"
)

type callRecorder struct{ calls []string }

type failingCredentials struct{ recorder *callRecorder }

func (f failingCredentials) EnsureRegistered(_ context.Context, containerName string) error {
	f.recorder.calls = append(f.recorder.calls, "credentials "+containerName)
	return errors.New("credentials failed")
}

type failingWorkspace struct{ recorder *callRecorder }

func (f failingWorkspace) EnsureSkillLinks(_ context.Context, containerName string) error {
	f.recorder.calls = append(f.recorder.calls, "workspace "+containerName)
	return errors.New("workspace failed")
}

func (f failingWorkspace) EnsureGitIdentity(_ context.Context, containerName string) error {
	f.recorder.calls = append(f.recorder.calls, "git identity "+containerName)
	return errors.New("git identity failed")
}

type failingBrowser struct{ recorder *callRecorder }

func (f failingBrowser) EnsureScript(_ context.Context, containerName string) error {
	f.recorder.calls = append(f.recorder.calls, "browser script "+containerName)
	return errors.New("browser script failed")
}

func (f failingBrowser) EnsureSkill(_ context.Context, containerName string) error {
	f.recorder.calls = append(f.recorder.calls, "browser skill "+containerName)
	return errors.New("browser skill failed")
}

func (f failingBrowser) EnsureNesting(_ context.Context, containerName string) error {
	f.recorder.calls = append(f.recorder.calls, "browser nesting "+containerName)
	return errors.New("browser nesting failed")
}

type failingCodeServer struct{ recorder *callRecorder }

func (f failingCodeServer) Ensure(_ context.Context, containerName, displayName, projectSlug string) error {
	f.recorder.calls = append(f.recorder.calls, "code-server "+containerName+" "+displayName+" "+projectSlug)
	return errors.New("code-server failed")
}

type failingScheduleTools struct{ recorder *callRecorder }

func (f failingScheduleTools) Ensure(_ context.Context, containerName string) error {
	f.recorder.calls = append(f.recorder.calls, "schedule tools "+containerName)
	return errors.New("schedule tools failed")
}

func TestProvisionKeepsBestEffortCapabilityOrder(t *testing.T) {
	recorder := &callRecorder{}
	provisioner := NewProvisioner(
		failingCredentials{recorder: recorder},
		failingWorkspace{recorder: recorder},
		failingBrowser{recorder: recorder},
		failingCodeServer{recorder: recorder},
		failingScheduleTools{recorder: recorder},
	)

	provisioner.Provision(context.Background(), "project-1", "My Project", "my-project")

	want := []string{
		"credentials project-1",
		"workspace project-1",
		"git identity project-1",
		"browser script project-1",
		"browser skill project-1",
		"browser nesting project-1",
		"schedule tools project-1",
		"code-server project-1 My Project my-project",
	}
	if !slices.Equal(recorder.calls, want) {
		t.Fatalf("calls: got %q, want %q", recorder.calls, want)
	}
}
