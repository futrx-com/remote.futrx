// Package fileemail is file-backed storage for the server's single SMTP
// configuration, at <dataDir>/smtp.json, mode 0600. The file's absence -
// Configuration returns (nil, nil) - is the correct "not configured" state,
// not an error.
package fileemail

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	emailapplication "github.com/futrx-com/remote.futrx.com/internal/model/email/application"
	emailoutbound "github.com/futrx-com/remote.futrx.com/internal/port/email/outbound"
)

var _ emailoutbound.ConfigurationStore = (*Store)(nil)

const fileName = "smtp.json"

// recordVersion is the persisted schema version. Version 1 was the
// unreleased Gmail-only MVP record (address + appPassword); it is not
// migrated, per the provider-neutral SMTP refactor plan, because no
// version-1 install was ever a supported release.
const recordVersion = 2

type Store struct {
	dataDir string
	mu      sync.Mutex
}

func New(dataDir string) *Store {
	return &Store{dataDir: dataDir}
}

// configurationRecord is the private on-disk shape of the SMTP
// configuration. It is intentionally not exported: the application model is
// the public contract, and this record is an implementation detail mapped
// to and from it only at this store's boundary.
type configurationRecord struct {
	Version        int    `json:"version"`
	Host           string `json:"host"`
	Port           uint16 `json:"port"`
	TLSMode        string `json:"tlsMode"`
	Authentication string `json:"authentication"`
	Username       string `json:"username"`
	Password       string `json:"password"`
	FromAddress    string `json:"fromAddress"`
}

func (s *Store) Configuration(ctx context.Context) (*emailapplication.SMTPConfiguration, error) {
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
	var record configurationRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, fmt.Errorf("fileemail: malformed configuration record: %w", err)
	}
	if record.Version != recordVersion {
		return nil, fmt.Errorf("fileemail: unsupported configuration record version %d", record.Version)
	}
	return &emailapplication.SMTPConfiguration{
		Host:           record.Host,
		Port:           record.Port,
		TLSMode:        emailapplication.TLSMode(record.TLSMode),
		Authentication: emailapplication.AuthenticationMode(record.Authentication),
		Username:       record.Username,
		Password:       record.Password,
		FromAddress:    record.FromAddress,
	}, nil
}

func (s *Store) Save(ctx context.Context, cfg emailapplication.SMTPConfiguration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeJSONLocked(configurationRecord{
		Version:        recordVersion,
		Host:           cfg.Host,
		Port:           cfg.Port,
		TLSMode:        string(cfg.TLSMode),
		Authentication: string(cfg.Authentication),
		Username:       cfg.Username,
		Password:       cfg.Password,
		FromAddress:    cfg.FromAddress,
	})
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
