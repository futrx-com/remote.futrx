package workspaceaccess_test

import (
	"context"
	"errors"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/rbac"
	"github.com/futrx-com/remote.futrx.com/internal/service/workspaceaccess"
	"github.com/futrx-com/remote.futrx.com/internal/stores/filepermissions"
)

type identity struct{}

func (identity) IsAdmin(_ context.Context, email string) (bool, error) {
	return email == "admin@example.com", nil
}

func (identity) IsRegistered(context.Context, string) (bool, error) { return true, nil }

// Chats outside a project offered files and git history to every registered
// user before permissions existed; that stays the default until denied.
func TestHostToolsDefaultToRegisteredUsersUntilDenied(t *testing.T) {
	ctx := context.Background()
	store, err := filepermissions.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	registry := rbac.MustRegistry(rbac.ManagementDefinitions(), workspaceaccess.PermissionDefinitions())
	permissions, err := rbac.NewService(ctx, registry, store, identity{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	member := rbac.ContextWithActor(ctx, rbac.UserActor("member@example.com"))
	for _, capability := range []string{"files", "git", "terminal", "browser"} {
		if err := workspaceaccess.Require(member, permissions, capability, ""); err != nil {
			t.Fatalf("host %s for a member = %v, want allowed", capability, err)
		}
	}

	admin := rbac.ContextWithActor(ctx, rbac.UserActor("admin@example.com"))
	if _, err := permissions.SetAssignment(admin, rbac.AssignmentInput{
		UserEmail: "member@example.com", Permission: "workspace.hostfiles.use",
		Effect: rbac.Deny, Scope: rbac.PlatformScope(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := workspaceaccess.Require(member, permissions, "files", ""); !errors.Is(err, rbac.ErrDenied) {
		t.Fatalf("denied host files = %v, want ErrDenied", err)
	}
}
