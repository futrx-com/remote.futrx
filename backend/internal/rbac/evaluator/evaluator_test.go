package evaluator

import (
	"errors"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/rbac/models"
)

const key models.Key = "projects.lifecycle.manage"

var (
	projectA = models.ProjectScope("a")
	projectB = models.ProjectScope("b")
	errLook  = errors.New("lookup failed")
)

func def(b models.BaselinePolicy) models.Definition {
	return models.Definition{Key: key, Baseline: b}
}

func user(mod ...func(*Facts)) Facts {
	f := Facts{ActorPresent: true, Email: "u@example.com", Registered: true}
	for _, m := range mod {
		m(&f)
	}
	return f
}

func assignment(scope models.Scope, effect models.Effect) models.Assignment {
	return models.Assignment{UserEmail: "u@example.com", Permission: key, Scope: scope, Effect: effect}
}

func role(scope models.Scope, effect models.Effect) models.State {
	return models.State{
		Roles:    []models.Role{{ID: "r", Rules: []models.RoleRule{{Permission: key, Effect: effect}}}},
		Bindings: []models.RoleBinding{{UserEmail: "u@example.com", RoleID: "r", Scope: scope}},
	}
}

func TestDecide(t *testing.T) {
	onA := models.Check{Permission: key, Scope: projectA}
	onPlatform := models.Check{Permission: key, Scope: models.PlatformScope()}
	admin := func(f *Facts) { f.Admin = true }
	member := func(f *Facts) { f.MembershipResolved = true; f.ProjectMember = true }
	lookupFailed := func(f *Facts) { f.MembershipResolved = true; f.MembershipErr = errLook }
	nonMember := func(f *Facts) { f.MembershipResolved = true }
	unregistered := func(f *Facts) { f.Registered = false }
	allowA := models.State{Assignments: []models.Assignment{assignment(projectA, models.Allow)}}
	allowB := models.State{Assignments: []models.Assignment{assignment(projectB, models.Allow)}}
	denyA := models.State{Assignments: []models.Assignment{assignment(projectA, models.Deny)}}
	allowAndDenyA := models.State{Assignments: []models.Assignment{
		assignment(projectA, models.Allow), assignment(projectA, models.Deny),
	}}
	roleDenyAllowA := role(projectA, models.Deny)
	roleDenyAllowA.Assignments = allowA.Assignments

	tests := []struct {
		name    string
		def     models.Definition
		policy  models.State
		facts   Facts
		check   models.Check
		want    bool
		why     Reason
		wantErr error
	}{
		{"no actor fails closed", def(models.BaselineAuthenticated), models.State{}, Facts{}, onA, false, ReasonActorNotPresent, ErrActorRequired},
		{"system", def(models.BaselineNone), models.State{}, Facts{ActorPresent: true, System: true}, onA, true, ReasonSystem, nil},
		{"admin beats deny", def(models.BaselineNone), denyA, user(admin), onA, true, ReasonAdministrator, nil},
		{"unregistered", def(models.BaselineAuthenticated), models.State{}, user(unregistered), onA, false, ReasonUnknownActor, nil},
		{"explicit deny beats allow", def(models.BaselineNone), allowAndDenyA, user(), onA, false, ReasonExplicitDeny, nil},
		{"explicit deny beats baseline", def(models.BaselineAuthenticated), denyA, user(), onA, false, ReasonExplicitDeny, nil},
		{"explicit allow", def(models.BaselineNone), allowA, user(), onA, true, ReasonExplicitAllow, nil},
		{"allow at other scope ignored", def(models.BaselineNone), allowB, user(), onA, false, ReasonDefaultDeny, nil},
		{"project allow ignored on platform", def(models.BaselineNone), allowA, user(), onPlatform, false, ReasonDefaultDeny, nil},
		{"role allow via binding", def(models.BaselineNone), role(projectA, models.Allow), user(), onA, true, ReasonExplicitAllow, nil},
		{"role deny via binding", def(models.BaselineAuthenticated), role(projectA, models.Deny), user(), onA, false, ReasonExplicitDeny, nil},
		{"role bound at other scope ignored", def(models.BaselineNone), role(projectB, models.Allow), user(), onA, false, ReasonDefaultDeny, nil},
		{"role deny beats direct allow", def(models.BaselineNone), roleDenyAllowA, user(), onA, false, ReasonExplicitDeny, nil},
		{"baseline authenticated", def(models.BaselineAuthenticated), models.State{}, user(), onA, true, ReasonBaseline, nil},
		{"baseline none", def(models.BaselineNone), models.State{}, user(), onA, false, ReasonDefaultDeny, nil},
		{"baseline admin denies non-admin", def(models.BaselineAdmin), models.State{}, user(), onA, false, ReasonDefaultDeny, nil},
		{"baseline member", def(models.BaselineProjectMember), models.State{}, user(member), onA, true, ReasonBaseline, nil},
		{"baseline non-member", def(models.BaselineProjectMember), models.State{}, user(nonMember), onA, false, ReasonDefaultDeny, nil},
		{"member baseline ignores platform scope", def(models.BaselineProjectMember), models.State{}, user(member, lookupFailed), onPlatform, false, ReasonDefaultDeny, nil},
		{"membership not yet resolved asks the caller", def(models.BaselineProjectMember), models.State{}, user(), onA, false, ReasonDefaultDeny, ErrMembershipUnresolved},
		{"lookup failure fails closed", def(models.BaselineProjectMember), models.State{}, user(lookupFailed), onA, false, ReasonDefaultDeny, errLook},
		{"membership error irrelevant when allow decides", def(models.BaselineProjectMember), allowA, user(lookupFailed), onA, true, ReasonExplicitAllow, nil},
		{"unknown baseline fails closed", def("bogus"), models.State{}, user(), onA, false, ReasonDefaultDeny, errors.New("any")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Decide(tc.def, tc.policy, tc.facts, tc.check)
			switch {
			case tc.wantErr == nil && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tc.wantErr != nil && err == nil:
				t.Fatalf("expected error %v", tc.wantErr)
			case tc.wantErr != nil && tc.wantErr.Error() != "any":
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
			}
			if got.Allowed != tc.want || got.Reason != tc.why {
				t.Fatalf("got %+v, want allowed=%v reason=%s", got, tc.want, tc.why)
			}
		})
	}
}
