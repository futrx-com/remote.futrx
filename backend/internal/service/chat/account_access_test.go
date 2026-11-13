package chat

import (
	"context"
	"errors"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	"github.com/futrx-com/remote.futrx.com/internal/rbac"
	agentaccountaccess "github.com/futrx-com/remote.futrx.com/internal/service/agent/accountaccess"
	agentauth "github.com/futrx-com/remote.futrx.com/internal/service/agent/auth"
	"github.com/futrx-com/remote.futrx.com/internal/stores/filepermissions"
)

type deniedAccountSelection struct {
	provider agent.ProviderID
	id       string
}

type chatPolicyIdentities struct{}

func (chatPolicyIdentities) IsAdmin(_ context.Context, email string) (bool, error) {
	return email == "admin@example.com", nil
}
func (chatPolicyIdentities) IsRegistered(_ context.Context, email string) (bool, error) {
	return email == "admin@example.com" || email == "member@example.com", nil
}

type chatSavedAccounts struct{ agentauth.AccountController }

func (chatSavedAccounts) AccountsSnapshot() agentauth.AccountsSnapshot {
	return agentauth.AccountsSnapshot{ActiveAccountID: "team", Items: []agentauth.Account{{ID: "team", Label: "Team", Active: true}}}
}

type accountSelectionRepository struct {
	*forkRepository
	writes int
}

func (r *accountSelectionRepository) Create(ctx context.Context, meta Meta) (Meta, error) {
	r.writes++
	return r.forkRepository.Create(ctx, meta)
}
func (r *accountSelectionRepository) Update(ctx context.Context, id ID, mutate func(*Meta)) (Meta, error) {
	r.writes++
	return r.forkRepository.Update(ctx, id, mutate)
}

func TestChatProviderAccountPolicyEnforcesSelectionGrantsAndRevocation(t *testing.T) {
	for _, tc := range []struct {
		provider  agent.ProviderID
		accountID string
	}{
		{agent.ProviderMiniMax, "team"},
		{agent.ProviderKimi, ""},
		{agent.ProviderAntigravity, ""},
	} {
		for _, operation := range []string{"create", "fork", "account", "provider"} {
			t.Run(string(tc.provider)+"/"+operation, func(t *testing.T) {
				store, err := filepermissions.New(t.TempDir())
				if err != nil {
					t.Fatal(err)
				}
				policy, err := rbac.NewService(context.Background(), rbac.MustRegistry(rbac.ManagementDefinitions(), agentauth.PermissionDefinitions(), PermissionDefinitions()), store, chatPolicyIdentities{}, nil)
				if err != nil {
					t.Fatal(err)
				}
				binding := agentauth.NewExternalBinding(tc.provider)
				if tc.accountID != "" {
					binding = binding.WithAccounts(chatSavedAccounts{})
				}
				access := agentaccountaccess.New(policy, []agentauth.Binding{binding})
				repo := &accountSelectionRepository{forkRepository: &forkRepository{source: Meta{ID: "deadbeef", Provider: tc.provider, AccountID: tc.accountID}}}
				if operation == "account" || operation == "provider" {
					repo.source.AccountID = "old"
				}
				if operation == "provider" {
					repo.source.Provider = agent.ProviderCodex
				}
				service := New(repo, nil, nil, nil, WithAuthorizer(policy), WithAccountAccess(access))
				member := rbac.ContextWithActor(context.Background(), rbac.UserActor("member@example.com"))
				selectAccount := func() (Meta, error) {
					switch operation {
					case "create":
						return service.Create(member, CreateInput{Provider: tc.provider, AccountID: tc.accountID})
					case "fork":
						return service.Fork(member, "deadbeef")
					case "account":
						return service.Update(member, "deadbeef", UpdateInput{AccountID: &tc.accountID})
					default:
						return service.Update(member, "deadbeef", UpdateInput{Provider: &tc.provider})
					}
				}
				if _, err := selectAccount(); !errors.Is(err, rbac.ErrDenied) {
					t.Fatalf("ungranted selection: %v", err)
				}
				if repo.writes != 0 {
					t.Fatal("denied selection wrote chat state")
				}
				admin := rbac.ContextWithActor(context.Background(), rbac.UserActor("admin@example.com"))
				assignment := rbac.AssignmentInput{UserEmail: "member@example.com", Permission: agentauth.PermissionAccountUse, Scope: rbac.ProviderAccountScope(string(tc.provider), tc.accountID), Effect: rbac.Allow}
				if _, err := policy.SetAssignment(admin, assignment); err != nil {
					t.Fatal(err)
				}
				selected, err := selectAccount()
				if err != nil {
					t.Fatal(err)
				}
				wantID := tc.accountID
				if operation == "provider" {
					wantID = ""
				}
				if repo.writes != 1 || selected.Provider != tc.provider || selected.AccountID != wantID {
					t.Fatalf("authorized selection: %+v writes=%d", selected, repo.writes)
				}
				assignment.Effect = rbac.Deny
				if _, err := policy.SetAssignment(admin, assignment); err != nil {
					t.Fatal(err)
				}
				if _, err := selectAccount(); !errors.Is(err, rbac.ErrDenied) {
					t.Fatalf("revoked selection: %v", err)
				}
				if repo.writes != 1 {
					t.Fatal("revoked selection wrote chat state")
				}
			})
		}
	}
}

func (d *deniedAccountSelection) Resolve(_ context.Context, p agent.ProviderID, id string) (string, error) {
	d.provider = p
	d.id = id
	return "", rbac.ErrDenied
}
func TestChatAccountSelectionDeniesCreateForkAndChangesBeforeWrites(t *testing.T) {
	for _, operation := range []string{"create", "fork", "account", "provider"} {
		repo := &forkRepository{source: Meta{ID: "deadbeef", Provider: ProviderCodex, AccountID: "old"}}
		policy := &deniedAccountSelection{}
		service := New(repo, nil, nil, nil, WithAccountAccess(policy))
		ctx := rbac.ContextWithSystemActor(context.Background())
		var err error
		switch operation {
		case "create":
			_, err = service.Create(ctx, CreateInput{Provider: ProviderCodex, AccountID: "forged"})
		case "fork":
			_, err = service.Fork(ctx, "deadbeef")
		case "account":
			id := "forged"
			_, err = service.Update(ctx, "deadbeef", UpdateInput{AccountID: &id})
		case "provider":
			provider := ProviderClaude
			_, err = service.Update(ctx, "deadbeef", UpdateInput{Provider: &provider})
		}
		if !errors.Is(err, rbac.ErrDenied) {
			t.Fatalf("%s: %v", operation, err)
		}
		if repo.created.ID != "" || repo.source.AccountID != "old" {
			t.Fatalf("%s modified chat", operation)
		}
		if operation == "provider" && (policy.provider != agent.ProviderClaude || policy.id != "") {
			t.Fatalf("provider change must check its default: %+v", policy)
		}
	}
}
