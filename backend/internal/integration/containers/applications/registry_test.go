package applications

import (
	"io/fs"
	"path"
	"reflect"
	"testing"
	"testing/fstest"

	svc "github.com/futrx-com/remote.futrx.com/internal/service/applications"
)

func TestRegistryLoadsCatalog(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	imgs := r.List()
	if len(imgs) == 0 {
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
		// Ports and install scripts belong to images that run something; a UI
		// image is its ui/ directory and a backend image its plugin/, and
		// nothing else.
		if img.Type.NeedsContainer() {
			if img.Port.Internal <= 0 {
				t.Errorf("image %s has invalid internal port %d", img.ID, img.Port.Internal)
			}
			if _, ok := r.Script(img.ID); !ok {
				t.Errorf("image %s missing install script", img.ID)
			}
			continue
		}
		if img.Port.Internal != 0 {
			t.Errorf("%s image %s declares port %d", img.Type, img.ID, img.Port.Internal)
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

// docs/ shares the catalog directory with the images. Every other directory
// there is loaded as an image, so a reserved name that stopped being skipped
// would take the whole catalog — and the server — down at startup.
func TestRegistrySkipsReservedDirectories(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	for _, name := range []string{"docs"} {
		if _, err := fs.Stat(catalogFS, path.Join("images", name)); err != nil {
			continue // reserved but not present; nothing to skip
		}
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
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	for id, want := range map[string]svc.Kind{
		"mysql":              svc.KindService,
		"postgresql":         svc.KindService,
		"redis":              svc.KindService,
		"ui-playground":      svc.KindUI,
		"backend-playground": svc.KindBackend,
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
			Name:   "Test",
			Type:   svc.KindService,
			Scopes: []svc.Scope{svc.ScopeGlobal},
			Port:   svc.Port{Internal: 1234},
		}
	}
	for _, tc := range []struct {
		name   string
		mutate func(*svc.Image)
	}{
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

func TestRegistryGetKnownImage(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	pg, ok := r.Get("postgresql")
	if !ok {
		t.Fatal("expected postgresql image")
	}
	if !pg.SupportsScope(svc.ScopeGlobal) || !pg.SupportsScope(svc.ScopeProject) {
		t.Errorf("postgresql should support both scopes, got %v", pg.Scopes)
	}
	if pg.Port.Internal != 5432 {
		t.Errorf("postgresql internal port = %d, want 5432", pg.Port.Internal)
	}
}

func TestRegistryDiscoversImageUI(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	mysql, ok := r.Get("mysql")
	if !ok {
		t.Fatal("expected mysql image")
	}
	if mysql.UI == nil {
		t.Fatal("mysql ships a ui/ directory, want a UI descriptor")
	}
	if mysql.UI.Entry != "scripts/main.js" {
		t.Errorf("entry = %q, want scripts/main.js", mysql.UI.Entry)
	}
	if len(mysql.UI.Styles) == 0 {
		t.Error("want stylesheets discovered under style/")
	}
	if _, ok := mysql.UI.Views["popup"]; !ok {
		t.Errorf("want a %q view, got %v", "popup", mysql.UI.Views)
	}

	// An image without ui/ must stay nil so the SPA loads nothing for it.
	redis, ok := r.Get("redis")
	if !ok {
		t.Fatal("expected redis image")
	}
	if redis.UI != nil {
		t.Errorf("redis has no ui/ directory, got %+v", redis.UI)
	}
}

// ui-playground declares its ui block explicitly rather than relying on the
// layout convention, so the catalog exercises both paths for real.
func TestRegistryLoadsDeclaredImageUI(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	img, ok := r.Get("ui-playground")
	if !ok {
		t.Fatal("expected ui-playground image")
	}
	if img.UI == nil {
		t.Fatal("ui-playground declares a ui block, want a UI descriptor")
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
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	if _, ok := r.UIAsset("mysql", "scripts/main.js"); !ok {
		t.Error("want mysql entry module to be readable")
	}
	for _, tc := range []struct {
		name  string
		image string
		asset string
	}{
		{"traversal out of ui", "mysql", "../install.sh"},
		{"traversal into another image", "mysql", "../../redis/install.sh"},
		{"absolute path", "mysql", "/etc/passwd"},
		{"empty path", "mysql", ""},
		{"missing file", "mysql", "scripts/nope.js"},
		{"image without ui", "redis", "scripts/main.js"},
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
