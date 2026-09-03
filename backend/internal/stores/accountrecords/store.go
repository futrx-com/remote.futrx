// Package accountrecords persists one typed JSON record per account using a
// normalized, hashed email as the filename.
package accountrecords

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Store owns the common file lifecycle for account-scoped records. The
// directory and record names keep capability-specific paths and error text
// intact while centralizing the persistence mechanics.
type Store[T any] struct {
	root        string
	directory   string
	recordName  string
	tempPattern string
	mu          sync.Mutex
}

func New[T any](dataDir, directory, recordName string) (*Store[T], error) {
	root := filepath.Join(dataDir, directory)
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("create %s dir: %w", directory, err)
	}
	return &Store[T]{
		root:        root,
		directory:   directory,
		recordName:  recordName,
		tempPattern: directory + "-*.tmp",
	}, nil
}

func (s *Store[T]) Get(ctx context.Context, email string) (*T, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	path, err := s.path(email)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s: %w", s.recordName, err)
	}
	var record T
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, fmt.Errorf("parse %s: %w", s.recordName, err)
	}
	return &record, nil
}

func (s *Store[T]) Save(ctx context.Context, email string, record T) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	path, err := s.path(email)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", s.recordName, err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.root, 0o700); err != nil {
		return fmt.Errorf("create %s dir: %w", s.directory, err)
	}
	tmp, err := os.CreateTemp(s.root, s.tempPattern)
	if err != nil {
		return fmt.Errorf("create temp %s: %w", s.recordName, err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod temp %s: %w", s.recordName, err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp %s: %w", s.recordName, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp %s: %w", s.recordName, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace %s: %w", s.recordName, err)
	}
	return nil
}

func (s *Store[T]) Delete(ctx context.Context, email string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	path, err := s.path(email)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete %s: %w", s.recordName, err)
	}
	return nil
}

func (s *Store[T]) path(email string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(email))
	if value == "" {
		return "", errors.New("email is required")
	}
	sum := sha256.Sum256([]byte(value))
	return filepath.Join(s.root, hex.EncodeToString(sum[:])+".json"), nil
}
