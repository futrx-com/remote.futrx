package filechat

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestChatStoreSchemaInitializesOnce(t *testing.T) {
	root := t.TempDir()
	store, err := openChatStoreDB(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.close() }()

	var version int
	if err := store.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != chatStoreSchemaVersion {
		t.Fatalf("user_version = %d, want %d", version, chatStoreSchemaVersion)
	}

	for _, table := range []string{"chat_events", "chat_event_state"} {
		var count int
		err := store.db.QueryRow(
			"SELECT COUNT(*) FROM sqlite_schema WHERE type = 'table' AND name = ?", table,
		).Scan(&count)
		if err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("table %s missing", table)
		}
	}
	var ftsCount int
	if err := store.db.QueryRow(
		"SELECT COUNT(*) FROM sqlite_schema WHERE name = 'chat_events_fts'",
	).Scan(&ftsCount); err != nil {
		t.Fatal(err)
	}
	if ftsCount != 1 {
		t.Fatal("fts5 virtual table missing")
	}

	reopened, err := openChatStoreDB(root)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	_ = reopened.close()
}

func TestChatStoreSearchTextStaysInSyncWithEvents(t *testing.T) {
	root := t.TempDir()
	store, err := openChatStoreDB(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.close() }()

	insert := `INSERT INTO chat_events
		(chat_id, seq, t, type, search_text, payload)
		VALUES ('abcd', 1, 10, 'assistant_text', 'hello world', '{}')`
	if _, err := store.db.Exec(insert); err != nil {
		t.Fatal(err)
	}

	var hits int
	if err := store.db.QueryRow(
		`SELECT COUNT(*) FROM chat_events_fts WHERE chat_events_fts MATCH ?`,
		"hello",
	).Scan(&hits); err != nil {
		t.Fatal(err)
	}
	if hits != 1 {
		t.Fatalf("fts hits = %d, want 1", hits)
	}

	if _, err := store.db.Exec(`DELETE FROM chat_events WHERE chat_id = 'abcd'`); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(
		`SELECT COUNT(*) FROM chat_events_fts WHERE chat_events_fts MATCH ?`,
		"hello",
	).Scan(&hits); err != nil {
		t.Fatal(err)
	}
	if hits != 0 {
		t.Fatalf("fts hits after delete = %d, want 0", hits)
	}
}

func TestChatStoreRefusesNewerSchemaVersion(t *testing.T) {
	root := t.TempDir()
	store, err := openChatStoreDB(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec("PRAGMA user_version = 99"); err != nil {
		t.Fatal(err)
	}
	if err := store.close(); err != nil {
		t.Fatal(err)
	}

	if _, err := openChatStoreDB(root); err == nil {
		t.Fatal("expected newer schema version to be refused")
	}
}

func TestChatStoreQuarantinesCorruptDatabase(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, chatStoreFilename)
	if err := os.WriteFile(path, []byte("this is not a database"), 0o600); err != nil {
		t.Fatal(err)
	}

	store, err := openChatStoreDB(root)
	if err != nil {
		t.Fatalf("corrupt database should be quarantined, got: %v", err)
	}
	defer func() { _ = store.close() }()

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	quarantined := false
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".corrupt-") {
			quarantined = true
		}
	}
	if !quarantined {
		t.Fatalf("corrupt database was not quarantined: %v", entries)
	}
}

func TestChatStoreRefusesSymlinkedDatabase(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "elsewhere")
	if err := os.WriteFile(target, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, chatStoreFilename)); err != nil {
		t.Fatal(err)
	}

	if _, err := openChatStoreDB(root); err == nil {
		t.Fatal("expected symlinked database to be refused")
	}
}

func TestChatStoreKeepsPrivatePermissions(t *testing.T) {
	root := t.TempDir()
	store, err := openChatStoreDB(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.close(); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(filepath.Join(root, chatStoreFilename))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != sqliteFileMode {
		t.Fatalf("database mode = %o, want %o", got, sqliteFileMode)
	}
}

func TestChatStoreMigrationsAreIdempotent(t *testing.T) {
	root := t.TempDir()
	store, err := openChatStoreDB(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec("PRAGMA user_version = 0"); err != nil {
		t.Fatal(err)
	}
	if err := store.initializeSchema(); err != nil {
		t.Fatal(err)
	}
	var version int
	if err := store.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != chatStoreSchemaVersion {
		t.Fatalf("user_version = %d, want %d", version, chatStoreSchemaVersion)
	}
	_ = store.close()
}
