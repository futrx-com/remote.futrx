package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

type apiKeyTestStore struct {
	keys map[agent.ProviderID]string
	err  error
}

type rejectingStoredKeyValidator struct{}

func (rejectingStoredKeyValidator) ValidateAPIKey(context.Context, string) error {
	return nil
}

func (rejectingStoredKeyValidator) ValidateAPIKeyFormat(key string) error {
	if key == "legacy-key" {
		return ErrAPIKeyRejected
	}
	return nil
}

func (s *apiKeyTestStore) AgentAPIKey(_ context.Context, id agent.ProviderID) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	return s.keys[id], nil
}

func (s *apiKeyTestStore) SaveAgentAPIKey(_ context.Context, id agent.ProviderID, key string) error {
	if s.err != nil {
		return s.err
	}
	s.keys[id] = key
	return nil
}

func (s *apiKeyTestStore) DeleteAgentAPIKey(_ context.Context, id agent.ProviderID) error {
	if s.err != nil {
		return s.err
	}
	delete(s.keys, id)
	return nil
}

func TestAPIKeyServiceLoadsMutatesAndPublishesOnlyStatus(t *testing.T) {
	store := &apiKeyTestStore{keys: map[agent.ProviderID]string{agent.ProviderMiniMax: "stored-key"}}
	service, err := NewAPIKeyService(context.Background(), agent.ProviderMiniMax, store, nil)
	if err != nil {
		t.Fatal(err)
	}
	if key, ok := service.APIKey(); !ok || key != "stored-key" || !service.Status().Authenticated {
		t.Fatalf("initial service = (%q, %t, %#v)", key, ok, service.Status())
	}

	updates, unsubscribe := service.Subscribe()
	defer unsubscribe()
	if status := <-updates; !status.Authenticated {
		t.Fatalf("initial status = %#v", status)
	}
	if err := service.Set(context.Background(), " replacement-key "); err != nil {
		t.Fatal(err)
	}
	if status := <-updates; !status.Authenticated {
		t.Fatalf("replacement status = %#v", status)
	}
	if key, _ := service.APIKey(); key != "replacement-key" {
		t.Fatalf("key = %q", key)
	}
	if err := service.Delete(context.Background()); err != nil {
		t.Fatal(err)
	}
	if status := <-updates; status.Authenticated {
		t.Fatalf("delete status = %#v", status)
	}
	if _, ok := service.APIKey(); ok {
		t.Fatal("deleted key remains available")
	}
}

func TestAPIKeyServiceDoesNotActivateAStoredUnsupportedCredentialClass(t *testing.T) {
	store := &apiKeyTestStore{keys: map[agent.ProviderID]string{
		agent.ProviderMiniMax: "legacy-key",
	}}
	service, err := NewAPIKeyService(
		context.Background(),
		agent.ProviderMiniMax,
		store,
		rejectingStoredKeyValidator{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if service.Authenticated() {
		t.Fatal("unsupported stored credential was activated")
	}
	if _, ok := service.APIKey(); ok {
		t.Fatal("unsupported stored credential remains available to runs")
	}
	if got := store.keys[agent.ProviderMiniMax]; got != "legacy-key" {
		t.Fatalf("stored credential was unexpectedly mutated: %q", got)
	}
}

func TestAPIKeyServiceMigratesAndSelectsNamedAccounts(t *testing.T) {
	ctx := context.Background()
	legacy := &apiKeyTestStore{keys: map[agent.ProviderID]string{agent.ProviderMiniMax: "legacy-key"}}
	accounts := &fakeAccountStore{sets: map[agent.ProviderID]AccountSet{agent.ProviderMiniMax: {}}}
	service, err := NewAPIKeyService(ctx, agent.ProviderMiniMax, legacy, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.EnableAccounts(ctx, NewAccountVault(accounts), "MiniMax"); err != nil {
		t.Fatal(err)
	}
	snapshot := service.AccountsSnapshot()
	if len(snapshot.Items) != 1 || snapshot.Items[0].Label != "Default" || !snapshot.Items[0].Active {
		t.Fatalf("migrated accounts = %#v", snapshot)
	}
	defaultID := snapshot.ActiveAccountID
	if key, ok := service.APIKeyFor(defaultID); !ok || key != "legacy-key" {
		t.Fatalf("migrated key = (%q, %t)", key, ok)
	}
	if _, remains := legacy.keys[agent.ProviderMiniMax]; remains {
		t.Fatal("legacy key remains after successful migration")
	}
	binding := NewAPIKeyBinding(agent.ProviderMiniMax, service)
	if !binding.AccountsAvailable() || binding.Snapshot().Accounts == nil {
		t.Fatalf("named API-key accounts were not attached to the auth binding: %#v", binding.Snapshot())
	}

	if err := service.SetAccount(ctx, "Work", "", "work-key"); err != nil {
		t.Fatal(err)
	}
	snapshot = service.AccountsSnapshot()
	if len(snapshot.Items) != 2 || snapshot.ActiveAccountID == defaultID {
		t.Fatalf("named accounts after add = %#v", snapshot)
	}
	workID := snapshot.ActiveAccountID
	if key, ok := service.APIKeyFor(workID); !ok || key != "work-key" {
		t.Fatalf("work key = (%q, %t)", key, ok)
	}
	if err := service.ActivateAccount(ctx, defaultID); err != nil {
		t.Fatal(err)
	}
	if key, ok := service.APIKey(); !ok || key != "legacy-key" {
		t.Fatalf("active key after switch = (%q, %t)", key, ok)
	}
	if err := service.Set(ctx, "rotated-key"); err != nil {
		t.Fatal(err)
	}
	if key, ok := service.APIKey(); !ok || key != "rotated-key" {
		t.Fatalf("active key after compatible replace = (%q, %t)", key, ok)
	}
	if got := service.AccountsSnapshot().Items[0].Label; got != "Default" {
		t.Fatalf("compatible replace renamed active account to %q", got)
	}
	if err := service.DeleteAccount(ctx, workID); err != nil {
		t.Fatal(err)
	}
	if len(service.AccountsSnapshot().Items) != 1 {
		t.Fatalf("accounts after delete = %#v", service.AccountsSnapshot())
	}
}

func TestAPIKeyServiceRejectsBlankAndUnavailableStorage(t *testing.T) {
	service, err := NewAPIKeyService(context.Background(), agent.ProviderMiniMax, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Set(context.Background(), "  "); !errors.Is(err, ErrAPIKeyRequired) {
		t.Fatalf("blank key error = %v", err)
	}
	if err := service.Set(context.Background(), "key"); !errors.Is(err, ErrAPIKeyStoreUnavailable) {
		t.Fatalf("unavailable store error = %v", err)
	}
	if err := service.Delete(context.Background()); !errors.Is(err, ErrAPIKeyStoreUnavailable) {
		t.Fatalf("unavailable delete error = %v", err)
	}
}

func TestAPIKeyServiceValidatesBeforeReplacingStoredKey(t *testing.T) {
	store := &apiKeyTestStore{keys: map[agent.ProviderID]string{
		agent.ProviderMiniMax: "working-key",
	}}
	validationErr := ErrAPIKeyRejected
	validatedKey := ""
	service, err := NewAPIKeyService(
		context.Background(),
		agent.ProviderMiniMax,
		store,
		APIKeyValidatorFunc(func(_ context.Context, key string) error {
			validatedKey = key
			return validationErr
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	updates, unsubscribe := service.Subscribe()
	defer unsubscribe()
	<-updates
	if err := service.Set(context.Background(), " rejected-key "); !errors.Is(err, ErrAPIKeyRejected) {
		t.Fatalf("Set error = %v, want ErrAPIKeyRejected", err)
	}
	if validatedKey != "rejected-key" {
		t.Fatalf("validated key = %q", validatedKey)
	}
	if key, _ := service.APIKey(); key != "working-key" {
		t.Fatalf("active key changed after rejection: %q", key)
	}
	if key := store.keys[agent.ProviderMiniMax]; key != "working-key" {
		t.Fatalf("stored key changed after rejection: %q", key)
	}
	select {
	case status := <-updates:
		t.Fatalf("rejected key published status %#v", status)
	default:
	}

	validationErr = nil
	if err := service.Set(context.Background(), "replacement-key"); err != nil {
		t.Fatal(err)
	}
	if key, _ := service.APIKey(); key != "replacement-key" {
		t.Fatalf("active key = %q", key)
	}
	if status := <-updates; !status.Authenticated {
		t.Fatalf("replacement status = %#v", status)
	}
}

func TestNewAPIKeyServiceReturnsStorageErrors(t *testing.T) {
	want := errors.New("read failed")
	_, err := NewAPIKeyService(context.Background(), agent.ProviderMiniMax, &apiKeyTestStore{
		keys: map[agent.ProviderID]string{},
		err:  want,
	}, nil)
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}
