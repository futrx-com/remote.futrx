package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	"github.com/futrx-com/remote.futrx.com/internal/rbac"
)

type apiKeyAccountAuthorizer func(context.Context, rbac.Check) error

func (f apiKeyAccountAuthorizer) Require(ctx context.Context, check rbac.Check) error {
	return f(ctx, check)
}

func newAuthorizedAPIKeyService(t *testing.T, authorizer AccountAuthorizer, named bool) *APIKeyService {
	t.Helper()
	store := &apiKeyTestStore{keys: map[agent.ProviderID]string{agent.ProviderMiniMax: "legacy-key"}}
	service, err := NewAPIKeyService(context.Background(), agent.ProviderMiniMax, store, nil, authorizer)
	if err != nil {
		t.Fatal(err)
	}
	if named {
		accounts := &fakeAccountStore{sets: map[agent.ProviderID]AccountSet{
			agent.ProviderMiniMax: {
				ActiveAccountID: "work",
				Accounts: []AccountRecord{
					{ID: "work", Label: "Work", Credential: []byte(`"work-key"`)},
					{ID: "home", Label: "Home", Credential: []byte(`"home-key"`)},
				},
			},
		}}
		if err := service.EnableAccounts(context.Background(), NewAccountVault(accounts, authorizer), "MiniMax"); err != nil {
			t.Fatal(err)
		}
	}
	return service
}

func TestAPIKeyRunDenialDoesNotReturnCredentials(t *testing.T) {
	for _, named := range []bool{false, true} {
		deny := &denyAccountUse{}
		service := newAuthorizedAPIKeyService(t, deny, named)
		for _, id := range []string{"", "work", "home"} {
			credential, ok, err := service.APIKeyForRun(context.Background(), id)
			if !errors.Is(err, rbac.ErrDenied) || ok || credential.Key != "" {
				t.Fatalf("named=%v account=%q: denied acquisition returned credential or wrong error: %v", named, id, err)
			}
		}
		resolved := ""
		if named {
			resolved = "work"
		}
		if got := deny.checks[0].Scope; got != rbac.ProviderAccountScope("minimax", resolved) {
			t.Fatalf("default authorization scope = %#v", got)
		}
		service.authorizer = nil
		if credential, ok, err := service.APIKeyForRun(context.Background(), ""); !errors.Is(err, rbac.ErrDenied) || ok || credential.Key != "" {
			t.Fatalf("missing authorizer returned a credential or wrong error: %v", err)
		}
	}
}

func TestAPIKeyRunRechecksChangedDefault(t *testing.T) {
	authorizer := apiKeyAccountAuthorizer(func(_ context.Context, check rbac.Check) error {
		if check.Permission == PermissionAccountsManage || check.Scope == rbac.ProviderAccountScope("minimax", "work") {
			return nil
		}
		return rbac.ErrDenied
	})
	service := newAuthorizedAPIKeyService(t, authorizer, true)
	credential, ok, err := service.APIKeyForRun(context.Background(), "")
	if err != nil || !ok || credential.AccountID != "work" || credential.Key != "work-key" {
		t.Fatalf("authorized default acquisition failed: ok=%v err=%v", ok, err)
	}
	if err := service.ActivateAccount(context.Background(), "home"); err != nil {
		t.Fatal(err)
	}
	if credential, ok, err := service.APIKeyForRun(context.Background(), ""); !errors.Is(err, rbac.ErrDenied) || ok || credential.Key != "" {
		t.Fatalf("changed default bypassed authorization: %v", err)
	}
	if credential, ok, err := service.APIKeyForRun(context.Background(), "work"); err != nil || !ok || credential.AccountID != "work" || credential.Key != "work-key" {
		t.Fatalf("pinned account changed with default: ok=%v err=%v", ok, err)
	}
}

func TestAPIKeyRunAccessKeepsAcquiredDefaultScopeAfterAccountMigration(t *testing.T) {
	allowLegacy := true
	authorizer := apiKeyAccountAuthorizer(func(_ context.Context, check rbac.Check) error {
		if check.Scope == rbac.ProviderAccountScope("minimax", "") && !allowLegacy {
			return rbac.ErrDenied
		}
		return nil
	})
	service := newAuthorizedAPIKeyService(t, authorizer, false)
	credential, ok, err := service.APIKeyForRun(context.Background(), "")
	if err != nil || !ok || credential.AccountID != "" {
		t.Fatalf("legacy acquisition failed: ok=%v err=%v", ok, err)
	}
	// A named default is configured during run setup. Its grant cannot
	// authorize the previously acquired key's now-denied default scope.
	accounts := &fakeAccountStore{sets: map[agent.ProviderID]AccountSet{
		agent.ProviderMiniMax: {
			ActiveAccountID: "work",
			Accounts:        []AccountRecord{{ID: "work", Label: "Work", Credential: []byte(`"work-key"`)}},
		},
	}}
	if err := service.EnableAccounts(context.Background(), NewAccountVault(accounts, authorizer), "MiniMax"); err != nil {
		t.Fatal(err)
	}
	allowLegacy = false
	if _, ok, err := service.APIKeyForRun(context.Background(), ""); err != nil || !ok {
		t.Fatalf("new saved default should be permitted: ok=%v err=%v", ok, err)
	}
	if err := service.RequireRunAccess(context.Background(), credential.AccountID); !errors.Is(err, rbac.ErrDenied) {
		t.Fatalf("old credential was authorized using new default scope: %v", err)
	}
}

func TestAPIKeyRunAccessRejectsDeletedCredential(t *testing.T) {
	for _, named := range []bool{false, true} {
		service := newAuthorizedAPIKeyService(t, allowAccountUse{}, named)
		credential, ok, err := service.APIKeyForRun(context.Background(), "")
		if err != nil || !ok {
			t.Fatalf("acquisition failed: ok=%v err=%v", ok, err)
		}
		if err := service.Delete(context.Background()); err != nil {
			t.Fatal(err)
		}
		if err := service.RequireRunAccess(context.Background(), credential.AccountID); !errors.Is(err, ErrAccountNotFound) {
			t.Fatalf("named=%v deleted credential remained usable: %v", named, err)
		}
	}
}

func TestAPIKeyManagementDenialPrecedesValidationAndMutation(t *testing.T) {
	for _, named := range []bool{false, true} {
		deny := &denyAccountUse{}
		service := newAuthorizedAPIKeyService(t, deny, named)
		validated := false
		service.validator = APIKeyValidatorFunc(func(context.Context, string) error {
			validated = true
			return nil
		})
		operations := []func() error{
			func() error { return service.Set(context.Background(), "replacement-key") },
			func() error { return service.Delete(context.Background()) },
		}
		if named {
			operations = append(operations,
				func() error { return service.SetAccount(context.Background(), "New", "", "new-key") },
				func() error { return service.ActivateAccount(context.Background(), "home") },
				func() error { return service.DeleteAccount(context.Background(), "home") },
			)
		}
		for _, operation := range operations {
			if err := operation(); !errors.Is(err, rbac.ErrDenied) {
				t.Fatalf("named=%v management denial = %v", named, err)
			}
		}
		if validated {
			t.Fatal("denied mutation called the provider validator")
		}
		for _, check := range deny.checks {
			if check.Permission != PermissionAccountsManage || check.Scope != rbac.PlatformScope() {
				t.Fatalf("management checked wrong permission: %#v", check)
			}
		}
		wantKey := "legacy-key"
		if named {
			wantKey = "work-key"
			if snapshot := service.AccountsSnapshot(); snapshot.ActiveAccountID != "work" || len(snapshot.Items) != 2 {
				t.Fatalf("denied operation changed accounts: %#v", snapshot)
			}
		}
		if key, ok := service.APIKey(); !ok || key != wantKey {
			t.Fatal("denied operation changed the stored key")
		}
	}
}
