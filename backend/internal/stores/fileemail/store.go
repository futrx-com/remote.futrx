// Package fileemail is file-backed storage for the server's single Gmail
// SMTP credential, at <dataDir>/smtp.json, mode 0600. The file's absence -
// Credentials returns (nil, nil) - is the correct "not configured" state,
// not an error.
package fileemail

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	serviceemail "github.com/futrx-com/remote.futrx.com/internal/service/email"
)

var _ serviceemail.Store = (*Store)(nil)

const fileName = "smtp.json"

type Store struct {
	dataDir string
	mu      sync.Mutex
}

func New(dataDir string) *Store {
	return &Store{dataDir: dataDir}
}

type credentialsRecord struct {
	Address     string `json:"address"`
	AppPassword string `json:"appPassword"`
}

func (s *Store) Credentials(ctx context.Context) (*serviceemail.Credentials, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(filepath.Join(s.dataDir, fileName))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var record credentialsRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, err
	}
	return &serviceemail.Credentials{Address: record.Address, AppPassword: record.AppPassword}, nil
}

func (s *Store) Save(ctx context.Context, creds serviceemail.Credentials) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeJSONLocked(credentialsRecord{Address: creds.Address, AppPassword: creds.AppPassword})
}

func (s *Store) Delete(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.Remove(filepath.Join(s.dataDir, fileName)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *Store) writeJSONLocked(value any) error {
	if err := os.MkdirAll(s.dataDir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.dataDir, ".smtp-*.json.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	encoder := json.NewEncoder(tmp)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, filepath.Join(s.dataDir, fileName))
}
