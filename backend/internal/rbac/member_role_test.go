package rbac

import (
	"context"
	"slices"
	"testing"
	"time"
)

const permAccountUse = Key("agents.account.use") // provider-account scope, no baseline

func memberTestRegistry() *Registry {
	return MustRegistry(
		testRegistry().Definitions(),
		[]Definition{{Key: permAccountUse, Description: "Use a provider account.",
			Scopes: []ScopeKind{ScopeProviderAccount}, Delegable: true}},
	)
}

func newMemberService(t *testing.T, repo *memoryRepository) *Service {
	t.Helper()
	service, err := NewService(
		context.Background(), memberTestRegistry(), repo,
		&fakeIdentity{
			admins:     map[string]bool{testAdmin: true},
			registered: map[string]bool{testAlice: true, testBob: true},
		},
		fakeMembers{projectA: {testAlice: true, testBob: true}},
		WithClock(func() time.Time { return time.Unix(1_700_000_000, 0) }),
		WithBuiltinMemberRole(permAccountUse, permSecrets),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return service
}

func ruleKeys(role Role) []Key {
	keys := make([]Key, 0, len(role.Rules))
	for _, rule := range role.Rules {
		keys = append(keys, rule.Permission)
	}
	return keys
}

func TestBuiltinMemberRoleAllowsMemberDefaultsAndIsSeededOnce(t *testing.T) {
	repo := &memoryRepository{}
	service := newMemberService(t, repo)

	state, _ := repo.Load(context.Background())
	role, ok := state.Role(MemberRoleID)
	if !ok || role.Name != "member" || !state.MemberRoleSeeded {
		t.Fatalf("seeded state = %+v", state)
	}
	keys := ruleKeys(role)
	for _, want := range []Key{permLifecycle, permAccess, permReports, permAccountUse, permSecrets} {
		if !slices.Contains(keys, want) {
			t.Errorf("member role is missing %s: %v", want, keys)
		}
	}
	for _, unwanted := range []Key{permBilling, permAdminOnly, permDocs, PermissionRolesManage, PermissionAssignmentsManage} {
		if slices.Contains(keys, unwanted) {
			t.Errorf("member role grants %s: %v", unwanted, keys)
		}
	}

	if err := service.DeleteRole(as(testAdmin), MemberRoleID, DeleteRoleOptions{}); err != nil {
		t.Fatalf("DeleteRole() error = %v", err)
	}
	newMemberService(t, repo)
	state, _ = repo.Load(context.Background())
	if _, ok := state.Role(MemberRoleID); ok {
		t.Fatal("deleted member role was seeded again on restart")
	}
}

func TestMemberRoleAppliesOnlyToUsersWithoutAnotherPlatformRole(t *testing.T) {
	service := newMemberService(t, &memoryRepository{})
	account := ProviderAccountScope("claude", "work")

	// Any account, including one connected later, without a grant.
	requireAllowed(t, service, as(testAlice), permAccountUse, account, true)
	requireAllowed(t, service, as(testAlice), permAccountUse, ProviderAccountScope("codex", "team"), true)

	// A user who already holds a platform role keeps it instead of member.
	viewer := mustRole(t, service, "viewer", RoleRule{Permission: permReports, Effect: Allow})
	mustBind(t, service, as(testAdmin), viewer.ID, testBob, PlatformScope())
	requireAllowed(t, service, as(testBob), permAccountUse, account, false)
	requireAllowed(t, service, as(testBob), permReports, PlatformScope(), true)

	// A project-scoped binding is an extra grant, not a replacement role.
	editor := mustRole(t, service, "editor", RoleRule{Permission: permDocs, Effect: Allow})
	mustBind(t, service, as(testAdmin), editor.ID, testAlice, ProjectScope(projectA))
	requireAllowed(t, service, as(testAlice), permAccountUse, account, true)
}

func TestMemberRoleProjectAllowRequiresMembership(t *testing.T) {
	service := newMemberService(t, &memoryRepository{})
	requireAllowed(t, service, as(testAlice), permSecrets, ProjectScope(projectA), true)
	requireAllowed(t, service, as(testAlice), permSecrets, ProjectScope(projectB), false)
}

func TestMemberRoleDenyRemovesADefaultUnlessTheUserIsGrantedIt(t *testing.T) {
	service := newMemberService(t, &memoryRepository{})
	account := ProviderAccountScope("claude", "work")
	if _, err := service.UpdateRole(as(testAdmin), MemberRoleID, RoleInput{
		Name: "member",
		Rules: []RoleRule{
			{Permission: permAccountUse, Effect: Deny},
			{Permission: permReports, Effect: Deny},
		},
	}); err != nil {
		t.Fatalf("UpdateRole() error = %v", err)
	}
	requireAllowed(t, service, as(testAlice), permAccountUse, account, false)
	requireAllowed(t, service, as(testAlice), permReports, PlatformScope(), false)
	// Removing a rule falls back to the code-owned baseline.
	requireAllowed(t, service, as(testAlice), permLifecycle, ProjectScope(projectA), true)

	mustSet(t, service, as(testAdmin), testAlice, permAccountUse, Allow, account)
	requireAllowed(t, service, as(testAlice), permAccountUse, account, true)
	requireAllowed(t, service, as(testAlice), permAccountUse, ProviderAccountScope("claude", "other"), false)
	requireAllowed(t, service, as(testAdmin), permAccountUse, account, true)
}

func TestMemberRoleCannotBeBound(t *testing.T) {
	service := newMemberService(t, &memoryRepository{})
	_, err := service.BindRole(as(testAdmin), BindingInput{
		RoleID: MemberRoleID, UserEmail: testAlice, Scope: PlatformScope(),
	})
	requireError(t, err, ErrInvalidRole)
}
