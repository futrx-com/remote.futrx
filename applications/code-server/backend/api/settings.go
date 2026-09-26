package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/futrx-com/remote.futrx.com/pkg/applications"
)

// settingsStore owns the installed instance's settings lifecycle and the
// container boundary. The API serializes calls to Read and Save.
type settingsStore struct {
	instance applications.Instance
	read     func(string) ([]byte, error)
	write    func(string, []byte) error
}

func newSettingsStore() *settingsStore {
	return &settingsStore{read: readSettings, write: writeSettings}
}

func (s *settingsStore) Init(instance applications.Instance) error {
	if instance.Scope != "project" || instance.ContainerName == "" || instance.DataDir == "" {
		return fmt.Errorf("Code Server requires a project container and backend data directory")
	}
	s.instance = instance
	marker := filepath.Join(instance.DataDir, "settings-initialized")
	if _, err := os.Stat(marker); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect Code Server settings state: %w", err)
	}
	// A new install gets a new DataDir. Its saved install form must replace any
	// settings left in the durable workspace by a previous uninstalled copy.
	initial, err := validateSettings([]byte(instance.Env["CODE_SERVER_SETTINGS_JSON"]))
	if err != nil {
		return fmt.Errorf("initial Code Server settings: %w", err)
	}
	if err := s.write(instance.ContainerName, initial); err != nil {
		return fmt.Errorf("initialize Code Server settings: %w", err)
	}
	if err := os.WriteFile(marker, []byte("1\n"), 0o600); err != nil {
		return fmt.Errorf("record Code Server settings initialization: %w", err)
	}
	return nil
}

type invalidStoredSettingsError struct{ cause error }

func (e *invalidStoredSettingsError) Error() string { return e.cause.Error() }

type invalidInputSettingsError struct{ cause error }

func (e *invalidInputSettingsError) Error() string { return e.cause.Error() }

func (s *settingsStore) Read() ([]byte, error) {
	settings, err := s.read(s.instance.ContainerName)
	if err != nil {
		return nil, err
	}
	if _, err := validateSettings(settings); err != nil {
		return nil, &invalidStoredSettingsError{cause: err}
	}
	return settings, nil
}

func (s *settingsStore) Save(content []byte) ([]byte, error) {
	settings, err := validateSettings(content)
	if err != nil {
		return nil, &invalidInputSettingsError{cause: err}
	}
	if err := s.write(s.instance.ContainerName, settings); err != nil {
		return nil, err
	}
	return settings, nil
}

func validateSettings(content []byte) ([]byte, error) {
	if len(content) > maxSettingsBytes {
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
