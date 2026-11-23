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
	"hash"
	"os"
	"path/filepath"
	"time"

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

const importSchema = `
CREATE TABLE IF NOT EXISTS conversation_migration (
 id INTEGER PRIMARY KEY CHECK (id = 1),
 status TEXT NOT NULL CHECK (status IN ('importing','complete')),
 source_manifest TEXT NOT NULL,
 started_at INTEGER NOT NULL,
 completed_at INTEGER
);
CREATE TABLE IF NOT EXISTS conversation_imports (
 chat_id TEXT PRIMARY KEY REFERENCES chats(id) ON DELETE CASCADE,
 branch_id TEXT NOT NULL REFERENCES branches(id) ON DELETE CASCADE,
 fingerprint TEXT NOT NULL,
 byte_offset INTEGER NOT NULL DEFAULT 0,
 event_count INTEGER NOT NULL DEFAULT 0,
 last_seq INTEGER NOT NULL DEFAULT 0,
 hash_state BLOB,
 status TEXT NOT NULL DEFAULT 'importing' CHECK (status IN ('importing','complete'))
);`

type ImportProgress struct {
	ChatID      servicechat.ID
	BranchID    string
	Fingerprint string
	ByteOffset  int64
	EventCount  int64
	LastSeq     int64
	HashState   []byte
	Complete    bool
}

type ImportEvent struct {
	Event servicechat.Event
	Raw   []byte
}

// HashImportEvent frames the original bytes with their effective sequence.
// This also covers legacy events without an explicit seq field.
func HashImportEvent(h hash.Hash, seq int64, raw []byte) {
	var frame [16]byte
	binary.BigEndian.PutUint64(frame[:8], uint64(seq))
	binary.BigEndian.PutUint64(frame[8:], uint64(len(raw)))
	_, _ = h.Write(frame[:])
	_, _ = h.Write(raw)
}

func (s *Store) MigrationComplete(ctx context.Context) (bool, error) {
	var status string
	err := s.DB().QueryRowContext(ctx, `SELECT status FROM conversation_migration WHERE id=1`).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return status == "complete", err
}

// EnsureRuntimeReady refuses to silently start with an empty SQLite database
// over an existing legacy installation. Empty fresh installs need no import.
func (s *Store) EnsureRuntimeReady(ctx context.Context, root string) error {
	var status string
	err := s.DB().QueryRowContext(ctx, `SELECT status FROM conversation_migration WHERE id=1`).Scan(&status)
	if err == nil {
		if status == "complete" {
			return nil
		}
		return errors.New("conversation migration is incomplete; stop the backend and run remote migrate-chats")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	entries, err := os.ReadDir(filepath.Join(root, "chats"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if len(entries) != 0 {
		return errors.New("legacy conversations require migration; stop the backend and run remote migrate-chats")
	}
	_, err = s.DB().ExecContext(ctx, `INSERT INTO conversation_migration(id,status,source_manifest,started_at,completed_at) VALUES(1,'complete','fresh',?,?)`, time.Now().UnixMilli(), time.Now().UnixMilli())
	return err
}

func (s *Store) BeginImport(ctx context.Context, manifest string) error {
	_, err := s.DB().ExecContext(ctx, `INSERT INTO conversation_migration(id,status,source_manifest,started_at) VALUES(1,'importing',?,?) ON CONFLICT(id) DO NOTHING`, manifest, time.Now().UnixMilli())
	if err != nil {
		return err
	}
	var status, existing string
	if err := s.DB().QueryRowContext(ctx, `SELECT status,source_manifest FROM conversation_migration WHERE id=1`).Scan(&status, &existing); err != nil {
		return err
	}
	if status == "complete" {
		return errors.New("conversation migration already complete")
	}
	if existing != manifest {
		return errors.New("legacy conversation source changed since migration started; restore the frozen source before retrying")
	}
	return nil
}

func (s *Store) BeginImportChat(ctx context.Context, meta servicechat.Meta, rawMeta []byte, fingerprint string) (ImportProgress, error) {
	var p ImportProgress
	p.ChatID = meta.ID
	var status string
	err := s.DB().QueryRowContext(ctx, `SELECT branch_id,fingerprint,byte_offset,event_count,last_seq,hash_state,status FROM conversation_imports WHERE chat_id=?`, meta.ID).Scan(&p.BranchID, &p.Fingerprint, &p.ByteOffset, &p.EventCount, &p.LastSeq, &p.HashState, &status)
	if err == nil {
		if p.Fingerprint != fingerprint {
			return p, fmt.Errorf("legacy chat %s changed since its import started", meta.ID)
		}
		p.Complete = status == "complete"
		return p, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return p, err
	}
	tx, err := s.DB().BeginTx(ctx, nil)
	if err != nil {
		return p, err
	}
	defer tx.Rollback()
	p.BranchID, err = s.createChatTx(ctx, tx, meta, rawMeta)
	if err != nil {
		return p, err
	}
	p.Fingerprint = fingerprint
	if _, err := tx.ExecContext(ctx, `INSERT INTO conversation_imports(chat_id,branch_id,fingerprint) VALUES(?,?,?)`, meta.ID, p.BranchID, fingerprint); err != nil {
		return p, err
	}
	return p, tx.Commit()
}

// ImportEventBatch commits records and their source checkpoint together.
func (s *Store) ImportEventBatch(ctx context.Context, p ImportProgress, events []ImportEvent) error {
	tx, err := s.DB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, record := range events {
		if _, err := s.appendEventTx(ctx, tx, p.BranchID, record.Event, record.Raw); err != nil {
			return err
		}
	}
	result, err := tx.ExecContext(ctx, `UPDATE conversation_imports SET byte_offset=?,event_count=?,last_seq=?,hash_state=? WHERE chat_id=? AND status='importing' AND event_count=?`, p.ByteOffset, p.EventCount, p.LastSeq, p.HashState, p.ChatID, p.EventCount-int64(len(events)))
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return errors.New("import checkpoint does not match committed event count")
	}
	return tx.Commit()
}

func (s *Store) FinishImportChat(ctx context.Context, meta servicechat.Meta, rawMeta []byte, p ImportProgress, expectedHash string) error {
	// Read incrementally so validation never materializes a whole conversation.
	rows, err := s.DB().QueryContext(ctx, `SELECT seq,raw_json,payload_json FROM events WHERE branch_id=? ORDER BY seq`, p.BranchID)
	if err != nil {
		return err
	}
	h := sha256.New()
	var count, last int64
	for rows.Next() {
		var seq int64
		var raw, payload []byte
		if err := rows.Scan(&seq, &raw, &payload); err != nil {
			rows.Close()
			return err
		}
		if seq <= last {
			rows.Close()
			return errors.New("imported sequence is not strictly increasing")
		}
		var event servicechat.Event
		if err := json.Unmarshal(raw, &event); err != nil {
			rows.Close()
			return err
		}
		event.Seq = seq
		event.NormalizeSession()
		expected, err := json.Marshal(event)
		if err != nil {
			rows.Close()
			return err
		}
		if string(expected) != string(payload) {
			rows.Close()
			return fmt.Errorf("chat %s event %d normalized content validation failed", meta.ID, seq)
		}
		HashImportEvent(h, seq, raw)
		count++
		last = seq
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if count != p.EventCount || last != p.LastSeq || hex.EncodeToString(h.Sum(nil)) != expectedHash {
		return fmt.Errorf("chat %s import count/order/content validation failed", meta.ID)
	}
	var tail int64
	if err := s.DB().QueryRowContext(ctx, `SELECT last_seq FROM branches WHERE id=?`, p.BranchID).Scan(&tail); err != nil {
		return err
	}
	if tail != last {
		return fmt.Errorf("chat %s imported branch boundary validation failed", meta.ID)
	}
	var storedMeta []byte
	if err := s.DB().QueryRowContext(ctx, `SELECT meta_json FROM chats WHERE id=?`, meta.ID).Scan(&storedMeta); err != nil {
		return err
	}
	if string(storedMeta) != string(rawMeta) {
		return fmt.Errorf("chat %s metadata validation failed", meta.ID)
	}
	if p.Complete {
		return nil
	}
	tx, err := s.DB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := SeedLegacySessionsTx(ctx, tx, meta, p.LastSeq); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE conversation_imports SET status='complete' WHERE chat_id=?`, meta.ID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) CompleteImport(ctx context.Context) error {
	var incomplete int
	if err := s.DB().QueryRowContext(ctx, `SELECT count(*) FROM conversation_imports WHERE status!='complete'`).Scan(&incomplete); err != nil {
		return err
	}
	if incomplete != 0 {
		return errors.New("cannot complete migration with unfinished chats")
	}
	var integrity string
	if err := s.DB().QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&integrity); err != nil {
		return err
	}
	if integrity != "ok" {
		return fmt.Errorf("conversation database integrity check: %s", integrity)
	}
	rows, err := s.DB().QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return err
	}
	invalid := rows.Next()
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if invalid {
		return errors.New("conversation database foreign key check failed")
	}
	_, err = s.DB().ExecContext(ctx, `UPDATE conversation_migration SET status='complete',completed_at=? WHERE id=1 AND status='importing'`, time.Now().UnixMilli())
	return err
}
