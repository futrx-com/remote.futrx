package permission

import (
	"context"
	"errors"
	"sync"
	"testing"
)

// grantManagement makes testManager a delegated policy manager who also
// belongs to project A.
func grantManagement(t *testing.T, f *fixture, keys ...Key) {
	t.Helper()
	for _, key := range keys {
		mustSet(t, f.service, as(testAdmin), testManager, key, Allow, PlatformScope())
	}
}

func TestAdministratorMayAssignAnyRegisteredPermission(t *testing.T) {
	f := newFixture(t)
	for _, key := range []Key{permBilling, permAdminOnly, PermissionAssignmentsManage, PermissionRolesManage} {
		mustSet(t, f.service, as(testAdmin), testBob, key, Allow, PlatformScope())
	}
	mustSet(t, f.service, as(testAdmin), testBob, permSecrets, Allow, ProjectScope(projectA))
}

func TestDelegatedManagerMayAssignADelegablePermissionTheyHold(t *testing.T) {
	f := newFixture(t)
	grantManagement(t, f, PermissionAssignmentsManage)

	// The manager holds lifecycle on project A through membership.
	mustSet(t, f.service, as(testManager), testBob, permLifecycle, Allow, ProjectScope(projectA))
	requireAllowed(t, f.service, as(testBob), permLifecycle, ProjectScope(projectA), true)

	// And may take it back.
	err := f.service.RemoveAssignment(as(testManager), AssignmentTarget{
		UserEmail: testBob, Permission: permLifecycle, Scope: ProjectScope(projectA),
	})
	if err != nil {
		t.Fatalf("RemoveAssignment() error = %v", err)
	}
	requireAllowed(t, f.service, as(testBob), permLifecycle, ProjectScope(projectA), false)
}

func TestDelegatedManagerCannotGrantWhatTheyDoNotHold(t *testing.T) {
	f := newFixture(t)
	grantManagement(t, f, PermissionAssignmentsManage)
	// Held only at project scope A and only by baseline.
	mustSet(t, f.service, as(testAdmin), testManager, permDocs, Allow, ProjectScope(projectA))
	// Held at platform scope but not delegable.
	mustSet(t, f.service, as(testAdmin), testManager, permBilling, Allow, PlatformScope())

	tests := []struct {
		name string
		in   AssignmentInput
	}{
		{"a permission never held", AssignmentInput{testBob, permSecrets, Allow, ProjectScope(projectA)}},
		{"a project the manager is not a member of", AssignmentInput{testBob, permLifecycle, Allow, ProjectScope(projectB)}},
		{"a broader scope than the one held", AssignmentInput{testBob, permDocs, Allow, PlatformScope()}},
		{"a permission that is not delegable", AssignmentInput{testBob, permBilling, Allow, PlatformScope()}},
		{"the management permission itself", AssignmentInput{testBob, PermissionAssignmentsManage, Allow, PlatformScope()}},
		{"a deny of a permission never held", AssignmentInput{testBob, permSecrets, Deny, ProjectScope(projectA)}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := f.service.SetAssignment(as(testManager), test.in)
			requireError(t, err, ErrDenied)
		})
	}
}

func TestManagerWithoutManagementPermissionCannotMutate(t *testing.T) {
	f := newFixture(t)

	_, err := f.service.SetAssignment(as(testManager), AssignmentInput{
		UserEmail: testBob, Permission: permLifecycle, Effect: Allow, Scope: ProjectScope(projectA),
	})
	requireError(t, err, ErrDenied)
	_, err = f.service.CreateRole(as(testManager), RoleInput{Name: "X", Rules: []RoleRule{{permDocs, Allow}}})
	requireError(t, err, ErrDenied)
	// A roles.manage holder still cannot manage assignments.
	grantManagement(t, f, PermissionRolesManage)
	_, err = f.service.SetAssignment(as(testManager), AssignmentInput{
		UserEmail: testBob, Permission: permLifecycle, Effect: Allow, Scope: ProjectScope(projectA),
	})
	requireError(t, err, ErrDenied)
}

func TestAnExplicitDenyRemovesTheManagersAbilityToDelegate(t *testing.T) {
	f := newFixture(t)
	grantManagement(t, f, PermissionAssignmentsManage)
	mustSet(t, f.service, as(testAdmin), testManager, permLifecycle, Deny, ProjectScope(projectA))

	_, err := f.service.SetAssignment(as(testManager), AssignmentInput{
		UserEmail: testBob, Permission: permLifecycle, Effect: Allow, Scope: ProjectScope(projectA),
	})
	requireError(t, err, ErrDenied)

	mustSet(t, f.service, as(testAdmin), testManager, PermissionAssignmentsManage, Deny, PlatformScope())
	_, err = f.service.SetAssignment(as(testManager), AssignmentInput{
		UserEmail: testBob, Permission: permAccess, Effect: Allow, Scope: ProjectScope(projectA),
	})
	requireError(t, err, ErrDenied)
}

func TestManagerCannotSmuggleUnauthorizedKeysThroughARole(t *testing.T) {
	f := newFixture(t)
	grantManagement(t, f, PermissionAssignmentsManage, PermissionRolesManage)

	t.Run("a role holding a management permission the manager lacks is refused", func(t *testing.T) {
		g := newFixture(t)
		grantManagement(t, g, PermissionRolesManage) // roles.manage only
		_, err := g.service.CreateRole(as(testManager), RoleInput{
			Name: "Sneaky", Rules: []RoleRule{{PermissionAssignmentsManage, Allow}},
		})
		requireError(t, err, ErrDenied)
	})

	t.Run("a manager holding the exact management permission may include it", func(t *testing.T) {
		if _, err := f.service.CreateRole(as(testManager), RoleInput{
			Name: "Holds it", Rules: []RoleRule{{PermissionAssignmentsManage, Allow}},
		}); err != nil {
			t.Fatalf("CreateRole() error = %v", err)
		}
	})

	t.Run("binding a role with unauthorized keys is refused", func(t *testing.T) {
		role, err := f.service.CreateRole(as(testManager), RoleInput{
			Name: "Mixed", Rules: []RoleRule{{permLifecycle, Allow}, {permSecrets, Allow}},
		})
		if err != nil {
			t.Fatal(err)
		}
		_, err = f.service.BindRole(as(testManager), BindingInput{
			RoleID: role.ID, UserEmail: testBob, Scope: ProjectScope(projectA),
		})
		requireError(t, err, ErrDenied)
	})

	t.Run("a role of only held keys may be bound", func(t *testing.T) {
		role, err := f.service.CreateRole(as(testManager), RoleInput{
			Name: "Held", Rules: []RoleRule{{permLifecycle, Allow}, {permAccess, Allow}},
		})
		if err != nil {
			t.Fatal(err)
		}
		mustBind(t, f.service, as(testManager), role.ID, testBob, ProjectScope(projectA))
		requireAllowed(t, f.service, as(testBob), permAccess, ProjectScope(projectA), true)
	})

	t.Run("binding at a project the manager cannot delegate in is refused", func(t *testing.T) {
		role, err := f.service.CreateRole(as(testManager), RoleInput{
			Name: "Held elsewhere", Rules: []RoleRule{{permLifecycle, Allow}},
		})
		if err != nil {
			t.Fatal(err)
		}
		_, err = f.service.BindRole(as(testManager), BindingInput{
			RoleID: role.ID, UserEmail: testBob, Scope: ProjectScope(projectB),
		})
		requireError(t, err, ErrDenied)
	})
}

func TestManagerCannotWidenABoundRoleBeyondWhatTheyHold(t *testing.T) {
	f := newFixture(t)
	grantManagement(t, f, PermissionAssignmentsManage, PermissionRolesManage)
	role, err := f.service.CreateRole(as(testManager), RoleInput{
		Name: "Held", Rules: []RoleRule{{permLifecycle, Allow}},
	})
	if err != nil {
		t.Fatal(err)
	}
	mustBind(t, f.service, as(testManager), role.ID, testBob, ProjectScope(projectA))

	// Adding a permission the manager lacks would reach bob without the manager
	// ever being allowed to grant it to bob directly.
	_, err = f.service.UpdateRole(as(testManager), role.ID, RoleInput{
		Name: "Held", Rules: []RoleRule{{permLifecycle, Allow}, {permSecrets, Allow}},
	})
	requireError(t, err, ErrDenied)
	requireAllowed(t, f.service, as(testBob), permSecrets, ProjectScope(projectA), false)

	// An unbound role can still be reshaped freely by a roles manager.
	free, err := f.service.CreateRole(as(testManager), RoleInput{Name: "Free", Rules: []RoleRule{{permDocs, Allow}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.UpdateRole(as(testManager), free.ID, RoleInput{
		Name: "Free", Rules: []RoleRule{{permSecrets, Allow}},
	}); err != nil {
		t.Fatalf("UpdateRole(unbound) error = %v", err)
	}

	// Deleting a bound role with unbind takes back only what the manager could give.
	if err := f.service.DeleteRole(as(testManager), role.ID, DeleteRoleOptions{Unbind: true}); err != nil {
		t.Fatalf("DeleteRole(Unbind) error = %v", err)
	}
}

func TestOnlyManagementHoldersMayReadPolicy(t *testing.T) {
	f := newFixture(t)
	mustSet(t, f.service, as(testAdmin), testAlice, permSecrets, Allow, ProjectScope(projectA))

	if _, err := f.service.Assignments(as(testManager)); !errors.Is(err, ErrDenied) {
		t.Fatalf("Assignments() error = %v, want ErrDenied", err)
	}
	if _, err := f.service.Roles(context.Background()); !errors.Is(err, ErrActorRequired) {
		t.Fatalf("Roles() error = %v, want ErrActorRequired", err)
	}
	grantManagement(t, f, PermissionRolesManage)
	if _, err := f.service.Bindings(as(testManager)); err != nil {
		t.Fatalf("Bindings() error = %v", err)
	}
	list, err := f.service.Assignments(as(testAdmin))
	if err != nil || len(list) != 2 {
		t.Fatalf("Assignments() = %v, %v; want the two assignments", list, err)
	}
}

func TestMutationsRequireAnActor(t *testing.T) {
	f := newFixture(t)
	_, err := f.service.SetAssignment(context.Background(), AssignmentInput{
		UserEmail: testAlice, Permission: permSecrets, Effect: Allow, Scope: ProjectScope(projectA),
	})
	requireError(t, err, ErrActorRequired)
	_, err = f.service.CreateRole(context.Background(), RoleInput{Name: "X", Rules: []RoleRule{{permDocs, Allow}}})
	requireError(t, err, ErrActorRequired)
}

func TestEverySuccessfulMutationIsAudited(t *testing.T) {
	f := newFixture(t)
	scope := ProjectScope(projectA)
	role := mustRole(t, f.service, "Operators", RoleRule{permDocs, Allow})
	mustSet(t, f.service, as(testAdmin), testAlice, permSecrets, Allow, scope)
	mustBind(t, f.service, as(testAdmin), role.ID, testAlice, scope)
	if _, err := f.service.UpdateRole(as(testAdmin), role.ID, RoleInput{Name: "Ops", Rules: []RoleRule{{permDocs, Deny}}}); err != nil {
		t.Fatal(err)
	}
	if err := f.service.UnbindRole(as(testAdmin), BindingInput{RoleID: role.ID, UserEmail: testAlice, Scope: scope}); err != nil {
		t.Fatal(err)
	}
	if err := f.service.RemoveAssignment(as(testAdmin), AssignmentTarget{UserEmail: testAlice, Permission: permSecrets, Scope: scope}); err != nil {
		t.Fatal(err)
	}
	if err := f.service.DeleteRole(as(testAdmin), role.ID, DeleteRoleOptions{}); err != nil {
		t.Fatal(err)
	}

	want := []AuditOperation{
		AuditRoleCreated, AuditAssignmentSet, AuditRoleBound, AuditRoleUpdated,
		AuditRoleUnbound, AuditAssignmentRemoved, AuditRoleDeleted,
	}
	log := f.repo.auditLog()
	if len(log) != len(want) {
		t.Fatalf("audit log has %d events, want %d: %#v", len(log), len(want), log)
	}
	for i, operation := range want {
		if log[i].Operation != operation || log[i].Actor != testAdmin || log[i].At == 0 {
			t.Fatalf("event %d = %#v, want %s by %s with a timestamp", i, log[i], operation, testAdmin)
		}
	}
	if log[1].TargetUser != testAlice || log[1].Permission != permSecrets || log[1].Scope != scope || log[1].NewEffect != Allow {
		t.Fatalf("assignment event = %#v", log[1])
	}
}

func TestAuditFailureRejectsTheMutation(t *testing.T) {
	f := newFixture(t)
	f.repo.failAudit = errors.New("disk full")

	_, err := f.service.SetAssignment(as(testAdmin), AssignmentInput{
		UserEmail: testAlice, Permission: permSecrets, Effect: Allow, Scope: ProjectScope(projectA),
	})
	requireError(t, err, ErrAuditFailed)
	_, err = f.service.CreateRole(as(testAdmin), RoleInput{Name: "X", Rules: []RoleRule{{permDocs, Allow}}})
	requireError(t, err, ErrAuditFailed)

	state, _ := f.repo.Load(context.Background())
	if len(state.Assignments)+len(state.Roles)+len(state.Bindings) != 0 {
		t.Fatalf("policy changed despite audit failure: %#v", state)
	}
	requireAllowed(t, f.service, as(testAlice), permSecrets, ProjectScope(projectA), false)
}

// A manager whose management permission is revoked concurrently with their
// grant must never complete a grant that was authorized against a stale view:
// in the audit log, a successful grant precedes the revocation, and a refused
// one follows it.
func TestConcurrentRevocationAndGrantCannotPassAStaleCheck(t *testing.T) {
	for range 200 {
		f := newFixture(t)
		grantManagement(t, f, PermissionAssignmentsManage)

		var wg sync.WaitGroup
		var grantErr, revokeErr error
		start := make(chan struct{})
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			_, grantErr = f.service.SetAssignment(as(testManager), AssignmentInput{
				UserEmail: testBob, Permission: permLifecycle, Effect: Allow, Scope: ProjectScope(projectA),
			})
		}()
		go func() {
			defer wg.Done()
			<-start
			revokeErr = f.service.RemoveAssignment(as(testAdmin), AssignmentTarget{
				UserEmail: testManager, Permission: PermissionAssignmentsManage, Scope: PlatformScope(),
			})
		}()
		close(start)
		wg.Wait()

		if revokeErr != nil {
			t.Fatalf("revocation error = %v", revokeErr)
		}
		var grantIndex, revokeIndex = -1, -1
		for i, event := range f.repo.auditLog() {
			switch {
			case event.Operation == AuditAssignmentRemoved && event.TargetUser == testManager:
				revokeIndex = i
			case event.Operation == AuditAssignmentSet && event.TargetUser == testBob:
				grantIndex = i
			}
		}
		switch {
		case grantErr == nil && (grantIndex < 0 || grantIndex > revokeIndex):
			t.Fatalf("grant succeeded after the revocation took effect (grant %d, revoke %d)", grantIndex, revokeIndex)
		case grantErr != nil && !errors.Is(grantErr, ErrDenied):
			t.Fatalf("grant error = %v, want nil or ErrDenied", grantErr)
		case grantErr != nil && grantIndex >= 0:
			t.Fatal("a refused grant was still recorded")
		}
	}
}
