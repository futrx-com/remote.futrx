// Package filepermissions persists the permission policy in
// DATA_DIR/permissions.json and appends every successful policy mutation to
// DATA_DIR/permission-audit.jsonl. Both files are mode 0600 in a 0700
// directory.
package filepermissions

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/futrx-com/remote.futrx.com/internal/service/permission"
)

var _ permission.Repository = (*Store)(nil)

const (
	stateFileName = "permissions.json"
	auditFileName = "permission-audit.jsonl"
)

// Store is the file-backed permission.Repository. One in-process mutex
// serializes all mutations; the policy is small and held in memory after it
// has been fully validated at startup.
type Store struct {
	dir string

	mu    sync.Mutex
	state permission.State

	// appendAudit is a seam so tests can prove a failed audit write prevents
	// the policy mutation.
	appendAudit func(data []byte) error
}

// New loads and validates DATA_DIR/permissions.json. An absent file is an
// empty policy. A corrupt file fails startup instead of being ignored, since
// ignoring it would silently drop deny rules.
func New(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create permissions dir: %w", err)
	}
	store := &Store{dir: dataDir}
	store.appendAudit = store.appendAuditFile
	state, err := store.readState()
	if err != nil {
		return nil, err
	}
	store.state = state
	return store, nil
}

func (s *Store) statePath() string { return filepath.Join(s.dir, stateFileName) }
func (s *Store) auditPath() string { return filepath.Join(s.dir, auditFileName) }

func (s *Store) readState() (permission.State, error) {
	raw, err := os.ReadFile(s.statePath())
	if errors.Is(err, os.ErrNotExist) {
		return permission.State{}, nil
	}
	if err != nil {
		return permission.State{}, fmt.Errorf("read %s: %w", stateFileName, err)
	}
	var record fileRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		return permission.State{}, fmt.Errorf("%w: parse %s: %v", permission.ErrInvalidState, stateFileName, err)
	}
	if record.Version != schemaVersion {
		return permission.State{}, fmt.Errorf("%w: %s has unsupported schema version %d (want %d)",
			permission.ErrInvalidState, stateFileName, record.Version, schemaVersion)
	}
	state := stateFromRecord(record)
	if err := state.ValidateStructure(); err != nil {
		return permission.State{}, fmt.Errorf("%s: %w", stateFileName, err)
	}
	return state, nil
}

// Load returns a private copy of the current policy.
func (s *Store) Load(ctx context.Context) (permission.State, error) {
	if err := ctx.Err(); err != nil {
		return permission.State{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state.Clone(), nil
}

// Mutate applies change under the exclusive lock. The new state is written to
// a temporary file first, the audit events are appended next, and only then is
// the temporary file renamed into place, so a failed audit leaves the policy
// exactly as it was.
func (s *Store) Mutate(
	ctx context.Context,
	change func(permission.State) (permission.State, []permission.AuditEvent, error),
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	next, events, err := change(s.state.Clone())
	if err != nil {
		return err
	}
	if len(events) == 0 {
		return nil
	}
	if err := next.ValidateStructure(); err != nil {
		return err
	}

	tmpName, err := s.writeTemp(next)
	if err != nil {
		return err
	}
	published := false
	defer func() {
		if !published {
			_ = os.Remove(tmpName)
		}
	}()

	auditLines, err := encodeAudit(events)
	if err != nil {
		return err
	}
	if err := s.appendAudit(auditLines); err != nil {
		return fmt.Errorf("%w: %v", permission.ErrAuditFailed, err)
	}
	if err := os.Rename(tmpName, s.statePath()); err != nil {
		return fmt.Errorf("publish %s: %w", stateFileName, err)
	}
	published = true
	s.syncDir()
	s.state = next.Clone()
	return nil
}

func (s *Store) writeTemp(state permission.State) (string, error) {
	data, err := json.MarshalIndent(recordFromState(state), "", "  ")
	if err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(s.dir, ".permissions-*.tmp")
	if err != nil {
		return "", fmt.Errorf("create temp %s: %w", stateFileName, err)
	}
	name := tmp.Name()
	fail := func(err error) (string, error) {
		_ = tmp.Close()
		_ = os.Remove(name)
		return "", fmt.Errorf("write temp %s: %w", stateFileName, err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		return fail(err)
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		return fail(err)
	}
	if err := tmp.Sync(); err != nil {
		return fail(err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return "", fmt.Errorf("close temp %s: %w", stateFileName, err)
	}
	return name, nil
}

func encodeAudit(events []permission.AuditEvent) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	for _, event := range events {
		if err := encoder.Encode(auditRecordFromEvent(event)); err != nil {
			return nil, err
		}
	}
	return buffer.Bytes(), nil
}

func (s *Store) appendAuditFile(data []byte) error {
	file, err := os.OpenFile(s.auditPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

// syncDir makes the rename durable. It is best effort: the state file itself
// has already been fsynced.
func (s *Store) syncDir() {
	dir, err := os.Open(s.dir)
	if err != nil {
		return
	}
	_ = dir.Sync()
	_ = dir.Close()
}
