package rbac

import (
	"context"
	"testing"
	"time"
)

func newSeededService(t *testing.T, repo *memoryRepository) *Service {
	t.Helper()
	service, err := NewService(
		context.Background(), testRegistry(), repo,
		&fakeIdentity{admins: map[string]bool{testAdmin: true}, registered: map[string]bool{testAlice: true}}, fakeMembers{},
		WithClock(func() time.Time { return time.Unix(1_700_000_000, 0) }),
		WithBuiltinAdminRole(),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return service
}

func TestBuiltinAdminRoleIsSeededOnce(t *testing.T) {
	repo := &memoryRepository{}
	service := newSeededService(t, repo)

	state, _ := repo.Load(context.Background())
	role, ok := state.Role(AdminRoleID)
	if !ok || role.Name != "admin" || len(role.Rules) != 2 || !state.AdminRoleSeeded {
		t.Fatalf("seeded state = %+v", state)
	}

	if err := service.DeleteRole(as(testAdmin), AdminRoleID, DeleteRoleOptions{}); err != nil {
		t.Fatalf("DeleteRole() error = %v", err)
	}
	newSeededService(t, repo)
	state, _ = repo.Load(context.Background())
	if _, ok := state.Role(AdminRoleID); ok {
		t.Fatal("deleted admin role was seeded again on restart")
	}
}

func TestBuiltinAdminRoleCanBeBoundToANonAdministrator(t *testing.T) {
	repo := &memoryRepository{}
	service := newSeededService(t, repo)
	if _, err := service.BindRole(as(testAdmin), BindingInput{
		RoleID: AdminRoleID, UserEmail: testAlice, Scope: PlatformScope(),
	}); err != nil {
		t.Fatalf("BindRole() error = %v", err)
	}
	requireAllowed(t, service, as(testAlice), PermissionRolesManage, PlatformScope(), true)
}

func TestBuiltinAdminRoleIsSkippedWhenTheNameIsTaken(t *testing.T) {
	repo := &memoryRepository{state: State{Roles: []Role{{ID: "custom", Name: "Admin", Rules: []RoleRule{
		{Permission: PermissionRolesManage, Effect: Allow},
	}}}}}
	newSeededService(t, repo)
	state, _ := repo.Load(context.Background())
	if len(state.Roles) != 1 || !state.AdminRoleSeeded {
		t.Fatalf("state = %+v, want the existing role kept and the seed marked done", state)
	}
}
