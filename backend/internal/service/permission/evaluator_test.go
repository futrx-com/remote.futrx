package permission

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestEvaluationTable(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, f *fixture)
		actor string
		key   Key
		scope Scope
		want  bool
		why   Reason
	}{
		{
			name: "no baseline denies by default", actor: testAlice, key: permSecrets,
			scope: ProjectScope(projectA), want: false, why: ReasonDefaultDeny,
		},
		{
			name: "project member baseline allows a member", actor: testAlice, key: permLifecycle,
			scope: ProjectScope(projectA), want: true, why: ReasonBaseline,
		},
		{
			name: "project member baseline denies a non-member", actor: testOutsider, key: permLifecycle,
			scope: ProjectScope(projectA), want: false, why: ReasonDefaultDeny,
		},
		{
			name: "membership is per project", actor: testAlice, key: permLifecycle,
			scope: ProjectScope(projectB), want: false, why: ReasonDefaultDeny,
		},
		{
			name: "authenticated baseline allows any registered user", actor: testOutsider, key: permReports,
			scope: PlatformScope(), want: true, why: ReasonBaseline,
		},
		{
			name: "admin baseline denies a member", actor: testManager, key: permAdminOnly,
			scope: PlatformScope(), want: false, why: ReasonDefaultDeny,
		},
		{
			name: "administrators are allowed every registered permission", actor: testAdmin, key: permBilling,
			scope: PlatformScope(), want: true, why: ReasonAdministrator,
		},
		{
			name: "unregistered human is denied even with an authenticated baseline", actor: testUnknown,
			key: permReports, scope: PlatformScope(), want: false, why: ReasonUnknownActor,
		},
		{
			name: "direct allow works at the exact scope", actor: testAlice, key: permSecrets,
			scope: ProjectScope(projectA), want: true, why: ReasonExplicitAllow,
			setup: func(t *testing.T, f *fixture) {
				mustSet(t, f.service, as(testAdmin), testAlice, permSecrets, Allow, ProjectScope(projectA))
			},
		},
		{
			name: "a rule for project A does not match project B", actor: testAlice, key: permSecrets,
			scope: ProjectScope(projectB), want: false, why: ReasonDefaultDeny,
			setup: func(t *testing.T, f *fixture) {
				mustSet(t, f.service, as(testAdmin), testAlice, permSecrets, Allow, ProjectScope(projectA))
			},
		},
		{
			name: "a platform allow does not match a project check", actor: testAlice, key: permDocs,
			scope: ProjectScope(projectA), want: false, why: ReasonDefaultDeny,
			setup: func(t *testing.T, f *fixture) {
				mustSet(t, f.service, as(testAdmin), testAlice, permDocs, Allow, PlatformScope())
			},
		},
		{
			name: "a project allow does not match a platform check", actor: testAlice, key: permDocs,
			scope: PlatformScope(), want: false, why: ReasonDefaultDeny,
			setup: func(t *testing.T, f *fixture) {
				mustSet(t, f.service, as(testAdmin), testAlice, permDocs, Allow, ProjectScope(projectA))
			},
		},
		{
			name: "direct deny overrides the compatibility baseline", actor: testAlice, key: permLifecycle,
			scope: ProjectScope(projectA), want: false, why: ReasonExplicitDeny,
			setup: func(t *testing.T, f *fixture) {
				mustSet(t, f.service, as(testAdmin), testAlice, permLifecycle, Deny, ProjectScope(projectA))
			},
		},
		{
			name: "direct deny overrides a role allow", actor: testAlice, key: permSecrets,
			scope: ProjectScope(projectA), want: false, why: ReasonExplicitDeny,
			setup: func(t *testing.T, f *fixture) {
				role := mustRole(t, f.service, "Revealers", RoleRule{permSecrets, Allow})
				mustBind(t, f.service, as(testAdmin), role.ID, testAlice, ProjectScope(projectA))
				mustSet(t, f.service, as(testAdmin), testAlice, permSecrets, Deny, ProjectScope(projectA))
			},
		},
		{
			name: "role allow grants through a binding", actor: testAlice, key: permSecrets,
			scope: ProjectScope(projectA), want: true, why: ReasonExplicitAllow,
			setup: func(t *testing.T, f *fixture) {
				role := mustRole(t, f.service, "Revealers", RoleRule{permSecrets, Allow})
				mustBind(t, f.service, as(testAdmin), role.ID, testAlice, ProjectScope(projectA))
			},
		},
		{
			name: "a role deny overrides another role's allow", actor: testAlice, key: permSecrets,
			scope: ProjectScope(projectA), want: false, why: ReasonExplicitDeny,
			setup: func(t *testing.T, f *fixture) {
				allow := mustRole(t, f.service, "Allow", RoleRule{permSecrets, Allow})
				deny := mustRole(t, f.service, "Deny", RoleRule{permSecrets, Deny})
				mustBind(t, f.service, as(testAdmin), allow.ID, testAlice, ProjectScope(projectA))
				mustBind(t, f.service, as(testAdmin), deny.ID, testAlice, ProjectScope(projectA))
			},
		},
		{
			name: "a role binding at project A does not match project B", actor: testAlice, key: permSecrets,
			scope: ProjectScope(projectB), want: false, why: ReasonDefaultDeny,
			setup: func(t *testing.T, f *fixture) {
				role := mustRole(t, f.service, "Revealers", RoleRule{permSecrets, Allow})
				mustBind(t, f.service, as(testAdmin), role.ID, testAlice, ProjectScope(projectA))
			},
		},
		{
			name: "assignments for another user do not apply", actor: testBob, key: permSecrets,
			scope: ProjectScope(projectA), want: false, why: ReasonDefaultDeny,
			setup: func(t *testing.T, f *fixture) {
				mustSet(t, f.service, as(testAdmin), testAlice, permSecrets, Allow, ProjectScope(projectA))
			},
		},
		{
			name: "an explicit allow for a non-member is honored by the evaluator", actor: testOutsider,
			key: permLifecycle, scope: ProjectScope(projectA), want: true, why: ReasonExplicitAllow,
			setup: func(t *testing.T, f *fixture) {
				mustSet(t, f.service, as(testAdmin), testOutsider, permLifecycle, Allow, ProjectScope(projectA))
			},
		},
		{
			name: "administrators keep root access despite a deny assignment", actor: testAdmin, key: permLifecycle,
			scope: ProjectScope(projectA), want: true, why: ReasonAdministrator,
			setup: func(t *testing.T, f *fixture) {
				mustSet(t, f.service, as(testAdmin), testAdmin, permLifecycle, Deny, ProjectScope(projectA))
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f := newFixture(t)
			if test.setup != nil {
				test.setup(t, f)
			}
			decision, err := f.service.Evaluate(as(test.actor), Check{Permission: test.key, Scope: test.scope})
			if err != nil {
				t.Fatalf("Evaluate() error = %v", err)
			}
			if decision.Allowed != test.want || decision.Reason != test.why {
				t.Fatalf("decision = %+v, want allowed=%v reason=%s", decision, test.want, test.why)
			}
		})
	}
}

func TestRequireReturnsStableErrorsWithoutRevealingTheReason(t *testing.T) {
	f := newFixture(t)
	mustSet(t, f.service, as(testAdmin), testAlice, permLifecycle, Deny, ProjectScope(projectA))

	err := f.service.Require(as(testAlice), Check{Permission: permLifecycle, Scope: ProjectScope(projectA)})
	requireError(t, err, ErrDenied)
	for _, leak := range []string{"deny", "role", "explicit", "assignment"} {
		if strings.Contains(strings.ToLower(err.Error()), leak) {
			t.Fatalf("error %q reveals %q", err, leak)
		}
	}
	if err := f.service.Require(as(testBob), Check{Permission: permReports, Scope: PlatformScope()}); err != nil {
		t.Fatalf("Require() = %v, want nil", err)
	}
}

func TestMissingActorFailsClosed(t *testing.T) {
	f := newFixture(t)
	check := Check{Permission: permReports, Scope: PlatformScope()}

	for name, ctx := range map[string]context.Context{
		"no actor":           context.Background(),
		"empty email":        ContextWithActor(context.Background(), UserActor("  ")),
		"zero value actor":   ContextWithActor(context.Background(), Actor{}),
		"wrong value stored": context.WithValue(context.Background(), actorContextKey{}, "system"),
	} {
		t.Run(name, func(t *testing.T) {
			requireError(t, f.service.Require(ctx, check), ErrActorRequired)
		})
	}
}

func TestSystemActorIsAllowedOnlyWhenExplicit(t *testing.T) {
	f := newFixture(t)
	check := Check{Permission: permBilling, Scope: PlatformScope()}

	if err := f.service.Require(ContextWithSystemActor(context.Background()), check); err != nil {
		t.Fatalf("system Require() = %v, want nil", err)
	}
	if err := f.service.Require(as("system"), check); !errors.Is(err, ErrDenied) {
		t.Fatalf("a human named system: error = %v, want ErrDenied", err)
	}
	// The system flag cannot be forged from outside the package: a literal
	// Actor has no way to set it.
	if UserActor("admin@example.com").IsSystem() {
		t.Fatal("UserActor produced a system actor")
	}
}

func TestInvalidChecksAreRejected(t *testing.T) {
	f := newFixture(t)
	tests := []struct {
		name  string
		check Check
		want  error
	}{
		{"unregistered key", Check{Permission: "projects.nothing.manage", Scope: PlatformScope()}, ErrUnknownPermission},
		{"unsupported scope kind", Check{Permission: permLifecycle, Scope: PlatformScope()}, ErrInvalidScope},
		{"malformed scope", Check{Permission: permLifecycle, Scope: ProjectScope("../etc")}, ErrInvalidScope},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requireError(t, f.service.Require(as(testAdmin), test.check), test.want)
			requireError(t, f.service.Require(ContextWithSystemActor(context.Background()), test.check), test.want)
		})
	}
}

func TestDependencyErrorsFailClosed(t *testing.T) {
	f := newFixture(t)
	f.identity.err = errors.New("directory unavailable")

	err := f.service.Require(as(testAlice), Check{Permission: permReports, Scope: PlatformScope()})
	if err == nil || errors.Is(err, ErrDenied) {
		t.Fatalf("error = %v, want the dependency error", err)
	}
}

func TestNewServiceRejectsStoredPolicyForUnregisteredKeys(t *testing.T) {
	f := newFixture(t)
	repo := &memoryRepository{state: State{Assignments: []Assignment{{
		ID: "a1", UserEmail: testAlice, Permission: "projects.removed.manage",
		Effect: Deny, Scope: ProjectScope(projectA),
	}}}}

	_, err := NewService(context.Background(), testRegistry(), repo, f.identity, f.members)
	requireError(t, err, ErrInvalidState)
	if !strings.Contains(err.Error(), "projects.removed.manage") {
		t.Fatalf("error %q does not name the unknown key", err)
	}
}

func TestNewServiceRequiresItsDependencies(t *testing.T) {
	f := newFixture(t)
	if _, err := NewService(context.Background(), nil, f.repo, f.identity, nil); err == nil {
		t.Fatal("nil registry accepted")
	}
	if _, err := NewService(context.Background(), testRegistry(), nil, f.identity, nil); err == nil {
		t.Fatal("nil repository accepted")
	}
	if _, err := NewService(context.Background(), testRegistry(), f.repo, nil, nil); err == nil {
		t.Fatal("nil identity accepted")
	}
}
