package chatmigration

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	"github.com/futrx-com/remote.futrx.com/internal/stores/sqlitechat"
)

func legacyChat(t *testing.T, root, id, meta, events string) {
	t.Helper()
	dir := filepath.Join(root, "chats", id)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "meta.json"), []byte(meta), 0600); err != nil {
		t.Fatal(err)
	}
	if events != "<missing>" {
		if err := os.WriteFile(filepath.Join(dir, "events.jsonl"), []byte(events), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestMigrationResumesWithoutDuplicatesAndPreservesRawHistory(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	meta := `{"id":"abcd","title":"Migration","provider":"claude","accountId":"account","sessions":{"claude":"claude-native","codex":"codex-native"},"createdAt":1,"lastMessageAt":4,"projectId":"project","unknown":{"keep":true}}`
	legacyChat(t, root, "abcd", meta, "{\"type\":\"user\",\"t\":1,\"text\":\"hello\",\"unknown\":\"preserved\"}\n{\"seq\":7,\"type\":\"assistant_text\",\"t\":2,\"text\":\"hi\"}\n{\"type\":\"complete\",\"t\":3}\n")
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if err := os.WriteFile(filepath.Join(root, "transcript-index.sqlite")+suffix, []byte("obsolete"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	interrupted := errors.New("interrupted")
	_, err := Run(ctx, root, Options{BatchEvents: 1, Progress: func(p sqlitechat.ImportProgress) error {
		if p.EventCount == 1 {
			return interrupted
		}
		return nil
	}})
	if !errors.Is(err, interrupted) {
		t.Fatalf("got %v", err)
	}
	if status, err := Status(ctx, root); err != nil || status != "importing" {
		t.Fatalf("status %q: %v", status, err)
	}
	store, err := sqlitechat.New(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureRuntimeReady(ctx, root); err == nil {
		t.Fatal("incomplete migration allowed runtime")
	}
	var count int
	if err := store.DB().QueryRow(`SELECT count(*) FROM events`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	store.Close()
	result, err := Run(ctx, root, Options{BatchEvents: 1})
	if err != nil {
		t.Fatal(err)
	}
	if result.Chats != 1 || result.Events != 3 || result.AlreadyComplete {
		t.Fatalf("result %+v", result)
	}
	store, err = sqlitechat.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.EnsureRuntimeReady(ctx, root); err != nil {
		t.Fatal(err)
	}
	events, err := store.ReadEvents(ctx, "abcd")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 || events[0].Seq != 1 || events[1].Seq != 7 || events[2].Seq != 8 {
		t.Fatalf("events %+v", events)
	}
	var storedMeta, raw, state, native string
	var through int64
	if err := store.DB().QueryRow(`SELECT meta_json FROM chats WHERE id='abcd'`).Scan(&storedMeta); err != nil {
		t.Fatal(err)
	}
	if storedMeta != meta {
		t.Fatal("metadata changed")
	}
	if err := store.DB().QueryRow(`SELECT raw_json FROM events WHERE seq=1`).Scan(&raw); err != nil || !strings.Contains(raw, "preserved") {
		t.Fatalf("raw=%s err=%v", raw, err)
	}
	if err := store.DB().QueryRow(`SELECT native_session_id,history_through_seq,state FROM harness_sessions`).Scan(&native, &through, &state); err != nil {
		t.Fatal(err)
	}
	if native != "claude-native" || through != 8 || state != "legacy_assumed" {
		t.Fatalf("native=%s through=%d state=%s", native, through, state)
	}
	if err := store.DB().QueryRow(`SELECT count(*) FROM harness_sessions`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("session count=%d err=%v", count, err)
	}
	store.Close()
	result, err = Run(ctx, root, Options{})
	if err != nil || !result.AlreadyComplete {
		t.Fatalf("rerun %+v %v", result, err)
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if _, err := os.Stat(filepath.Join(root, "transcript-index.sqlite") + suffix); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("obsolete projection remains")
		}
	}
	if _, err := os.Stat(filepath.Join(root, "chats", "abcd", "events.jsonl")); err != nil {
		t.Fatal("legacy backup removed")
	}
}

func TestMigrationRejectsInvalidSources(t *testing.T) {
	for _, tc := range []struct{ name, meta, events, want string }{
		{"identity", `{"id":"dcba"}`, "", "does not match"},
		{"missingIdentity", `{"title":"missing"}`, "", "does not match"},
		{"metadata", `{"id":`, "", "metadata"},
		{"tail", `{"id":"abcd"}`, "{\"type\":\"user\"}\n{\"type\":", "malformed event"},
		{"null", `{"id":"abcd"}`, "null\n", "no type"},
		{"sequence", `{"id":"abcd"}`, "{\"type\":\"user\",\"seq\":2}\n{\"type\":\"complete\",\"seq\":2}\n", "does not follow"},
		{"missingHistory", `{"id":"abcd","createdAt":1,"lastMessageAt":2}`, "<missing>", "missing events"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			legacyChat(t, root, "abcd", tc.meta, tc.events)
			_, err := Run(context.Background(), root, Options{BatchEvents: 1})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want %s", err, tc.want)
			}
			if status, _ := Status(context.Background(), root); status == "complete" {
				t.Fatal("invalid import marked complete")
			}
		})
	}
}

func TestMigrationAllowsEmptyChatAndFreshInstall(t *testing.T) {
	for _, withChat := range []bool{false, true} {
		root := t.TempDir()
		if withChat {
			legacyChat(t, root, "abcd", `{"id":"abcd","createdAt":1,"lastMessageAt":1}`, "<missing>")
		}
		if _, err := Run(context.Background(), root, Options{}); err != nil {
			t.Fatal(err)
		}
		if status, err := Status(context.Background(), root); err != nil || status != "complete" {
			t.Fatalf("%q %v", status, err)
		}
	}
}

func TestMigrationRejectsSourceChangesAfterCheckpoint(t *testing.T) {
	root := t.TempDir()
	legacyChat(t, root, "abcd", `{"id":"abcd"}`, "{\"type\":\"user\",\"text\":\"one\"}\n{\"type\":\"complete\"}\n")
	_, err := Run(context.Background(), root, Options{BatchEvents: 1, Progress: func(sqlitechat.ImportProgress) error { return errors.New("stop") }})
	if err == nil {
		t.Fatal("not interrupted")
	}
	path := filepath.Join(root, "chats", "abcd", "events.jsonl")
	stat, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data = []byte(strings.Replace(string(data), "one", "two", 1))
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, stat.ModTime(), stat.ModTime()); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(context.Background(), root, Options{}); err == nil || !strings.Contains(err.Error(), "source changed") {
		t.Fatalf("same-size changed source accepted: %v", err)
	}
}

func TestImporterRefusesLiveStoreAndRuntimeRefusesUnmigratedData(t *testing.T) {
	root := t.TempDir()
	legacyChat(t, root, "abcd", `{"id":"abcd"}`, "")
	store, err := sqlitechat.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.EnsureRuntimeReady(context.Background(), root); err == nil {
		t.Fatal("runtime ignored legacy source")
	}
	if _, err := Run(context.Background(), root, Options{}); err == nil || !strings.Contains(err.Error(), "already open") {
		t.Fatalf("import bypassed lock: %v", err)
	}
}

func TestMigrationDoesNotOverwriteLiveSQLiteOnSubsequentUpgrade(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	if _, err := Run(ctx, root, Options{}); err != nil {
		t.Fatal(err)
	}
	store, err := sqlitechat.New(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(ctx, servicechat.Meta{ID: "abcd", Title: "New SQLite chat"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendEvent(ctx, "abcd", servicechat.Event{Type: "user", Text: "new"}); err != nil {
		t.Fatal(err)
	}
	store.Close()
	result, err := Run(ctx, root, Options{})
	if err != nil || !result.AlreadyComplete {
		t.Fatalf("%+v %v", result, err)
	}
	store, err = sqlitechat.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	events, err := store.ReadEvents(ctx, "abcd")
	if err != nil || len(events) != 1 || events[0].Text != "new" {
		t.Fatalf("events %+v: %v", events, err)
	}
}

func TestMigrationValidatesImportedContentBeforeCutover(t *testing.T) {
	for _, statement := range []string{
		`UPDATE events SET raw_json='{"type":"user","text":"tampered"}' WHERE seq=1`,
		`UPDATE events SET payload_json='{"seq":1,"type":"user","text":"tampered"}' WHERE seq=1`,
		`UPDATE chats SET meta_json='{"id":"abcd","title":"tampered"}'`,
	} {
		t.Run(statement, func(t *testing.T) {
			root := t.TempDir()
			ctx := context.Background()
			legacyChat(t, root, "abcd", `{"id":"abcd"}`, "{\"type\":\"user\",\"text\":\"original\"}\n{\"type\":\"complete\"}\n")
			_, err := Run(ctx, root, Options{BatchEvents: 1, Progress: func(sqlitechat.ImportProgress) error { return errors.New("interrupt") }})
			if err == nil {
				t.Fatal("not interrupted")
			}
			store, err := sqlitechat.New(root)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := store.DB().Exec(statement); err != nil {
				t.Fatal(err)
			}
			store.Close()
			if _, err := Run(ctx, root, Options{}); err == nil || !strings.Contains(err.Error(), "validation failed") {
				t.Fatalf("tampered import accepted: %v", err)
			}
			if status, _ := Status(ctx, root); status == "complete" {
				t.Fatal("tampered import marked complete")
			}
		})
	}
}

func TestImportedConversationCanBeDeleted(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	legacyChat(t, root, "abcd", `{"id":"abcd"}`, "{\"type\":\"user\",\"text\":\"remove\"}\n")
	if _, err := Run(ctx, root, Options{}); err != nil {
		t.Fatal(err)
	}
	store, err := sqlitechat.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Delete(ctx, "abcd"); err != nil {
		t.Fatal(err)
	}
}

func TestQAVerifierChecksMigratedHistoryAndUnchangedBackup(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 unavailable")
	}
	root := t.TempDir()
	legacyChat(t, root, "abcd", `{"id":"abcd","provider":"claude","sessions":{"claude":"session"}}`, "{\"type\":\"user\",\"text\":\"original\"}\n{\"type\":\"complete\"}\n")
	if _, err := Run(context.Background(), root, Options{}); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join("..", "..", "..", "..", "infra", "qa", "verify-conversation-migration.py")
	output, err := exec.Command(python, script, "--data-dir", root).CombinedOutput()
	if err != nil {
		t.Fatalf("QA verification: %s: %v", output, err)
	}
	if !strings.Contains(string(output), `"events": 2`) || strings.Contains(string(output), "original") {
		t.Fatalf("unexpected verification output: %s", output)
	}
	path := filepath.Join(root, "chats", "abcd", "events.jsonl")
	if err := os.WriteFile(path, []byte("{}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command(python, script, "--data-dir", root).Run(); err == nil {
		t.Fatal("QA verifier accepted changed backup")
	}
}
