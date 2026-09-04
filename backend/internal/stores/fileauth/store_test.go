package fileauth

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
)

func TestLocalAdminCredentialIsPrivateAndCreateOnly(t *testing.T) {
	dir := t.TempDir()
	store := New(dir)
	credential := serviceauth.LocalAdminCredential{
		Email: "admin@example.com", PasswordHash: "$argon2id$hash",
	}
	if err := store.CreateLocalAdmin(context.Background(), credential); err != nil {
		t.Fatalf("CreateLocalAdmin: %v", err)
	}
	info, err := os.Stat(filepath.Join(dir, "local-admin.json"))
	if err != nil {
		t.Fatalf("stat local-admin.json: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("local-admin.json mode = %o, want 600", got)
	}
	if err := store.CreateLocalAdmin(context.Background(), credential); !errors.Is(err, serviceauth.ErrLocalAdminAlreadyClaimed) {
		t.Fatalf("second CreateLocalAdmin error = %v", err)
	}
}

func TestDeleteLocalAdminRequiresExpectedCredential(t *testing.T) {
	dir := t.TempDir()
	store := New(dir)
	credential := serviceauth.LocalAdminCredential{
		Email: "admin@example.com", PasswordHash: "$argon2id$hash",
	}
	if err := store.CreateLocalAdmin(context.Background(), credential); err != nil {
		t.Fatalf("CreateLocalAdmin: %v", err)
	}

	changed := credential
	changed.PasswordHash = "$argon2id$different"
	if err := store.DeleteLocalAdmin(context.Background(), changed); !errors.Is(err, serviceauth.ErrLocalAdminCredentialChanged) {
		t.Fatalf("DeleteLocalAdmin changed credential error = %v", err)
	}
	if persisted, err := store.LocalAdmin(context.Background()); err != nil || persisted == nil {
		t.Fatalf("credential after refused delete = %#v, %v", persisted, err)
	}

	if err := store.DeleteLocalAdmin(context.Background(), credential); err != nil {
		t.Fatalf("DeleteLocalAdmin: %v", err)
	}
	if persisted, err := store.LocalAdmin(context.Background()); err != nil || persisted != nil {
		t.Fatalf("credential after delete = %#v, %v", persisted, err)
	}
	if err := store.DeleteLocalAdmin(context.Background(), credential); err != nil {
		t.Fatalf("idempotent DeleteLocalAdmin: %v", err)
	}
}

func TestOAuthSecretIsPrivate(t *testing.T) {
	dir := t.TempDir()
	store := New(dir)
	if err := store.SaveOAuthConfig(context.Background(), serviceauth.OAuthConfig{
		GoogleClientID: "id", GoogleClientSecret: "secret",
	}); err != nil {
		t.Fatalf("SaveOAuthConfig: %v", err)
	}
	info, err := os.Stat(filepath.Join(dir, "oauth.json"))
	if err != nil {
		t.Fatalf("stat oauth.json: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("oauth.json mode = %o, want 600", got)
	}
}

// The setup token is a bearer credential for the very first claim, so its
// record must be no more readable than the credential it protects.
func TestSetupTokenRecordIsPrivateAndRotates(t *testing.T) {
	dir := t.TempDir()
	store := New(dir)
	ctx := context.Background()

	if record, err := store.SetupToken(ctx); err != nil || record != nil {
		t.Fatalf("SetupToken before any issue = %#v, %v; want nil, nil", record, err)
	}

	first := serviceauth.SetupTokenRecord{
		Hash: "hash-one", ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := store.SaveSetupToken(ctx, first); err != nil {
		t.Fatalf("SaveSetupToken: %v", err)
	}
	info, err := os.Stat(filepath.Join(dir, "setup-token.json"))
	if err != nil {
		t.Fatalf("stat setup-token.json: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("setup-token.json mode = %o, want 600", got)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "setup-token.json"))
	if err != nil {
		t.Fatalf("read setup-token.json: %v", err)
	}
	for _, field := range []string{`"hash"`, `"expiresAt"`, `"used"`} {
		if !strings.Contains(string(raw), field) {
			t.Fatalf("setup-token.json = %s; missing field %s", raw, field)
		}
	}

	// Saving again replaces rather than appends: a reissue must invalidate
	// whatever was printed before, never leave two live tokens.
	second := serviceauth.SetupTokenRecord{
		Hash: "hash-two", ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := store.SaveSetupToken(ctx, second); err != nil {
		t.Fatalf("SaveSetupToken rotate: %v", err)
	}
	persisted, err := store.SetupToken(ctx)
	if err != nil || persisted == nil || persisted.Hash != "hash-two" {
		t.Fatalf("rotated record = %#v, %v; want hash-two", persisted, err)
	}
}

func TestAgentAPIKeysArePrivateAndReplaceable(t *testing.T) {
	dir := t.TempDir()
	store := New(dir)
	ctx := context.Background()

	if key, err := store.AgentAPIKey(ctx, "minimax"); err != nil || key != "" {
		t.Fatalf("initial AgentAPIKey = %q, %v", key, err)
	}
	if err := store.SaveAgentAPIKey(ctx, "minimax", "first-key"); err != nil {
		t.Fatalf("SaveAgentAPIKey: %v", err)
	}
	if err := store.SaveAgentAPIKey(ctx, "minimax", "replacement-key"); err != nil {
		t.Fatalf("replace AgentAPIKey: %v", err)
	}
	if key, err := store.AgentAPIKey(ctx, "minimax"); err != nil || key != "replacement-key" {
		t.Fatalf("stored AgentAPIKey = %q, %v", key, err)
	}

	info, err := os.Stat(filepath.Join(dir, agentAPIKeysFile))
	if err != nil {
		t.Fatalf("stat %s: %v", agentAPIKeysFile, err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("%s mode = %o, want 600", agentAPIKeysFile, got)
	}

	if err := store.DeleteAgentAPIKey(ctx, "minimax"); err != nil {
		t.Fatalf("DeleteAgentAPIKey: %v", err)
	}
	if key, err := store.AgentAPIKey(ctx, "minimax"); err != nil || key != "" {
		t.Fatalf("AgentAPIKey after delete = %q, %v", key, err)
	}
}
