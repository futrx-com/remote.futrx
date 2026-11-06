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

func TestFileManagerSupportsProjectAndGlobalContainerInstalls(t *testing.T) {
	registry, err := NewRegistry(EmbeddedCatalog(), nil)
	if err != nil {
		t.Fatal(err)
	}
	app, ok := registry.Get("file-manager")
	if !ok {
		t.Fatal("missing File Manager catalog entry")
	}
	// Both scopes must be declared; globallyInstalledInsideContainers defaults
	// to true when omitted, placing global installs in each project container.
	if !app.SupportsScope(svc.ScopeProject) || !app.SupportsScope(svc.ScopeGlobal) {
		t.Fatalf("unexpected scopes: %v", app.Scopes)
	}
	if !app.GloballyInstalledInsideContainers {
		t.Fatal("global installs must place File Manager in each project container")
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
	// The drawer reaches the workspace through backend routes; no container
	// web server, port, or application hostname remains.
	if app.Service != nil || app.Web != nil || app.NeedsPort() {
		t.Fatalf("unexpected container web surface: service=%+v web=%+v", app.Service, app.Web)
	}
	if app.Backend == nil || app.Backend.Timeout() != 300000 {
		t.Fatalf("expected a host backend with room for archives: %+v", app.Backend)
	}
	if !strings.Contains(string(script), "systemctl disable --now \"$legacy\"") {
		t.Fatal("installer does not retire the legacy container web service")
	}
	if app.UI == nil || app.UI.Entry != "scripts/main.js" || !slices.Equal(app.UI.Styles, []string{"style/files.css"}) {
		t.Fatalf("expected drawer UI with its stylesheet: %+v", app.UI)
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
