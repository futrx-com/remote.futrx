package fileemail

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	emailapplication "github.com/futrx-com/remote.futrx.com/internal/model/email/application"
)

func TestStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := New(dir)
	ctx := context.Background()

	cfg, err := store.Configuration(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg != nil {
		t.Fatalf("Configuration before any write = %+v, want nil", cfg)
	}

	want := emailapplication.SMTPConfiguration{
		Host:           "smtp.example.com",
		Port:           587,
		TLSMode:        emailapplication.TLSModeSTARTTLS,
		Authentication: emailapplication.AuthenticationPlain,
		Username:       "mailer@example.com",
		Password:       "s3cret-value",
		FromAddress:    "mailer@example.com",
	}
	if err := store.Save(ctx, want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := store.Configuration(ctx)
	if err != nil {
		t.Fatalf("Configuration after write: %v", err)
	}
	if got == nil || *got != want {
		t.Fatalf("Configuration = %+v, want %+v", got, want)
	}

	info, err := os.Stat(filepath.Join(dir, fileName))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("mode = %o, want 0600", perm)
	}
	raw, err := os.ReadFile(filepath.Join(dir, fileName))
	if err != nil {
		t.Fatalf("read raw record: %v", err)
	}
	if !strings.Contains(string(raw), `"version": 2`) {
		t.Errorf("record does not carry version 2: %s", raw)
	}

	if err := store.Delete(ctx); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := store.Delete(ctx); err != nil {
		t.Fatalf("Delete on already-missing file should be idempotent: %v", err)
	}

	cfg, err = store.Configuration(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg != nil {
		t.Fatalf("Configuration after delete = %+v, want nil", cfg)
	}
}

func TestStoreUnauthenticatedRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := New(dir)
	ctx := context.Background()

	want := emailapplication.SMTPConfiguration{
		Host:           "relay.internal",
		Port:           25,
		TLSMode:        emailapplication.TLSModeNone,
		Authentication: emailapplication.AuthenticationNone,
		FromAddress:    "noreply@example.com",
	}
	if err := store.Save(ctx, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := store.Configuration(ctx)
	if err != nil {
		t.Fatalf("Configuration: %v", err)
	}
	if got == nil || *got != want {
		t.Fatalf("Configuration = %+v, want %+v", got, want)
	}
}

func TestStoreRejectsUnsupportedVersion(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, fileName), []byte(`{"version":1,"address":"a@example.com","appPassword":"abcdefghijklmnop"}`), 0o600); err != nil {
		t.Fatalf("seed legacy record: %v", err)
	}
	store := New(dir)

	_, err := store.Configuration(context.Background())
	if err == nil {
		t.Fatal("expected an error for an unsupported record version")
	}
	if strings.Contains(err.Error(), "abcdefghijklmnop") {
		t.Errorf("error leaks a secret field: %v", err)
	}
}

func TestStoreCancelledContextPerformsNoWrite(t *testing.T) {
	dir := t.TempDir()
	store := New(dir)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := store.Save(ctx, emailapplication.SMTPConfiguration{Host: "smtp.example.com", Port: 587}); err == nil {
		t.Fatal("expected an error from a cancelled context")
	}
	if _, err := os.Stat(filepath.Join(dir, fileName)); !os.IsNotExist(err) {
		t.Fatalf("Save wrote a file despite a cancelled context: err = %v", err)
	}
	if err := store.Delete(ctx); err == nil {
		t.Fatal("expected an error from a cancelled context")
	}
}
