package httphandlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	servicetemplates "github.com/futrx-com/remote.futrx.com/internal/service/container/templates"
)

// templateRuntimeStub answers the two probes the template service makes when
// listing: it never runs anything, so the handler test stays hermetic.
type templateRuntimeStub struct {
	available bool
	images    map[string]bool
	imageErr  error
}

func (s templateRuntimeStub) Available() bool { return s.available }

func (s templateRuntimeStub) FileExists(context.Context, string, string) (bool, error) {
	return false, nil
}

func (s templateRuntimeStub) EnsureDirectory(context.Context, string, string) error { return nil }

func (s templateRuntimeStub) PushFile(context.Context, string, []byte, string, string) error {
	return nil
}

func (s templateRuntimeStub) RunScript(context.Context, string, string) (string, error) {
	return "", nil
}

func (s templateRuntimeStub) ImageExists(_ context.Context, alias string) (bool, error) {
	if s.imageErr != nil {
		return false, s.imageErr
	}
	return s.images[alias], nil
}

func newTemplateTestHandler(runtime servicetemplates.Runtime) *TemplateHandler {
	return NewTemplateHandler(servicetemplates.NewService(servicetemplates.MustLoad(), runtime))
}

func TestTemplateHandlerListsTheShippedCatalog(t *testing.T) {
	handler := newTemplateTestHandler(templateRuntimeStub{
		available: true,
		images:    map[string]bool{"futrx-remote-wordpress-base": true},
	})

	request := httptest.NewRequest(http.MethodGet, "/api/templates", nil)
	response := httptest.NewRecorder()
	handler.HandleCollection(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	var body []servicetemplates.Descriptor
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body) == 0 {
		t.Fatal("empty catalog")
	}
	if body[0].Name != "blank" || !body[0].Default {
		t.Fatalf("first entry = %+v, want the default template first", body[0])
	}
	byName := map[string]servicetemplates.Descriptor{}
	for _, descriptor := range body {
		byName[descriptor.Name] = descriptor
		if descriptor.Title == "" || descriptor.Description == "" || descriptor.Icon == "" {
			t.Fatalf("descriptor %+v is missing presentation metadata", descriptor)
		}
	}
	wordpress, ok := byName["wordpress"]
	if !ok {
		t.Fatalf("wordpress missing from %+v", body)
	}
	if wordpress.PrebuiltImage != "futrx-remote-wordpress-base" || !wordpress.PrebuiltImageAvailable {
		t.Fatalf("wordpress = %+v, want the published image reported", wordpress)
	}
	laravel := byName["laravel"]
	if laravel.PrebuiltImage != "futrx-remote-laravel-base" || laravel.PrebuiltImageAvailable {
		t.Fatalf("laravel = %+v, want its unpublished image reported as unavailable", laravel)
	}
	if byName["node"].PrebuiltImage != "" {
		t.Fatalf("node = %+v, want no dedicated image", byName["node"])
	}
}

func TestTemplateHandlerReportsNoPrebuiltImagesWithoutARuntime(t *testing.T) {
	handler := newTemplateTestHandler(templateRuntimeStub{available: false})

	request := httptest.NewRequest(http.MethodGet, "/api/templates", nil)
	response := httptest.NewRecorder()
	handler.HandleCollection(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	var body []servicetemplates.Descriptor
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	for _, descriptor := range body {
		if descriptor.PrebuiltImageAvailable {
			t.Fatalf("%q reported an available image without a runtime", descriptor.Name)
		}
	}
}

func TestTemplateHandlerTreatsAnImageProbeFailureAsUnavailable(t *testing.T) {
	handler := newTemplateTestHandler(templateRuntimeStub{
		available: true,
		imageErr:  errors.New("lxd is down"),
	})

	request := httptest.NewRequest(http.MethodGet, "/api/templates", nil)
	response := httptest.NewRecorder()
	handler.HandleCollection(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; a failing probe must not fail the list", response.Code, http.StatusOK)
	}
}

func TestTemplateHandlerRejectsNonGET(t *testing.T) {
	handler := newTemplateTestHandler(templateRuntimeStub{available: true})
	request := httptest.NewRequest(http.MethodPost, "/api/templates", nil)
	response := httptest.NewRecorder()

	handler.HandleCollection(response, request)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
}

func TestTemplateHandlerWithoutAService(t *testing.T) {
	handler := NewTemplateHandler(nil)
	request := httptest.NewRequest(http.MethodGet, "/api/templates", nil)
	response := httptest.NewRecorder()

	handler.HandleCollection(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
}

func TestTemplateHandlerRegistersItsRoute(t *testing.T) {
	mux := http.NewServeMux()
	newTemplateTestHandler(templateRuntimeStub{available: true}).RegisterRoutes(mux)

	request := httptest.NewRequest(http.MethodGet, "/api/templates", nil)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
}
