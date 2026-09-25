package applications

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/futrx-com/remote.futrx.com/pkg/applications"
)

func TestBackendEntryPrefersTheRootAndKeepsLegacyAPIFallback(t *testing.T) {
	tests := []struct {
		name  string
		files fstest.MapFS
		want  string
	}{
		{
			name: "canonical root",
			files: fstest.MapFS{
				"main.go":    {Data: []byte("package main")},
				"api/api.go": {Data: []byte("package api")},
			},
			want: ".",
		},
		{
			name: "legacy api executable",
			files: fstest.MapFS{
				"api/main.go": {Data: []byte("package main")},
			},
			want: "./api",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := backendEntry(test.files); got != test.want {
				t.Fatalf("backendEntry() = %q, want %q", got, test.want)
			}
		})
	}
}

// The fallback is used when build info is unavailable, which is exactly the
// case under `go test` — so a stale constant would be invisible until a backend
// failed to build on a server. Pin it to the server's own requirement.
func TestGoPluginFallbackMatchesGoMod(t *testing.T) {
	raw, err := os.ReadFile("../../../go.mod")
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	pattern := regexp.MustCompile(
		`(?m)^\s*` + regexp.QuoteMeta(goPluginModule) + `\s+(v\S+)`)
	match := pattern.FindSubmatch(raw)
	if match == nil {
		t.Fatalf("%s is not required by the server's go.mod", goPluginModule)
	}
	if got := string(match[1]); got != goPluginFallbackVersion {
		t.Errorf("goPluginFallbackVersion = %s, go.mod requires %s", goPluginFallbackVersion, got)
	}
}

// The generated module files are part of a backend's build fingerprint, so an
// unstable rendering would invalidate every cached binary on every start.
func TestGeneratedModuleFilesAreStable(t *testing.T) {
	builder := NewBuilder(t.TempDir(), "")
	for name, render := range map[string]func() string{
		"backend": func() string { return builder.backendModuleFile("example") },
		"sdk":     builder.sdkModuleFile,
	} {
		first, second := render(), render()
		if first != second {
			t.Errorf("%s go.mod is not stable:\n%s\n---\n%s", name, first, second)
		}
		if !strings.Contains(first, goPluginModule) {
			t.Errorf("%s go.mod does not pin %s:\n%s", name, goPluginModule, first)
		}
	}
	backend := builder.backendModuleFile("example")
	// The replace is what keeps the SDK's canonical import path working, so
	// the same backend source compiles in a checkout and in a build directory.
	if !strings.Contains(backend, "replace "+applications.ModulePath+" => ../sdk") {
		t.Errorf("backend go.mod does not replace the SDK:\n%s", backend)
	}
	if strings.Count(backend, applications.ModulePath+" v") > 1 {
		t.Errorf("the SDK is required more than once:\n%s", backend)
	}
	if !strings.Contains(backend, "module futrx.local/catalog/applications/example/backend") {
		t.Errorf("backend go.mod does not match the catalog package path:\n%s", backend)
	}
}

func TestBackendBuildDisablesExternalGoWorkspaces(t *testing.T) {
	environment := goEnv(t.TempDir(), false)
	for index := len(environment) - 1; index >= 0; index-- {
		if strings.HasPrefix(environment[index], "GOWORK=") {
			if environment[index] != "GOWORK=off" {
				t.Fatalf("GOWORK = %q, want off", environment[index])
			}
			return
		}
	}
	t.Fatal("backend build environment does not set GOWORK")
}

// Fingerprints must change when any input does, and only then: a collision is
// a stale binary someone has to diagnose, and needless churn is a rebuild on
// every start.
func TestFingerprintCoversEveryInput(t *testing.T) {
	base := []sourceFile{{path: "main.go", data: []byte("package main")}}
	sdk := []sourceFile{{path: "contract.go", data: []byte("package applications")}}
	reference := fingerprintOf(base, sdk, "module a", "module b", "1.25.0")

	if again := fingerprintOf(base, sdk, "module a", "module b", "1.25.0"); again != reference {
		t.Error("identical inputs produced different fingerprints")
	}
	for name, changed := range map[string]string{
		"backend source": fingerprintOf(
			[]sourceFile{{path: "main.go", data: []byte("package main // edited")}},
			sdk, "module a", "module b", "1.25.0"),
		"backend file name": fingerprintOf(
			[]sourceFile{{path: "other.go", data: []byte("package main")}},
			sdk, "module a", "module b", "1.25.0"),
		"sdk source":     fingerprintOf(base, []sourceFile{{path: "contract.go", data: []byte("x")}}, "module a", "module b", "1.25.0"),
		"backend go.mod": fingerprintOf(base, sdk, "module a2", "module b", "1.25.0"),
		"sdk go.mod":     fingerprintOf(base, sdk, "module a", "module b2", "1.25.0"),
		"go version":     fingerprintOf(base, sdk, "module a", "module b", "1.26.0"),
	} {
		if changed == reference {
			t.Errorf("changing the %s did not change the fingerprint", name)
		}
	}

	// Length prefixing is what stops "ab" + "c" hashing as "a" + "bc".
	split := fingerprintOf(
		[]sourceFile{{path: "ma", data: []byte("in.go")}}, sdk, "module a", "module b", "1.25.0")
	joined := fingerprintOf(
		[]sourceFile{{path: "m", data: []byte("ain.go")}}, sdk, "module a", "module b", "1.25.0")
	if split == joined {
		t.Error("fingerprint inputs are not length-prefixed")
	}
}

// Pruning an application's old binaries must not reach into another application's. Application
// ids may contain a dash, so "s3" and "s3-disk" both produce names starting
// "s3-" — and deleting a live binary out from under a running application would take
// it down until something rebuilt it.
func TestPruneStaleLeavesAnotherApplicationAlone(t *testing.T) {
	builder := NewBuilder(t.TempDir(), "")
	if err := os.MkdirAll(builder.binaryDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	names := []string{
		"s3-aaaaaaaaaaaaaaaa",      // this application, current
		"s3-bbbbbbbbbbbbbbbb",      // this application, stale
		"s3-disk-cccccccccccccccc", // a different application entirely
		"s3-disk-dddddddddddddddd.tmp",
	}
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(builder.binaryDir(), name), nil, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	builder.pruneStale("s3", "s3-aaaaaaaaaaaaaaaa")

	for name, want := range map[string]bool{
		"s3-aaaaaaaaaaaaaaaa":          true,
		"s3-bbbbbbbbbbbbbbbb":          false,
		"s3-disk-cccccccccccccccc":     true,
		"s3-disk-dddddddddddddddd.tmp": true,
	} {
		_, err := os.Stat(filepath.Join(builder.binaryDir(), name))
		if got := err == nil; got != want {
			t.Errorf("%s exists = %v, want %v", name, got, want)
		}
	}
}
