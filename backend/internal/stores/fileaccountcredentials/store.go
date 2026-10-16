// Package fileaccountcredentials is file-backed storage for the password
// credentials of invited accounts. A single JSON object at
// <dataDir>/account-credentials.json, mode 0600, maps a normalized email to
// its credential. The file is absent until the first password invite, and an
// account without an entry simply has no invited password: Get returns
// (nil, nil), which is a state, not an error.
package fileaccountcredentials

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
)

var _ serviceauth.AccountCredentialStore = (*Store)(nil)

const fileName = "account-credentials.json"

type Store struct {
	dir  string
	path string

	mu sync.Mutex
}

func New(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	return &Store{dir: dataDir, path: filepath.Join(dataDir, fileName)}, nil
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// loadLocked never returns a partial map: a file that cannot be read or
// parsed fails every operation, so a write can never overwrite credentials
// that were merely unreadable.
func (s *Store) loadLocked() (map[string]serviceauth.AccountCredential, error) {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]serviceauth.AccountCredential{}, nil
		}
		return nil, fmt.Errorf("read %s: %w", fileName, err)
	}
	if len(raw) == 0 {
		return map[string]serviceauth.AccountCredential{}, nil
	}
	var stored map[string]serviceauth.AccountCredential
	if err := json.Unmarshal(raw, &stored); err != nil {
		return nil, fmt.Errorf("parse %s: %w", fileName, err)
	}
	credentials := make(map[string]serviceauth.AccountCredential, len(stored))
	for email, credential := range stored {
		email = normalizeEmail(email)
		if email == "" || credential.PasswordHash == "" {
			return nil, fmt.Errorf("parse %s: incomplete entry", fileName)
		}
		credentials[email] = credential
	}
	return credentials, nil
}

func (s *Store) saveLocked(credentials map[string]serviceauth.AccountCredential) error {
	tmp, err := os.CreateTemp(s.dir, ".account-credentials-*.json.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	enc := json.NewEncoder(tmp)
	enc.SetIndent("", "  ")
	if err := enc.Encode(credentials); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, s.path)
}

func (s *Store) Get(ctx context.Context, email string) (*serviceauth.AccountCredential, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	email = normalizeEmail(email)
	if email == "" {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	credentials, err := s.loadLocked()
	if err != nil {
		return nil, err
	}
	credential, ok := credentials[email]
	if !ok {
		return nil, nil
	}
	return &credential, nil
}

func (s *Store) Save(ctx context.Context, email string, credential serviceauth.AccountCredential) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	email = normalizeEmail(email)
	if email == "" || credential.PasswordHash == "" || credential.Version < 1 {
		return errors.New("account credential is incomplete")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	credentials, err := s.loadLocked()
	if err != nil {
		return err
	}
	credentials[email] = credential
	return s.saveLocked(credentials)
}

// Delete removes email's credential. Removing one that does not exist is a
// no-op that does not touch the file, so account removal can be retried.
func (s *Store) Delete(ctx context.Context, email string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	email = normalizeEmail(email)
	if email == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	credentials, err := s.loadLocked()
	if err != nil {
		return err
	}
	if _, ok := credentials[email]; !ok {
		return nil
	}
	delete(credentials, email)
	return s.saveLocked(credentials)
}

func (s *Store) List(ctx context.Context) (map[string]serviceauth.AccountCredential, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked()
}
