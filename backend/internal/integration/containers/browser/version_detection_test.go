package browser

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBrowserVersionDetection(t *testing.T) {
	tests := []struct {
		name    string
		version string
		wantOK  bool
	}{
		{name: "pinned Chrome for Testing", version: "Google Chrome for Testing 148.0.7778.96", wantOK: true},
		{name: "Chrome for Testing trailing whitespace", version: "Google Chrome for Testing 148.0.7778.96 \t", wantOK: true},
		{name: "Chromium ARM patch", version: "Chromium 148.0.7778.0", wantOK: true},
		{name: "Chromium alternate patch and trailing whitespace", version: "Chromium 148.0.7778.123 \t", wantOK: true},
		{name: "Chrome for Testing stale patch", version: "Google Chrome for Testing 148.0.7778.95"},
		{name: "Chrome for Testing stale build", version: "Google Chrome for Testing 148.0.7777.96"},
		{name: "Chromium stale build", version: "Chromium 148.0.7777.0"},
		{name: "Chromium stale major", version: "Chromium 147.0.7778.0"},
		{name: "Chrome for Testing malformed separators", version: "Google Chrome for Testing 148x0x7778x96"},
		{name: "Chromium malformed separators", version: "Chromium 148x0x7778.0"},
		{name: "Chromium nonnumeric patch", version: "Chromium 148.0.7778.dev"},
		{name: "unsupported brand", version: "Google Chrome 148.0.7778.96"},
		{name: "extra version suffix", version: "Chromium 148.0.7778.0 unexpected"},
	}

	for _, script := range browserDetectionScripts(t) {
		t.Run(script.name, func(t *testing.T) {
			for _, layout := range []string{"chrome-linux64", "chrome-linux"} {
				t.Run(layout, func(t *testing.T) {
					for _, tt := range tests {
						t.Run(tt.name, func(t *testing.T) {
							fixture := newBrowserDetectionFixture(t)
							browserPath := fixture.browser(t, "chromium-1223", layout, tt.version, 0o755)
							fixture.check(t, script, tt.wantOK, browserPath)
						})
					}
				})
			}
			t.Run("missing browser", func(t *testing.T) {
				newBrowserDetectionFixture(t).check(t, script, false, "")
			})
			t.Run("browser is not executable", func(t *testing.T) {
				fixture := newBrowserDetectionFixture(t)
				fixture.browser(t, "chromium-1223", "chrome-linux64", "Google Chrome for Testing 148.0.7778.96", 0o644)
				fixture.check(t, script, false, "")
			})
			t.Run("skip stale cache and select compatible browser", func(t *testing.T) {
				fixture := newBrowserDetectionFixture(t)
				fixture.browser(t, "chromium-1000", "chrome-linux64", "Google Chrome for Testing 147.0.7778.96", 0o755)
				browserPath := fixture.browser(t, "chromium-1223", "chrome-linux", "Chromium 148.0.7778.0 ", 0o755)
				fixture.check(t, script, true, browserPath)
			})
		})
	}
}

type browserDetectionScript struct {
	name          string
	text          string
	printsBrowser bool
}

func browserDetectionScripts(t *testing.T) []browserDetectionScript {
	t.Helper()
	renderer := browserAssetRenderer{pin: func(key string) string {
		if key == "PW_CFT_VERSION" {
			return "148.0.7778.96"
		}
		return "unused-test-pin"
	}}

	// Execute the selection steps emitted by the real renderer without running
	// package installation, starting the GUI, or touching the real browser cache.
	install := renderer.installScript()
	smokeStart := strings.Index(install, "# Sanity check")
	smokeEnd := strings.Index(install, "# A present executable")
	if smokeStart < 0 || smokeEnd <= smokeStart {
		t.Fatal("cannot locate browser selection in rendered installation script")
	}
	launcher := string(renderer.launcherScript())
	launcherEnd := strings.Index(launcher, "\nlog()")
	if launcherEnd < 0 {
		t.Fatal("cannot locate browser selection in rendered GUI launcher")
	}
	return []browserDetectionScript{
		{
			name:          "installation smoke test",
			text:          "set -e\nPW_CFT_VERSION=148.0.7778.96\n" + install[smokeStart:smokeEnd],
			printsBrowser: true,
		},
		{name: "GUI launcher", text: launcher[:launcherEnd], printsBrowser: true},
		{name: "provisioning stack check", text: renderer.stackCheck()},
	}
}

type browserDetectionFixture struct {
	cache string
	bin   string
}

func newBrowserDetectionFixture(t *testing.T) browserDetectionFixture {
	t.Helper()
	dir := t.TempDir()
	fixture := browserDetectionFixture{cache: filepath.Join(dir, "cache"), bin: filepath.Join(dir, "bin")}
	if err := os.MkdirAll(fixture.bin, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"Xvfb", "x11vnc", "websockify", "openbox", "xdotool", "setsid"} {
		if err := os.WriteFile(filepath.Join(fixture.bin, tool), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return fixture
}

func (f browserDetectionFixture) browser(t *testing.T, revision, layout, version string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(f.cache, revision, layout, "chrome")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	// Versions are literal fixture data; quoting preserves the trailing spaces
	// that Chrome emits and that originally escaped the string-based tests.
	script := "#!/bin/sh\n[ \"$1\" = --version ] || exit 2\nprintf '%s\\n' '" + strings.ReplaceAll(version, "'", "'\"'\"'") + "'\n"
	if err := os.WriteFile(path, []byte(script), mode); err != nil {
		t.Fatal(err)
	}
	return path
}

func (f browserDetectionFixture) check(t *testing.T, script browserDetectionScript, wantOK bool, browserPath string) {
	t.Helper()
	shellScript := strings.ReplaceAll(script.text, "/root/.cache/ms-playwright", f.cache)
	if script.printsBrowser {
		shellScript += "\nprintf 'selected=%s\\n' \"$CHROME\"\n"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", shellScript)
	cmd.Env = append(os.Environ(), "PATH="+f.bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("browser selection timed out: %v\n%s", ctx.Err(), out)
	}
	if (err == nil) != wantOK {
		t.Fatalf("browser selection success = %v, want %v; error: %v\n%s", err == nil, wantOK, err, out)
	}
	if wantOK && script.printsBrowser && !strings.Contains(string(out), fmt.Sprintf("selected=%s\n", browserPath)) {
		t.Fatalf("browser selection did not select %q:\n%s", browserPath, out)
	}
}
