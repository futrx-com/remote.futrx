package workspaceide

import (
	"context"
	"errors"
	"github.com/futrx-com/remote.futrx.com/internal/rbac"
	"testing"
)

type ideDenyAuthorizer struct{ checks []rbac.Check }

func (a *ideDenyAuthorizer) Require(_ context.Context, c rbac.Check) error {
	a.checks = append(a.checks, c)
	return rbac.ErrDenied
}
func TestIDEOpenAuthorizesBeforeResolvingPaths(t *testing.T) {
	for _, projectID := range []string{"", "project-one"} {
		a := &ideDenyAuthorizer{}
		s := New(testBaseURL, testProjectsRoot, WithAuthorizer(a))
		if _, err := s.OpenURL(context.Background(), projectID, "invalid", "../invalid"); !errors.Is(err, rbac.ErrDenied) {
			t.Fatalf("denial must precede path lookup: %v", err)
		}
		want := rbac.Check{Permission: PermissionHostOpen, Scope: rbac.PlatformScope()}
		if projectID != "" {
			want = rbac.Check{Permission: PermissionProjectOpen, Scope: rbac.ProjectScope(projectID)}
		}
		if len(a.checks) != 1 || a.checks[0] != want {
			t.Fatalf("checks: %v", a.checks)
		}
	}
	s := New(testBaseURL, testProjectsRoot)
	if _, err := s.OpenURL(context.Background(), "project-one", projectWorkspace, "/workspace"); !errors.Is(err, rbac.ErrActorRequired) {
		t.Fatalf("no actor: %v", err)
	}
	ctx := rbac.ContextWithActor(context.Background(), rbac.UserActor("user@example.com"))
	if _, err := s.OpenURL(ctx, "project-one", projectWorkspace, "/workspace"); !errors.Is(err, rbac.ErrDenied) {
		t.Fatalf("no authorizer: %v", err)
	}
}
