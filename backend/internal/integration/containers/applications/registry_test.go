package applications

import (
	"testing"
	"testing/fstest"

	svc "github.com/futrx-com/remote.futrx.com/internal/service/applications"
)

// The shipped catalog and the fixture catalog are held to the same rules, so an
// application distributed outside this repository is checked exactly as one
// embedded in it.
func TestRegistryLoadsCatalog(t *testing.T) {
	for _, tc := range []struct {
		name string
		load func() (*Registry, error)
		// wantApplications is false for the shipped catalog: this repository holds
		// the format, not the apps, so applications/ may legitimately contain only
		// docs/. What is still worth asserting there is that such a catalog
		// loads at all rather than failing startup. The fixture catalog is the
		// one that must be non-empty — it exists to carry the invariants.
		wantApplications bool
	}{
		{"shipped", func() (*Registry, error) { return NewRegistry(EmbeddedCatalog(), nil) }, false},
		{"fixture", func() (*Registry, error) { return NewRegistry(fixtureCatalog(), nil) }, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, err := tc.load()
			if err != nil {
				t.Fatalf("load registry: %v", err)
			}
			assertCatalogInvariants(t, r, tc.wantApplications)
		})
	}
}

func assertCatalogInvariants(t *testing.T, r *Registry, wantApplications bool) {
	t.Helper()
	applications := r.List()
	if wantApplications && len(applications) == 0 {
		t.Fatal("expected at least one application in catalog")
	}
	for _, application := range applications {
		if application.ID == "" || application.Name == "" {
			t.Errorf("application missing id/name: %+v", application)
		}
		if len(application.Scopes) == 0 {
			t.Errorf("application %s has no scopes", application.ID)
		}
		if application.Install != "" || application.Container != nil {
			if _, ok := r.Script(application.ID); !ok {
				t.Errorf("application %s missing install script", application.ID)
			}
		}
		if application.NeedsPort() {
			if application.Port.Internal <= 0 {
				t.Errorf("application %s has invalid internal port %d", application.ID, application.Port.Internal)
			}
		}
		if application.Backend != nil {
			if _, ok := r.BackendSource(application.ID); !ok {
				t.Errorf("application %s exposes no backend source", application.ID)
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
	r, err := NewRegistry(catalog, nil)
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	for _, name := range []string{"docs"} {
		if _, ok := r.Get(name); ok {
			t.Errorf("reserved directory %q was loaded as an application", name)
		}
		for _, application := range r.List() {
			if application.ID == name {
				t.Errorf("reserved directory %q appears in the catalog", name)
			}
		}
	}
}

func TestRegistryInfersApplicationCapabilities(t *testing.T) {
	r := testRegistry(t)
	for id, want := range map[string]struct{ container, port, backend bool }{
		fixtureService:  {true, true, false},
		fixturePortless: {true, false, false},
		fixtureBackend:  {false, false, true},
	} {
		application, ok := r.Get(id)
		if !ok {
			t.Errorf("missing application %s", id)
			continue
		}
		got := struct{ container, port, backend bool }{application.NeedsContainer(), application.NeedsPort(), application.Backend != nil}
		if got != want {
			t.Errorf("%s capabilities = %+v, want %+v", id, got, want)
		}
	}
}

func TestRegistryCombinesInfrastructureAndBackend(t *testing.T) {
	catalog := fixtureCatalog()
	catalog["applications/"+fixtureService+"/backend/main.go"] = &fstest.MapFile{
		Data: []byte("package main\n\nfunc main() {}\n"),
	}
	r, err := NewRegistry(catalog, nil)
	if err != nil {
		t.Fatalf("load combined application: %v", err)
	}
	application, ok := r.Get(fixtureService)
	if !ok {
		t.Fatal("combined application is missing")
	}
	if !application.NeedsContainer() || !application.NeedsPort() || application.Backend == nil {
		t.Fatalf("capabilities were not combined: %+v", application)
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
		{"service without command", func(i *svc.Application) {
			i.Service = &svc.ApplicationService{Name: "unit"}
		}},
		{"service name with suffix", func(i *svc.Application) {
			i.Service = &svc.ApplicationService{Name: "unit.service", Command: []string{"/usr/local/bin/unit"}}
		}},
		{"service with relative executable", func(i *svc.Application) {
			i.Service = &svc.ApplicationService{Name: "unit", Command: []string{"unit"}}
		}},
		{"service environment references undeclared input", func(i *svc.Application) {
			i.Service = &svc.ApplicationService{
				Name: "unit", Command: []string{"/usr/local/bin/unit"},
				Environment: []svc.ServiceEnvironment{{Key: "TOKEN_B64", FromEnv: "TOKEN", Encoding: "base64"}},
			}
		}},
		{"no capabilities", func(i *svc.Application) { i.Install = ""; i.Port = svc.Port{} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			application := base()
			tc.mutate(&application)
			if err := validateApplication(application); err == nil {
				t.Error("want a validation error, got nil")
			}
		})
	}
}

func TestInfrastructureApplicationCanInstallWithoutExposingAPort(t *testing.T) {
	r := testRegistry(t)
	application, ok := r.Get(fixturePortless)
	if !ok {
		t.Fatal("expected the portless fixture application")
	}
	if !application.NeedsContainer() {
		t.Error("infrastructure must reach a container")
	}
	if application.NeedsPort() {
		t.Error("portless infrastructure must not need a host port")
	}
	if application.Port.Internal != 0 {
		t.Errorf("internal port = %d, want none", application.Port.Internal)
	}
	if _, ok := r.Script(application.ID); !ok {
		t.Error("portless infrastructure must ship an install script")
	}
	if application.SupportsScope(svc.ScopeGlobal) {
		t.Error("the fixture must not offer undeclared global scope")
	}
	if !application.SupportsScope(svc.ScopeProject) {
		t.Error("the fixture must offer its declared project scope")
	}
	// Stop and uninstall act on the unit, so infrastructure that provisions a
	// long-running thing has to name one.
	if application.Service == nil {
		t.Error("supervised infrastructure must name its systemd unit")
	}
}

func TestValidateAcceptsInfrastructureWithoutAPort(t *testing.T) {
	application := svc.Application{
		Name:    "Worker",
		Version: "1.0.0",
		Install: "infra/install.sh",
		Scopes:  []svc.Scope{svc.ScopeProject},
		Service: &svc.ApplicationService{Name: "unit", Command: []string{"/usr/local/bin/worker"}},
	}
	if err := validateApplication(application); err != nil {
		t.Errorf("validate(portless infrastructure) = %v, want nil", err)
	}
}

func TestValidateAcceptsADeclarativeServiceWithoutAnInstallScript(t *testing.T) {
	application := svc.Application{
		Name:    "Worker",
		Version: "1.0.0",
		Scopes:  []svc.Scope{svc.ScopeProject},
		Service: &svc.ApplicationService{
			Name: "worker", Command: []string{"/usr/local/bin/worker"},
		},
	}
	if err := validateApplication(application); err != nil {
		t.Errorf("validate(declarative service) = %v, want nil", err)
	}
	if !application.NeedsContainer() {
		t.Error("a declarative service must provision a container")
	}
}

func TestRegistryGetKnownApplication(t *testing.T) {
	r := testRegistry(t)
	application, ok := r.Get(fixtureService)
	if !ok {
		t.Fatal("expected the fixture service application")
	}
	if !application.SupportsScope(svc.ScopeGlobal) || !application.SupportsScope(svc.ScopeProject) {
		t.Errorf("application should support both scopes, got %v", application.Scopes)
	}
	if application.Port.Internal != 5432 {
		t.Errorf("internal port = %d, want 5432", application.Port.Internal)
	}
}
