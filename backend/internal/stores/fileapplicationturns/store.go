package fileapplicationturns

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"

	svc "github.com/futrx-com/remote.futrx.com/internal/service/applications"
)

// Store retains generic turn receipts independently of application process data.
// Names are hashed so caller-supplied request IDs never become filesystem paths.
type Store struct {
	root string
	mu   sync.Mutex
}

func New(dataDir string) *Store { return &Store{root: filepath.Join(dataDir, "application-turns")} }
func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
func (s *Store) directory(id string) string { return filepath.Join(s.root, digest(id)) }
func (s *Store) filename(id, request string) string {
	return filepath.Join(s.directory(id), digest(request)+".json")
}
func (s *Store) Get(ctx context.Context, id, request string) (svc.AgentTurnRecord, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return svc.AgentTurnRecord{}, false, err
	}
	data, err := os.ReadFile(s.filename(id, request))
	if errors.Is(err, os.ErrNotExist) {
		return svc.AgentTurnRecord{}, false, nil
	}
	if err != nil {
		return svc.AgentTurnRecord{}, false, err
	}
	var record svc.AgentTurnRecord
	err = json.Unmarshal(data, &record)
	return record, err == nil, err
}
func (s *Store) Put(ctx context.Context, id string, record svc.AgentTurnRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	dir := s.directory(id)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "turn-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(f.Name(), s.filename(id, record.Request.RequestID)); err != nil {
		return err
	}
	// Persist newly created directory entries as well as the receipt rename.
	for _, directory := range []string{dir, s.root, filepath.Dir(s.root)} {
		if err := syncDirectory(directory); err != nil {
			return err
		}
	}
	return nil
}
func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func (s *Store) Delete(ctx context.Context, id, request string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	err := os.Remove(s.filename(id, request))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return syncDirectory(s.directory(id))
}
func (s *Store) Remove(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.RemoveAll(s.directory(id)); err != nil {
		return err
	}
	if _, err := os.Stat(s.root); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return syncDirectory(s.root)
}
