package rbac

import (
	"context"
	"errors"
	"testing"
)

func TestEffectiveUsesCurrentActorExactScopeAndFreshPolicy(t *testing.T) {
	f := newFixture(t)
	if _, err := f.service.Effective(context.Background(), PlatformScope()); !errors.Is(err, ErrActorRequired) {
		t.Fatalf("no actor: %v", err)
	}
	if _, err := f.service.Effective(as(testAlice), ProjectScope("../bad")); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("bad scope: %v", err)
	}
	for _, scope := range []Scope{ProjectScope(projectA), ProjectScope(projectB)} {
		got, err := f.service.Effective(as(testAlice), scope)
		if err != nil {
			t.Fatal(err)
		}
		if got[permLifecycle] != (scope.ID == projectA) {
			t.Fatalf("scope %v: %v", scope, got)
		}
		if _, ok := got[permReports]; ok {
			t.Fatal("platform permission returned for project")
		}
	}
	mustSet(t, f.service, as(testAdmin), testAlice, permLifecycle, Deny, ProjectScope(projectA))
	got, err := f.service.Effective(as(testAlice), ProjectScope(projectA))
	if err != nil || got[permLifecycle] {
		t.Fatalf("revocation: %v %v", got, err)
	}
	got, err = f.service.Effective(as(testAdmin), ProjectScope(projectA))
	if err != nil || !got[permLifecycle] {
		t.Fatalf("admin: %v %v", got, err)
	}
	if _, _, err := f.service.Policy(as(testAlice)); !errors.Is(err, ErrDenied) {
		t.Fatalf("policy read: %v", err)
	}
	state, defs, err := f.service.Policy(as(testAdmin))
	if err != nil || len(defs) == 0 || len(state.Assignments) != 1 {
		t.Fatalf("policy: %v %v", state, err)
	}
}
