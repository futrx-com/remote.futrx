package applications

import (
	"testing"

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
		if img.Port.Internal <= 0 {
			t.Errorf("image %s has invalid internal port %d", img.ID, img.Port.Internal)
		}
		if _, ok := r.Script(img.ID); !ok {
			t.Errorf("image %s missing install script", img.ID)
		}
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
