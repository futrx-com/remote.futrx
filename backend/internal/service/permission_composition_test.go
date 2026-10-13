package service

import (
	"context"
	"errors"
	"testing"

	servicepermission "github.com/futrx-com/remote.futrx.com/internal/rbac"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
	"github.com/futrx-com/remote.futrx.com/internal/stores/filepermissions"
)

type permissionTestIdentity struct {
	admins     map[string]bool
	registered map[string]bool
}

func (i permissionTestIdentity) IsAdmin(_ context.Context, email string) (bool, error) {
	return i.admins[email], nil
}

func (i permissionTestIdentity) IsRegistered(_ context.Context, email string) (bool, error) {
	return i.registered[email] || i.admins[email], nil
}

func newTestPermissionIdentity() permissionTestIdentity {
	return permissionTestIdentity{
		admins:     map[string]bool{"admin@example.com": true},
		registered: map[string]bool{"member@example.com": true, "other@example.com": true},
	}
}

func actorContext(email string) context.Context {
	return servicepermission.ContextWithActor(context.Background(), servicepermission.UserActor(email))
}

// newPermissions builds a permission service the way the composition root does,
// from the production catalog, with a membership adapter over access.
func newPermissions(
	ctx context.Context,
	repo servicepermission.Repository,
	identity servicepermission.IdentityDirectory,
	access serviceproject.AccessRepository,
) (*servicepermission.Service, error) {
	registry, err := servicepermission.NewRegistry(
		servicepermission.ManagementDefinitions(),
		serviceproject.PermissionDefinitions(),
	)
	if err != nil {
		return nil, err
	}
	return servicepermission.NewService(ctx, registry, repo, identity, testMembership{access: access})
}

type testMembership struct {
	access serviceproject.AccessRepository
}

func (m testMembership) HasAccess(ctx context.Context, projectID string, email string) (bool, error) {
	if m.access == nil {
		return false, nil
	}
	return m.access.Has(ctx, serviceproject.ID(projectID), servicepermission.NormalizeEmail(email))
}

func TestPermissionPolicySurvivesRestartWithIdenticalDecisions(t *testing.T) {
	dataDir := t.TempDir()
	identity := newTestPermissionIdentity()
	build := func() *servicepermission.Service {
		store, err := filepermissions.New(dataDir)
		if err != nil {
			t.Fatal(err)
		}
		service, err := newPermissions(context.Background(), store, identity, nil)
		if err != nil {
			t.Fatal(err)
		}
		return service
	}
	check := servicepermission.Check{
		Permission: servicepermission.PermissionRolesManage,
		Scope:      servicepermission.PlatformScope(),
	}

	first := build()
	if _, err := first.SetAssignment(actorContext("admin@example.com"), servicepermission.AssignmentInput{
		UserEmail: "member@example.com", Permission: servicepermission.PermissionRolesManage,
		Effect: servicepermission.Allow, Scope: servicepermission.PlatformScope(),
	}); err != nil {
		t.Fatal(err)
	}
	decide := func(service *servicepermission.Service, email string) servicepermission.Decision {
		decision, err := service.Evaluate(actorContext(email), check)
		if err != nil {
			t.Fatal(err)
		}
		return decision
	}
	before := []servicepermission.Decision{decide(first, "member@example.com"), decide(first, "other@example.com")}

	second := build()
	after := []servicepermission.Decision{decide(second, "member@example.com"), decide(second, "other@example.com")}
	if before[0] != after[0] || before[1] != after[1] {
		t.Fatalf("decisions changed across a restart: before %+v, after %+v", before, after)
	}
	if !after[0].Allowed || after[1].Allowed {
		t.Fatalf("decisions after restart = %+v, want member allowed and other denied", after)
	}
}

func TestUserRemovalCleanupRemovesPermissionAssignmentsAndBindings(t *testing.T) {
	store, err := filepermissions.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service, err := newPermissions(context.Background(), store, newTestPermissionIdentity(), nil)
	if err != nil {
		t.Fatal(err)
	}
	admin := actorContext("admin@example.com")
	for _, email := range []string{"member@example.com", "other@example.com"} {
		if _, err := service.SetAssignment(admin, servicepermission.AssignmentInput{
			UserEmail: email, Permission: servicepermission.PermissionRolesManage,
			Effect: servicepermission.Allow, Scope: servicepermission.PlatformScope(),
		}); err != nil {
			t.Fatal(err)
		}
	}
	role, err := service.CreateRole(admin, servicepermission.RoleInput{
		Name:  "Role managers",
		Rules: []servicepermission.RoleRule{{Permission: servicepermission.PermissionRolesManage, Effect: servicepermission.Allow}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.BindRole(admin, servicepermission.BindingInput{
		RoleID: role.ID, UserEmail: "member@example.com", Scope: servicepermission.PlatformScope(),
	}); err != nil {
		t.Fatal(err)
	}

	cleanup := userRemovalCleanup{permissions: service}
	if err := cleanup.CleanupRemovedUser(context.Background(), "member@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := cleanup.CleanupRemovedUser(context.Background(), "member@example.com"); err != nil {
		t.Fatalf("second cleanup error = %v, want idempotent", err)
	}

	state, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Bindings) != 0 || len(state.Roles) != 1 {
		t.Fatalf("bindings = %d, roles = %d; want 0 bindings and the role kept", len(state.Bindings), len(state.Roles))
	}
	if len(state.Assignments) != 1 || state.Assignments[0].UserEmail != "other@example.com" {
		t.Fatalf("assignments = %#v, want only other@example.com's", state.Assignments)
	}
}

type failingPermissionCleanup struct{ err error }

func (f failingPermissionCleanup) RemoveUserPolicy(context.Context, string) error { return f.err }

func TestUserRemovalCleanupReportsPermissionFailuresWithoutSkippingTheRest(t *testing.T) {
	sessionRegistry := &cleanupSecurityStateStub{emails: map[string]bool{"member@example.com": true}}
	cleanup := userRemovalCleanup{
		permissions:     failingPermissionCleanup{err: errors.New("audit disk full")},
		sessionRegistry: sessionRegistry,
	}

	err := cleanup.CleanupRemovedUser(context.Background(), "member@example.com")
	if err == nil {
		t.Fatal("expected the permission cleanup failure to surface so the removal can be retried")
	}
	if sessionRegistry.emails["member@example.com"] {
		t.Fatal("remaining cleanup was skipped after the permission cleanup failed")
	}
}
