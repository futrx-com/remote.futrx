package filenotify

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	servicenotify "github.com/futrx-com/remote.futrx.com/internal/service/notify"
)

func TestLoadReturnsDefaultsBeforeAnythingIsSaved(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := servicenotify.DefaultConfig()
	if cfg.Enabled != want.Enabled || cfg.Events != want.Events {
		t.Fatalf("Load = %+v, want defaults %+v", cfg, want)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}

	saved := servicenotify.Config{
		Enabled:   true,
		Telegram:  servicenotify.TelegramConfig{BotToken: "  123:secret  ", ChatID: " -100200 "},
		Webhook:   servicenotify.WebhookConfig{URL: "https://hooks.example.com/remote", Secret: "shhh"},
		Events:    servicenotify.EventToggles{RunFinished: true, NeedsAttention: true},
		UpdatedAt: 1700000000000,
	}
	if err := store.Save(context.Background(), saved); err != nil {
		t.Fatalf("Save: %v", err)
	}

	reopened, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := reopened.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got.Telegram.BotToken != "123:secret" || got.Telegram.ChatID != "-100200" {
		t.Fatalf("telegram round trip = %+v, want trimmed values", got.Telegram)
	}
	if got.Webhook != (servicenotify.WebhookConfig{URL: "https://hooks.example.com/remote", Secret: "shhh"}) {
		t.Fatalf("webhook round trip = %+v", got.Webhook)
	}
	if !got.Enabled || got.UpdatedAt != 1700000000000 {
		t.Fatalf("round trip lost scalar fields: %+v", got)
	}
	if got.Events != (servicenotify.EventToggles{RunFinished: true, NeedsAttention: true}) {
		t.Fatalf("event toggles = %+v", got.Events)
	}
}

func TestSaveWritesAPrivateFile(t *testing.T) {
	dir := t.TempDir()
	store, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(context.Background(), servicenotify.DefaultConfig()); err != nil {
		t.Fatalf("Save: %v", err)
	}

	path := filepath.Join(dir, fileName)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// Windows does not implement POSIX permission bits; the deployment target
	// is Linux, where the bot token must not be world readable.
	if runtime.GOOS != "windows" {
		if mode := info.Mode().Perm(); mode != 0o600 {
			t.Fatalf("%s mode = %o, want 600", fileName, mode)
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".tmp") {
			t.Fatalf("temp file %q was left behind", entry.Name())
		}
	}
}

func TestLoadRejectsCorruptDocuments(t *testing.T) {
	dir := t.TempDir()
	store, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, fileName), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Load(context.Background()); err == nil {
		t.Fatal("expected an error for a corrupt document")
	}
}
