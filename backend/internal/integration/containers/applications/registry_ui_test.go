package applications

import (
	"io/fs"
	"reflect"
	"testing"
	"testing/fstest"

	svc "github.com/futrx-com/remote.futrx.com/internal/service/applications"
)

func TestRegistryDiscoversApplicationUI(t *testing.T) {
	r := testRegistry(t)
	img, ok := r.Get(fixtureService)
	if !ok {
		t.Fatal("expected the fixture service application")
	}
	if img.UI == nil {
		t.Fatal("the application ships a ui/ directory, want a UI descriptor")
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

	// An application without ui/ must stay nil so the SPA loads nothing for it.
	tool, ok := r.Get(fixturePortless)
	if !ok {
		t.Fatal("expected the fixture tool application")
	}
	if tool.UI != nil {
		t.Errorf("the tool has no ui/ directory, got %+v", tool.UI)
	}
}

// The fixture UI application declares its ui block explicitly rather than relying on
// the layout convention, so the catalog exercises both paths for real.
func TestRegistryLoadsDeclaredApplicationUI(t *testing.T) {
	r := testRegistry(t)
	img, ok := r.Get(fixtureUI)
	if !ok {
		t.Fatal("expected the fixture ui application")
	}
	if img.UI == nil {
		t.Fatal("the application declares a ui block, want a UI descriptor")
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
		name        string
		application string
		asset       string
	}{
		{"traversal out of ui", fixtureService, "../install.sh"},
		{"traversal into another application", fixtureService, "../../" + fixturePortless + "/install.sh"},
		{"absolute path", fixtureService, "/etc/passwd"},
		{"empty path", fixtureService, ""},
		{"missing file", fixtureService, "scripts/nope.js"},
		{"application without ui", fixturePortless, "scripts/main.js"},
		{"unknown application", "nope", "scripts/main.js"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, ok := r.UIAsset(tc.application, tc.asset); ok {
				t.Errorf("UIAsset(%q, %q) = ok, want not found", tc.application, tc.asset)
			}
		})
	}
}

func TestLoadApplicationUI(t *testing.T) {
	full := fstest.MapFS{
		"ui/scripts/main.js":  {Data: []byte("export default () => {}")},
		"ui/style/a.css":      {Data: []byte(".a{}")},
		"ui/style/b.css":      {Data: []byte(".b{}")},
		"ui/views/index.html": {Data: []byte("<p></p>")},
		"ui/views/popup.html": {Data: []byte("<p></p>")},
	}

	t.Run("convention fills every field", func(t *testing.T) {
		ui, err := loadApplicationUI(full, "ui", nil)
		if err != nil {
			t.Fatalf("loadApplicationUI: %v", err)
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
		ui, err := loadApplicationUI(full, "ui", &svc.ApplicationUI{
			Entry:  "scripts/main.js",
			Styles: []string{"style/b.css"},
			Views:  map[string]string{"dialog": "views/popup.html"},
		})
		if err != nil {
			t.Fatalf("loadApplicationUI: %v", err)
		}
		if !reflect.DeepEqual(ui.Styles, []string{"style/b.css"}) {
			t.Errorf("styles = %v, want only style/b.css", ui.Styles)
		}
		if ui.Views["dialog"] != "views/popup.html" {
			t.Errorf("views = %v", ui.Views)
		}
	})

	// An application may ship a ui/ holding nothing but an icon, with no code to load.
	t.Run("assets-only ui directory is valid", func(t *testing.T) {
		ui, err := loadApplicationUI(fstest.MapFS{"ui/assets/logo.svg": {Data: []byte("<svg/>")}}, "ui", nil)
		if err != nil {
			t.Fatalf("loadApplicationUI: %v", err)
		}
		if ui == nil {
			t.Fatal("want a UI descriptor")
		}
		if ui.Entry != "" || len(ui.Styles) != 0 || len(ui.Views) != 0 {
			t.Errorf("want nothing to load, got %+v", ui)
		}
	})

	t.Run("no ui directory", func(t *testing.T) {
		ui, err := loadApplicationUI(fstest.MapFS{"install.sh": {Data: []byte("#!/bin/sh")}}, "ui", nil)
		if err != nil {
			t.Fatalf("loadApplicationUI: %v", err)
		}
		if ui != nil {
			t.Errorf("want nil UI, got %+v", ui)
		}
	})

	for _, tc := range []struct {
		name     string
		fsys     fs.FS
		declared *svc.ApplicationUI
	}{
		{
			name:     "declared ui without a directory",
			fsys:     fstest.MapFS{"install.sh": {Data: []byte("#!/bin/sh")}},
			declared: &svc.ApplicationUI{Entry: "scripts/main.js"},
		},
		{
			name:     "declared entry missing",
			fsys:     full,
			declared: &svc.ApplicationUI{Entry: "scripts/other.js"},
		},
		{
			name:     "declared style missing",
			fsys:     full,
			declared: &svc.ApplicationUI{Styles: []string{"style/missing.css"}},
		},
		{
			name:     "declared view escapes ui/",
			fsys:     full,
			declared: &svc.ApplicationUI{Views: map[string]string{"x": "../install.sh"}},
		},
		{
			name: "entirely empty ui directory",
			fsys: fstest.MapFS{"ui": {Mode: fs.ModeDir}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := loadApplicationUI(tc.fsys, "ui", tc.declared); err == nil {
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
