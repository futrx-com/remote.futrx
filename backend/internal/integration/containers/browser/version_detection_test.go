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
		{name: "pinned Chrome for Testing", version: "Google Chrome for Testing 156.0.8078.80", wantOK: true},
		{name: "Chrome for Testing trailing whitespace", version: "Google Chrome for Testing 156.0.8078.80 \t", wantOK: true},
		{name: "Chromium ARM exact patch", version: "Chromium 156.0.8078.80", wantOK: true},
		{name: "Chromium ARM unpatched build", version: "Chromium 156.0.8078.0"},
		{name: "Chromium alternate patch and trailing whitespace", version: "Chromium 156.0.8078.79 \t"},
		{name: "Chrome for Testing stale patch", version: "Google Chrome for Testing 156.0.8078.73"},
		{name: "Chrome for Testing stale build", version: "Google Chrome for Testing 156.0.8077.80"},
		{name: "Chromium stale build", version: "Chromium 156.0.8077.0"},
		{name: "Chromium stale major", version: "Chromium 155.0.8078.0"},
		{name: "Chrome for Testing malformed separators", version: "Google Chrome for Testing 156x0x8078x80"},
		{name: "Chromium malformed separators", version: "Chromium 156x0x8078.0"},
		{name: "Chromium nonnumeric patch", version: "Chromium 156.0.8078.dev"},
		{name: "unsupported brand", version: "Google Chrome 156.0.8078.80"},
		{name: "extra version suffix", version: "Chromium 156.0.8078.0 unexpected"},
	}

	for _, script := range browserDetectionScripts(t) {
		t.Run(script.name, func(t *testing.T) {
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					fixture := newBrowserDetectionFixture(t)
					browserPath := fixture.browser(t, "", "chrome", tt.version, 0o755)
					fixture.check(t, script, tt.wantOK, browserPath)
				})
			}

			t.Run("missing browser", func(t *testing.T) {
				newBrowserDetectionFixture(t).check(t, script, false, "")
			})
			t.Run("browser is not executable", func(t *testing.T) {
				fixture := newBrowserDetectionFixture(t)
				fixture.browser(t, "", "chrome", "Google Chrome for Testing 156.0.8078.80", 0o644)
				fixture.check(t, script, false, "")
			})
			t.Run("ignore cached browsers and require managed runtime", func(t *testing.T) {
				fixture := newBrowserDetectionFixture(t)
				fixture.browser(t, "chromium-1000", "chrome-linux64", "Google Chrome for Testing 147.0.7778.96", 0o755)
				fixture.check(t, script, false, "")
			})
		})
	}
}

func TestBrowserCoreReadinessRequiresPatchedVersion(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is required")
	}
	script := string(renderedGUIUpScript())
	start := strings.Index(script, "core_ready() {")
	end := strings.Index(script[start:], "\nview_ready()") + start
	for _, tc := range []struct {
		name, response string
		ready          bool
	}{
		{"patched", `{"Browser":"Chrome/156.0.8078.80"}`, true},
		{"stale", `{"Browser":"Chrome/156.0.8078.4"}`, false},
		{"other service", `{}`, false},
		{"invalid response", `not json`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			curl := "#!/bin/sh\nprintf '%s' '" + tc.response + "'\n"
			if err := os.WriteFile(filepath.Join(dir, "curl"), []byte(curl), 0o755); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "sh", "-c", "CDP_PORT=9222\nEXPECTED_CHROME_VERSION=156.0.8078.80\n"+script[start:end]+"\ncore_ready")
			cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"))
			if out, err := cmd.CombinedOutput(); (err == nil) != tc.ready {
				t.Fatalf("ready=%v, want %v: %s", err == nil, tc.ready, out)
			}
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
		switch key {
		case "PW_CFT_VERSION":
			return "156.0.8078.80"
		case "PLAYWRIGHT_VERSION":
			return "1.64.1"
		case "PW_FFMPEG_BUILD":
			return "1013"
		default:
			return "unused-test-pin"
		}
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
			text:          "set -e\nPW_CFT_VERSION=156.0.8078.80\n" + install[smokeStart:smokeEnd],
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
	fixture := browserDetectionFixture{cache: filepath.Join(dir, "runtime"), bin: filepath.Join(dir, "bin")}
	for _, name := range []string{".ready", "node_modules/playwright/package.json", "browsers/ffmpeg-1013/ffmpeg-linux"} {
		path := filepath.Join(fixture.cache, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(""), 0o755); err != nil {
			t.Fatal(err)
		}
	}
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
	shellScript := strings.ReplaceAll(script.text, "/opt/remote-browser/pw-1.64.1-chrome-156.0.8078.80", f.cache)
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
