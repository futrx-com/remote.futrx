// Package fileversiontelemetry persists the pseudonymous installation ID and
// the last version-report attempt under DATA_DIR/telemetry.
package fileversiontelemetry

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	serviceversiontelemetry "github.com/futrx-com/remote.futrx.com/internal/service/versiontelemetry"
)

const (
	dirMode        = 0o700
	fileMode       = 0o600
	identityBytes  = 16
	identityLength = identityBytes * 2
)

var errInvalidInstallationID = errors.New("invalid installation identity")

var _ serviceversiontelemetry.Repository = (*Store)(nil)

type persistedAttempt struct {
	Version         string `json:"version"`
	LastAttemptUnix int64  `json:"lastAttemptUnix"`
}

type Store struct {
	identityPath string
	attemptPath  string
	random       io.Reader

	mu             sync.Mutex
	installationID string
}

func New(dataDir string) *Store {
	return newStore(dataDir, rand.Reader)
}

func newStore(dataDir string, random io.Reader) *Store {
	dir := filepath.Join(dataDir, "telemetry")
	return &Store{
		identityPath: filepath.Join(dir, "installation-id"),
		attemptPath:  filepath.Join(dir, "version-report-state.json"),
		random:       random,
	}
}

func (s *Store) InstallationID() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.installationID != "" {
		return s.installationID, nil
	}

	installationID, err := readInstallationID(s.identityPath)
	if err == nil {
		s.installationID = installationID
		return installationID, nil
	}
	if !errors.Is(err, os.ErrNotExist) && !errors.Is(err, errInvalidInstallationID) {
		return "", err
	}

	randomBytes := make([]byte, identityBytes)
	if _, err := io.ReadFull(s.random, randomBytes); err != nil {
		return "", err
	}
	installationID = hex.EncodeToString(randomBytes)
	if err := writePrivateFile(s.identityPath, []byte(installationID)); err != nil {
		return "", err
	}
	s.installationID = installationID
	return installationID, nil
}

func (s *Store) LastAttempt() (serviceversiontelemetry.Attempt, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	raw, err := os.ReadFile(s.attemptPath)
	if errors.Is(err, os.ErrNotExist) {
		return serviceversiontelemetry.Attempt{}, false, nil
	}
	if err != nil {
		return serviceversiontelemetry.Attempt{}, false, err
	}
	var persisted persistedAttempt
	if err := json.Unmarshal(raw, &persisted); err != nil || persisted.Version == "" || persisted.LastAttemptUnix <= 0 {
		return serviceversiontelemetry.Attempt{}, false, nil
	}
	return serviceversiontelemetry.Attempt{
		Version: persisted.Version,
		At:      time.Unix(persisted.LastAttemptUnix, 0),
	}, true, nil
}

func (s *Store) SaveAttempt(attempt serviceversiontelemetry.Attempt) error {
	encoded, err := json.Marshal(persistedAttempt{
		Version:         attempt.Version,
		LastAttemptUnix: attempt.At.Unix(),
	})
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return writePrivateFile(s.attemptPath, encoded)
}

func writePrivateFile(path string, content []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(dir, ".telemetry-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	if err := temporary.Chmod(fileMode); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(content); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func readInstallationID(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	installationID := string(raw)
	if len(installationID) != identityLength {
		return "", fmt.Errorf("%w: invalid length", errInvalidInstallationID)
	}
	for _, character := range installationID {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return "", fmt.Errorf("%w: invalid format", errInvalidInstallationID)
		}
	}
	return installationID, nil
}
