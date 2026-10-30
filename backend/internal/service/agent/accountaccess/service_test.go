package accountaccess

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	"github.com/futrx-com/remote.futrx.com/internal/rbac"
	agentauth "github.com/futrx-com/remote.futrx.com/internal/service/agent/auth"
	"github.com/futrx-com/remote.futrx.com/internal/stores/filepermissions"
)

type identities struct{}

func (identities) IsAdmin(_ context.Context, email string) (bool, error) {
	return email == "admin@example.com", nil
}
func (identities) IsRegistered(_ context.Context, email string) (bool, error) {
	return email == "admin@example.com" || email == "member@example.com", nil
}

type accountSource struct {
	agentauth.AccountController
	snapshot agentauth.AccountsSnapshot
}

func (s *accountSource) AccountsSnapshot() agentauth.AccountsSnapshot { return s.snapshot }
func actor(email string) context.Context {
	return rbac.ContextWithActor(context.Background(), rbac.UserActor(email))
}
func TestAccountPolicyUsesExistingRolesScopesAndPersistence(t *testing.T) {
	dir := t.TempDir()
	repo, err := filepermissions.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	registry := rbac.MustRegistry(rbac.ManagementDefinitions(), agentauth.PermissionDefinitions())
	policy, err := rbac.NewService(context.Background(), registry, repo, identities{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	source := &accountSource{snapshot: agentauth.AccountsSnapshot{ActiveAccountID: "private", Items: []agentauth.Account{{ID: "private", Label: "Private label", Email: "private@example.com", Active: true}, {ID: "team", Label: "Team"}}}}
	binding := agentauth.NewExternalBinding(agent.ProviderCodex).WithAccounts(source)
	access := New(policy, []agentauth.Binding{binding})
	member, admin := actor("member@example.com"), actor("admin@example.com")
	if _, err := access.Resolve(member, agent.ProviderCodex, ""); !errors.Is(err, rbac.ErrDenied) {
		t.Fatalf("default without grant: %v", err)
	}
	role, err := policy.CreateRole(admin, rbac.RoleInput{Name: "Account user", Rules: []rbac.RoleRule{{Permission: agentauth.PermissionAccountUse, Effect: rbac.Allow}}})
	if err != nil {
		t.Fatal(err)
	}
	scope := rbac.ProviderAccountScope("codex", "team")
	if _, err := policy.BindRole(admin, rbac.BindingInput{RoleID: role.ID, UserEmail: "member@example.com", Scope: scope}); err != nil {
		t.Fatal(err)
	}
	if id, err := access.Resolve(member, agent.ProviderCodex, "team"); err != nil || id != "team" {
		t.Fatalf("role grant: %q %v", id, err)
	}
	for _, id := range []string{"", "private", "forged"} {
		if _, err := access.Resolve(member, agent.ProviderCodex, id); !errors.Is(err, rbac.ErrDenied) {
			t.Fatalf("%q: %v", id, err)
		}
	}
	snapshot := binding.Snapshot()
	snapshot.Warning = "private diagnostic"
	snapshot.Login.URL = "private login URL"
	visible, err := access.Visible(member, agent.ProviderCodex, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(visible)
	if strings.Contains(string(encoded), "private") || len(visible.Accounts.Items) != 1 || visible.Accounts.Items[0].ID != "team" || *visible.Accounts.DefaultAllowed {
		t.Fatalf("visible metadata: %s", encoded)
	}
	// Exact scope: a Codex grant does not authorize the same ID on Claude.
	if err := agentauth.RequireAccountUse(member, policy, agent.ProviderClaude, "team"); !errors.Is(err, rbac.ErrDenied) {
		t.Fatalf("provider scope isolation: %v", err)
	}
	reopened, err := filepermissions.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := rbac.NewService(context.Background(), registry, reopened, identities{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := agentauth.RequireAccountUse(member, reloaded, agent.ProviderCodex, "team"); err != nil {
		t.Fatalf("persisted binding: %v", err)
	}
	if _, err := policy.SetAssignment(admin, rbac.AssignmentInput{UserEmail: "member@example.com", Permission: agentauth.PermissionAccountUse, Effect: rbac.Deny, Scope: scope}); err != nil {
		t.Fatal(err)
	}
	if _, err := access.Resolve(member, agent.ProviderCodex, "team"); !errors.Is(err, rbac.ErrDenied) {
		t.Fatalf("deny must override role: %v", err)
	}
	visible, err = access.Visible(member, agent.ProviderCodex, snapshot)
	if err != nil || len(visible.Accounts.Items) != 0 {
		t.Fatalf("revoked metadata: %+v %v", visible, err)
	}
	if id, err := access.Resolve(admin, agent.ProviderCodex, "private"); err != nil || id != "private" {
		t.Fatalf("admin recovery: %q %v", id, err)
	}
	if _, err := access.Resolve(admin, agent.ProviderCodex, "forged"); !errors.Is(err, agentauth.ErrAccountNotFound) {
		t.Fatalf("unknown account: %v", err)
	}
	source.snapshot = agentauth.AccountsSnapshot{Items: []agentauth.Account{}}
	defaultScope := rbac.ProviderAccountScope("codex", "")
	if _, err := policy.SetAssignment(admin, rbac.AssignmentInput{UserEmail: "member@example.com", Permission: agentauth.PermissionAccountUse, Effect: rbac.Allow, Scope: defaultScope}); err != nil {
		t.Fatal(err)
	}
	if id, err := access.Resolve(member, agent.ProviderCodex, ""); err != nil || id != "" {
		t.Fatalf("default host grant: %q %v", id, err)
	}
}
func TestAccountAccessFailsClosedWithoutPolicy(t *testing.T) {
	access := New(nil, []agentauth.Binding{agentauth.NewExternalBinding(agent.ProviderClaude)})
	if _, err := access.Resolve(actor("member@example.com"), agent.ProviderClaude, ""); !errors.Is(err, rbac.ErrDenied) {
		t.Fatalf("missing policy: %v", err)
	}
	for _, id := range []string{"claude:", "claude:../bad", "claude:a:b", "CLAUDE:one"} {
		if err := (rbac.Scope{Kind: rbac.ScopeProviderAccount, ID: id}).Validate(); err == nil {
			t.Fatalf("accepted malformed scope %q", id)
		}
	}
}
