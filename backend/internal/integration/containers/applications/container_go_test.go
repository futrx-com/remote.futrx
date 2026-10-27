package applications

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	configconstants "github.com/futrx-com/remote.futrx.com/internal/config/constants"
)

func TestContainerGoBootstrap(t *testing.T) {
	for _, tc := range []struct {
		name, mode, existing string
		wantCalls            int
		wantError            bool
	}{
		{"verified module ZIP", "valid", "", 1, false},
		{"connection failure stops without retry", "network", "old toolchain", 1, true},
		{"stalled connection stops without retry", "stall", "old toolchain", 1, true},
		{"corrupt download preserves old toolchain", "corrupt", "old toolchain", 1, true},
		{"interrupted publication restores old toolchain", "interrupt", "old toolchain", 1, true},
		{"matching toolchain needs no network", "valid", "go" + configconstants.ApplicationContainerGoVersion + " linux/amd64", 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, pins, environment, log := goBootstrapFixture(t, tc.mode)
			if tc.existing != "" {
				writeBootstrapExecutable(t, filepath.Join(root, "bin/go"), "#!/bin/sh\nprintf '%s\\n' '"+tc.existing+"'\n")
			}
			command := exec.Command("bash", "-e", "-u", "-o", "pipefail", "-s")
			command.Env = environment
			command.Stdin = strings.NewReader(containerGoBootstrap(root, pins))
			output, err := command.CombinedOutput()
			if (err != nil) != tc.wantError {
				t.Fatalf("bootstrap error=%v output=%s", err, output)
			}
			calls, _ := os.ReadFile(log)
			if got := strings.Count(string(calls), "https://"); got != tc.wantCalls {
				t.Fatalf("download attempts=%d want=%d; %s", got, tc.wantCalls, calls)
			}
			version, err := exec.Command(filepath.Join(root, "bin/go"), "version").CombinedOutput()
			if err != nil {
				t.Fatalf("installed toolchain is not executable: %v", err)
			}
			want := "go" + configconstants.ApplicationContainerGoVersion
			if tc.wantError {
				want = tc.existing
			}
			if !strings.Contains(string(version), want) {
				t.Fatalf("toolchain=%s want=%s", version, want)
			}
			stages, _ := filepath.Glob(filepath.Join(filepath.Dir(root), ".remote-go.*"))
			if len(stages) != 0 {
				t.Fatalf("bootstrap left staging directories: %v", stages)
			}
		})
	}
}

func goBootstrapFixture(t *testing.T, mode string) (string, map[string]string, []string, string) {
	t.Helper()
	for _, tool := range []string{"bash", "flock", "unzip", "sha256sum"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("shell regression requires %s", tool)
		}
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "tools")
	log := filepath.Join(dir, "curl.log")
	program := []byte("#!/bin/sh\necho 'go" + configconstants.ApplicationContainerGoVersion + " linux/amd64'\n")
	files := []string{"bin/go", "bin/gofmt", "pkg/tool/linux_amd64/compile"}
	var module bytes.Buffer
	zipper := zip.NewWriter(&module)
	for _, name := range files {
		header := &zip.FileHeader{Name: "golang.org/toolchain@v0.0.1-go" + configconstants.ApplicationContainerGoVersion + ".linux-amd64/" + name}
		header.SetMode(0644)
		file, err := zipper.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write(program); err != nil {
			t.Fatal(err)
		}
	}
	if err := zipper.Close(); err != nil {
		t.Fatal(err)
	}
	modulePath := filepath.Join(dir, "go.zip")
	if err := os.WriteFile(modulePath, module.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	pin := fmt.Sprintf("%x", sha256.Sum256(module.Bytes()))
	writeBootstrapExecutable(t, filepath.Join(bin, "dpkg"), "#!/bin/sh\necho amd64\n")
	if mode == "interrupt" {
		mv, err := exec.LookPath("mv")
		if err != nil {
			t.Fatal(err)
		}
		writeBootstrapExecutable(t, filepath.Join(bin, "mv"), "#!/bin/sh\ncase \"$1\" in */unpack/golang.org/*) kill -TERM \"$PPID\"; exit 143 ;; esac\nexec "+shellQuote(mv)+" \"$@\"\n")
	}
	writeBootstrapExecutable(t, filepath.Join(bin, "curl"), `#!/bin/bash
set -eu
destination=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -o) destination="$2"; shift 2 ;;
    https://*) url="$1"; shift ;;
    *) shift ;;
  esac
done
printf '%s\n' "$url" >> "$CURL_LOG"
case "$FIXTURE_MODE:$url" in
  network:*) printf 'partial' > "$destination"; echo 'curl: (56) Connection reset by peer' >&2; exit 56 ;;
  stall:*) echo 'curl: (28) Operation too slow' >&2; exit 28 ;;
  corrupt:*) printf 'corrupt' > "$destination"; exit 0 ;;
esac
case "$url" in
  https://proxy.golang.org/*) cp "$FIXTURE_ZIP" "$destination" ;;
  *) exit 1 ;;
esac
`)
	environment := append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "CURL_LOG="+log,
		"FIXTURE_MODE="+mode, "FIXTURE_ZIP="+modulePath)
	return filepath.Join(dir, "Go root's directory"), map[string]string{"amd64": pin, "arm64": pin}, environment, log
}

func writeBootstrapExecutable(t *testing.T, path, source string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(source), 0755); err != nil {
		t.Fatal(err)
	}
}
