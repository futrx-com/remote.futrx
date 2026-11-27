package tests

import (
	"slices"
	"strings"
	"testing"

	"github.com/futrx-com/remote.futrx.com/pkg/applicationtest"
)

func TestFileManagerSupportsProjectAndGlobalContainerInstalls(t *testing.T) {
	registry := applicationtest.Registry(t)
	app, ok := registry.Get("file-manager")
	if !ok {
		t.Fatal("missing File Manager catalog entry")
	}
	// Both scopes must be declared; globallyInstalledInsideContainers defaults
	// to true when omitted, placing global installs in each project container.
	if !app.SupportsScope("project") || !app.SupportsScope("global") {
		t.Fatalf("unexpected scopes: %v", app.Scopes)
	}
	if !app.GloballyInstalledInsideContainers {
		t.Fatal("global installs must place File Manager in each project container")
	}
	if !app.NeedsContainer() || app.Container == nil || app.Container.SourceDigest == "" || !slices.Equal(app.Container.Commands, []string{"file-manager"}) {
		t.Fatalf("missing container build: %+v", app.Container)
	}
	script, found := registry.Script("file-manager")
	if !found {
		t.Fatal("missing generated install script")
	}
	if !strings.Contains(string(script), `-o "$APP_BUILD_DIR"/'file-manager' './cmd/file-manager'`) {
		t.Fatal("installer does not build the file-manager command")
	}
	// File operations run as container commands through the backend routes.
	if app.Service != nil || app.Web != nil || app.NeedsPort() {
		t.Fatalf("unexpected service or network declaration: service=%+v web=%+v", app.Service, app.Web)
	}
	if app.Backend == nil || app.Backend.Timeout() != 300000 {
		t.Fatalf("expected a host backend with room for archives: %+v", app.Backend)
	}
	// Installation preparation must finish before the generated build starts.
	retireAt := strings.Index(string(script), "systemctl disable --now \"$legacy\"")
	buildAt := strings.Index(string(script), "APP_BUILD_VERSION=")
	if app.Preinstall != "infra/preinstall.sh" || retireAt < 0 || buildAt < 0 || retireAt > buildAt {
		t.Fatal("installer does not run preparation before the container build")
	}
	if app.UI == nil || app.UI.Entry != "scripts/main.js" || !slices.Equal(app.UI.Styles, []string{"style/files.css"}) {
		t.Fatalf("expected drawer UI with its stylesheet: %+v", app.UI)
	}
	if app.Uninstall != "infra/uninstall.sh" {
		t.Fatal("missing binary cleanup")
	}
}

func TestFileManagerContainerBuildsIndependently(t *testing.T) {
	registry := applicationtest.Registry(t)
	script, ok := registry.Script("file-manager")
	if !ok {
		t.Fatal("missing generated install script")
	}
	applicationtest.BuildContainer(t, script, "file-manager")
}

func TestBackendBuildsAndDescribesRoutes(t *testing.T) {
	applicationtest.BackendRoutes(t, "file-manager", []string{"GET list", "GET search", "POST archive"})
}
