package updatecli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStartUpdaterSelectsReleaseScript(t *testing.T) {
	for _, test := range []struct {
		kind       string
		wantScript string
	}{
		{"application", "deploy-app"},
		{"infrastructure", "update"},
	} {
		t.Run(test.kind, func(t *testing.T) {
			installDir := t.TempDir()
			infraDir := filepath.Join(installDir, "infra")
			if err := os.Mkdir(infraDir, 0o755); err != nil {
				t.Fatal(err)
			}
			markerPath := filepath.Join(installDir, "selected")
			for _, script := range []string{"deploy-app", "update"} {
				contents := "#!/usr/bin/env bash\nprintf '%s:%s' '" + script + "' \"$1\" > \"$MARKER_PATH\"\n"
				if err := os.WriteFile(filepath.Join(infraDir, script+".sh"), []byte(contents), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("MARKER_PATH", markerPath)

			logPath := filepath.Join(installDir, "run.log")
			donePath := filepath.Join(installDir, "done.json")
			if _, err := (Client{}).StartUpdater(installDir, "0.4.2", test.kind, logPath, donePath); err != nil {
				t.Fatal(err)
			}
			waitForDone(t, donePath)

			selected, err := os.ReadFile(markerPath)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := string(selected), test.wantScript+":--ref=0.4.2"; got != want {
				t.Fatalf("selected script = %q, want %q", got, want)
			}
		})
	}
}

func TestStartUpdaterRejectsUnknownKind(t *testing.T) {
	dir := t.TempDir()
	if _, err := (Client{}).StartUpdater(dir, "0.4.2", "surprise", filepath.Join(dir, "log"), filepath.Join(dir, "done")); err == nil {
		t.Fatal("StartUpdater accepted an unknown update kind")
	}
}

func waitForDone(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			var done struct {
				ExitCode int `json:"exitCode"`
			}
			if err := json.Unmarshal(data, &done); err != nil {
				t.Fatal(err)
			}
			if done.ExitCode != 0 {
				t.Fatalf("updater exited with %d", done.ExitCode)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}
