package applications

import (
	"testing"
	"testing/fstest"

	svc "github.com/futrx-com/remote.futrx.com/internal/service/applications"
)

// The shipped catalog and the fixture catalog are held to the same rules: the
// invariants below belong to the *kind* an application declares, not to any
// particular application, so an installable application distributed outside this repository
// is checked exactly as one embedded in it.
func TestRegistryLoadsCatalog(t *testing.T) {
	for _, tc := range []struct {
		name string
		load func() (*Registry, error)
		// wantImages is false for the shipped catalog: this repository holds
		// the format, not the apps, so applications/ may legitimately contain only
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
		t.Fatal("expected at least one application in catalog")
	}
	for _, img := range imgs {
		if img.ID == "" || img.Name == "" {
			t.Errorf("application missing id/name: %+v", img)
		}
		if len(img.Scopes) == 0 {
			t.Errorf("application %s has no scopes", img.ID)
		}
		if img.NeedsContainer() {
			if _, ok := r.Script(img.ID); !ok {
				t.Errorf("application %s missing install script", img.ID)
			}
		}
		if img.NeedsPort() {
			if img.Port.Internal <= 0 {
				t.Errorf("application %s has invalid internal port %d", img.ID, img.Port.Internal)
			}
		}
	}
}

// Every directory beside the applications is loaded as one, so a reserved name that
// stopped being skipped would fail to validate and take the whole catalog —
// and the server — down at startup. The shipped catalog carries no such
// directory today, so the fixture supplies one: the guard has to hold for
// whatever a catalog is later given, not for what happens to ship now.
func TestRegistrySkipsReservedDirectories(t *testing.T) {
	catalog := fixtureCatalog()
	for _, name := range []string{"docs"} {
		catalog["applications/"+name+"/README.md"] = &fstest.MapFile{Data: []byte("# not an application\n")}
	}
	r, err := NewRegistryFromFS(catalog)
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	for _, name := range []string{"docs"} {
		if _, ok := r.Get(name); ok {
			t.Errorf("reserved directory %q was loaded as an application", name)
		}
		for _, img := range r.List() {
			if img.ID == name {
				t.Errorf("reserved directory %q appears in the catalog", name)
			}
		}
	}
}

func TestRegistryInfersApplicationCapabilities(t *testing.T) {
	r := testRegistry(t)
	for id, want := range map[string]struct{ container, port bool }{
		fixtureService: {true, true},
		fixtureTool:    {true, false},
	} {
		img, ok := r.Get(id)
		if !ok {
			t.Errorf("missing application %s", id)
			continue
		}
		got := struct{ container, port bool }{img.NeedsContainer(), img.NeedsPort()}
		if got != want {
			t.Errorf("%s capabilities = %+v, want %+v", id, got, want)
		}
	}
}

func TestValidateRejectsBadApplications(t *testing.T) {
	base := func() svc.Application {
		return svc.Application{
			Name: "Test", Version: "1.0.0", Install: "infra/install.sh",
			Scopes: []svc.Scope{svc.ScopeGlobal}, Port: svc.Port{Internal: 1234},
		}
	}
	for _, tc := range []struct {
		name   string
		mutate func(*svc.Application)
	}{
		{"no version", func(i *svc.Application) { i.Version = "" }},
		{"blank version", func(i *svc.Application) { i.Version = "   " }},
		{"missing scopes", func(i *svc.Application) { i.Scopes = nil }},
		{"port without infra", func(i *svc.Application) { i.Install = "" }},
		{"healthcheck without internal port", func(i *svc.Application) { i.Port.Internal = 0; i.Healthcheck.Command = "true" }},
		{"service without infra", func(i *svc.Application) { i.Install = ""; i.Port = svc.Port{}; i.Service = "unit" }},
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

func TestInfrastructureApplicationCanInstallWithoutExposingAPort(t *testing.T) {
	r := testRegistry(t)
	img, ok := r.Get(fixtureTool)
	if !ok {
		t.Fatal("expected the fixture tool application")
	}
	if !img.NeedsContainer() {
		t.Error("infrastructure must reach a container")
	}
	if img.NeedsPort() {
		t.Error("portless infrastructure must not need a host port")
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

func TestValidateAcceptsInfrastructureWithoutAPort(t *testing.T) {
	img := svc.Application{
		Name:    "Tool",
		Version: "1.0.0",
		Install: "infra/install.sh",
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
		t.Fatal("expected the fixture service application")
	}
	if !img.SupportsScope(svc.ScopeGlobal) || !img.SupportsScope(svc.ScopeProject) {
		t.Errorf("application should support both scopes, got %v", img.Scopes)
	}
	if img.Port.Internal != 5432 {
		t.Errorf("internal port = %d, want 5432", img.Port.Internal)
	}
}
