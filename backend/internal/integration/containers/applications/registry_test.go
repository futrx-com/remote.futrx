package applications

import (
	"io/fs"
	"reflect"
	"testing"
	"testing/fstest"

	svc "github.com/futrx-com/remote.futrx.com/internal/service/applications"
)

// The shipped catalog and the fixture catalog are held to the same rules: the
// invariants below belong to the *kind* an image declares, not to any
// particular image, so an installable image distributed outside this repository
// is checked exactly as one embedded in it.
func TestRegistryLoadsCatalog(t *testing.T) {
	for _, tc := range []struct {
		name string
		load func() (*Registry, error)
		// wantImages is false for the shipped catalog: this repository holds
		// the format, not the apps, so images/ may legitimately contain only
		// docs/. What is still worth asserting there is that such a catalog
		// loads at all rather than failing startup. The fixture catalog is the
		// one that must be non-empty — it exists to carry the invariants.
		wantImages bool
	}{
		{"shipped", NewRegistry, false},
		{"fixture", func() (*Registry, error) { return NewRegistryFromFS(fixtureCatalog()) }, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, err := tc.load()
			if err != nil {
				t.Fatalf("load registry: %v", err)
			}
			assertCatalogInvariants(t, r, tc.wantImages)
		})
	}
}

func assertCatalogInvariants(t *testing.T, r *Registry, wantImages bool) {
	t.Helper()
	imgs := r.List()
	if wantImages && len(imgs) == 0 {
		t.Fatal("expected at least one image in catalog")
	}
	for _, img := range imgs {
		if img.ID == "" || img.Name == "" {
			t.Errorf("image missing id/name: %+v", img)
		}
		if len(img.Scopes) == 0 {
			t.Errorf("image %s has no scopes", img.ID)
		}
		if !img.Type.Valid() {
			t.Errorf("image %s has invalid type %q", img.ID, img.Type)
		}
		// Install scripts belong to images that provision something into a
		// container; a UI image is its ui/ directory and a backend image its
		// plugin/, and nothing else.
		if img.Type.NeedsContainer() {
			if _, ok := r.Script(img.ID); !ok {
				t.Errorf("image %s missing install script", img.ID)
			}
		}
		// A port belongs only to a kind that is reachable on one. A tool runs
		// in a container but exposes nothing.
		if img.Type.NeedsPort() {
			if img.Port.Internal <= 0 {
				t.Errorf("image %s has invalid internal port %d", img.ID, img.Port.Internal)
			}
			continue
		}
		if img.Port.Internal != 0 {
			t.Errorf("%s image %s declares port %d", img.Type, img.ID, img.Port.Internal)
		}
		if img.Type == svc.KindTool {
			for _, sc := range img.Scopes {
				if sc == svc.ScopeGlobal {
					t.Errorf("tool image %s claims global scope", img.ID)
				}
			}
			continue
		}
		switch img.Type {
		case svc.KindUI:
			if img.UI == nil {
				t.Errorf("ui image %s ships no ui/ directory", img.ID)
			}
		case svc.KindBackend:
			if img.Backend == nil {
				t.Errorf("backend image %s ships no plugin/ directory", img.ID)
			}
			if _, ok := r.PluginSource(img.ID); !ok {
				t.Errorf("backend image %s exposes no plugin source", img.ID)
			}
		}
	}
}

// Every directory beside the images is loaded as one, so a reserved name that
// stopped being skipped would fail to validate and take the whole catalog —
// and the server — down at startup. The shipped catalog carries no such
// directory today, so the fixture supplies one: the guard has to hold for
// whatever a catalog is later given, not for what happens to ship now.
func TestRegistrySkipsReservedDirectories(t *testing.T) {
	catalog := fixtureCatalog()
	for _, name := range []string{"docs"} {
		catalog["images/"+name+"/README.md"] = &fstest.MapFile{Data: []byte("# not an image\n")}
	}
	r, err := NewRegistryFromFS(catalog)
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	for _, name := range []string{"docs"} {
		if _, ok := r.Get(name); ok {
			t.Errorf("reserved directory %q was loaded as an image", name)
		}
		for _, img := range r.List() {
			if img.ID == name {
				t.Errorf("reserved directory %q appears in the catalog", name)
			}
		}
	}
}

func TestRegistryImageKinds(t *testing.T) {
	r := testRegistry(t)
	for id, want := range map[string]svc.Kind{
		fixtureService: svc.KindService,
		fixtureTool:    svc.KindTool,
		fixtureUI:      svc.KindUI,
		fixtureBackend: svc.KindBackend,
	} {
		img, ok := r.Get(id)
		if !ok {
			t.Errorf("missing image %s", id)
			continue
		}
		if img.Type != want {
			t.Errorf("%s type = %q, want %q", id, img.Type, want)
		}
	}
}

func TestValidateRejectsBadImages(t *testing.T) {
	base := func() svc.Image {
		return svc.Image{
			Name:    "Test",
			Version: "1.0.0",
			Type:    svc.KindService,
			Scopes:  []svc.Scope{svc.ScopeGlobal},
			Port:    svc.Port{Internal: 1234},
		}
	}
	for _, tc := range []struct {
		name   string
		mutate func(*svc.Image)
	}{
		{"no version", func(i *svc.Image) { i.Version = "" }},
		{"blank version", func(i *svc.Image) { i.Version = "   " }},
		{"unknown type", func(i *svc.Image) { i.Type = "daemon" }},
		{"service without a port", func(i *svc.Image) { i.Port.Internal = 0 }},
		{"ui image declaring a port", func(i *svc.Image) { i.Type = svc.KindUI }},
		{"ui image declaring a service", func(i *svc.Image) {
			i.Type = svc.KindUI
			i.Port.Internal = 0
			i.Service = "unit"
		}},
		{"ui image declaring a healthcheck", func(i *svc.Image) {
			i.Type = svc.KindUI
			i.Port.Internal = 0
			i.Healthcheck.Command = "true"
		}},
		{"backend image declaring a port", func(i *svc.Image) { i.Type = svc.KindBackend }},
		{"backend image declaring a service", func(i *svc.Image) {
			i.Type = svc.KindBackend
			i.Port.Internal = 0
			i.Service = "unit"
		}},
		{"tool image declaring a port", func(i *svc.Image) {
			i.Type = svc.KindTool
			i.Scopes = []svc.Scope{svc.ScopeProject}
		}},
		{"tool image declaring a healthcheck", func(i *svc.Image) {
			i.Type = svc.KindTool
			i.Scopes = []svc.Scope{svc.ScopeProject}
			i.Port.Internal = 0
			i.Healthcheck.Command = "true"
		}},
		{"tool image claiming global scope", func(i *svc.Image) {
			i.Type = svc.KindTool
			i.Port.Internal = 0
			i.Scopes = []svc.Scope{svc.ScopeGlobal}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			img := base()
			tc.mutate(&img)
			if err := validate(img); err == nil {
				t.Error("want a validation error, got nil")
			}
		})
	}
}

// A tool is the one kind that reaches a container without exposing anything:
// it needs an install script like a service, and no port like an extension.
// Getting either half wrong is what the kind exists to prevent.
func TestToolImageInstallsWithoutExposingAPort(t *testing.T) {
	r := testRegistry(t)
	img, ok := r.Get(fixtureTool)
	if !ok {
		t.Fatal("expected the fixture tool image")
	}
	if !img.Type.NeedsContainer() {
		t.Error("a tool must reach a container")
	}
	if img.Type.NeedsPort() {
		t.Error("a tool must not need a host port")
	}
	if img.Port.Internal != 0 {
		t.Errorf("internal port = %d, want none", img.Port.Internal)
	}
	if _, ok := r.Script(img.ID); !ok {
		t.Error("a tool must ship an install script")
	}
	if img.SupportsScope(svc.ScopeGlobal) {
		t.Error("a tool must not offer global scope: nobody works in that container")
	}
	if !img.SupportsScope(svc.ScopeProject) {
		t.Error("a tool must offer project scope")
	}
	// Stop and uninstall act on the unit, so a tool that provisions a
	// long-running thing has to name one.
	if img.Service == "" {
		t.Error("a tool that supervises something must name its systemd unit")
	}
}

// A validated tool image accepts the shape the kind is for.
func TestValidateAcceptsAToolImage(t *testing.T) {
	img := svc.Image{
		Name:    "Tool",
		Version: "1.0.0",
		Type:    svc.KindTool,
		Scopes:  []svc.Scope{svc.ScopeProject},
		Service: "unit",
	}
	if err := validate(img); err != nil {
		t.Errorf("validate(tool) = %v, want nil", err)
	}
}

func TestRegistryGetKnownImage(t *testing.T) {
	r := testRegistry(t)
	img, ok := r.Get(fixtureService)
	if !ok {
		t.Fatal("expected the fixture service image")
	}
	if !img.SupportsScope(svc.ScopeGlobal) || !img.SupportsScope(svc.ScopeProject) {
		t.Errorf("image should support both scopes, got %v", img.Scopes)
	}
	if img.Port.Internal != 5432 {
		t.Errorf("internal port = %d, want 5432", img.Port.Internal)
	}
}

func TestRegistryDiscoversImageUI(t *testing.T) {
	r := testRegistry(t)
	img, ok := r.Get(fixtureService)
	if !ok {
		t.Fatal("expected the fixture service image")
	}
	if img.UI == nil {
		t.Fatal("the image ships a ui/ directory, want a UI descriptor")
	}
	if img.UI.Entry != "scripts/main.js" {
		t.Errorf("entry = %q, want scripts/main.js", img.UI.Entry)
	}
	if len(img.UI.Styles) == 0 {
		t.Error("want stylesheets discovered under style/")
	}
	if _, ok := img.UI.Views["popup"]; !ok {
		t.Errorf("want a %q view, got %v", "popup", img.UI.Views)
	}

	// An image without ui/ must stay nil so the SPA loads nothing for it.
	tool, ok := r.Get(fixtureTool)
	if !ok {
		t.Fatal("expected the fixture tool image")
	}
	if tool.UI != nil {
		t.Errorf("the tool has no ui/ directory, got %+v", tool.UI)
	}
}

// The fixture UI image declares its ui block explicitly rather than relying on
// the layout convention, so the catalog exercises both paths for real.
func TestRegistryLoadsDeclaredImageUI(t *testing.T) {
	r := testRegistry(t)
	img, ok := r.Get(fixtureUI)
	if !ok {
		t.Fatal("expected the fixture ui image")
	}
	if img.UI == nil {
		t.Fatal("the image declares a ui block, want a UI descriptor")
	}
	if img.UI.Entry != "scripts/main.js" {
		t.Errorf("entry = %q, want scripts/main.js", img.UI.Entry)
	}
	for _, view := range []string{"panel", "context"} {
		rel, ok := img.UI.Views[view]
		if !ok {
			t.Errorf("missing view %q, got %v", view, img.UI.Views)
			continue
		}
		if _, ok := r.UIAsset(img.ID, rel); !ok {
			t.Errorf("view %q resolves to unreadable %q", view, rel)
		}
	}
	// Assets outside style/ and views/ are reachable too; only declared paths
	// are validated at load, not the whole tree.
	if _, ok := r.UIAsset(img.ID, "assets/logo.svg"); !ok {
		t.Error("want assets/logo.svg to be readable")
	}
	if _, ok := r.UIAsset(img.ID, "scripts/selftest.js"); !ok {
		t.Error("want the entry's relative import to be readable")
	}
}

func TestRegistryUIAsset(t *testing.T) {
	r := testRegistry(t)
	if _, ok := r.UIAsset(fixtureService, "scripts/main.js"); !ok {
		t.Error("want the entry module to be readable")
	}
	for _, tc := range []struct {
		name  string
		image string
		asset string
	}{
		{"traversal out of ui", fixtureService, "../install.sh"},
		{"traversal into another image", fixtureService, "../../" + fixtureTool + "/install.sh"},
		{"absolute path", fixtureService, "/etc/passwd"},
		{"empty path", fixtureService, ""},
		{"missing file", fixtureService, "scripts/nope.js"},
		{"image without ui", fixtureTool, "scripts/main.js"},
		{"unknown image", "nope", "scripts/main.js"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, ok := r.UIAsset(tc.image, tc.asset); ok {
				t.Errorf("UIAsset(%q, %q) = ok, want not found", tc.image, tc.asset)
			}
		})
	}
}

func TestLoadImageUI(t *testing.T) {
	full := fstest.MapFS{
		"ui/scripts/main.js":  {Data: []byte("export default () => {}")},
		"ui/style/a.css":      {Data: []byte(".a{}")},
		"ui/style/b.css":      {Data: []byte(".b{}")},
		"ui/views/index.html": {Data: []byte("<p></p>")},
		"ui/views/popup.html": {Data: []byte("<p></p>")},
	}

	t.Run("convention fills every field", func(t *testing.T) {
		ui, err := loadImageUI(full, "ui", nil)
		if err != nil {
			t.Fatalf("loadImageUI: %v", err)
		}
		if ui.Entry != "scripts/main.js" {
			t.Errorf("entry = %q", ui.Entry)
		}
		// Sorted, so injection order does not drift between builds.
		want := []string{"style/a.css", "style/b.css"}
		if !reflect.DeepEqual(ui.Styles, want) {
			t.Errorf("styles = %v, want %v", ui.Styles, want)
		}
		wantViews := map[string]string{
			"index": "views/index.html",
			"popup": "views/popup.html",
		}
		if !reflect.DeepEqual(ui.Views, wantViews) {
			t.Errorf("views = %v, want %v", ui.Views, wantViews)
		}
	})

	t.Run("declared block wins over convention", func(t *testing.T) {
		ui, err := loadImageUI(full, "ui", &svc.ImageUI{
			Entry:  "scripts/main.js",
			Styles: []string{"style/b.css"},
			Views:  map[string]string{"dialog": "views/popup.html"},
		})
		if err != nil {
			t.Fatalf("loadImageUI: %v", err)
		}
		if !reflect.DeepEqual(ui.Styles, []string{"style/b.css"}) {
			t.Errorf("styles = %v, want only style/b.css", ui.Styles)
		}
		if ui.Views["dialog"] != "views/popup.html" {
			t.Errorf("views = %v", ui.Views)
		}
	})

	// An image may ship a ui/ holding nothing but an icon, with no code to load.
	t.Run("assets-only ui directory is valid", func(t *testing.T) {
		ui, err := loadImageUI(fstest.MapFS{"ui/assets/logo.svg": {Data: []byte("<svg/>")}}, "ui", nil)
		if err != nil {
			t.Fatalf("loadImageUI: %v", err)
		}
		if ui == nil {
			t.Fatal("want a UI descriptor")
		}
		if ui.Entry != "" || len(ui.Styles) != 0 || len(ui.Views) != 0 {
			t.Errorf("want nothing to load, got %+v", ui)
		}
	})

	t.Run("no ui directory", func(t *testing.T) {
		ui, err := loadImageUI(fstest.MapFS{"install.sh": {Data: []byte("#!/bin/sh")}}, "ui", nil)
		if err != nil {
			t.Fatalf("loadImageUI: %v", err)
		}
		if ui != nil {
			t.Errorf("want nil UI, got %+v", ui)
		}
	})

	for _, tc := range []struct {
		name     string
		fsys     fs.FS
		declared *svc.ImageUI
	}{
		{
			name:     "declared ui without a directory",
			fsys:     fstest.MapFS{"install.sh": {Data: []byte("#!/bin/sh")}},
			declared: &svc.ImageUI{Entry: "scripts/main.js"},
		},
		{
			name:     "declared entry missing",
			fsys:     full,
			declared: &svc.ImageUI{Entry: "scripts/other.js"},
		},
		{
			name:     "declared style missing",
			fsys:     full,
			declared: &svc.ImageUI{Styles: []string{"style/missing.css"}},
		},
		{
			name:     "declared view escapes ui/",
			fsys:     full,
			declared: &svc.ImageUI{Views: map[string]string{"x": "../install.sh"}},
		},
		{
			name: "entirely empty ui directory",
			fsys: fstest.MapFS{"ui": {Mode: fs.ModeDir}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := loadImageUI(tc.fsys, "ui", tc.declared); err == nil {
				t.Error("want a load error, got nil")
			}
		})
	}
}

func TestCleanUIPath(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string
		ok   bool
	}{
		{"scripts/main.js", "scripts/main.js", true},
		{"./scripts/main.js", "scripts/main.js", true},
		{"scripts/../style/a.css", "style/a.css", true},
		{"../install.sh", "", false},
		{"/etc/passwd", "", false},
		{"", "", false},
		{"..", "", false},
		{`scripts\..\..\install.sh`, "", false},
	} {
		got, ok := cleanUIPath(tc.in)
		if ok != tc.ok || got != tc.want {
			t.Errorf("cleanUIPath(%q) = (%q, %v), want (%q, %v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}
