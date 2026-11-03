package browser

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBrowserHelperUsesManagedBrowser(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is required")
	}
	for _, tc := range []struct {
		name, version, command string
		wantOK                 bool
	}{
		{"patched headless", "156.0.8078.80", "screenshot", true},
		{"stale headless", "156.0.8078.4", "screenshot", false},
		{"stale CDP", "156.0.8078.4", "connect", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			runtime := filepath.Join(dir, "runtime")
			write := func(name, contents string, mode os.FileMode) {
				t.Helper()
				if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(name, []byte(contents), mode); err != nil {
					t.Fatal(err)
				}
			}
			write(filepath.Join(runtime, ".ready"), "", 0o644)
			write(filepath.Join(runtime, "chrome/chrome"), "#!/bin/sh\nprintf 'Google Chrome for Testing "+tc.version+"\\n'\n", 0o755)
			// The driver records the executable actually selected. No browser or
			// network is used; stale versions must fail before touching a page.
			driver := `const fs = require('fs');
const page = { goto: async () => {}, screenshot: async () => {}, close: async () => {} };
exports.chromium = {
  launch: async options => {
    fs.writeFileSync(process.env.TEST_LAUNCH, options.executablePath);
    return { newContext: async () => ({newPage: async () => page, close: async () => {}}), close: async () => {} };
  },
  connectOverCDP: async () => ({ version: () => '` + tc.version + `',
    close: async () => fs.writeFileSync(process.env.TEST_CLOSED, 'closed'),
    contexts: () => { throw new Error('must reject stale browser before reading contexts'); } })
};`
			write(filepath.Join(runtime, "node_modules/playwright/index.js"), driver, 0o644)
			renderer := newBrowserAssetRenderer()
			script := strings.ReplaceAll(renderer.render(string(browserScriptTemplate)), renderer.runtimeDir(), runtime)
			helper := filepath.Join(dir, "browser.mjs")
			write(helper, script, 0o644)
			config := filepath.Join(dir, "auth.json")
			write(config, "{}", 0o644)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			args := []string{helper, tc.command}
			if tc.command == "screenshot" {
				args = append(args, "https://example.invalid")
			}
			cmd := exec.CommandContext(ctx, "node", args...)
			launch := filepath.Join(dir, "launched")
			closed := filepath.Join(dir, "closed")
			cmd.Env = append(os.Environ(), "BROWSER_AUTH_CONFIG="+config, "BROWSER_OUT_DIR="+dir, "TEST_LAUNCH="+launch, "TEST_CLOSED="+closed)
			out, err := cmd.CombinedOutput()
			if (err == nil) != tc.wantOK {
				t.Fatalf("success=%v, want %v: %s", err == nil, tc.wantOK, out)
			}
			got, readErr := os.ReadFile(launch)
			if tc.wantOK && (readErr != nil || string(got) != filepath.Join(runtime, "chrome/chrome")) {
				t.Fatalf("wrong executable: %q (%v)", got, readErr)
			}
			if !tc.wantOK && !os.IsNotExist(readErr) {
				t.Fatal("stale browser was launched")
			}
			if tc.command == "connect" {
				if _, err := os.Stat(closed); err != nil {
					t.Fatal("stale CDP connection was not closed")
				}
			}
		})
	}
}
