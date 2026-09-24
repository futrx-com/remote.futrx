package applications

import (
	"context"
	"testing"
	"time"

	containerapplications "github.com/futrx-com/remote.futrx.com/internal/integration/containers/applications"
)

func TestFileManagementBackendCompilesThroughGeneratedModuleBuilder(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles the shipped backend with the Go toolchain")
	}
	if _, err := findGoTool(testGoToolOverride()); err != nil {
		t.Skipf("Go toolchain unavailable: %v", err)
	}
	registry, err := containerapplications.NewRegistry(containerapplications.EmbeddedCatalog(), nil)
	if err != nil {
		t.Fatal(err)
	}
	source, ok := registry.BackendSource("file-management")
	if !ok {
		t.Fatal("file-management backend source is missing")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if _, err := NewBuilder(t.TempDir(), testGoToolOverride()).Build(ctx, "file-management", source); err != nil {
		t.Fatalf("compile file-management backend: %v", err)
	}
}
