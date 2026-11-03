package chat

import (
	"context"
	"errors"
	permission "github.com/futrx-com/remote.futrx.com/internal/rbac"
	"testing"
)

type denyCreation struct{ checks []permission.Check }

func (a *denyCreation) Require(_ context.Context, check permission.Check) error {
	a.checks = append(a.checks, check)
	return permission.ErrDenied
}

func TestCreateAndForkAuthorizeBeforeSideEffects(t *testing.T) {
	for _, projectID := range []ProjectID{"", "project-one"} {
		for _, fork := range []bool{false, true} {
			a := &denyCreation{}
			// Unimplemented repository methods panic, so a denial must stop before
			// creation, event reads, native session forks or copied-event writes.
			repo := &forkRepository{source: Meta{ID: "abcd", ProjectID: projectID, Provider: ProviderCodex}}
			s := New(repo, nil, nil, nil, WithAuthorizer(a))
			var err error
			if fork {
				_, err = s.Fork(context.Background(), "abcd")
			} else {
				_, err = s.Create(context.Background(), CreateInput{ProjectID: projectID})
			}
			if !errors.Is(err, permission.ErrDenied) {
				t.Fatalf("project=%q fork=%v: %v", projectID, fork, err)
			}
			expected := permission.Check{Permission: PermissionHostCreate, Scope: permission.PlatformScope()}
			if projectID != "" {
				expected = permission.Check{Permission: PermissionProjectCreate, Scope: permission.ProjectScope(string(projectID))}
			}
			if len(a.checks) != 1 || a.checks[0] != expected {
				t.Fatalf("checks=%v want=%v", a.checks, expected)
			}
			if repo.created.ID != "" || len(repo.copied) != 0 {
				t.Fatal("denied operation wrote data")
			}
		}
	}
}
func TestCreateWithoutAuthorizerFailsClosed(t *testing.T) {
	s := New(nil, nil, nil, nil)
	if _, err := s.Create(context.Background(), CreateInput{}); !errors.Is(err, permission.ErrActorRequired) {
		t.Fatalf("missing actor: %v", err)
	}
	ctx := permission.ContextWithActor(context.Background(), permission.UserActor("user@example.com"))
	if _, err := s.Create(ctx, CreateInput{}); !errors.Is(err, permission.ErrDenied) {
		t.Fatalf("missing authorizer: %v", err)
	}
}
