package sqlitechat

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	chat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

// A frozen v1 schema tests the actual deployed format, independently of v2 DDL.
//
//go:embed testdata/schema_v1.sql
var deployedSchemaV1 string

func schemaV1Fixture(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(root, Filename))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	if _, err = db.Exec(deployedSchemaV1); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`
INSERT INTO chats VALUES
 ('aaaaaaaaaaaa','project','root','Original',66,cast('{ "id":"aaaaaaaaaaaa", "projectId":"project", "provider":"claude", "unknown": [1,2] }' AS BLOB)),
 ('bbbbbbbbbbbb','project','fork','Fork',55,cast('{"id":"bbbbbbbbbbbb","projectId":"project","provider":"codex"}' AS BLOB)),
 ('cccccccccccc','project','empty','Empty fork',33,cast('{"id":"cccccccccccc","projectId":"project","provider":"claude"}' AS BLOB));
INSERT INTO branches VALUES ('root','aaaaaaaaaaaa',NULL,0,6,2),('fork','bbbbbbbbbbbb','root',3,5,2),('empty','cccccccccccc','root',3,3,1);
INSERT INTO turns VALUES ('root',1,'first',1,1,4),('root',2,'second',1,5,6),('fork',2,'fork-turn',1,4,5);
INSERT INTO conversation_migration VALUES (1,'complete','original-source',1,2);
INSERT INTO conversation_imports VALUES ('aaaaaaaaaaaa','root','source-fingerprint',812,6,6,x'012345','complete');
INSERT INTO harness_sessions VALUES
 ('ready','root','claude','main','claude-native',6,'ready',1,2),
 ('old','root','claude','main','old-uncertain',3,'uncertain',1,999),
 ('other-account','root','claude','backup','claude-backup-native',4,'legacy_assumed',1,2),
 ('unfinished-ready','root','codex','main','codex-native',4,'ready',1,2),
 ('pending','fork','codex','main','pending-native',3,'pending',1,2),
 ('retired','fork','claude','main','retired-native',3,'retired',1,2);
INSERT INTO runs(id,branch_id,turn_id,harness_session_id,provider,account_id,model,status,history_through_seq,created_at)
 VALUES ('open-run','root','second','unfinished-ready','codex','main','model','accepted',4,1);
`); err != nil {
		t.Fatal(err)
	}
	events := []struct {
		branch string
		seq    int64
		raw    string
	}{
		{"root", 1, `{ "seq":1,"t":11,"type":"user","turnId":"first","text":"remember kiwi" }`},
		{"root", 2, `{"seq":2,"t":22,"type":"tool_use_start","turnId":"first","id":"tool-1","name":"lookup","input":{"q":"kiwi"},"raw":{"opaque":"native"}}`},
		{"root", 3, `{"seq":3,"t":33,"type":"tool_use_end","turnId":"first","id":"tool-1","output":"fruit"}`},
		{"root", 4, `{"seq":4,"t":44,"type":"assistant_text","turnId":"first","text":"kiwi is fruit"}`},
		{"root", 5, `{"seq":5,"t":55,"type":"user","turnId":"second","text":"discarded in fork"}`},
		{"root", 6, `{"seq":6,"t":66,"type":"complete","turnId":"second"}`},
		{"fork", 4, `{"seq":4,"t":44,"type":"user","turnId":"fork-turn","text":"fork only"}`},
		{"fork", 5, `{"seq":5,"t":55,"type":"assistant_text","turnId":"fork-turn","text":"isolated"}`},
	}
	for _, entry := range events {
		var event chat.Event
		if err = json.Unmarshal([]byte(entry.raw), &event); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(`INSERT INTO events VALUES(?,?,?,?,?,?,?)`, entry.branch, entry.seq, event.T, event.Type, event.TurnID, []byte(entry.raw), []byte(entry.raw+"\n")); err != nil {
			t.Fatal(err)
		}
	}
	digest, err := canonicalSchemaDigest(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	return root, digest
}

func TestUpgradeSchemaPreservesCanonicalDataAndCurrentSessions(t *testing.T) {
	root, before := schemaV1Fixture(t)
	ctx := context.Background()
	if store, err := New(root); err == nil {
		store.Close()
		t.Fatal("runtime silently upgraded schema v1")
	}
	result, err := UpgradeSchema(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if result.FromVersion != 1 || result.ToVersion != 2 || result.BackupPath == "" {
		t.Fatalf("upgrade result: %+v", result)
	}
	stat, err := os.Stat(result.BackupPath)
	if err != nil || stat.Mode().Perm() != 0600 {
		t.Fatalf("backup permissions: %v %v", stat, err)
	}
	backup, err := openSchemaDatabase(result.BackupPath, true)
	if err != nil {
		t.Fatal(err)
	}
	defer backup.Close()
	backupDigest, err := canonicalSchemaDigest(ctx, backup)
	if err != nil || backupDigest != before {
		t.Fatalf("backup canonical digest %q; expected %q: %v", backupDigest, before, err)
	}
	var version int
	if err = backup.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 1 {
		t.Fatalf("backup version %d: %v", version, err)
	}
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	after, err := canonicalSchemaDigest(ctx, store.DB())
	if err != nil || after != before {
		t.Fatalf("canonical digest changed: %q -> %q (%v)", before, after, err)
	}
	for _, table := range []string{"turns", "harness_sessions", "runs", "handoffs", "content_blobs", "transcript_items", "transcript_content_refs", "transcript_text_runs", "transcript_text_search", "transcript_text_pending"} {
		var count int
		if err = store.DB().QueryRow("SELECT count(*) FROM sqlite_schema WHERE name=?", table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("obsolete table %s remains: %v", table, err)
		}
	}
	var raw []byte
	if err = store.DB().QueryRow("SELECT sessions_json FROM chats WHERE id='aaaaaaaaaaaa'").Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var sessions map[string]sessionState
	if err = json.Unmarshal(raw, &sessions); err != nil {
		t.Fatal(err)
	}
	for _, want := range []struct {
		provider               chat.Provider
		account, native, state string
		through                int64
	}{
		{"claude", "main", "claude-native", "ready", 6},
		{"claude", "backup", "claude-backup-native", "legacy_assumed", 4},
		{"codex", "main", "codex-native", "uncertain", 4},
	} {
		got := sessions[sessionKey(want.provider, want.account)]
		if got.NativeSessionID != want.native || got.State != want.state || got.HistoryThroughSeq != want.through || got.RunID != "" {
			t.Fatalf("session mismatch: %+v, want %+v", got, want)
		}
	}
	if err = store.DB().QueryRow("SELECT sessions_json FROM chats WHERE id='bbbbbbbbbbbb'").Scan(&raw); err != nil {
		t.Fatal(err)
	}
	sessions = nil
	if err = json.Unmarshal(raw, &sessions); err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || sessions[sessionKey("codex", "main")].State != "uncertain" {
		t.Fatalf("pending/retired states: %s", raw)
	}
	var ordinal, start, itemStart int64
	var key, item string
	if err = store.DB().QueryRow(`SELECT turn_ordinal,turn_start_seq,turn_key,item_key,item_start_seq FROM events WHERE branch_id='root' AND seq=3`).Scan(&ordinal, &start, &key, &item, &itemStart); err != nil {
		t.Fatal(err)
	}
	if ordinal != 1 || start != 1 || key != "first" || item != "tool:tool-1" || itemStart != 2 {
		t.Fatalf("derived event fields: %d %d %s %s %d", ordinal, start, key, item, itemStart)
	}
	for _, id := range []chat.ID{"aaaaaaaaaaaa", "bbbbbbbbbbbb", "cccccccccccc"} {
		events, err := store.ReadEvents(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		want := map[chat.ID]int{"aaaaaaaaaaaa": 6, "bbbbbbbbbbbb": 5, "cccccccccccc": 3}[id]
		if len(events) != want {
			t.Fatalf("chat %s has %d events; want %d", id, len(events), want)
		}
		if id == "bbbbbbbbbbbb" && events[3].Text != "fork only" {
			t.Fatalf("fork history leaked: %+v", events)
		}
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	again, err := UpgradeSchema(ctx, root)
	if err != nil || again.FromVersion != 2 || again.ToVersion != 2 || again.BackupPath != "" {
		t.Fatalf("repeat upgrade: %+v %v", again, err)
	}
}

func TestUpgradeSchemaFailureRollsBackAndRetainsVerifiedBackup(t *testing.T) {
	root, _ := schemaV1Fixture(t)
	db, err := openSchemaDatabase(filepath.Join(root, Filename), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE events SET payload_json=x'7b' WHERE branch_id='root' AND seq=5`); err != nil {
		t.Fatal(err)
	}
	before, err := canonicalSchemaDigest(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	result, err := UpgradeSchema(context.Background(), root)
	if err == nil || !strings.Contains(err.Error(), "decode event") {
		t.Fatalf("expected migration decode error: %+v %v", result, err)
	}
	if result.BackupPath == "" {
		t.Fatal("no recovery backup")
	}
	db, err = openSchemaDatabase(filepath.Join(root, Filename), true)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var version int
	if err = db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 1 {
		t.Fatalf("failed migration changed version: %d %v", version, err)
	}
	after, err := canonicalSchemaDigest(context.Background(), db)
	if err != nil || before != after {
		t.Fatalf("failed migration changed canonical data: %s %s %v", before, after, err)
	}
	var columns int
	if err = db.QueryRow("SELECT count(*) FROM pragma_table_info('events') WHERE name='turn_ordinal'").Scan(&columns); err != nil || columns != 0 {
		t.Fatalf("partial migration committed: %v", err)
	}
}

func TestUpgradeSchemaRequiresStoppedStoreAndRejectsFutureVersion(t *testing.T) {
	root, _ := schemaV1Fixture(t)
	lock, err := acquireStoreLock(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = UpgradeSchema(context.Background(), root); err == nil {
		t.Fatal("upgraded while backend owns lock")
	}
	lock.Close()
	db, err := openSchemaDatabase(filepath.Join(root, Filename), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("PRAGMA user_version=99"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if _, err = UpgradeSchema(context.Background(), root); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("future schema accepted: %v", err)
	}
}

func TestUpgradeSchemaUncommittedConversionCanRetry(t *testing.T) {
	root, before := schemaV1Fixture(t)
	ctx := context.Background()
	db, err := openSchemaDatabase(filepath.Join(root, Filename), false)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate interruption after every conversion statement, including the
	// destructive cleanup and version bump, but before the durable commit.
	if err = upgradeSchemaV1(ctx, tx); err != nil {
		tx.Rollback()
		db.Close()
		t.Fatal(err)
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	after, err := canonicalSchemaDigest(ctx, db)
	if err != nil || after != before {
		t.Fatalf("uncommitted conversion changed history: %v", err)
	}
	var version int
	if err = db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 1 {
		t.Fatalf("uncommitted conversion changed schema version: %d %v", version, err)
	}
	db.Close()
	result, err := UpgradeSchema(ctx, root)
	if err != nil || result.FromVersion != 1 || result.ToVersion != 2 {
		t.Fatalf("retry failed: %+v %v", result, err)
	}
}

func TestUpgradeSchemaPreservesToolGroupingAcrossByteBoundedPages(t *testing.T) {
	root, _ := schemaV1Fixture(t)
	ctx := context.Background()
	db, err := openSchemaDatabase(filepath.Join(root, Filename), false)
	if err != nil {
		t.Fatal(err)
	}
	// The user and tool start together fill one byte-bounded page. The tool
	// result must retain that start pointer when processed on the next page.
	// A later oversized event must still make progress without being split.
	for _, row := range []struct{ seq, size int }{
		{1, schemaBackfillBatchBytes / 2},
		{2, schemaBackfillBatchBytes / 2},
		{4, schemaBackfillBatchBytes + 1024},
	} {
		if _, err = db.Exec(`UPDATE events SET payload_json=CAST(json_set(payload_json,'$.raw',json_object('large',?)) AS BLOB) WHERE branch_id='root' AND seq=?`, strings.Repeat("x", row.size), row.seq); err != nil {
			db.Close()
			t.Fatal(err)
		}
	}
	before, err := canonicalSchemaDigest(ctx, db)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = UpgradeSchema(ctx, root); err != nil {
		t.Fatal(err)
	}
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	after, err := canonicalSchemaDigest(ctx, store.db)
	if err != nil || before != after {
		t.Fatalf("byte-bounded migration changed canonical content: %v", err)
	}
	var itemStart, turnStart, ordinal int64
	if err = store.db.QueryRow(`SELECT item_start_seq,turn_start_seq,turn_ordinal FROM events WHERE branch_id='root' AND seq=3`).Scan(&itemStart, &turnStart, &ordinal); err != nil {
		t.Fatal(err)
	}
	if itemStart != 2 || turnStart != 1 || ordinal != 1 {
		t.Fatalf("tool lost grouping across page boundary: item=%d turn=%d ordinal=%d", itemStart, turnStart, ordinal)
	}
	var indexed int
	if err = store.db.QueryRow(`SELECT count(*) FROM events WHERE turn_ordinal>0 AND item_start_seq>0`).Scan(&indexed); err != nil || indexed != 8 {
		t.Fatalf("events after oversized record were skipped: count=%d error=%v", indexed, err)
	}
}

func TestSearchByteBoundedBatchRetainsLaterEvents(t *testing.T) {
	store := testStore(t)
	makeChat(t, store, "abcd")
	// Search text previews are bounded already; unusually large native message
	// identifiers can still fill the metadata budget before the row-count limit.
	messageID := strings.Repeat("m", searchBatchBytes/2)
	appendEvents(t, store, "abcd",
		chat.Event{Type: "assistant_text", TurnID: "one", MessageID: messageID, Text: "first "},
		chat.Event{Type: "assistant_text", TurnID: "one", MessageID: messageID, Text: "second "},
		chat.Event{Type: "assistant_text", TurnID: "one", Text: "afterlimit"},
	)
	branch, tail, err := activeBranch(testContext, store.db, "abcd")
	if err != nil {
		t.Fatal(err)
	}
	done, err := store.searchBatch(testContext, branchSegment{id: branch, through: tail})
	if err != nil || done {
		t.Fatalf("byte-limited batch marked the tail indexed: done=%v error=%v", done, err)
	}
	var cursor int64
	if err = store.db.QueryRow("SELECT search_seq FROM branches WHERE id=?", branch).Scan(&cursor); err != nil || cursor != 2 {
		t.Fatalf("byte-limited cursor=%d; expected 2: %v", cursor, err)
	}
	hits, err := store.SearchTranscript(testContext, "abcd", "afterlimit", 10)
	if err != nil || len(hits) != 1 {
		t.Fatalf("events after byte-limited page were skipped: %+v %v", hits, err)
	}
}
