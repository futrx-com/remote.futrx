package applications

import (
	"crypto/sha256"
	"errors"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	svc "github.com/futrx-com/remote.futrx.com/internal/service/applications"
)

const validBackendMain = `package main

func main() {}
`

func backendTree(files map[string]string) fs.FS {
	tree := fstest.MapFS{}
	for name, contents := range files {
		tree[name] = &fstest.MapFile{Data: []byte(contents)}
	}
	return tree
}

// The backend/ directory opts an application in, exactly as ui/ does, so an application
// with no directory and no declaration simply has no backend.
func TestLoadApplicationBackendIsOptional(t *testing.T) {
	backend, err := loadApplicationBackend(backendTree(nil), "backend", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if backend != nil {
		t.Errorf("backend = %+v, want nil", backend)
	}
}

func TestLoadApplicationBackendIsOptionalForContainerOnlySource(t *testing.T) {
	backend, err := loadApplicationBackend(backendTree(map[string]string{
		"backend/container/main.go": validBackendMain,
	}), "backend", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if backend != nil {
		t.Errorf("backend = %+v, want no host backend", backend)
	}
}

func TestLoadApplicationBackendDefaultsAreApplied(t *testing.T) {
	backend, err := loadApplicationBackend(
		backendTree(map[string]string{"backend/main.go": validBackendMain}), "backend", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if backend == nil {
		t.Fatal("backend = nil, want a descriptor")
	}
	if backend.Audience() != svc.BackendAccessRegistered {
		t.Errorf("access = %q, want registered by default", backend.Audience())
	}
	if backend.Timeout() != svc.DefaultBackendTimeoutMS {
		t.Errorf("timeout = %d, want the default", backend.Timeout())
	}
}

func TestLoadApplicationBackendKeepsDeclaredOverrides(t *testing.T) {
	declared := &svc.ApplicationBackend{Access: svc.BackendAccessAdmin, TimeoutMS: 500}
	backend, err := loadApplicationBackend(
		backendTree(map[string]string{"backend/main.go": validBackendMain}), "backend", declared)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if backend.Audience() != svc.BackendAccessAdmin || backend.Timeout() != 500 {
		t.Errorf("backend = %+v, want the declared values", backend)
	}
}

func TestLoadApplicationBackendAcceptsLegacyTimeoutAboveRuntimeMaximum(t *testing.T) {
	declaredTimeout := svc.MaxBackendTimeoutMS + 1
	declared := &svc.ApplicationBackend{TimeoutMS: declaredTimeout}
	backend, err := loadApplicationBackend(
		backendTree(map[string]string{"backend/main.go": validBackendMain}), "backend", declared)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if backend.TimeoutMS != declaredTimeout {
		t.Errorf("declared timeout = %d, want %d", backend.TimeoutMS, declaredTimeout)
	}
	if backend.Timeout() != svc.MaxBackendTimeoutMS {
		t.Errorf("effective timeout = %d, want runtime cap %d", backend.Timeout(), svc.MaxBackendTimeoutMS)
	}
}

// Every rejection here is a compiler error the server would otherwise hit at
// install time, on a machine where nobody is watching.
func TestLoadApplicationBackendRejectsBrokenLayouts(t *testing.T) {
	for _, tc := range []struct {
		name     string
		files    map[string]string
		declared *svc.ApplicationBackend
		contains string
	}{
		{
			name:     "declared but absent",
			files:    nil,
			declared: &svc.ApplicationBackend{},
			contains: "does not exist",
		},
		{
			name:     "no Go source",
			files:    map[string]string{"backend/README.md": "notes"},
			contains: "no package main",
		},
		{
			name:     "a library rather than a program",
			files:    map[string]string{"backend/helper.go": "package helper\n"},
			contains: "want main",
		},
		{
			name: "its own module file",
			files: map[string]string{
				"backend/main.go": validBackendMain,
				"backend/go.mod":  "module example.com/backend\n",
			},
			contains: "generates the backend module",
		},
		{
			name:     "unparseable source",
			files:    map[string]string{"backend/main.go": "package \n"},
			contains: "parse",
		},
		{
			name:     "an unknown access level",
			files:    map[string]string{"backend/main.go": validBackendMain},
			declared: &svc.ApplicationBackend{Access: "everyone"},
			contains: "invalid access",
		},
		{
			name:     "a negative timeout",
			files:    map[string]string{"backend/main.go": validBackendMain},
			declared: &svc.ApplicationBackend{TimeoutMS: -1},
			contains: "negative",
		},
		{
			name: "root entry point is not package main",
			files: map[string]string{
				"backend/api/main.go": validBackendMain,
				"backend/helper.go":   "package helper\n",
			},
			contains: "want main",
		},
		{
			name: "a module file beside backend/api",
			files: map[string]string{
				"backend/api/main.go": validBackendMain,
				"backend/go.mod":      "module example.com/backend\n",
			},
			contains: "generates the backend module",
		},
		{
			name: "a workspace file beside backend/api",
			files: map[string]string{
				"backend/api/main.go": validBackendMain,
				"backend/go.work":     "go 1.25\n",
			},
			contains: "generates the backend module",
		},
		{
			name: "backend/api carrying its own module file",
			files: map[string]string{
				"backend/api/main.go": validBackendMain,
				"backend/api/go.mod":  "module example.com/backend\n",
			},
			contains: "generates the backend module",
		},
		{
			name: "backend lifecycle carrying its own module file",
			files: map[string]string{
				"backend/api/main.go":         validBackendMain,
				"backend/lifecycle/events.go": "package lifecycle\n",
				"backend/lifecycle/go.mod":    "module example.com/lifecycle\n",
			},
			contains: "generates the backend module",
		},
		{
			name: "backend/api holding a library rather than a program",
			files: map[string]string{
				"backend/api/helper.go": "package helper\n",
			},
			contains: "want main",
		},
		{
			name: "backend/api with no Go source",
			files: map[string]string{
				"backend/api/README.md": "notes",
			},
			contains: "backend/api contains no package main",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := loadApplicationBackend(backendTree(tc.files), "backend", tc.declared)
			if err == nil {
				t.Fatal("want a validation error, got nil")
			}
			if !strings.Contains(err.Error(), tc.contains) {
				t.Errorf("error = %v, want it to mention %q", err, tc.contains)
			}
		})
	}
}

// Test files are the one kind of Go source a application may ship that is not
// package main: they never reach the generated build's package clause check
// through the compiler's eyes, and rejecting them would ban backend tests.
func TestLoadApplicationBackendAllowsTestFilesAndAssets(t *testing.T) {
	_, err := loadApplicationBackend(backendTree(map[string]string{
		"backend/main.go":           validBackendMain,
		"backend/main_test.go":      "package main\n",
		"backend/assets/schema.sql": "select 1;",
	}), "backend", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Backend source is compiled, never served. The registry hands out the whole
// subtree or nothing, and only for applications that actually declare a backend.
func TestRegistryBackendSource(t *testing.T) {
	r := testRegistry(t)
	source, ok := r.BackendSource(fixtureBackend)
	if !ok {
		t.Fatal("the fixture backend application exposes no backend source")
	}
	if _, err := fs.Stat(source, "main.go"); err != nil {
		t.Errorf("backend source is not rooted at backend/: %v", err)
	}
	for _, id := range []string{fixtureService, fixturePortless, fixtureUI, "no-such-application"} {
		if _, ok := r.BackendSource(id); ok {
			t.Errorf("%s reports backend source it does not have", id)
		}
	}
}

// The backend root is the host executable. api/ and lifecycle/ are importable
// host layers, while container source is built only inside the target container.
func TestLoadApplicationBackendAcceptsTheLayeredLayout(t *testing.T) {
	_, err := loadApplicationBackend(backendTree(map[string]string{
		"backend/main.go":                        validBackendMain,
		"backend/api/api.go":                     "package api\n",
		"backend/api/api_test.go":                "package api\n",
		"backend/lifecycle/events.go":            "package lifecycle\n",
		"backend/container/cmd/agent/main.go":    validBackendMain,
		"backend/container/internal/x/helper.go": "package x\n",
	}), "backend", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadApplicationBackendAcceptsLegacyAPIExecutable(t *testing.T) {
	_, err := loadApplicationBackend(backendTree(map[string]string{
		"backend/api/main.go":         validBackendMain,
		"backend/lifecycle/events.go": "package lifecycle\n",
	}), "backend", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Child packages belong to the same generated host module as the root
// executable. That lets API and lifecycle code have independent owners without
// creating extra processes.
func TestBackendSourceIncludesHostSiblingsAndExcludesContainerSource(t *testing.T) {
	files := backendTree(map[string]string{
		"applications/split/application.json":            `{ "name": "Split", "version": "1", "scopes": ["global"] }`,
		"applications/split/backend/main.go":             validBackendMain,
		"applications/split/backend/api/api.go":          "package api\n",
		"applications/split/backend/lifecycle/events.go": "package lifecycle\n",
		"applications/split/backend/container/main.go":   validBackendMain,
	})
	registry, err := NewRegistry(files, nil)
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	source, ok := registry.BackendSource("split")
	if !ok {
		t.Fatal("no host backend source")
	}
	for _, name := range []string{"main.go", "api/api.go", "lifecycle/events.go"} {
		if _, err := fs.Stat(source, name); err != nil {
			t.Errorf("host source is missing %s: %v", name, err)
		}
	}
	for _, name := range []string{"container", "container/main.go"} {
		if _, err := fs.Stat(source, name); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("container source %s is visible to host compiler: %v", name, err)
		}
	}
}

// The builder fingerprints every file the registry hands it. This pins the
// boundary at that handoff: editing a child host package must invalidate the
// binary, while editing container-only code must not.
func TestBackendSourceFingerprintInputsIncludeLifecycleNotContainer(t *testing.T) {
	fingerprintInput := func(lifecycle, container string) [sha256.Size]byte {
		t.Helper()
		files := backendTree(map[string]string{
			"applications/split/application.json":            `{ "name": "Split", "version": "1", "scopes": ["global"] }`,
			"applications/split/backend/main.go":             validBackendMain,
			"applications/split/backend/api/api.go":          "package api\n",
			"applications/split/backend/lifecycle/events.go": lifecycle,
			"applications/split/backend/container/main.go":   container,
		})
		registry, err := NewRegistry(files, nil)
		if err != nil {
			t.Fatalf("load catalog: %v", err)
		}
		source, ok := registry.BackendSource("split")
		if !ok {
			t.Fatal("no host backend source")
		}

		digest := sha256.New()
		err = fs.WalkDir(source, ".", func(name string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil || entry.IsDir() {
				return walkErr
			}
			contents, readErr := fs.ReadFile(source, name)
			if readErr != nil {
				return readErr
			}
			_, _ = digest.Write([]byte(name))
			_, _ = digest.Write(contents)
			return nil
		})
		if err != nil {
			t.Fatalf("read host source: %v", err)
		}
		var sum [sha256.Size]byte
		copy(sum[:], digest.Sum(nil))
		return sum
	}

	base := fingerprintInput("package lifecycle\nconst Event = 1\n", "package main\nconst Build = 1\n")
	if edited := fingerprintInput("package lifecycle\nconst Event = 2\n", "package main\nconst Build = 1\n"); edited == base {
		t.Fatal("editing lifecycle source did not change the builder's source inputs")
	}
	if edited := fingerprintInput("package lifecycle\nconst Event = 1\n", "package main\nconst Build = 2\n"); edited != base {
		t.Fatal("editing container source changed the builder's source inputs")
	}
}

// Root main is canonical; backend/api main remains the legacy fallback.
func TestResolveBackendSourceResolvesTheCompiledRoot(t *testing.T) {
	canonical := backendTree(map[string]string{
		"backend/main.go":    validBackendMain,
		"backend/api/api.go": "package api\n",
	})
	if got, ok := resolveBackendSource(canonical, "backend"); !ok || got != "backend" {
		t.Errorf("resolveBackendSource = %q, %t; want backend, true", got, ok)
	}
	legacy := backendTree(map[string]string{"backend/api/main.go": validBackendMain})
	if got, ok := resolveBackendSource(legacy, "backend"); !ok || got != "backend/api" {
		t.Errorf("resolveBackendSource = %q, %t; want backend/api, true", got, ok)
	}
}
