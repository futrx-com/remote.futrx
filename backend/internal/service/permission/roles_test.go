package permission

import (
	"context"
	"testing"
)

func TestOneRoleGroupsSeveralPermissionsAndIsBoundOnce(t *testing.T) {
	f := newFixture(t)
	role := mustRole(t, f.service, "Operators",
		RoleRule{permSecrets, Allow}, RoleRule{permDocs, Allow})

	mustBind(t, f.service, as(testAdmin), role.ID, testBob, ProjectScope(projectB))

	requireAllowed(t, f.service, as(testBob), permSecrets, ProjectScope(projectB), true)
	requireAllowed(t, f.service, as(testBob), permDocs, ProjectScope(projectB), true)
	requireAllowed(t, f.service, as(testBob), permDocs, ProjectScope(projectA), false)
}

func TestUpdatingARoleChangesItsBindingsWithoutRewritingUsers(t *testing.T) {
	f := newFixture(t)
	role := mustRole(t, f.service, "Operators", RoleRule{permSecrets, Allow})
	mustBind(t, f.service, as(testAdmin), role.ID, testBob, ProjectScope(projectB))
	before, _ := f.repo.Load(context.Background())

	updated, err := f.service.UpdateRole(as(testAdmin), role.ID, RoleInput{
		Name: "Operators", Rules: []RoleRule{{permSecrets, Deny}},
	})
	if err != nil {
		t.Fatal(err)
	}

	requireAllowed(t, f.service, as(testBob), permSecrets, ProjectScope(projectB), false)
	after, _ := f.repo.Load(context.Background())
	if len(after.Bindings) != 1 || after.Bindings[0] != before.Bindings[0] {
		t.Fatalf("bindings were rewritten: before %#v, after %#v", before.Bindings, after.Bindings)
	}
	if updated.UpdatedAt <= role.UpdatedAt || updated.CreatedAt != role.CreatedAt {
		t.Fatalf("timestamps: created %d->%d, updated %d->%d",
			role.CreatedAt, updated.CreatedAt, role.UpdatedAt, updated.UpdatedAt)
	}
}

func TestBoundRolesCannotBeDeletedByAccident(t *testing.T) {
	f := newFixture(t)
	role := mustRole(t, f.service, "Operators", RoleRule{permSecrets, Allow})
	mustBind(t, f.service, as(testAdmin), role.ID, testBob, ProjectScope(projectB))

	requireError(t, f.service.DeleteRole(as(testAdmin), role.ID, DeleteRoleOptions{}), ErrRoleInUse)
	if roles, _ := f.repo.Load(context.Background()); len(roles.Roles) != 1 || len(roles.Bindings) != 1 {
		t.Fatalf("state changed by a refused delete: %#v", roles)
	}

	if err := f.service.DeleteRole(as(testAdmin), role.ID, DeleteRoleOptions{Unbind: true}); err != nil {
		t.Fatalf("DeleteRole(Unbind) error = %v", err)
	}
	state, _ := f.repo.Load(context.Background())
	if len(state.Roles) != 0 || len(state.Bindings) != 0 {
		t.Fatalf("state after delete-with-unbind = %#v", state)
	}
	requireAllowed(t, f.service, as(testBob), permSecrets, ProjectScope(projectB), false)
}

func TestDeletingAMissingRoleIsANoOp(t *testing.T) {
	f := newFixture(t)
	if err := f.service.DeleteRole(as(testAdmin), "nope", DeleteRoleOptions{}); err != nil {
		t.Fatal(err)
	}
	if len(f.repo.auditLog()) != 0 {
		t.Fatal("a no-op delete was audited")
	}
}

func TestRoleValidation(t *testing.T) {
	f := newFixture(t)
	mustRole(t, f.service, "Operators", RoleRule{permSecrets, Allow})

	tests := []struct {
		name string
		in   RoleInput
		want error
	}{
		{"empty name", RoleInput{Name: "  ", Rules: []RoleRule{{permSecrets, Allow}}}, ErrInvalidRole},
		{"long name", RoleInput{Name: string(make([]byte, 81)), Rules: []RoleRule{{permSecrets, Allow}}}, ErrInvalidRole},
		{"duplicate name ignoring case", RoleInput{Name: "OPERATORS", Rules: []RoleRule{{permDocs, Allow}}}, ErrInvalidRole},
		{"no rules", RoleInput{Name: "Empty"}, ErrInvalidRole},
		{"unknown key", RoleInput{Name: "Bad", Rules: []RoleRule{{"projects.nothing.manage", Allow}}}, ErrUnknownPermission},
		{"bad effect", RoleInput{Name: "Bad", Rules: []RoleRule{{permDocs, "maybe"}}}, ErrInvalidEffect},
		{"repeated permission", RoleInput{Name: "Bad", Rules: []RoleRule{{permDocs, Allow}, {permDocs, Deny}}}, ErrInvalidRole},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := f.service.CreateRole(as(testAdmin), test.in)
			requireError(t, err, test.want)
		})
	}
}

func TestRoleCannotBecomeBroaderThanItsBindingScope(t *testing.T) {
	f := newFixture(t)
	projectOnly := mustRole(t, f.service, "Project only", RoleRule{permSecrets, Allow})

	_, err := f.service.BindRole(as(testAdmin), BindingInput{
		RoleID: projectOnly.ID, UserEmail: testBob, Scope: PlatformScope(),
	})
	requireError(t, err, ErrInvalidScope)

	// Narrowing a role's rules to a kind its existing bindings do not support
	// is refused for the same reason.
	platformBound := mustRole(t, f.service, "Docs", RoleRule{permDocs, Allow})
	mustBind(t, f.service, as(testAdmin), platformBound.ID, testBob, PlatformScope())
	_, err = f.service.UpdateRole(as(testAdmin), platformBound.ID, RoleInput{
		Name: "Docs", Rules: []RoleRule{{permLifecycle, Allow}},
	})
	requireError(t, err, ErrInvalidScope)
}

func TestRepeatedCommandsAreIdempotent(t *testing.T) {
	f := newFixture(t)
	scope := ProjectScope(projectA)
	role := mustRole(t, f.service, "Operators", RoleRule{permSecrets, Allow})

	mustSet(t, f.service, as(testAdmin), testAlice, permSecrets, Allow, scope)
	mustSet(t, f.service, as(testAdmin), testAlice, permSecrets, Allow, scope)
	mustBind(t, f.service, as(testAdmin), role.ID, testAlice, scope)
	mustBind(t, f.service, as(testAdmin), role.ID, testAlice, scope)

	state, _ := f.repo.Load(context.Background())
	if len(state.Assignments) != 1 || len(state.Bindings) != 1 {
		t.Fatalf("state = %#v, want one assignment and one binding", state)
	}
	// role.created + assignment.set + binding.created; the repeats add nothing.
	if got := len(f.repo.auditLog()); got != 3 {
		t.Fatalf("audit events = %d, want 3", got)
	}
}

func TestSettingADifferentEffectReplacesTheAssignment(t *testing.T) {
	f := newFixture(t)
	scope := ProjectScope(projectA)
	mustSet(t, f.service, as(testAdmin), testAlice, permSecrets, Allow, scope)
	mustSet(t, f.service, as(testAdmin), testAlice, permSecrets, Deny, scope)

	state, _ := f.repo.Load(context.Background())
	if len(state.Assignments) != 1 || state.Assignments[0].Effect != Deny {
		t.Fatalf("assignments = %#v, want one deny", state.Assignments)
	}
	last := f.repo.auditLog()[1]
	if last.OldEffect != Allow || last.NewEffect != Deny {
		t.Fatalf("audit event = %#v, want allow -> deny", last)
	}
}

func TestRemovingMissingAssignmentsAndBindingsIsANoOp(t *testing.T) {
	f := newFixture(t)
	role := mustRole(t, f.service, "Operators", RoleRule{permSecrets, Allow})
	events := len(f.repo.auditLog())

	err := f.service.RemoveAssignment(as(testAdmin), AssignmentTarget{
		UserEmail: testAlice, Permission: permSecrets, Scope: ProjectScope(projectA),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.service.UnbindRole(as(testAdmin), BindingInput{
		RoleID: role.ID, UserEmail: testAlice, Scope: ProjectScope(projectA),
	}); err != nil {
		t.Fatal(err)
	}
	if got := len(f.repo.auditLog()); got != events {
		t.Fatalf("no-op removals were audited: %d events, want %d", got, events)
	}
}

func TestRemovingAssignmentsAndBindingsRevokesAccess(t *testing.T) {
	f := newFixture(t)
	scope := ProjectScope(projectA)
	role := mustRole(t, f.service, "Operators", RoleRule{permDocs, Allow})
	mustSet(t, f.service, as(testAdmin), testAlice, permSecrets, Allow, scope)
	mustBind(t, f.service, as(testAdmin), role.ID, testAlice, scope)

	if err := f.service.RemoveAssignment(as(testAdmin), AssignmentTarget{
		UserEmail: " Alice@Example.com ", Permission: permSecrets, Scope: scope,
	}); err != nil {
		t.Fatal(err)
	}
	if err := f.service.UnbindRole(as(testAdmin), BindingInput{RoleID: role.ID, UserEmail: testAlice, Scope: scope}); err != nil {
		t.Fatal(err)
	}

	requireAllowed(t, f.service, as(testAlice), permSecrets, scope, false)
	requireAllowed(t, f.service, as(testAlice), permDocs, scope, false)
}

func TestMutationTargetsMustBeRegisteredUsers(t *testing.T) {
	f := newFixture(t)
	role := mustRole(t, f.service, "Operators", RoleRule{permSecrets, Allow})

	_, err := f.service.SetAssignment(as(testAdmin), AssignmentInput{
		UserEmail: testUnknown, Permission: permSecrets, Effect: Allow, Scope: ProjectScope(projectA),
	})
	requireError(t, err, ErrUserNotRegistered)
	_, err = f.service.BindRole(as(testAdmin), BindingInput{
		RoleID: role.ID, UserEmail: testUnknown, Scope: ProjectScope(projectA),
	})
	requireError(t, err, ErrUserNotRegistered)
}

func TestAssignmentInputIsValidated(t *testing.T) {
	f := newFixture(t)
	tests := []struct {
		name string
		in   AssignmentInput
		want error
	}{
		{"unknown key", AssignmentInput{testAlice, "projects.nothing.manage", Allow, PlatformScope()}, ErrUnknownPermission},
		{"bad effect", AssignmentInput{testAlice, permSecrets, "maybe", ProjectScope(projectA)}, ErrInvalidEffect},
		{"wrong scope kind", AssignmentInput{testAlice, permSecrets, Allow, PlatformScope()}, ErrInvalidScope},
		{"malformed scope", AssignmentInput{testAlice, permSecrets, Allow, ProjectScope("")}, ErrInvalidScope},
		{"no user", AssignmentInput{"  ", permSecrets, Allow, ProjectScope(projectA)}, ErrUserNotRegistered},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := f.service.SetAssignment(as(testAdmin), test.in)
			requireError(t, err, test.want)
		})
	}
}

func TestRoleRecordsAreIndependentOfTheUserDirectory(t *testing.T) {
	f := newFixture(t)
	role := mustRole(t, f.service, "Operators", RoleRule{permSecrets, Allow})
	mustBind(t, f.service, as(testAdmin), role.ID, testBob, ProjectScope(projectB))

	// Binding a custom role must not make its holder an administrator.
	admin, err := f.identity.IsAdmin(context.Background(), testBob)
	if err != nil || admin {
		t.Fatalf("IsAdmin(bob) = %v, %v; want false", admin, err)
	}
	requireAllowed(t, f.service, as(testBob), permAdminOnly, PlatformScope(), false)
}

func TestRemoveUserPolicyDeletesAssignmentsAndBindingsButKeepsRoles(t *testing.T) {
	f := newFixture(t)
	scope := ProjectScope(projectA)
	role := mustRole(t, f.service, "Operators", RoleRule{permDocs, Allow})
	mustSet(t, f.service, as(testAdmin), testAlice, permSecrets, Allow, scope)
	mustSet(t, f.service, as(testAdmin), testBob, permSecrets, Allow, scope)
	mustBind(t, f.service, as(testAdmin), role.ID, testAlice, scope)
	mustBind(t, f.service, as(testAdmin), role.ID, testBob, scope)

	if err := f.service.RemoveUserPolicy(context.Background(), " Alice@Example.com "); err != nil {
		t.Fatal(err)
	}
	if err := f.service.RemoveUserPolicy(context.Background(), testAlice); err != nil {
		t.Fatalf("second cleanup error = %v, want idempotent", err)
	}

	state, _ := f.repo.Load(context.Background())
	for _, a := range state.Assignments {
		if a.UserEmail == testAlice {
			t.Fatalf("assignment survived cleanup: %#v", a)
		}
	}
	for _, b := range state.Bindings {
		if b.UserEmail == testAlice {
			t.Fatalf("binding survived cleanup: %#v", b)
		}
	}
	if len(state.Assignments) != 1 || len(state.Bindings) != 1 || len(state.Roles) != 1 {
		t.Fatalf("cleanup removed other users' policy or the role: %#v", state)
	}
	log := f.repo.auditLog()
	last := log[len(log)-1]
	if last.Actor != SystemActorName || last.TargetUser != testAlice {
		t.Fatalf("last audit event = %#v, want a system-attributed cleanup for alice", last)
	}
}
