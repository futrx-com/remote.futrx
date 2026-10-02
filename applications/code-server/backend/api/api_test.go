package api

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"futrx.local/catalog/applications/code-server/backend/settings"

	"github.com/futrx-com/remote.futrx.com/pkg/applications"
)

func testInstance(t *testing.T, dir string) applications.Instance {
	t.Helper()
	return applications.Instance{
		Scope: "project", ContainerName: "project-test", DataDir: dir,
		Env: map[string]string{"CODE_SERVER_SETTINGS_JSON": `{"editor.fontSize": 14}`},
	}
}

func TestSettingsBackendPreservesExistingAndSavesValidatedSettings(t *testing.T) {
	dir := t.TempDir()
	stored := []byte(`{"editor.fontSize": 22}`)
	read := func(name string) ([]byte, error) { return stored, nil }
	write := func(name string, content []byte) error {
		if name != "project-test" {
			t.Fatalf("unexpected container %q", name)
		}
		stored = append([]byte(nil), content...)
		return nil
	}
	b := New(settings.New(read, write))
	instance := testInstance(t, dir)
	if err := b.Init(instance); err != nil {
		t.Fatal(err)
	}
	if string(stored) != `{"editor.fontSize": 22}` {
		t.Fatalf("backend initialization overwrote editor settings: %s", stored)
	}

	response, err := b.Handle(applications.Request{Method: "POST", Path: "settings", Body: []byte(`{"settings":"{\"editor.fontSize\": 18}"}`)})
	if err != nil || response.Status != 200 {
		t.Fatalf("save: %+v, %v", response, err)
	}
	if !strings.Contains(string(stored), `"editor.fontSize": 18`) {
		t.Fatalf("saved settings: %s", stored)
	}

	response, err = b.Handle(applications.Request{Method: "GET", Path: "settings"})
	if err != nil || response.Status != 200 {
		t.Fatalf("read: %+v, %v", response, err)
	}
	var result map[string]string
	if err := json.Unmarshal(response.Body, &result); err != nil {
		t.Fatal(err)
	}
	if result["settings"] != string(stored) {
		t.Fatalf("read = %q, want %q", result["settings"], stored)
	}

	// Restarts and container replacement keep the backend DataDir. A new
	// backend must not roll edited settings back to the original install env.
	restarted := New(settings.New(read, func(string, []byte) error { t.Fatal("initial settings overwritten on restart"); return nil }))
	if err := restarted.Init(instance); err != nil {
		t.Fatal(err)
	}
}

func TestSettingsBackendRejectsInvalidAndFailedWrites(t *testing.T) {
	writes := 0
	write := func(string, []byte) error { writes++; return errors.New("container unavailable") }
	b := New(settings.New(nil, write))
	if err := b.Init(testInstance(t, t.TempDir())); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{`{"settings":"[]"}`, `{"settings":"null"}`, `{"settings":"{"}`} {
		response, err := b.Handle(applications.Request{Method: "POST", Path: "settings", Body: []byte(value)})
		if err != nil || response.Status != 400 {
			t.Fatalf("invalid %q: %+v, %v", value, response, err)
		}
	}
	if writes != 0 {
		t.Fatalf("invalid settings caused %d writes", writes)
	}
	response, err := b.Handle(applications.Request{Method: "POST", Path: "settings", Body: []byte(`{"settings":"{}"}`)})
	if err != nil || response.Status != 502 {
		t.Fatalf("write failure: %+v, %v", response, err)
	}
}
