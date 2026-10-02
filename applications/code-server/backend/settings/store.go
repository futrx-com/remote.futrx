package settings

import (
	"bytes"
	"encoding/json"
	"fmt"

	"futrx.local/catalog/applications/code-server/backend/config"

	"github.com/futrx-com/remote.futrx.com/pkg/applications"
)

// Store validates settings and tracks the installed instance. Container I/O is
// supplied by the composition root; the API serializes calls to Read and Save.
type Store struct {
	instance applications.Instance
	read     func(string) ([]byte, error)
	write    func(string, []byte) error
}

func New(read func(string) ([]byte, error), write func(string, []byte) error) *Store {
	return &Store{read: read, write: write}
}

func (s *Store) Init(instance applications.Instance) error {
	if instance.Scope != "project" || instance.ContainerName == "" || instance.DataDir == "" {
		return fmt.Errorf("Code Server requires a project container and backend data directory")
	}
	s.instance = instance
	// Provisioning seeds settings once. Backend startup must never overwrite
	// editor changes with the original install form.
	return nil
}

type InvalidStoredError struct{ cause error }

func (e *InvalidStoredError) Error() string { return e.cause.Error() }

type InvalidInputError struct{ cause error }

func (e *InvalidInputError) Error() string { return e.cause.Error() }

func (s *Store) Read() ([]byte, error) {
	settings, err := s.read(s.instance.ContainerName)
	if err != nil {
		return nil, err
	}
	if _, err := validateSettings(settings); err != nil {
		return nil, &InvalidStoredError{cause: err}
	}
	return settings, nil
}

func (s *Store) Save(content []byte) ([]byte, error) {
	settings, err := validateSettings(content)
	if err != nil {
		return nil, &InvalidInputError{cause: err}
	}
	if err := s.write(s.instance.ContainerName, settings); err != nil {
		return nil, err
	}
	return settings, nil
}

func validateSettings(content []byte) ([]byte, error) {
	if len(content) > config.MaxSettingsBytes {
		return nil, fmt.Errorf("settings exceed 128 KiB")
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(content, &object); err != nil || object == nil {
		return nil, fmt.Errorf("settings must be a JSON object")
	}
	var formatted bytes.Buffer
	if err := json.Indent(&formatted, content, "", "  "); err != nil {
		return nil, fmt.Errorf("format settings: %w", err)
	}
	formatted.WriteByte('\n')
	return formatted.Bytes(), nil
}
