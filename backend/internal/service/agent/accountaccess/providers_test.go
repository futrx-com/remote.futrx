package accountaccess

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	"github.com/futrx-com/remote.futrx.com/internal/rbac"
	agentauth "github.com/futrx-com/remote.futrx.com/internal/service/agent/auth"
	"github.com/futrx-com/remote.futrx.com/internal/stores/filepermissions"
)

func newProviderPolicy(t *testing.T) *rbac.Service {
	t.Helper()
	repo, err := filepermissions.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	policy, err := rbac.NewService(context.Background(), rbac.MustRegistry(rbac.ManagementDefinitions(), agentauth.PermissionDefinitions()), repo, identities{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return policy
}

func TestEveryRegisteredProviderRequiresItsOwnDefaultGrant(t *testing.T) {
	// Include a catalog extension so a new provider cannot silently bypass RBAC.
	for _, provider := range []agent.ProviderID{agent.ProviderClaude, agent.ProviderCodex, agent.ProviderMiniMax, agent.ProviderKimi, agent.ProviderAntigravity, "future-agent"} {
		t.Run(string(provider), func(t *testing.T) {
			policy := newProviderPolicy(t)
			binding := agentauth.NewExternalBinding(provider)
			access := New(policy, []agentauth.Binding{binding})
			member, admin := actor("member@example.com"), actor("admin@example.com")
			for _, id := range []string{"", "invented"} {
				if _, err := access.Resolve(member, provider, id); !errors.Is(err, rbac.ErrDenied) {
					t.Fatalf("ungranted account %q: %v", id, err)
				}
			}
			if _, err := New(nil, []agentauth.Binding{binding}).Resolve(member, provider, ""); !errors.Is(err, rbac.ErrDenied) {
				t.Fatalf("missing policy: %v", err)
			}
			if _, err := access.Resolve(admin, "unregistered", ""); !errors.Is(err, rbac.ErrDenied) {
				t.Fatalf("unregistered provider: %v", err)
			}
			assignment := rbac.AssignmentInput{UserEmail: "member@example.com", Permission: agentauth.PermissionAccountUse, Effect: rbac.Allow, Scope: rbac.ProviderAccountScope(string(provider), "")}
			if _, err := policy.SetAssignment(admin, assignment); err != nil {
				t.Fatal(err)
			}
			if id, err := access.Resolve(member, provider, ""); err != nil || id != "" {
				t.Fatalf("granted default: %q %v", id, err)
			}
			if _, err := access.Resolve(member, provider, "invented"); !errors.Is(err, rbac.ErrDenied) {
				t.Fatalf("default grant authorized named account: %v", err)
			}
			snapshot := agentauth.Snapshot{Authenticated: true, Warning: "private warning", Login: agentauth.LoginSnapshot{URL: "private login"}}
			visible, err := access.Visible(member, provider, snapshot)
			if err != nil || !visible.Authenticated || visible.Accounts == nil || !visible.Accounts.DefaultOnly || !*visible.Accounts.DefaultAllowed || visible.Warning != "" || visible.Login.URL != "" {
				t.Fatalf("allowed snapshot: %+v %v", visible, err)
			}
			assignment.Effect = rbac.Deny
			if _, err := policy.SetAssignment(admin, assignment); err != nil {
				t.Fatal(err)
			}
			if _, err := access.Resolve(member, provider, ""); !errors.Is(err, rbac.ErrDenied) {
				t.Fatalf("revoked default: %v", err)
			}
			visible, err = access.Visible(member, provider, snapshot)
			if err != nil || visible.Authenticated || visible.Accounts == nil || *visible.Accounts.DefaultAllowed {
				t.Fatalf("revoked snapshot: %+v %v", visible, err)
			}
			if allowed, err := access.CanViewAccount(member, provider, ""); err != nil || allowed {
				t.Fatalf("revoked quota visibility: %v %v", allowed, err)
			}
			if _, err := access.Resolve(admin, provider, ""); err != nil {
				t.Fatalf("administrator recovery: %v", err)
			}
		})
	}
}

func TestTargetsFollowRegisteredProvidersAndIncludeSavedAccounts(t *testing.T) {
	providers := []agent.ProviderID{agent.ProviderClaude, agent.ProviderCodex, agent.ProviderMiniMax, agent.ProviderKimi, agent.ProviderAntigravity, "future-agent"}
	bindings := make([]agentauth.Binding, 0, len(providers))
	want := make([]string, 0, len(providers)+1)
	for _, provider := range providers {
		binding := agentauth.NewExternalBinding(provider)
		want = append(want, string(provider)+":default")
		if provider == agent.ProviderMiniMax {
			binding = binding.WithAccounts(&accountSource{snapshot: agentauth.AccountsSnapshot{Items: []agentauth.Account{{ID: "team", Label: "Team"}}}})
			want = append(want, "minimax:team")
		}
		bindings = append(bindings, binding)
	}
	access := New(newProviderPolicy(t), bindings)
	if _, err := access.Targets(actor("member@example.com")); !errors.Is(err, rbac.ErrDenied) {
		t.Fatalf("member targets: %v", err)
	}
	targets, err := access.Targets(actor("admin@example.com"))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, target := range targets {
		got = append(got, target.Scope.ID)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("targets = %v, want %v", got, want)
	}
}
