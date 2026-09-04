package pluginhost

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	containerapplications "github.com/futrx-com/remote.futrx.com/internal/integration/containers/applications"
	svc "github.com/futrx-com/remote.futrx.com/internal/service/applications"
	"github.com/futrx-com/remote.futrx.com/pkg/appplugin"
)

// This is the end-to-end proof: the catalog the server embeds, compiled and
// run by the host the server uses, answering on the routes it advertises. The
// synthetic plugins elsewhere in this package check the machinery; this checks
// that what actually ships works.
func TestBackendPlaygroundRunsFromTheEmbeddedCatalog(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles a plugin with the Go toolchain")
	}
	if _, err := findGoTool(testGoToolOverride()); err != nil {
		t.Skipf("no Go toolchain available: %v", err)
	}
	registry, err := containerapplications.NewRegistry()
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	image, ok := registry.Get("backend-playground")
	if !ok || image.Backend == nil {
		t.Fatal("backend-playground is missing from the catalog")
	}

	host := New(sharedRoot(t), registry, Options{GoTool: testGoToolOverride()})
	t.Cleanup(host.Shutdown)
	spec := svc.BackendSpec{
		ImageID: image.ID,
		Instance: appplugin.Instance{
			ID:      "playground-e2e",
			ImageID: image.ID,
			Scope:   string(svc.ScopeGlobal),
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	descriptor, err := host.Ensure(ctx, spec)
	if err != nil {
		t.Fatalf("start the playground plugin: %v", err)
	}
	if descriptor.APIVersion != appplugin.APIVersion {
		t.Fatalf("descriptor = %+v", descriptor)
	}
	// The panel and the self-test both render this table, so an empty one is a
	// broken fixture rather than a cosmetic problem.
	if len(descriptor.Routes) < 10 {
		t.Errorf("only %d routes advertised: %+v", len(descriptor.Routes), descriptor.Routes)
	}

	admin := appplugin.Caller{Email: "admin@example.com", IsAdmin: true}
	get := func(path string) map[string]any {
		t.Helper()
		return decode(t, host, spec, appplugin.Request{Method: "GET", Path: path, Caller: admin})
	}
	post := func(path string, body any) map[string]any {
		t.Helper()
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("encode body: %v", err)
		}
		return decode(t, host, spec, appplugin.Request{
			Method: "POST", Path: path, Body: raw, Caller: admin,
		})
	}

	health := get("health")
	if health["ok"] != true || health["pid"] == nil {
		t.Errorf("health = %v", health)
	}

	// State in the process, then state on disk: the two things a plugin can do
	// that a browser extension cannot.
	post("kv/e2e", map[string]string{"value": "kept"})
	if read := get("kv/e2e"); read["value"] != "kept" {
		t.Errorf("kv round-trip returned %v", read)
	}
	post("notes", map[string]string{"note": "from the end-to-end test"})
	note := get("notes")
	if note["note"] != "from the end-to-end test" || note["saved"] != true {
		t.Errorf("note round-trip returned %v", note)
	}

	if computed := post("compute", map[string]int{"n": 30}); computed["fibonacci"] != float64(832040) {
		t.Errorf("compute returned %v", computed)
	}

	// The two deliberate failures the fixture exists to demonstrate.
	if _, err := host.Call(ctx, spec, appplugin.Request{
		Method: "GET", Path: "boom", Caller: admin,
	}); err == nil || !strings.Contains(err.Error(), "panic") {
		t.Errorf("the panicking route reported %v", err)
	}
	unknown, err := host.Call(ctx, spec, appplugin.Request{
		Method: "GET", Path: "no/such/route", Caller: admin,
	})
	if err != nil || unknown.Status != 404 {
		t.Errorf("unknown route = %d, %v", unknown.Status, err)
	}

	// The plugin authorizes its own callers; the platform only gated access to
	// the plugin as a whole.
	refused, err := host.Call(ctx, spec, appplugin.Request{
		Method: "GET", Path: "admin", Caller: appplugin.Caller{Email: "user@example.com"},
	})
	if err != nil || refused.Status != 403 {
		t.Errorf("admin route for an ordinary caller = %d, %v", refused.Status, err)
	}

	// The process survived every one of those.
	if after := get("health"); after["pid"] != health["pid"] {
		t.Errorf("the plugin restarted during the run: %v then %v", health["pid"], after["pid"])
	}
}

func decode(t *testing.T, host *Host, spec svc.BackendSpec, request appplugin.Request) map[string]any {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	response, err := host.Call(ctx, spec, request)
	if err != nil {
		t.Fatalf("%s %s: %v", request.Method, request.Path, err)
	}
	if response.Status != 200 {
		t.Fatalf("%s %s: status %d, body %s", request.Method, request.Path, response.Status, response.Body)
	}
	var payload map[string]any
	if err := json.Unmarshal(response.Body, &payload); err != nil {
		t.Fatalf("%s %s: body %s is not JSON: %v", request.Method, request.Path, response.Body, err)
	}
	return payload
}
