package applications

import (
	"context"
	"errors"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/rbac"
)

type recordingWorkspacePolicy struct {
	checks []rbac.Check
	err    error
}

func (p *recordingWorkspacePolicy) Require(_ context.Context, check rbac.Check) error {
	p.checks = append(p.checks, check)
	return p.err
}

func TestFileManagerWebAccessRequiresFilesPermission(t *testing.T) {
	store := &fakeStore{
		global: []Instance{{ID: "files-global", ApplicationID: "file-manager", Scope: ScopeGlobal}},
		byProject: map[string][]Instance{"beef": {
			{ID: "files-project", ApplicationID: "file-manager", Scope: ScopeProject, ProjectID: "beef"},
		}},
	}
	for _, tc := range []struct {
		instance string
		want     rbac.Check
	}{
		{"files-project", rbac.Check{Permission: "workspace.files.use", Scope: rbac.ProjectScope("beef")}},
		{"files-global", rbac.Check{Permission: "workspace.hostfiles.use", Scope: rbac.PlatformScope()}},
	} {
		t.Run(tc.instance, func(t *testing.T) {
			policy := &recordingWorkspacePolicy{err: rbac.ErrDenied}
			service := New(nil, store, nil, nil, nil, WithWorkspaceAuthorizer(policy))
			if err := service.RequireWebAccess(context.Background(), tc.instance); !errors.Is(err, rbac.ErrDenied) {
				t.Fatalf("denied RequireWebAccess = %v", err)
			}
			if len(policy.checks) != 1 || policy.checks[0] != tc.want {
				t.Fatalf("checks = %v, want %v", policy.checks, tc.want)
			}
			policy.err = nil
			if err := service.RequireWebAccess(context.Background(), tc.instance); err != nil {
				t.Fatalf("allowed RequireWebAccess = %v", err)
			}
		})
	}
}
