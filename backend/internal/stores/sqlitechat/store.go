// Package sqlitechat implements Remote's authoritative conversation store.
package sqlitechat

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	_ "modernc.org/sqlite"
)

const Filename = "conversations.sqlite"
const schemaVersion = 1

var errInvalidProjection = errors.New("invalid transcript projection")
var _ servicechat.Repository = (*Store)(nil)
var _ servicechat.TranscriptEventSource = (*Store)(nil)
var _ servicechat.TranscriptEventWindowSource = (*Store)(nil)
var _ servicechat.TranscriptProjectionSource = (*Store)(nil)
var _ servicechat.CopiedEventAppender = (*Store)(nil)

type Store struct {
	db        *sql.DB
	root      string
	mu        sync.Mutex
	lockFile  *os.File
	closeOnce sync.Once
	closeErr  error
}

type rowQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}
type sqlQuerier interface {
	rowQuerier
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func New(root string) (*Store, error) {
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	lock, err := acquireStoreLock(root)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(root, Filename)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		lock.Close()
		return nil, err
	}
	file.Close()
	if err = os.Chmod(path, 0600); err != nil {
		lock.Close()
		return nil, err
	}
	u := url.URL{Scheme: "file", Path: path}
	q := u.Query()
	// modernc's driver uses repeated _pragma parameters, rather than the
	// similarly named mattn DSN flags. Apply settings on every connection.
	for _, pragma := range []string{"busy_timeout(5000)", "foreign_keys(ON)", "journal_mode(WAL)", "synchronous(FULL)", "trusted_schema(OFF)"} {
		q.Add("_pragma", pragma)
	}
	q.Set("_txlock", "immediate")
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		lock.Close()
		return nil, err
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)
	s := &Store{db: db, root: root, lockFile: lock}
	if err = s.initialize(); err != nil {
		s.Close()
		return nil, fmt.Errorf("open authoritative conversation database: %w", err)
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if err = os.Chmod(path+suffix, 0600); err != nil && !errors.Is(err, os.ErrNotExist) {
			s.Close()
			return nil, err
		}
	}
	return s, nil
}

func (s *Store) initialize() error {
	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version != 0 && version != schemaVersion {
		return fmt.Errorf("unsupported conversation schema %d (expected %d)", version, schemaVersion)
	}
	if version == schemaVersion {
		// Missing authoritative tables must fail startup; never recreate a broken
		// database and pretend that existing history was successfully loaded.
		for _, table := range []string{"chats", "branches", "events", "turns", "transcript_items", "transcript_search", "transcript_content_refs", "transcript_text_runs", "transcript_text_search", "transcript_text_pending", "harness_sessions", "handoffs", "runs", "content_blobs", "conversation_migration", "conversation_imports"} {
			var count int
			if err := s.db.QueryRow("SELECT count(*) FROM sqlite_schema WHERE name=?", table).Scan(&count); err != nil {
				return err
			}
			if count != 1 {
				return fmt.Errorf("missing conversation table %s", table)
			}
		}
		return nil
	}
	var tables int
	if err := s.db.QueryRow("SELECT count(*) FROM sqlite_schema WHERE type='table' AND name NOT LIKE 'sqlite_%'").Scan(&tables); err != nil {
		return err
	}
	if tables != 0 {
		return errors.New("unversioned nonempty conversation database; refusing to overwrite")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(schema + sessionSchema + importSchema); err != nil {
		return err
	}
	if _, err = tx.Exec(fmt.Sprintf("PRAGMA user_version=%d", schemaVersion)); err != nil {
		return err
	}
	return tx.Commit()
}

// DB is for migration and maintenance commands. Runtime callers use the typed
// repository ports rather than issuing SQL outside this package.
func (s *Store) DB() *sql.DB { return s.db }
func (s *Store) Close() error {
	s.closeOnce.Do(func() { s.closeErr = errors.Join(s.db.Close(), s.lockFile.Close()) })
	return s.closeErr
}

func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}
func normalizeMeta(meta servicechat.Meta) servicechat.Meta {
	now := time.Now().UnixMilli()
	if meta.ID == "" {
		meta.ID = servicechat.ID(newID()[:12])
	}
	if meta.CreatedAt == 0 {
		meta.CreatedAt = now
	}
	if meta.LastMessageAt == 0 {
		meta.LastMessageAt = meta.CreatedAt
	}
	if meta.LastReadAt == 0 {
		meta.LastReadAt = meta.LastMessageAt
	}
	if meta.Title == "" {
		meta.Title = "New chat"
	}
	if meta.Mode == "" {
		meta.Mode = "default"
	}
	meta.Provider = servicechat.NormalizeProvider(meta.Provider)
	meta.NormalizeSessions()
	meta.ReasoningEffort = servicechat.NormalizeReasoningEffort(meta.ReasoningEffort)
	meta.ServiceTier = servicechat.NormalizeServiceTier(meta.ServiceTier)
	meta.SelectedSkills = servicechat.NormalizeSelectedSkills(meta.SelectedSkills, meta.Provider)
	meta.Running = false
	return meta
}

func (s *Store) createChatTx(ctx context.Context, tx *sql.Tx, meta servicechat.Meta, rawMeta []byte) (string, error) {
	branch := newID()
	encoded := rawMeta
	var err error
	if len(encoded) == 0 {
		encoded, err = json.Marshal(meta)
		if err != nil {
			return "", err
		}
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO chats(id,project_id,active_branch_id,title,last_message_at,meta_json) VALUES(?,?,?,?,?,?)", meta.ID, meta.ProjectID, branch, meta.Title, meta.LastMessageAt, encoded); err != nil {
		return "", err
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO branches(id,chat_id) VALUES(?,?)", branch, meta.ID)
	return branch, err
}

func mergeMeta(meta servicechat.Meta, raw []byte) ([]byte, error) {
	encoded, err := json.Marshal(meta)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return encoded, nil
	}
	var old, known map[string]json.RawMessage
	if err = json.Unmarshal(raw, &old); err != nil {
		return nil, err
	}
	if err = json.Unmarshal(encoded, &known); err != nil {
		return nil, err
	}
	// Clear optional known fields that normalization removed, preserving only
	// genuinely unknown fields from future/legacy formats.
	for _, key := range metaFields {
		delete(old, key)
	}
	for key, value := range known {
		old[key] = value
	}
	return json.Marshal(old)
}

var metaFields = func() []string {
	t := reflect.TypeOf(servicechat.Meta{})
	fields := make([]string, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		key := strings.Split(t.Field(i).Tag.Get("json"), ",")[0]
		if key != "" && key != "-" {
			fields = append(fields, key)
		}
	}
	return fields
}()

func decodeMeta(raw []byte) (servicechat.Meta, error) {
	var meta servicechat.Meta
	if err := json.Unmarshal(raw, &meta); err != nil {
		return meta, err
	}
	meta.Provider = servicechat.NormalizeProvider(meta.Provider)
	meta.NormalizeSessions()
	meta.ReasoningEffort = servicechat.NormalizeReasoningEffort(meta.ReasoningEffort)
	meta.ServiceTier = servicechat.NormalizeServiceTier(meta.ServiceTier)
	meta.ApprovalPolicy = servicechat.NormalizeApprovalPolicy(meta.ApprovalPolicy)
	meta.SandboxPolicy = servicechat.NormalizeSandboxPolicy(meta.SandboxPolicy)
	meta.SelectedSkills = servicechat.NormalizeSelectedSkills(meta.SelectedSkills, meta.Provider)
	if meta.LastReadAt == 0 {
		meta.LastReadAt = meta.LastMessageAt
	}
	meta.Running = false
	return meta, nil
}

func (s *Store) Create(ctx context.Context, meta servicechat.Meta) (servicechat.Meta, error) {
	meta = normalizeMeta(meta)
	if !servicechat.ValidID(meta.ID) {
		return servicechat.Meta{}, servicechat.ErrInvalidID
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return meta, err
	}
	defer tx.Rollback()
	if _, err = s.createChatTx(ctx, tx, meta, nil); err != nil {
		return meta, err
	}
	return meta, tx.Commit()
}
func readMeta(ctx context.Context, q rowQuerier, id servicechat.ID) (servicechat.Meta, []byte, error) {
	if !servicechat.ValidID(id) {
		return servicechat.Meta{}, nil, servicechat.ErrInvalidID
	}
	var raw []byte
	err := q.QueryRowContext(ctx, "SELECT meta_json FROM chats WHERE id=?", id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		err = servicechat.ErrNotFound
	}
	if err != nil {
		return servicechat.Meta{}, nil, err
	}
	m, err := decodeMeta(raw)
	if err != nil {
		return m, nil, err
	}
	return m, raw, nil
}
func (s *Store) Get(ctx context.Context, id servicechat.ID) (servicechat.Meta, error) {
	m, _, err := readMeta(ctx, s.db, id)
	return m, err
}
func (s *Store) List(ctx context.Context) ([]servicechat.Meta, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT meta_json FROM chats ORDER BY last_message_at DESC,id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []servicechat.Meta{}
	for rows.Next() {
		var raw []byte
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		meta, err := decodeMeta(raw)
		if err != nil {
			return nil, err
		}
		out = append(out, meta)
	}
	return out, rows.Err()
}
func writeMetaTx(ctx context.Context, tx *sql.Tx, meta servicechat.Meta, raw []byte) error {
	data, err := mergeMeta(meta, raw)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "UPDATE chats SET project_id=?,title=?,last_message_at=?,meta_json=? WHERE id=?", meta.ProjectID, meta.Title, meta.LastMessageAt, data, meta.ID)
	return err
}
func (s *Store) Update(ctx context.Context, id servicechat.ID, fn func(*servicechat.Meta)) (servicechat.Meta, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return servicechat.Meta{}, err
	}
	defer tx.Rollback()
	meta, raw, err := readMeta(ctx, tx, id)
	if err != nil {
		return meta, err
	}
	fn(&meta)
	meta.ID = id
	meta.Provider = servicechat.NormalizeProvider(meta.Provider)
	meta.NormalizeSessions()
	meta.ReasoningEffort = servicechat.NormalizeReasoningEffort(meta.ReasoningEffort)
	meta.ServiceTier = servicechat.NormalizeServiceTier(meta.ServiceTier)
	meta.SelectedSkills = servicechat.NormalizeSelectedSkills(meta.SelectedSkills, meta.Provider)
	meta.Running = false
	if err = writeMetaTx(ctx, tx, meta, raw); err != nil {
		return meta, err
	}
	return meta, tx.Commit()
}
func activeBranch(ctx context.Context, q rowQuerier, id servicechat.ID) (string, int64, error) {
	if !servicechat.ValidID(id) {
		return "", 0, servicechat.ErrInvalidID
	}
	var branch string
	var seq int64
	err := q.QueryRowContext(ctx, "SELECT b.id,b.last_seq FROM chats c JOIN branches b ON b.id=c.active_branch_id WHERE c.id=?", id).Scan(&branch, &seq)
	if errors.Is(err, sql.ErrNoRows) {
		err = servicechat.ErrNotFound
	}
	return branch, seq, err
}

// HistoryVersion changes when a rewind replaces the active branch. Clients
// compare it before reusing a sequence cursor from an earlier connection.
func (s *Store) HistoryVersion(ctx context.Context, id servicechat.ID) (string, error) {
	branch, _, err := activeBranch(ctx, s.db, id)
	return branch, err
}
