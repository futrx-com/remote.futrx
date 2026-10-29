package applications

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	svc "github.com/futrx-com/remote.futrx.com/internal/service/applications"
)

func TestFileManagerIsAnOptionalProjectApplication(t *testing.T) {
	registry, err := NewRegistry(EmbeddedCatalog(), nil)
	if err != nil {
		t.Fatal(err)
	}
	app, ok := registry.Get("file-manager")
	if !ok {
		t.Fatal("missing File Manager catalog entry")
	}
	if !app.SupportsScope(svc.ScopeProject) || app.SupportsScope(svc.ScopeGlobal) {
		t.Fatalf("unexpected scopes: %v", app.Scopes)
	}
	if !app.NeedsContainer() || app.Container == nil || app.Container.SourceDigest == "" || !slices.Equal(app.Container.Commands, []string{"file-manager"}) {
		t.Fatalf("missing container build: %+v", app.Container)
	}
	_, script, err := loadApplication(EmbeddedCatalog(), "file-manager")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(script), `-o "$APP_BUILD_DIR"/'file-manager' './cmd/file-manager'`) {
		t.Fatal("installer does not build the file-manager command")
	}
	if app.Service == nil || app.Service.Name != "remote-file-manager" || !slices.Equal(app.Service.Command, []string{"/usr/local/bin/file-manager", "--port", "8844"}) {
		t.Fatalf("unexpected service: %+v", app.Service)
	}
	if app.Web == nil || app.Web.Port != 8844 || app.Web.Subdomain != "files" {
		t.Fatalf("unexpected web route: %+v", app.Web)
	}
	if app.UI == nil || app.UI.Entry != "scripts/main.js" || app.Backend != nil {
		t.Fatal("expected drawer UI and no host backend")
	}
	if app.Uninstall != "infra/uninstall.sh" {
		t.Fatal("missing binary cleanup")
	}
}

func TestFileManagerContainerBuildsIndependently(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles the generated container module")
	}
	goCommand, err := exec.LookPath("go")
	if err != nil {
		t.Skip("Go toolchain is unavailable")
	}
	source, err := packContainerSource(EmbeddedCatalog(), "applications/file-manager", "file-manager")
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	for name, content := range unpackTestPayload(t, source.payload) {
		relative := strings.TrimPrefix(name, "infra/")
		target := filepath.Join(directory, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, goCommand, "build", "-mod=mod", "-buildvcs=false", "-o", filepath.Join(t.TempDir(), "file-manager"), "./cmd/file-manager")
	command.Dir = directory
	command.Env = append(os.Environ(), "GOWORK=off", "CGO_ENABLED=0")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("generated container module: %v\n%s", err, output)
	}
}
