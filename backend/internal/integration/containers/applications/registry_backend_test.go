package applications

import (
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	svc "github.com/futrx-com/remote.futrx.com/internal/service/applications"
)

const validPluginMain = `package main

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

func TestLoadApplicationBackendDefaultsAreApplied(t *testing.T) {
	backend, err := loadApplicationBackend(
		backendTree(map[string]string{"backend/main.go": validPluginMain}), "backend", nil)
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
		backendTree(map[string]string{"backend/main.go": validPluginMain}), "backend", declared)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if backend.Audience() != svc.BackendAccessAdmin || backend.Timeout() != 500 {
		t.Errorf("backend = %+v, want the declared values", backend)
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
				"backend/main.go": validPluginMain,
				"backend/go.mod":  "module example.com/backend\n",
			},
			contains: "generates the plugin module",
		},
		{
			name:     "unparseable source",
			files:    map[string]string{"backend/main.go": "package \n"},
			contains: "parse",
		},
		{
			name:     "an unknown access level",
			files:    map[string]string{"backend/main.go": validPluginMain},
			declared: &svc.ApplicationBackend{Access: "everyone"},
			contains: "invalid access",
		},
		{
			name:     "a negative timeout",
			files:    map[string]string{"backend/main.go": validPluginMain},
			declared: &svc.ApplicationBackend{TimeoutMS: -1},
			contains: "negative",
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

// Test files are the one kind of Go source a plugin may ship that is not
// package main: they never reach the generated build's package clause check
// through the compiler's eyes, and rejecting them would ban plugin tests.
func TestLoadApplicationBackendAllowsTestFilesAndAssets(t *testing.T) {
	_, err := loadApplicationBackend(backendTree(map[string]string{
		"backend/main.go":           validPluginMain,
		"backend/main_test.go":      "package main\n",
		"backend/assets/schema.sql": "select 1;",
	}), "backend", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Plugin source is compiled, never served. The registry hands out the whole
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
	for _, id := range []string{fixtureService, fixtureTool, "no-such-application"} {
		if _, ok := r.BackendSource(id); ok {
			t.Errorf("%s reports backend source it does not have", id)
		}
	}
}
