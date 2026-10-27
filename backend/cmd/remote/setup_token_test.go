package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileauth"
)

const testSetupTokenTTL = 30 * time.Minute

func TestSetupTokenCommandPrintsAQueryURLAndStoresOnlyTheHash(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer

	if err := runSetupToken(context.Background(), dir, "https://remote.example.com/", testSetupTokenTTL, &out); err != nil {
		t.Fatalf("runSetupToken: %v", err)
	}

	printed := out.String()
	if !strings.Contains(printed, "https://remote.example.com/?token=") {
		t.Fatalf("printed output did not carry a query token URL:\n%s", printed)
	}

	record, err := fileauth.New(dir).SetupToken(context.Background())
	if err != nil || record == nil {
		t.Fatalf("SetupToken record = %#v, %v", record, err)
	}
	token := strings.TrimSpace(strings.SplitN(strings.SplitN(printed, "?token=", 2)[1], "\n", 2)[0])
	if token == "" {
		t.Fatal("could not read the printed token back")
	}
	if record.Hash == token {
		t.Fatal("the plaintext token was persisted; only its hash may be stored")
	}
	if !strings.Contains(printed, token) {
		t.Fatal("printed token did not round-trip")
	}
}

// Reissuing against a configured server would print a setup URL that cannot
// work, since the claim is refused as already-claimed regardless.
func TestSetupTokenCommandRefusesOnceClaimed(t *testing.T) {
	dir := t.TempDir()
	credential, err := json.Marshal(serviceauth.LocalAdminCredential{
		Email: "admin@example.com", PasswordHash: "$argon2id$hash",
	})
	if err != nil {
		t.Fatalf("marshal credential: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "local-admin.json"), credential, 0o600); err != nil {
		t.Fatalf("seed local-admin.json: %v", err)
	}

	var out bytes.Buffer
	if err := runSetupToken(context.Background(), dir, "https://remote.example.com", testSetupTokenTTL, &out); err == nil {
		t.Fatal("runSetupToken succeeded against an already-configured server")
	}
	if out.Len() != 0 {
		t.Fatalf("refused command still printed: %s", out.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "setup-token.json")); !os.IsNotExist(err) {
		t.Fatalf("refused command still wrote a token record (stat err = %v)", err)
	}
}

// A server whose directory already holds an administrator is not token-gated:
// that administrator authorises the local password themselves. Reissuing there
// prints a link whose token the claim never checks - the same dead end the
// startup path was fixed to stop printing.
func TestSetupTokenCommandRefusesWhenAnAdministratorExists(t *testing.T) {
	dir := t.TempDir()
	directory := `{"users":[{"email":"googleadmin@example.com","role":"admin","addedAt":1700000000000}]}`
	if err := os.WriteFile(filepath.Join(dir, "users.json"), []byte(directory), 0o600); err != nil {
		t.Fatalf("seed users.json: %v", err)
	}

	var out bytes.Buffer
	if err := runSetupToken(context.Background(), dir, "https://remote.example.com", testSetupTokenTTL, &out); err == nil {
		t.Fatal("runSetupToken issued a token for a claim an administrator authorises")
	}
	if out.Len() != 0 {
		t.Fatalf("refused command still printed: %s", out.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "setup-token.json")); !os.IsNotExist(err) {
		t.Fatalf("refused command still wrote a token record (stat err = %v)", err)
	}
}

// The command must not mint a session key. It never signs anything, and an
// operator running this under sudo before the service has started would leave
// session.key owned by root - which the service then cannot read, so it
// refuses to start at all.
func TestSetupTokenCommandDoesNotCreateASessionKey(t *testing.T) {
	dir := t.TempDir()

	var out bytes.Buffer
	if err := runSetupToken(context.Background(), dir, "https://remote.example.com", testSetupTokenTTL, &out); err != nil {
		t.Fatalf("runSetupToken: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "session.key")); !os.IsNotExist(err) {
		t.Fatalf("the reissue command created session.key (stat err = %v)", err)
	}
}
