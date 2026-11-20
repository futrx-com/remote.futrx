package auth

import (
	"context"
	"errors"
	"github.com/futrx-com/remote.futrx.com/internal/rbac"
	"sync"
	"testing"
)

// Existing credential lifecycle tests explicitly allow access; policy tests
// inject real RBAC or a denying authorizer instead.
type allowAccountUse struct{}

func (allowAccountUse) Require(context.Context, rbac.Check) error { return nil }

type denyAccountUse struct{ checks []rbac.Check }

func (a *denyAccountUse) Require(_ context.Context, c rbac.Check) error {
	a.checks = append(a.checks, c)
	return rbac.ErrDenied
}
func TestCredentialAcquisitionChecksResolvedDefaultBeforeReturningSecrets(t *testing.T) {
	h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"))
	deny := &denyAccountUse{}
	h.service.authorizer = deny
	writes := h.credentials.writeCount()
	for _, id := range []string{"", "work", "home"} {
		credential, ok, err := h.service.CredentialForRun(context.Background(), id)
		if !errors.Is(err, rbac.ErrDenied) || ok || len(credential.Credential) != 0 {
			t.Fatalf("denied credential %q: ok=%v err=%v", id, ok, err)
		}
		release, err := h.service.BeginRunFor(context.Background(), id)
		if !errors.Is(err, rbac.ErrDenied) || release != nil {
			t.Fatalf("denied lease %q: %v", id, err)
		}
	}
	if h.credentials.writeCount() != writes || h.service.activeRuns != 0 || h.store.saveCount() != 0 {
		t.Fatal("denied account use changed credentials or leases")
	}
	if deny.checks[0].Scope != rbac.ProviderAccountScope(string(testAccountProvider), "work") {
		t.Fatalf("default was not resolved before authorization: %v", deny.checks)
	}
	h.service.authorizer = nil
	if _, _, err := h.service.CredentialForRun(context.Background(), "work"); !errors.Is(err, rbac.ErrDenied) {
		t.Fatalf("missing authorizer: %v", err)
	}
}

type scopedAccountUse struct{}

func (scopedAccountUse) Require(ctx context.Context, c rbac.Check) error {
	actor, ok := rbac.ActorFromContext(ctx)
	if !ok {
		return rbac.ErrActorRequired
	}
	if c.Permission == PermissionAccountUse && ((actor.Email == "work@example.com" && c.Scope.ID == "codex:work") || (actor.Email == "home@example.com" && c.Scope.ID == "codex:home")) {
		return nil
	}
	return rbac.ErrDenied
}
func TestAuthorizedAccountsKeepIndependentConcurrentCredentials(t *testing.T) {
	h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"))
	h.service.authorizer = scopedAccountUse{}
	var wg sync.WaitGroup
	failures := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := "work"
			if i%2 != 0 {
				id = "home"
			}
			ctx := rbac.ContextWithActor(context.Background(), rbac.UserActor(id+"@example.com"))
			credential, ok, err := h.service.CredentialForRun(ctx, id)
			if err != nil {
				failures <- err
				return
			}
			if !ok || credential.AccountID != id {
				failures <- errors.New("wrong account snapshot")
				return
			}
			release := h.service.BeginIsolatedRun(id)
			defer release()
		}(i)
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}
	h.requireActive(t, "work")
	h.requireNoSaves(t)
}
func TestDefaultSelectionRechecksTheActualAccount(t *testing.T) {
	h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"))
	h.service.authorizer = scopedAccountUse{}
	ctx := rbac.ContextWithActor(context.Background(), rbac.UserActor("work@example.com"))
	if selected, ok, err := h.service.CredentialForRun(ctx, ""); err != nil || !ok || selected.AccountID != "work" {
		t.Fatalf("initial default: ok=%v err=%v", ok, err)
	}
	// Simulate an administrator changing the default after an earlier selection
	// preflight; acquiring with an empty ID must authorize the new actual ID.
	h.service.mu.Lock()
	h.service.accounts.ActiveAccountID = "home"
	h.service.mu.Unlock()
	if _, _, err := h.service.CredentialForRun(ctx, ""); !errors.Is(err, rbac.ErrDenied) {
		t.Fatalf("changed default: %v", err)
	}
	if release, err := h.service.BeginRunFor(ctx, ""); !errors.Is(err, rbac.ErrDenied) || release != nil {
		t.Fatalf("changed default lease: %v", err)
	}
}
func TestAccountManagementDenialPrecedesCredentialChanges(t *testing.T) {
	h := newAccountHarness(t, twoAccounts(), testCredential("w", "t1"))
	h.service.authorizer = &denyAccountUse{}
	for _, operation := range []func() error{
		func() error { return h.service.ImportCurrent(context.Background(), "label") },
		func() error { _, err := h.service.StartAccountLogin(context.Background(), "label", ""); return err },
		func() error { return h.service.ActivateAccount(context.Background(), "home") },
		func() error { return h.service.DeleteAccount(context.Background(), "home") },
	} {
		if err := operation(); !errors.Is(err, rbac.ErrDenied) {
			t.Fatalf("management denial: %v", err)
		}
	}
	h.requireActive(t, "work")
	h.requireNoSaves(t)
}
