package applications

import (
	"context"
	"encoding/json"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	containerapplications "github.com/futrx-com/remote.futrx.com/internal/integration/containers/applications"
	svc "github.com/futrx-com/remote.futrx.com/internal/service/applications"
	"github.com/futrx-com/remote.futrx.com/pkg/applications"
)

// catalogBackendMain is a whole installable application's backend, written against the
// public applications contract exactly as a distributed backend package is. It is
// deliberately not one of the applications this repository ships: installable applications
// are separately distributed packages, so the seam that has to keep working is
// "a catalog entry, whatever it is, compiles and serves" — not "this particular
// backend still exists".
const catalogBackendMain = `package main

import (
	"encoding/json"
	"net/http"
	"os"

	"github.com/futrx-com/remote.futrx.com/pkg/applications"
	"github.com/futrx-com/remote.futrx.com/pkg/applications/rpc"
)

type backend struct {
	router   *applications.Router
	instance applications.Instance
}

func main() {
	b := &backend{router: applications.NewRouter()}
	b.router.GET("health", "Liveness and process identity", b.health)
	b.router.GET("admin", "Admin-only route", b.admin)
	b.router.GET("boom", "Deliberate panic", b.boom)
	b.router.POST("echo", "Round-trip a value through the process", b.echo)
	rpc.Serve(b)
}

func (b *backend) Describe() (applications.Descriptor, error) {
	return applications.Descriptor{Name: "catalog-fixture", Version: "1", APIVersion: applications.APIVersion, Routes: b.router.Routes()}, nil
}
func (b *backend) Init(instance applications.Instance) error { b.instance = instance; return nil }
func (b *backend) Handle(r applications.Request) (applications.Response, error) {
	return b.router.Serve(r), nil
}

func (b *backend) health(applications.Request) applications.Response {
	return applications.JSON(http.StatusOK, map[string]any{"ok": true, "pid": os.Getpid(), "instance": b.instance.ID})
}

// The backend authorizes its own callers; the host only decided that the caller
// may reach the backend at all.
func (b *backend) admin(r applications.Request) applications.Response {
	if !r.Caller.IsAdmin {
		return applications.Errorf(http.StatusForbidden, "admins only")
	}
	return applications.JSON(http.StatusOK, map[string]any{"ok": true})
}

func (b *backend) boom(applications.Request) applications.Response { panic("deliberate") }

func (b *backend) echo(r applications.Request) applications.Response {
	var body struct {
		Value string ` + "`json:\"value\"`" + `
	}
	if err := json.Unmarshal(r.Body, &body); err != nil {
		return applications.Errorf(http.StatusBadRequest, "invalid body")
	}
	return applications.JSON(http.StatusOK, map[string]any{"value": body.Value})
}
`

// This is the end-to-end proof: an application read through the real catalog loader,
// compiled and run by the host the server uses, answering on the routes it
// advertises. The synthetic catalogs elsewhere in this package hand the host a
// backend source directly; this one makes it go through the registry first.
func TestBackendFromTheImageCatalogCompilesAndServes(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles a backend with the Go toolchain")
	}
	if _, err := findGoTool(testGoToolOverride()); err != nil {
		t.Skipf("no Go toolchain available: %v", err)
	}
	file := func(data string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(data)} }
	registry, err := containerapplications.NewRegistry(fstest.MapFS{
		"applications/catalog-fixture/application.json": file(`{
			"name": "Catalog Fixture",
			"version": "1.0.0",
			"scopes": ["global", "project"],
			"backend": {"access": "registered", "timeoutMs": 10000}
		}`),
		"applications/catalog-fixture/backend/api/main.go": file(catalogBackendMain),
	}, nil)
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	application, ok := registry.Get("catalog-fixture")
	if !ok || application.Backend == nil {
		t.Fatal("the fixture application did not load with a backend")
	}

	host := New(sharedRoot(t), registry, Options{GoTool: testGoToolOverride()})
	t.Cleanup(host.Shutdown)
	spec := applications.Instance{
		ID:                 "catalog-e2e",
		ApplicationID:      application.ID,
		ApplicationName:    application.Name,
		ApplicationVersion: application.Version,
		Scope:              string(svc.ScopeGlobal),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	descriptor, err := host.Ensure(ctx, spec)
	if err != nil {
		t.Fatalf("start the catalog backend: %v", err)
	}
	if descriptor.Name != application.Name || descriptor.Version != application.Version || descriptor.APIVersion != applications.APIVersion {
		t.Fatalf("descriptor = %+v", descriptor)
	}
	// Routes are discovery, so the SPA can only find what the backend declares.
	if len(descriptor.Routes) != 4 {
		t.Errorf("%d routes advertised: %+v", len(descriptor.Routes), descriptor.Routes)
	}

	admin := applications.Caller{Email: "admin@example.com", IsAdmin: true}
	get := func(path string) map[string]any {
		t.Helper()
		return decode(t, host, spec, applications.Request{Method: "GET", Path: path, Caller: admin})
	}

	health := get("health")
	if health["ok"] != true || health["pid"] == nil {
		t.Errorf("health = %v", health)
	}
	if health["instance"] != spec.ID {
		t.Errorf("instance = %v, want the one Init was given", health["instance"])
	}

	body, err := json.Marshal(map[string]string{"value": "kept"})
	if err != nil {
		t.Fatal(err)
	}
	if echoed := decode(t, host, spec, applications.Request{
		Method: "POST", Path: "echo", Body: body, Caller: admin,
	}); echoed["value"] != "kept" {
		t.Errorf("round-trip returned %v", echoed)
	}

	// A panicking route is reported as a failed call, not a dead host.
	if _, err := host.Call(ctx, spec, applications.Request{
		Method: "GET", Path: "boom", Caller: admin,
	}); err == nil || !strings.Contains(err.Error(), "panic") {
		t.Errorf("the panicking route reported %v", err)
	}
	unknown, err := host.Call(ctx, spec, applications.Request{
		Method: "GET", Path: "no/such/route", Caller: admin,
	})
	if err != nil || unknown.Status != 404 {
		t.Errorf("unknown route = %d, %v", unknown.Status, err)
	}

	// The backend authorizes its own callers; the platform only gated access to
	// the backend as a whole.
	refused, err := host.Call(ctx, spec, applications.Request{
		Method: "GET", Path: "admin", Caller: applications.Caller{Email: "user@example.com"},
	})
	if err != nil || refused.Status != 403 {
		t.Errorf("admin route for an ordinary caller = %d, %v", refused.Status, err)
	}

	// The process survived every one of those.
	if after := get("health"); after["pid"] != health["pid"] {
		t.Errorf("the backend restarted during the run: %v then %v", health["pid"], after["pid"])
	}
}

func decode(t *testing.T, host *Host, instance applications.Instance, request applications.Request) map[string]any {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	response, err := host.Call(ctx, instance, request)
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

// The same proof for the layered host/container layout: backend/main.go is the
// composition root, api/ and lifecycle/ are importable siblings, and
// container/ stays out of the host build entirely.
func TestBackendWithTheLayeredLayoutCompilesAndServes(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles a backend with the Go toolchain")
	}
	if _, err := findGoTool(testGoToolOverride()); err != nil {
		t.Skipf("no Go toolchain available: %v", err)
	}
	file := func(data string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(data)} }
	rootMain := `package main

import (
	appapi "futrx.local/catalog/applications/api-fixture/backend/api"
	_ "futrx.local/catalog/applications/api-fixture/backend/lifecycle"
	"github.com/futrx-com/remote.futrx.com/pkg/applications/rpc"
)

func main() { rpc.Serve(appapi.New()) }
`
	apiSource := `package api

import (
	"net/http"
	"github.com/futrx-com/remote.futrx.com/pkg/applications"
)

type backend struct{}

func New() applications.Backend { return &backend{} }
func (*backend) Describe() (applications.Descriptor, error) {
	return applications.Descriptor{
		APIVersion: applications.APIVersion,
		Routes: []applications.Route{{Method: "GET", Path: "health"}},
	}, nil
}
func (*backend) Init(applications.Instance) error { return nil }
func (*backend) Handle(request applications.Request) (applications.Response, error) {
	if request.Path == "health" {
		return applications.JSON(http.StatusOK, map[string]any{"ok": true}), nil
	}
	return applications.Errorf(http.StatusNotFound, "not found"), nil
}
`
	registry, err := containerapplications.NewRegistry(fstest.MapFS{
		"applications/api-fixture/application.json": file(`{
			"name": "API Fixture",
			"version": "1.0.0",
			"scopes": ["global", "project"],
			"backend": {"access": "registered", "timeoutMs": 10000}
		}`),
		"applications/api-fixture/backend/main.go":    file(rootMain),
		"applications/api-fixture/backend/api/api.go": file(apiSource),
		"applications/api-fixture/backend/lifecycle/events.go": file(
			"package lifecycle\n\nconst Ready = true\n"),
		// Not package main, and referencing nothing the host build provides.
		// Reaching the compiler at all would fail this test.
		"applications/api-fixture/backend/container/cmd/agent/main.go": file(
			"package main\n\nfunc main() { panic(\"never built on the host\") }\n"),
		"applications/api-fixture/backend/container/internal/info/info.go": file(
			"package info\n\nfunc Read() string { return \"\" }\n"),
	}, nil)
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	application, ok := registry.Get("api-fixture")
	if !ok || application.Backend == nil {
		t.Fatal("the api-layout application did not load with a backend")
	}

	source, ok := registry.BackendSource("api-fixture")
	if !ok {
		t.Fatal("no backend source")
	}
	if _, err := fs.Stat(source, "main.go"); err != nil {
		t.Errorf("backend source has no composition root: %v", err)
	}
	if _, err := fs.Stat(source, "api/api.go"); err != nil {
		t.Errorf("backend source omitted its API package: %v", err)
	}
	if _, err := fs.Stat(source, "lifecycle/events.go"); err != nil {
		t.Errorf("backend source omitted its lifecycle package: %v", err)
	}
	if _, err := fs.Stat(source, "container"); err == nil {
		t.Error("container source reached the host build tree")
	}

	host := New(sharedRoot(t), registry, Options{GoTool: testGoToolOverride()})
	t.Cleanup(host.Shutdown)
	spec := applications.Instance{
		ID:                 "api-layout-e2e",
		ApplicationID:      application.ID,
		ApplicationName:    application.Name,
		ApplicationVersion: application.Version,
		Scope:              string(svc.ScopeGlobal),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	descriptor, err := host.Ensure(ctx, spec)
	if err != nil {
		t.Fatalf("start the api-layout backend: %v", err)
	}
	if descriptor.Name != application.Name || descriptor.Version != application.Version || descriptor.APIVersion != applications.APIVersion {
		t.Fatalf("descriptor = %+v", descriptor)
	}
	health := decode(t, host, spec, applications.Request{
		Method: "GET", Path: "health",
		Caller: applications.Caller{Email: "admin@example.com", IsAdmin: true},
	})
	if health["ok"] != true {
		t.Errorf("health = %v", health)
	}
}
