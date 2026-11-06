package tests

import (
	"slices"
	"strings"
	"testing"

	"github.com/futrx-com/remote.futrx.com/pkg/applicationtest"
)

func TestTerminalKeepsShellsInTheContainerWithoutAPort(t *testing.T) {
	registry := applicationtest.Registry(t)
	app, ok := registry.Get("terminal")
	if !ok {
		t.Fatal("missing Terminal catalog entry")
	}
	if !app.SupportsScope("project") || !app.SupportsScope("global") || !app.GloballyInstalledInsideContainers {
		t.Fatalf("Terminal must install into each project container: scopes=%v", app.Scopes)
	}
	if app.Container == nil || !slices.Equal(app.Container.Commands, []string{"remote-terminal"}) {
		t.Fatalf("missing container build: %+v", app.Container)
	}
	// The container service owns the shells, as before, but answers only on a
	// Unix socket: no port, web route, or application hostname is declared.
	if app.Service == nil || app.Service.Name != "remote-terminal" ||
		!slices.Equal(app.Service.Command, []string{"/usr/local/bin/remote-terminal", "serve"}) {
		t.Fatalf("unexpected service: %+v", app.Service)
	}
	if app.Web != nil || app.NeedsPort() {
		t.Fatalf("unexpected network surface: web=%+v port=%+v", app.Web, app.Port)
	}
	// An output read waits up to 20 seconds, which must fit inside one call.
	if app.Backend == nil || app.Backend.Timeout() < 25000 {
		t.Fatalf("expected a host backend with room for waiting reads: %+v", app.Backend)
	}
	script, found := registry.Script("terminal")
	if !found {
		t.Fatal("missing generated install script")
	}
	if !strings.Contains(string(script), `-o "$APP_BUILD_DIR"/'remote-terminal' './cmd/remote-terminal'`) {
		t.Fatal("installer must build the terminal command")
	}
	if app.UI == nil || app.UI.Entry != "scripts/main.js" ||
		!slices.Equal(app.UI.Styles, []string{"style/xterm.css", "style/terminal.css"}) {
		t.Fatalf("expected drawer UI with xterm's stylesheet first: %+v", app.UI)
	}
}

func TestBackendBuildsAndDescribesRoutes(t *testing.T) {
	applicationtest.BackendRoutes(t, "terminal", []string{"GET output", "POST input", "POST resize", "POST sessions"})
}
