package fileversiontelemetry

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	serviceversiontelemetry "github.com/futrx-com/remote.futrx.com/internal/service/versiontelemetry"
)

func TestInstallationIDPersistsAcrossStoreInstances(t *testing.T) {
	dataDir := t.TempDir()
	first := newStore(dataDir, bytes.NewReader(bytes.Repeat([]byte{0xab}, identityBytes)))
	firstID, err := first.InstallationID()
	if err != nil {
		t.Fatal(err)
	}
	second := newStore(dataDir, bytes.NewReader(bytes.Repeat([]byte{0xcd}, identityBytes)))
	secondID, err := second.InstallationID()
	if err != nil {
		t.Fatal(err)
	}

	wantID := strings.Repeat("ab", identityBytes)
	if firstID != wantID || secondID != wantID {
		t.Fatalf("installation IDs = %q, %q; want %q", firstID, secondID, wantID)
	}
	assertPrivateFile(t, filepath.Join(dataDir, "telemetry", "installation-id"))
}

func TestInstallationIDReplacesInvalidFile(t *testing.T) {
	for _, existing := range []string{"abc123", strings.Repeat("G", identityLength)} {
		t.Run(existing, func(t *testing.T) {
			dataDir := t.TempDir()
			path := filepath.Join(dataDir, "telemetry", "installation-id")
			if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(existing), fileMode); err != nil {
				t.Fatal(err)
			}

			store := newStore(dataDir, bytes.NewReader(bytes.Repeat([]byte{0xcd}, identityBytes)))
			got, err := store.InstallationID()
			if err != nil {
				t.Fatal(err)
			}
			want := strings.Repeat("cd", identityBytes)
			if got != want {
				t.Fatalf("installation ID = %q, want %q", got, want)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(raw) != want {
				t.Fatalf("persisted installation ID = %q, want %q", raw, want)
			}
			assertPrivateFile(t, path)
		})
	}
}

func TestAttemptPersistsAcrossStoreInstances(t *testing.T) {
	dataDir := t.TempDir()
	want := serviceversiontelemetry.Attempt{
		Version: "0.21.0",
		At:      time.Date(2026, time.September, 27, 12, 0, 0, 0, time.UTC),
	}
	if err := New(dataDir).SaveAttempt(want); err != nil {
		t.Fatal(err)
	}

	got, found, err := New(dataDir).LastAttempt()
	if err != nil {
		t.Fatal(err)
	}
	if !found || got.Version != want.Version || !got.At.Equal(want.At) {
		t.Fatalf("attempt = %+v, found = %t; want %+v", got, found, want)
	}
	assertPrivateFile(t, filepath.Join(dataDir, "telemetry", "version-report-state.json"))
}

func TestMalformedAttemptIsTreatedAsMissing(t *testing.T) {
	dataDir := t.TempDir()
	path := filepath.Join(dataDir, "telemetry", "version-report-state.json")
	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"version":""}`), fileMode); err != nil {
		t.Fatal(err)
	}

	got, found, err := New(dataDir).LastAttempt()
	if err != nil {
		t.Fatal(err)
	}
	if found || got != (serviceversiontelemetry.Attempt{}) {
		t.Fatalf("attempt = %+v, found = %t; want missing", got, found)
	}
}

func assertPrivateFile(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != fileMode {
		t.Fatalf("%s mode = %o, want %o", path, got, fileMode)
	}
}
