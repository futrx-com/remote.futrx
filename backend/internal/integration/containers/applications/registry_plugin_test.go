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

func pluginTree(files map[string]string) fs.FS {
	tree := fstest.MapFS{}
	for name, contents := range files {
		tree[name] = &fstest.MapFile{Data: []byte(contents)}
	}
	return tree
}

// The plugin/ directory opts an image in, exactly as ui/ does, so an image
// with no directory and no declaration simply has no backend.
func TestLoadImagePluginIsOptional(t *testing.T) {
	backend, err := loadImagePlugin(pluginTree(nil), "plugin", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if backend != nil {
		t.Errorf("backend = %+v, want nil", backend)
	}
}

func TestLoadImagePluginDefaultsAreApplied(t *testing.T) {
	backend, err := loadImagePlugin(
		pluginTree(map[string]string{"plugin/main.go": validPluginMain}), "plugin", nil)
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

func TestLoadImagePluginKeepsDeclaredOverrides(t *testing.T) {
	declared := &svc.ImageBackend{Access: svc.BackendAccessAdmin, TimeoutMS: 500}
	backend, err := loadImagePlugin(
		pluginTree(map[string]string{"plugin/main.go": validPluginMain}), "plugin", declared)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if backend.Audience() != svc.BackendAccessAdmin || backend.Timeout() != 500 {
		t.Errorf("backend = %+v, want the declared values", backend)
	}
}

// Every rejection here is a compiler error the server would otherwise hit at
// install time, on a machine where nobody is watching.
func TestLoadImagePluginRejectsBrokenLayouts(t *testing.T) {
	for _, tc := range []struct {
		name     string
		files    map[string]string
		declared *svc.ImageBackend
		contains string
	}{
		{
			name:     "declared but absent",
			files:    nil,
			declared: &svc.ImageBackend{},
			contains: "does not exist",
		},
		{
			name:     "no Go source",
			files:    map[string]string{"plugin/README.md": "notes"},
			contains: "no package main",
		},
		{
			name:     "a library rather than a program",
			files:    map[string]string{"plugin/helper.go": "package helper\n"},
			contains: "want main",
		},
		{
			name: "its own module file",
			files: map[string]string{
				"plugin/main.go": validPluginMain,
				"plugin/go.mod":  "module example.com/plugin\n",
			},
			contains: "generates the plugin module",
		},
		{
			name:     "unparseable source",
			files:    map[string]string{"plugin/main.go": "package \n"},
			contains: "parse",
		},
		{
			name:     "an unknown access level",
			files:    map[string]string{"plugin/main.go": validPluginMain},
			declared: &svc.ImageBackend{Access: "everyone"},
			contains: "invalid access",
		},
		{
			name:     "a negative timeout",
			files:    map[string]string{"plugin/main.go": validPluginMain},
			declared: &svc.ImageBackend{TimeoutMS: -1},
			contains: "negative",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := loadImagePlugin(pluginTree(tc.files), "plugin", tc.declared)
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
func TestLoadImagePluginAllowsTestFilesAndAssets(t *testing.T) {
	_, err := loadImagePlugin(pluginTree(map[string]string{
		"plugin/main.go":           validPluginMain,
		"plugin/main_test.go":      "package main\n",
		"plugin/assets/schema.sql": "select 1;",
	}), "plugin", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Plugin source is compiled, never served. The registry hands out the whole
// subtree or nothing, and only for images that actually declare a backend.
func TestRegistryPluginSource(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	source, ok := r.PluginSource("backend-playground")
	if !ok {
		t.Fatal("backend-playground exposes no plugin source")
	}
	if _, err := fs.Stat(source, "main.go"); err != nil {
		t.Errorf("plugin source is not rooted at plugin/: %v", err)
	}
	for _, id := range []string{"mysql", "ui-playground", "no-such-image"} {
		if _, ok := r.PluginSource(id); ok {
			t.Errorf("%s reports plugin source it does not have", id)
		}
	}
}
