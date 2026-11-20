package sqlitechat

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	chat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

type SchemaUpgradeResult struct {
	FromVersion int    `json:"fromVersion"`
	ToVersion   int    `json:"toVersion"`
	BackupPath  string `json:"backupPath,omitempty"`
}

// UpgradeSchema runs only in the offline migration command. The exclusive
// store lease keeps the server stopped for the backup and conversion. DDL,
// backfill, validation, and the version change share one SQLite transaction:
// interruption leaves v1 intact and the next invocation safely retries.
func UpgradeSchema(ctx context.Context, root string) (result SchemaUpgradeResult, err error) {
	path := filepath.Join(root, Filename)
	if _, err = os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return result, nil
	} else if err != nil {
		return result, err
	}
	lock, err := acquireStoreLock(root)
	if err != nil {
		return result, err
	}
	defer func() { err = errors.Join(err, lock.Close()) }()
	db, err := openSchemaDatabase(path, false)
	if err != nil {
		return result, err
	}
	defer func() { err = errors.Join(err, db.Close()) }()
	if err = db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&result.FromVersion); err != nil {
		return result, err
	}
	result.ToVersion = result.FromVersion
	if result.FromVersion == 0 || result.FromVersion == schemaVersion {
		return result, nil
	}
	if result.FromVersion != 1 {
		return result, fmt.Errorf("unsupported conversation schema %d", result.FromVersion)
	}
	// SQLite performs the backup, including committed WAL contents. Never copy
	// just the main file of an open WAL database.
	backup, err := os.CreateTemp(root, "conversations.before-v2-"+time.Now().UTC().Format("20060102T150405")+"-*.sqlite")
	if err != nil {
		return result, err
	}
	result.BackupPath = backup.Name()
	if err = backup.Close(); err != nil {
		return result, err
	}
	if _, err = db.ExecContext(ctx, "VACUUM INTO ?", result.BackupPath); err != nil {
		return result, fmt.Errorf("back up conversations: %w", err)
	}
	backupDB, err := openSchemaDatabase(result.BackupPath, true)
	if err != nil {
		return result, err
	}
	err = verifySchemaDatabase(ctx, backupDB)
	err = errors.Join(err, backupDB.Close())
	if err != nil {
		return result, fmt.Errorf("verify conversation backup: %w", err)
	}
	// VACUUM INTO does not sync the destination on every SQLite version. Sync
	// the verified backup and its directory before changing the source database.
	backup, err = os.Open(result.BackupPath)
	if err != nil {
		return result, err
	}
	err = errors.Join(backup.Sync(), backup.Close())
	if err != nil {
		return result, err
	}
	dir, err := os.Open(root)
	if err != nil {
		return result, err
	}
	err = errors.Join(dir.Sync(), dir.Close())
	if err != nil {
		return result, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	if err = upgradeSchemaV1(ctx, tx); err != nil {
		return result, fmt.Errorf("upgrade conversation schema (v1 retained; backup %s): %w", result.BackupPath, err)
	}
	if err = tx.Commit(); err != nil {
		return result, err
	}
	result.ToVersion = schemaVersion
	return result, nil
}

// SchemaVersion is read-only and does not take the exclusive runtime lease.
// Deployment uses it to check rollback compatibility against the actual DB.
func SchemaVersion(ctx context.Context, root string) (int, error) {
	path := filepath.Join(root, Filename)
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return 0, nil
	} else if err != nil {
		return 0, err
	}
	db, err := openSchemaDatabase(path, true)
	if err != nil {
		return 0, err
	}
	defer db.Close()
	var version int
	err = db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version)
	return version, err
}

func openSchemaDatabase(path string, readOnly bool) (*sql.DB, error) {
	u := url.URL{Scheme: "file", Path: path}
	q := u.Query()
	q.Set("mode", "rw")
	if readOnly {
		q.Set("mode", "ro")
		q.Add("_pragma", "query_only(ON)")
	}
	for _, pragma := range []string{"busy_timeout(5000)", "foreign_keys(ON)", "synchronous(FULL)", "trusted_schema(OFF)"} {
		q.Add("_pragma", pragma)
	}
	q.Set("_txlock", "immediate")
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err == nil {
		db.SetMaxOpenConns(1)
	}
	return db, err
}

func upgradeSchemaV1(ctx context.Context, tx *sql.Tx) error {
	before, err := canonicalSchemaDigest(ctx, tx)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `
ALTER TABLE chats ADD COLUMN sessions_json BLOB NOT NULL DEFAULT '{}';
ALTER TABLE branches ADD COLUMN last_turn_id TEXT NOT NULL DEFAULT '';
ALTER TABLE branches ADD COLUMN last_turn_start_seq INTEGER NOT NULL DEFAULT 0;
ALTER TABLE branches ADD COLUMN last_turn_has_user INTEGER NOT NULL DEFAULT 0;
ALTER TABLE branches ADD COLUMN search_seq INTEGER NOT NULL DEFAULT 0;
ALTER TABLE branches ADD COLUMN search_pending_json BLOB NOT NULL DEFAULT '{}';
ALTER TABLE events ADD COLUMN turn_ordinal INTEGER NOT NULL DEFAULT 0;
ALTER TABLE events ADD COLUMN turn_start_seq INTEGER NOT NULL DEFAULT 0;
ALTER TABLE events ADD COLUMN turn_key TEXT NOT NULL DEFAULT '';
ALTER TABLE events ADD COLUMN item_key TEXT NOT NULL DEFAULT '';
ALTER TABLE events ADD COLUMN item_mode INTEGER NOT NULL DEFAULT 0;
ALTER TABLE events ADD COLUMN item_start_seq INTEGER NOT NULL DEFAULT 0;
ALTER TABLE events ADD COLUMN visible INTEGER NOT NULL DEFAULT 0;
UPDATE branches SET
 last_turn_id=coalesce((SELECT source_turn_id FROM turns WHERE branch_id=branches.id ORDER BY ordinal DESC LIMIT 1),''),
 last_turn_start_seq=coalesce((SELECT start_seq FROM turns WHERE branch_id=branches.id ORDER BY ordinal DESC LIMIT 1),0),
 last_turn_has_user=coalesce((SELECT has_user FROM turns WHERE branch_id=branches.id ORDER BY ordinal DESC LIMIT 1),0);
`); err != nil {
		return err
	}
	if err = migrateEventFields(ctx, tx); err != nil {
		return err
	}
	if err = migrateSessionMaps(ctx, tx); err != nil {
		return err
	}
	after, err := canonicalSchemaDigest(ctx, tx)
	if err != nil {
		return err
	}
	if before != after {
		return errors.New("canonical conversation data changed during schema migration")
	}
	// These contain derived snapshots or obsolete delivery bookkeeping. All
	// canonical data and imported-source checkpoints remain in their own tables.
	if _, err = tx.ExecContext(ctx, `
DROP TABLE handoffs;
DROP TABLE runs;
DROP TABLE harness_sessions;
DROP TABLE transcript_content_refs;
DROP TABLE content_blobs;
DROP TABLE transcript_items;
DROP TABLE transcript_text_pending;
DROP TABLE transcript_text_runs;
DROP TABLE transcript_text_search;
DROP TABLE transcript_search;
DROP TABLE turns;
DROP INDEX events_by_time;
`+eventIndexSchema+searchSchema); err != nil {
		return err
	}
	if err = verifySchemaDatabase(ctx, tx); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version=%d", schemaVersion))
	return err
}

const schemaBackfillBatchBytes = 4 * 1024 * 1024

func migrateEventFields(ctx context.Context, tx *sql.Tx) error {
	var overlaps int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM (
 SELECT start_seq,lag(end_seq) OVER (PARTITION BY branch_id ORDER BY start_seq) previous_end FROM turns
 ) WHERE previous_end>=start_seq`).Scan(&overlaps); err != nil {
		return err
	}
	if overlaps != 0 {
		return errors.New("overlapping legacy turn ranges")
	}
	var afterBranch string
	var afterSeq int64
	// A bounded page prevents a multi-gigabyte history from becoming a single
	// allocation. The existing turn ranges are authoritative grouping metadata.
	type record struct {
		branch, turn        string
		seq, ordinal, start int64
		raw                 []byte
	}
	var currentBranch string
	var currentOrdinal int64
	itemStarts := map[string]int64{}
	for {
		rows, err := tx.QueryContext(ctx, `SELECT e.branch_id,e.seq,e.payload_json,t.ordinal,t.start_seq,t.source_turn_id
 FROM events e LEFT JOIN turns t ON t.branch_id=e.branch_id AND t.ordinal=(
 SELECT ordinal FROM turns WHERE branch_id=e.branch_id AND start_seq<=e.seq ORDER BY start_seq DESC LIMIT 1
 ) AND e.seq<=t.end_seq
 WHERE (e.branch_id,e.seq)>(?,?)
 ORDER BY e.branch_id,e.seq LIMIT 128`, afterBranch, afterSeq)
		if err != nil {
			return err
		}
		var page []record
		pageBytes := 0
		for rows.Next() {
			var r record
			if err = rows.Scan(&r.branch, &r.seq, &r.raw, &r.ordinal, &r.start, &r.turn); err != nil {
				rows.Close()
				return fmt.Errorf("event has missing or invalid turn grouping: %w", err)
			}
			page = append(page, r)
			pageBytes += len(r.raw)
			// Always admit one complete event, including oversized legacy
			// records, but do not retain the next rows once this page is full.
			if pageBytes >= schemaBackfillBatchBytes {
				break
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		if len(page) == 0 {
			return nil
		}
		for _, r := range page {
			var event chat.Event
			if err = json.Unmarshal(r.raw, &event); err != nil {
				return fmt.Errorf("decode event %s/%d: %w", r.branch, r.seq, err)
			}
			if r.branch == afterBranch && r.seq <= afterSeq {
				return errors.New("overlapping legacy turn ranges")
			}
			event.Seq = r.seq
			key, mode, visible := transcriptItemIdentity(event, r.seq)
			if r.branch != currentBranch || r.ordinal != currentOrdinal {
				itemStarts = map[string]int64{}
				currentBranch, currentOrdinal = r.branch, r.ordinal
			}
			start := r.seq
			if visible && mode != transcriptItemSingle {
				if prior, ok := itemStarts[key]; ok {
					start = prior
				} else {
					itemStarts[key] = start
				}
			}
			_, err = tx.ExecContext(ctx, `UPDATE events SET turn_ordinal=?,turn_start_seq=?,turn_key=?,item_key=?,item_mode=?,item_start_seq=?,visible=? WHERE branch_id=? AND seq=?`, r.ordinal, r.start, r.turn, key, mode, start, visible, r.branch, r.seq)
			if err != nil {
				return err
			}
			afterBranch, afterSeq = r.branch, r.seq
		}
	}
}

func migrateSessionMaps(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `SELECT c.id,s.provider,s.account_id,s.native_session_id,s.history_through_seq,s.state,
 EXISTS(SELECT 1 FROM runs r WHERE r.harness_session_id=s.id AND r.status IN ('pending','accepted'))
 FROM chats c JOIN harness_sessions s ON s.branch_id=c.active_branch_id
 WHERE s.state<>'retired' ORDER BY c.id,(s.state IN ('ready','legacy_assumed','pending')),s.updated_at,s.id`)
	if err != nil {
		return err
	}
	maps := map[string]map[string]sessionState{}
	for rows.Next() {
		var id, account string
		var provider chat.Provider
		var state sessionState
		var openRun bool
		if err = rows.Scan(&id, &provider, &account, &state.NativeSessionID, &state.HistoryThroughSeq, &state.State, &openRun); err != nil {
			rows.Close()
			return err
		}
		if openRun || (state.State != "ready" && state.State != "legacy_assumed") || state.NativeSessionID == "" {
			state.State = "uncertain"
		}
		if maps[id] == nil {
			maps[id] = map[string]sessionState{}
		}
		maps[id][sessionKey(provider, account)] = state
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for id, states := range maps {
		raw, err := json.Marshal(states)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "UPDATE chats SET sessions_json=? WHERE id=?", raw, id); err != nil {
			return err
		}
		var saved []byte
		if err = tx.QueryRowContext(ctx, "SELECT sessions_json FROM chats WHERE id=?", id).Scan(&saved); err != nil {
			return err
		}
		if string(saved) != string(raw) {
			return fmt.Errorf("session map verification failed for chat %s", id)
		}
	}
	return nil
}

// Include every original canonical column, including exact payload/raw bytes,
// metadata, ancestry, and import checkpoints. Length/type framing distinguishes
// NULL, empty data, and concatenation boundaries without decoding JSON.
func canonicalSchemaDigest(ctx context.Context, q sqlQuerier) (string, error) {
	h := sha256.New()
	queries := []string{
		"SELECT id,project_id,active_branch_id,title,last_message_at,meta_json FROM chats ORDER BY id",
		"SELECT id,chat_id,parent_branch_id,fork_seq,last_seq,last_turn_ordinal FROM branches ORDER BY id",
		"SELECT branch_id,seq,t,event_type,turn_id,payload_json,raw_json FROM events ORDER BY branch_id,seq",
		"SELECT id,status,source_manifest,started_at,completed_at FROM conversation_migration ORDER BY id",
		"SELECT chat_id,branch_id,fingerprint,byte_offset,event_count,last_seq,hash_state,status FROM conversation_imports ORDER BY chat_id",
	}
	for _, query := range queries {
		h.Write([]byte(query))
		rows, err := q.QueryContext(ctx, query)
		if err != nil {
			return "", err
		}
		columns, err := rows.Columns()
		if err != nil {
			rows.Close()
			return "", err
		}
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}
		for rows.Next() {
			if err = rows.Scan(pointers...); err != nil {
				rows.Close()
				return "", err
			}
			for _, value := range values {
				var data []byte
				switch v := value.(type) {
				case nil:
					data = []byte{0}
				case []byte:
					data = append([]byte{1}, v...)
				case string:
					data = append([]byte{2}, v...)
				case int64:
					data = make([]byte, 9)
					data[0] = 3
					binary.BigEndian.PutUint64(data[1:], uint64(v))
				default:
					rows.Close()
					return "", fmt.Errorf("unexpected canonical field type %T", v)
				}
				var size [8]byte
				binary.BigEndian.PutUint64(size[:], uint64(len(data)))
				h.Write(size[:])
				h.Write(data)
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func verifySchemaDatabase(ctx context.Context, q sqlQuerier) error {
	for _, check := range []string{"integrity_check", "foreign_key_check"} {
		rows, err := q.QueryContext(ctx, "PRAGMA "+check)
		if err != nil {
			return err
		}
		if check == "integrity_check" {
			for rows.Next() {
				var result string
				if err = rows.Scan(&result); err != nil {
					rows.Close()
					return err
				}
				if strings.TrimSpace(result) != "ok" {
					rows.Close()
					return fmt.Errorf("SQLite integrity check: %s", result)
				}
			}
		} else if rows.Next() {
			rows.Close()
			return errors.New("SQLite foreign key check failed")
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
