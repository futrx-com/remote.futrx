package pluginhost

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	containerapplications "github.com/futrx-com/remote.futrx.com/internal/integration/containers/applications"
	svc "github.com/futrx-com/remote.futrx.com/internal/service/applications"
	"github.com/futrx-com/remote.futrx.com/pkg/appplugin"
)

// catalogPluginMain is a whole installable image's backend, written against the
// public appplugin contract exactly as a distributed plugin package is. It is
// deliberately not one of the images this repository ships: installable images
// are separately distributed packages, so the seam that has to keep working is
// "a catalog entry, whatever it is, compiles and serves" — not "this particular
// plugin still exists".
const catalogPluginMain = `package main

import (
	"encoding/json"
	"net/http"
	"os"

	"github.com/futrx-com/remote.futrx.com/pkg/appplugin"
	"github.com/futrx-com/remote.futrx.com/pkg/appplugin/pluginrpc"
)

type backend struct {
	mux      *appplugin.Mux
	instance appplugin.Instance
}

func main() {
	b := &backend{mux: appplugin.NewMux()}
	b.mux.GET("health", "Liveness and process identity", b.health)
	b.mux.GET("admin", "Admin-only route", b.admin)
	b.mux.GET("boom", "Deliberate panic", b.boom)
	b.mux.POST("echo", "Round-trip a value through the process", b.echo)
	pluginrpc.Serve(b)
}

func (b *backend) Describe() (appplugin.Descriptor, error) {
	return appplugin.Descriptor{Name: "catalog-fixture", Version: "1", APIVersion: appplugin.APIVersion, Routes: b.mux.Routes()}, nil
}
func (b *backend) Init(instance appplugin.Instance) error { b.instance = instance; return nil }
func (b *backend) Handle(r appplugin.Request) (appplugin.Response, error) {
	return b.mux.Serve(r), nil
}

func (b *backend) health(appplugin.Request) appplugin.Response {
	return appplugin.JSON(http.StatusOK, map[string]any{"ok": true, "pid": os.Getpid(), "instance": b.instance.ID})
}

// The plugin authorizes its own callers; the host only decided that the caller
// may reach the plugin at all.
func (b *backend) admin(r appplugin.Request) appplugin.Response {
	if !r.Caller.IsAdmin {
		return appplugin.Errorf(http.StatusForbidden, "admins only")
	}
	return appplugin.JSON(http.StatusOK, map[string]any{"ok": true})
}

func (b *backend) boom(appplugin.Request) appplugin.Response { panic("deliberate") }

func (b *backend) echo(r appplugin.Request) appplugin.Response {
	var body struct {
		Value string ` + "`json:\"value\"`" + `
	}
	if err := json.Unmarshal(r.Body, &body); err != nil {
		return appplugin.Errorf(http.StatusBadRequest, "invalid body")
	}
	return appplugin.JSON(http.StatusOK, map[string]any{"value": body.Value})
}
`

// This is the end-to-end proof: an image read through the real catalog loader,
// compiled and run by the host the server uses, answering on the routes it
// advertises. The synthetic catalogs elsewhere in this package hand the host a
// plugin source directly; this one makes it go through the registry first.
func TestPluginFromTheImageCatalogCompilesAndServes(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles a plugin with the Go toolchain")
	}
	if _, err := findGoTool(testGoToolOverride()); err != nil {
		t.Skipf("no Go toolchain available: %v", err)
	}
	file := func(data string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(data)} }
	registry, err := containerapplications.NewRegistryFromFS(fstest.MapFS{
		"images/catalog-fixture/image.json": file(`{
			"name": "Catalog Fixture",
			"version": "1.0.0",
			"type": "backend",
			"scopes": ["global", "project"],
			"backend": {"access": "registered", "timeoutMs": 10000}
		}`),
		"images/catalog-fixture/plugin/main.go": file(catalogPluginMain),
	})
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	image, ok := registry.Get("catalog-fixture")
	if !ok || image.Backend == nil {
		t.Fatal("the fixture image did not load with a backend")
	}

	host := New(sharedRoot(t), registry, Options{GoTool: testGoToolOverride()})
	t.Cleanup(host.Shutdown)
	spec := svc.BackendSpec{
		ImageID: image.ID,
		Instance: appplugin.Instance{
			ID:      "catalog-e2e",
			ImageID: image.ID,
			Scope:   string(svc.ScopeGlobal),
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	descriptor, err := host.Ensure(ctx, spec)
	if err != nil {
		t.Fatalf("start the catalog plugin: %v", err)
	}
	if descriptor.APIVersion != appplugin.APIVersion {
		t.Fatalf("descriptor = %+v", descriptor)
	}
	// Routes are discovery, so the SPA can only find what the plugin declares.
	if len(descriptor.Routes) != 4 {
		t.Errorf("%d routes advertised: %+v", len(descriptor.Routes), descriptor.Routes)
	}

	admin := appplugin.Caller{Email: "admin@example.com", IsAdmin: true}
	get := func(path string) map[string]any {
		t.Helper()
		return decode(t, host, spec, appplugin.Request{Method: "GET", Path: path, Caller: admin})
	}

	health := get("health")
	if health["ok"] != true || health["pid"] == nil {
		t.Errorf("health = %v", health)
	}
	if health["instance"] != spec.Instance.ID {
		t.Errorf("instance = %v, want the one Init was given", health["instance"])
	}

	body, err := json.Marshal(map[string]string{"value": "kept"})
	if err != nil {
		t.Fatal(err)
	}
	if echoed := decode(t, host, spec, appplugin.Request{
		Method: "POST", Path: "echo", Body: body, Caller: admin,
	}); echoed["value"] != "kept" {
		t.Errorf("round-trip returned %v", echoed)
	}

	// A panicking route is reported as a failed call, not a dead host.
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
