// Package filepush persists Web Push registrations and the server's VAPID key
// pair under DATA_DIR, following the same temp-file-plus-rename discipline as
// the other file stores.
package filepush

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	servicepush "github.com/futrx-com/remote.futrx.com/internal/service/push"
)

var _ servicepush.Repository = (*Store)(nil)

// vapidFile holds the application server key pair. It is a long-lived secret:
// rotating it invalidates every browser subscription on the box.
const vapidFile = "webpush-vapid.json"

type Store struct {
	root    string
	dataDir string
	mu      sync.Mutex
}

type userRecord struct {
	Email         string                     `json:"email"`
	Subscriptions []servicepush.Subscription `json:"subscriptions"`
}

type vapidRecord struct {
	PrivateKey string `json:"privateKey"`
	PublicKey  string `json:"publicKey"`
}

func New(dataDir string) (*Store, error) {
	root := filepath.Join(dataDir, "push-subscriptions")
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("create push subscriptions dir: %w", err)
	}
	return &Store{root: root, dataDir: dataDir}, nil
}

func (s *Store) List(ctx context.Context, email string) ([]servicepush.Subscription, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	record, err := s.read(email)
	if err != nil {
		return nil, err
	}
	return record.Subscriptions, nil
}

func (s *Store) Save(ctx context.Context, email string, subscription servicepush.Subscription) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	record, err := s.read(email)
	if err != nil {
		return err
	}
	record.Email = servicepush.NormalizeEmail(email)

	replaced := false
	for index, existing := range record.Subscriptions {
		if existing.Endpoint == subscription.Endpoint {
			record.Subscriptions[index] = subscription
			replaced = true
			break
		}
	}
	if !replaced {
		record.Subscriptions = append(record.Subscriptions, subscription)
	}
	sort.SliceStable(record.Subscriptions, func(i, j int) bool {
		return record.Subscriptions[i].CreatedAt < record.Subscriptions[j].CreatedAt
	})
	return s.write(email, record)
}

func (s *Store) Delete(ctx context.Context, email, endpoint string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	record, err := s.read(email)
	if err != nil {
		return err
	}
	kept := record.Subscriptions[:0]
	for _, subscription := range record.Subscriptions {
		if subscription.Endpoint != endpoint {
			kept = append(kept, subscription)
		}
	}
	if len(kept) == len(record.Subscriptions) {
		return nil
	}
	record.Subscriptions = kept

	if len(record.Subscriptions) == 0 {
		return s.removeUserRecord(email)
	}
	return s.write(email, record)
}

// DeleteAll removes every device registration for one account. The operation
// is idempotent so account-removal cleanup can be retried safely.
func (s *Store) DeleteAll(ctx context.Context, email string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.removeUserRecord(email)
}

func (s *Store) removeUserRecord(email string) error {
	path, err := s.path(email)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove push subscriptions: %w", err)
	}
	return nil
}

// VAPIDKeys loads the application server key pair, minting one on first use.
// Both halves are base64url; the public half is handed to browsers.
func (s *Store) VAPIDKeys(generate func() (private string, public string, err error)) (string, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := filepath.Join(s.dataDir, vapidFile)
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		var record vapidRecord
		if err := json.Unmarshal(data, &record); err != nil {
			return "", "", fmt.Errorf("parse vapid keys: %w", err)
		}
		if strings.TrimSpace(record.PrivateKey) == "" {
			return "", "", errors.New("vapid key file has no private key")
		}
		return record.PrivateKey, record.PublicKey, nil
	case !errors.Is(err, os.ErrNotExist):
		return "", "", fmt.Errorf("read vapid keys: %w", err)
	}

	private, public, err := generate()
	if err != nil {
		return "", "", err
	}
	encoded, err := json.MarshalIndent(vapidRecord{PrivateKey: private, PublicKey: public}, "", "  ")
	if err != nil {
		return "", "", fmt.Errorf("marshal vapid keys: %w", err)
	}
	if err := writeFileAtomic(s.dataDir, path, encoded); err != nil {
		return "", "", err
	}
	return private, public, nil
}

func (s *Store) read(email string) (userRecord, error) {
	path, err := s.path(email)
	if err != nil {
		return userRecord{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return userRecord{Email: servicepush.NormalizeEmail(email)}, nil
		}
		return userRecord{}, fmt.Errorf("read push subscriptions: %w", err)
	}
	var record userRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return userRecord{}, fmt.Errorf("parse push subscriptions: %w", err)
	}
	return record, nil
}

func (s *Store) write(email string, record userRecord) error {
	path, err := s.path(email)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal push subscriptions: %w", err)
	}
	if err := os.MkdirAll(s.root, 0o700); err != nil {
		return fmt.Errorf("create push subscriptions dir: %w", err)
	}
	return writeFileAtomic(s.root, path, data)
}

// path hashes the email so a directory listing does not enumerate the users of
// this server, matching the user-settings store.
func (s *Store) path(email string) (string, error) {
	normalized := servicepush.NormalizeEmail(email)
	if normalized == "" {
		return "", servicepush.ErrInvalidIdentity
	}
	sum := sha256.Sum256([]byte(normalized))
	return filepath.Join(s.root, "sha256-"+hex.EncodeToString(sum[:])+".json"), nil
}

func writeFileAtomic(dir, path string, data []byte) error {
	tmp, err := os.CreateTemp(dir, "push-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp push file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("secure temp push file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp push file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp push file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace push file: %w", err)
	}
	return nil
}
