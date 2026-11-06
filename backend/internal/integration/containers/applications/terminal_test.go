package applications

import (
	"slices"
	"strings"
	"testing"

	svc "github.com/futrx-com/remote.futrx.com/internal/service/applications"
)

func TestTerminalKeepsShellsInTheContainerWithoutAPort(t *testing.T) {
	registry, err := NewRegistry(EmbeddedCatalog(), nil)
	if err != nil {
		t.Fatal(err)
	}
	app, ok := registry.Get("terminal")
	if !ok {
		t.Fatal("missing Terminal catalog entry")
	}
	if !app.SupportsScope(svc.ScopeProject) || !app.SupportsScope(svc.ScopeGlobal) || !app.GloballyInstalledInsideContainers {
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
	_, script, err := loadApplication(EmbeddedCatalog(), "terminal")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(script), `-o "$APP_BUILD_DIR"/'remote-terminal' './cmd/remote-terminal'`) {
		t.Fatal("installer must build the terminal command")
	}
	if app.UI == nil || app.UI.Entry != "scripts/main.js" ||
		!slices.Equal(app.UI.Styles, []string{"style/xterm.css", "style/terminal.css"}) {
		t.Fatalf("expected drawer UI with xterm's stylesheet first: %+v", app.UI)
	}
}
