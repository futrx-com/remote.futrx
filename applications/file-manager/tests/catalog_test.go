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
	// The drawer reaches the workspace through backend routes; no container
	// web server, port, or application hostname remains.
	if app.Service != nil || app.Web != nil || app.NeedsPort() {
		t.Fatalf("unexpected container web surface: service=%+v web=%+v", app.Service, app.Web)
	}
	if app.Backend == nil || app.Backend.Timeout() != 300000 {
		t.Fatalf("expected a host backend with room for archives: %+v", app.Backend)
	}
	// The legacy web service runs the binary the build replaces, so it is
	// retired before the build rather than after it.
	retireAt := strings.Index(string(script), "systemctl disable --now \"$legacy\"")
	buildAt := strings.Index(string(script), "APP_BUILD_VERSION=")
	if app.Preinstall != "infra/preinstall.sh" || retireAt < 0 || buildAt < 0 || retireAt > buildAt {
		t.Fatal("installer does not retire the legacy container web service before the build")
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
	applicationtest.BackendRoutes(t, "file-manager", []string{"GET archive", "GET list", "GET search"})
}
