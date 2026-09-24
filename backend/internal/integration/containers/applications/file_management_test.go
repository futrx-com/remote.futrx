package applications

import (
	"testing"

	svc "github.com/futrx-com/remote.futrx.com/internal/service/applications"
)

func TestFileManagementApplicationContract(t *testing.T) {
	registry, err := NewRegistry(EmbeddedCatalog(), nil)
	if err != nil {
		t.Fatal(err)
	}
	application, ok := registry.Get("file-management")
	if !ok {
		t.Fatal("file-management is missing from the built-in catalog")
	}
	if application.ID != "file-management" || application.Name != "File Management" ||
		application.Version != "1.0.0" || application.Source != svc.SourceBuiltin {
		t.Fatalf("identity = %+v", application)
	}
	if !application.SupportsScope(svc.ScopeGlobal) || !application.SupportsScope(svc.ScopeProject) {
		t.Fatalf("scopes = %v, want global and project", application.Scopes)
	}
	if application.UI == nil || application.UI.Entry != "scripts/main.js" ||
		len(application.UI.Styles) != 1 || application.Backend == nil {
		t.Fatalf("UI/backend contract = ui=%+v backend=%+v", application.UI, application.Backend)
	}
	if application.Backend.Audience() != svc.BackendAccessRegistered ||
		application.Backend.Timeout() != svc.MaxBackendTimeoutMS {
		t.Fatalf("backend policy = %+v", application.Backend)
	}
	if application.NeedsContainer() || application.NeedsPort() || application.Install != "" ||
		application.Service != nil || len(application.Env) != 0 || len(application.Publishers) != 0 ||
		len(application.Subscriptions) != 0 || len(application.Skills) != 0 {
		t.Fatalf("file manager declares unrelated capabilities: %+v", application)
	}
	if _, ok := registry.BackendSource(application.ID); !ok {
		t.Fatal("file-management backend source is missing")
	}
}

func TestProductionDefaultApplicationsAreInstallableWithoutInput(t *testing.T) {
	registry, err := NewRegistry(EmbeddedCatalog(), nil)
	if err != nil {
		t.Fatal(err)
	}
	ids := svc.DefaultApplicationIDs()
	if len(ids) == 0 {
		t.Fatal("production default application policy is empty")
	}
	for _, id := range ids {
		application, ok := registry.Get(id)
		if !ok {
			t.Errorf("default application %q is missing from the built-in catalog", id)
			continue
		}
		if application.Source != svc.SourceBuiltin {
			t.Errorf("default application %q source = %q, want built-in", id, application.Source)
		}
		if !application.SupportsScope(svc.ScopeGlobal) {
			t.Errorf("default application %q does not support global scope", id)
		}
		for _, input := range application.Env {
			if input.Required && input.Default == "" && input.Generate == "" {
				t.Errorf("default application %q requires interactive input %q", id, input.Key)
			}
		}
	}
}
