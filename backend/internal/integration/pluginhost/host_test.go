package pluginhost

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	svc "github.com/futrx-com/remote.futrx.com/internal/service/applications"
	"github.com/futrx-com/remote.futrx.com/pkg/appplugin"
)

// These tests compile and run a real plugin process, because the parts most
// likely to break — the generated module, the handshake, the RPC shapes — are
// exactly the parts a mock would replace. One build is shared by every test in
// the package through the builder's fingerprint cache.

// testPluginSource is a complete plugin, held as source because that is what
// the catalog ships and what the host consumes.
const testPluginSource = `package main

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/futrx-com/remote.futrx.com/pkg/appplugin"
	"github.com/futrx-com/remote.futrx.com/pkg/appplugin/pluginrpc"
)

type backend struct{ instance appplugin.Instance }

func (b *backend) Describe() (appplugin.Descriptor, error) {
	return appplugin.Descriptor{
		Name:       "test",
		Version:    "1",
		APIVersion: appplugin.APIVersion,
		Routes:     []appplugin.Route{{Method: "GET", Path: "pid"}},
	}, nil
}

func (b *backend) Init(instance appplugin.Instance) error {
	b.instance = instance
	return nil
}

func (b *backend) Handle(request appplugin.Request) (appplugin.Response, error) {
	switch request.Path {
	case "pid":
		return appplugin.JSON(http.StatusOK, map[string]any{
			"pid":      os.Getpid(),
			"instance": b.instance.ID,
			"project":  b.instance.ProjectID,
			"env":      b.instance.Env,
			"caller":   request.Caller.Email,
			"admin":    request.Caller.IsAdmin,
			"query":    request.QueryValue("q"),
			"body":     string(request.Body),
		}), nil
	case "write":
		path := filepath.Join(b.instance.DataDir, "kept.txt")
		if err := os.WriteFile(path, request.Body, 0o600); err != nil {
			return appplugin.Errorf(http.StatusInternalServerError, "%v", err), nil
		}
		return appplugin.Text(http.StatusOK, path), nil
	case "read":
		data, err := os.ReadFile(filepath.Join(b.instance.DataDir, "kept.txt"))
		if err != nil {
			return appplugin.Errorf(http.StatusNotFound, "%v", err), nil
		}
		return appplugin.Text(http.StatusOK, string(data)), nil
	case "boom":
		panic("deliberate")
	case "slow":
		select {}
	}
	return appplugin.Errorf(http.StatusNotFound, "no route %q", request.Path), nil
}

func main() { pluginrpc.Serve(&backend{}) }
`

// wrongVersionSource reports a contract version the host does not speak.
const wrongVersionSource = `package main

import (
	"github.com/futrx-com/remote.futrx.com/pkg/appplugin"
	"github.com/futrx-com/remote.futrx.com/pkg/appplugin/pluginrpc"
)

type backend struct{}

func (backend) Describe() (appplugin.Descriptor, error) {
	return appplugin.Descriptor{Name: "old", APIVersion: appplugin.APIVersion + 1}, nil
}
func (backend) Init(appplugin.Instance) error { return nil }
func (backend) Handle(appplugin.Request) (appplugin.Response, error) {
	return appplugin.Response{}, nil
}

func main() { pluginrpc.Serve(backend{}) }
`

type fakeCatalog map[string]fs.FS

func (c fakeCatalog) PluginSource(imageID string) (fs.FS, bool) {
	source, ok := c[imageID]
	return source, ok
}

func sourceFS(main string) fs.FS {
	return fstest.MapFS{"main.go": &fstest.MapFile{Data: []byte(main)}}
}

// testGoToolOverride mirrors the application edge's configuration injection.
// It keeps the tests able to select a wrapper toolchain without teaching the
// production integration how to read environment variables.
func testGoToolOverride() string {
	return os.Getenv("REMOTE_PLUGIN_GO")
}

func testSpec(imageID, instanceID string) svc.BackendSpec {
	return svc.BackendSpec{
		ImageID: imageID,
		Instance: appplugin.Instance{
			ID:        instanceID,
			ImageID:   imageID,
			Scope:     string(svc.ScopeProject),
			ProjectID: "project-1",
			Env:       map[string]string{"TOKEN": "secret-value"},
		},
	}
}

// newTestHost builds a host over a temporary root. The build cache is shared
// across tests in one run via t.TempDir's parent, so only the first test in a
// package pays for compilation.
func newTestHost(t *testing.T, catalog fakeCatalog) *Host {
	t.Helper()
	if testing.Short() {
		t.Skip("compiles a plugin with the Go toolchain")
	}
	if _, err := findGoTool(testGoToolOverride()); err != nil {
		t.Skipf("no Go toolchain available: %v", err)
	}
	host := New(sharedRoot(t), catalog, Options{GoTool: testGoToolOverride()})
	t.Cleanup(host.Shutdown)
	return host
}

// sharedRoot keeps one build cache for the whole package so the plugin is
// compiled once rather than once per test.
var packageRoot string

func sharedRoot(t *testing.T) string {
	t.Helper()
	if packageRoot == "" {
		root, err := os.MkdirTemp("", "pluginhost-test-")
		if err != nil {
			t.Fatalf("temp root: %v", err)
		}
		packageRoot = root
	}
	return packageRoot
}

func TestMain(m *testing.M) {
	code := m.Run()
	if packageRoot != "" {
		_ = os.RemoveAll(packageRoot)
	}
	os.Exit(code)
}

func call(t *testing.T, host *Host, spec svc.BackendSpec, request appplugin.Request) appplugin.Response {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	response, err := host.Call(ctx, spec, request)
	if err != nil {
		t.Fatalf("call %q: %v", request.Path, err)
	}
	return response
}

func TestHostCompilesAndServesAPlugin(t *testing.T) {
	host := newTestHost(t, fakeCatalog{"test-image": sourceFS(testPluginSource)})
	spec := testSpec("test-image", "instance-1")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	descriptor, err := host.Ensure(ctx, spec)
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if descriptor.Name != "test" || descriptor.APIVersion != appplugin.APIVersion {
		t.Fatalf("descriptor = %+v", descriptor)
	}
	if len(descriptor.Routes) != 1 || descriptor.Routes[0].Path != "pid" {
		t.Errorf("routes = %+v", descriptor.Routes)
	}

	response := call(t, host, spec, appplugin.Request{
		Method: "POST",
		Path:   "pid",
		Query:  map[string][]string{"q": {"asked"}},
		Body:   []byte("payload"),
		Caller: appplugin.Caller{Email: "admin@example.com", IsAdmin: true},
	})
	if response.Status != 200 {
		t.Fatalf("status = %d, body %s", response.Status, response.Body)
	}
	// The plugin sees the instance it was initialized with and the caller the
	// service stamped, including the install's secret env — that combination is
	// the whole reason a backend plugin can do anything useful.
	for _, want := range []string{
		`"instance":"instance-1"`,
		`"project":"project-1"`,
		`"caller":"admin@example.com"`,
		`"admin":true`,
		`"query":"asked"`,
		`"body":"payload"`,
		`"TOKEN":"secret-value"`,
	} {
		if !strings.Contains(string(response.Body), want) {
			t.Errorf("response %s does not contain %s", response.Body, want)
		}
	}
}

// One process per instance is the contract the whole feature rests on: state a
// plugin keeps between requests is only meaningful if the process is the same.
func TestHostReusesOneProcessPerInstance(t *testing.T) {
	host := newTestHost(t, fakeCatalog{"test-image": sourceFS(testPluginSource)})
	spec := testSpec("test-image", "instance-reuse")

	first := string(call(t, host, spec, appplugin.Request{Method: "GET", Path: "pid"}).Body)
	second := string(call(t, host, spec, appplugin.Request{Method: "GET", Path: "pid"}).Body)
	if first != second {
		t.Errorf("two calls hit different processes:\n%s\n%s", first, second)
	}

	other := testSpec("test-image", "instance-other")
	third := string(call(t, host, other, appplugin.Request{Method: "GET", Path: "pid"}).Body)
	if third == first {
		t.Error("two instances share one process; they must not")
	}
}

func TestHostStopEndsTheProcessAndCallRestartsIt(t *testing.T) {
	host := newTestHost(t, fakeCatalog{"test-image": sourceFS(testPluginSource)})
	spec := testSpec("test-image", "instance-restart")

	before := string(call(t, host, spec, appplugin.Request{Method: "GET", Path: "pid"}).Body)
	if err := host.Stop(context.Background(), spec.Instance.ID); err != nil {
		t.Fatalf("stop: %v", err)
	}
	// A call after a stop starts the plugin again rather than failing, which is
	// what lets installed backends survive a server restart with no sweep.
	after := string(call(t, host, spec, appplugin.Request{Method: "GET", Path: "pid"}).Body)
	if before == after {
		t.Errorf("stop did not end the process: %s", after)
	}
}

// Stop keeps a plugin's data; only Remove discards it. That split is what
// makes stop and start safe to use freely on an app someone relies on.
func TestStopKeepsPluginDataAndRemoveDiscardsIt(t *testing.T) {
	host := newTestHost(t, fakeCatalog{"test-image": sourceFS(testPluginSource)})
	spec := testSpec("test-image", "instance-data")

	written := call(t, host, spec, appplugin.Request{
		Method: "POST", Path: "write", Body: []byte("durable"),
	})
	if written.Status != 200 {
		t.Fatalf("write: %s", written.Body)
	}
	if err := host.Stop(context.Background(), spec.Instance.ID); err != nil {
		t.Fatalf("stop: %v", err)
	}
	read := call(t, host, spec, appplugin.Request{Method: "GET", Path: "read"})
	if string(read.Body) != "durable" {
		t.Errorf("after stop, read = %q (%d)", read.Body, read.Status)
	}

	dataDir := host.dataDir(spec.Instance.ID)
	if err := host.Remove(context.Background(), spec.Instance.ID); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := os.Stat(dataDir); !os.IsNotExist(err) {
		t.Errorf("data directory %s survived uninstall", dataDir)
	}
	gone := call(t, host, spec, appplugin.Request{Method: "GET", Path: "read"})
	if gone.Status != 404 {
		t.Errorf("after remove, read = %d %s", gone.Status, gone.Body)
	}
}

// A panicking route must cost one request, not the process. Every other
// request in flight on that plugin depends on it.
func TestPanickingRouteFailsOneCallAndKeepsTheProcess(t *testing.T) {
	host := newTestHost(t, fakeCatalog{"test-image": sourceFS(testPluginSource)})
	spec := testSpec("test-image", "instance-panic")

	before := string(call(t, host, spec, appplugin.Request{Method: "GET", Path: "pid"}).Body)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if _, err := host.Call(ctx, spec, appplugin.Request{Method: "GET", Path: "boom"}); err == nil {
		t.Fatal("a panicking route reported success")
	} else if !strings.Contains(err.Error(), "panicked") {
		t.Errorf("error = %v, want it to mention the panic", err)
	}

	after := string(call(t, host, spec, appplugin.Request{Method: "GET", Path: "pid"}).Body)
	if before != after {
		t.Errorf("the plugin restarted after a panic:\n%s\n%s", before, after)
	}
}

// A plugin that never answers must not hold the caller's connection: the
// deadline belongs to the host, since the transport has no notion of one.
func TestCallRespectsTheCallerDeadline(t *testing.T) {
	host := newTestHost(t, fakeCatalog{"test-image": sourceFS(testPluginSource)})
	spec := testSpec("test-image", "instance-timeout")

	// Start the plugin first so the deadline covers only the call.
	call(t, host, spec, appplugin.Request{Method: "GET", Path: "pid"})

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	started := time.Now()
	if _, err := host.Call(ctx, spec, appplugin.Request{Method: "GET", Path: "slow"}); err == nil {
		t.Fatal("a call that never answers reported success")
	} else if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("error = %v, want a timeout", err)
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Errorf("the deadline was not enforced: waited %s", elapsed)
	}
}

// A plugin built against a different contract is refused at connect time, so
// the mismatch is one clear error instead of an unreadable failure later.
func TestPluginWithAWrongContractVersionIsRefused(t *testing.T) {
	host := newTestHost(t, fakeCatalog{"old-image": sourceFS(wrongVersionSource)})
	spec := testSpec("old-image", "instance-old")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	_, err := host.Ensure(ctx, spec)
	if err == nil {
		t.Fatal("a mismatched contract version was accepted")
	}
	if !strings.Contains(err.Error(), "contract version") {
		t.Errorf("error = %v", err)
	}
}

func TestEnsureRejectsAnImageWithNoPluginSource(t *testing.T) {
	host := newTestHost(t, fakeCatalog{})
	_, err := host.Ensure(context.Background(), testSpec("missing", "instance-missing"))
	if err == nil {
		t.Fatal("an image with no plugin source was accepted")
	}
}

// The compiled binary is cached by a fingerprint of its inputs, so editing a
// plugin produces a new binary and leaves no stale one behind.
func TestBuildIsCachedByFingerprintAndPrunesStaleBinaries(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles a plugin with the Go toolchain")
	}
	if _, err := findGoTool(testGoToolOverride()); err != nil {
		t.Skipf("no Go toolchain available: %v", err)
	}
	// Share the package build cache: a cold GOCACHE would dominate the runtime.
	builder := NewBuilder(sharedRoot(t), testGoToolOverride())
	ctx := context.Background()

	first, err := builder.Build(ctx, "img", sourceFS(testPluginSource))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	again, err := builder.Build(ctx, "img", sourceFS(testPluginSource))
	if err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	if first != again {
		t.Errorf("identical source produced two binaries:\n%s\n%s", first, again)
	}

	edited, err := builder.Build(ctx, "img", sourceFS(
		strings.Replace(testPluginSource, `Name:       "test"`, `Name:       "edited"`, 1)))
	if err != nil {
		t.Fatalf("build edited: %v", err)
	}
	if edited == first {
		t.Fatal("edited source reused the old binary")
	}
	if _, err := os.Stat(first); !os.IsNotExist(err) {
		t.Errorf("the superseded binary %s was not pruned", filepath.Base(first))
	}
}

func TestBuildReportsCompilerErrors(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles a plugin with the Go toolchain")
	}
	if _, err := findGoTool(testGoToolOverride()); err != nil {
		t.Skipf("no Go toolchain available: %v", err)
	}
	// Share the package build cache: a cold GOCACHE would dominate the runtime.
	builder := NewBuilder(sharedRoot(t), testGoToolOverride())
	_, err := builder.Build(context.Background(), "broken", sourceFS("package main\nfunc main() { undefined() }\n"))
	if err == nil {
		t.Fatal("source that does not compile produced a binary")
	}
	if !strings.Contains(err.Error(), "undefined") {
		t.Errorf("error does not carry the compiler's message: %v", err)
	}
}

// Concurrent calls must not cross: each caller has to get the answer to its
// own request, even while other requests to the same process are in flight and
// some of them are panicking. A plugin serves one instance for every user who
// can reach it, so a crossed response would be one user's data handed to
// another — the one failure here that would be worse than an outage.
func TestConcurrentCallsDoNotCrossResponses(t *testing.T) {
	host := newTestHost(t, fakeCatalog{"test-image": sourceFS(testPluginSource)})
	spec := testSpec("test-image", "instance-concurrent")

	// Start the plugin once so every goroutine below races on calling, not on
	// launching.
	call(t, host, spec, appplugin.Request{Method: "GET", Path: "pid"})

	const callers = 40
	var group sync.WaitGroup
	failures := make(chan string, callers*2)

	for i := range callers {
		group.Add(1)
		go func() {
			defer group.Done()
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()

			// Every other caller asks the plugin to panic, so the well-behaved
			// callers are answering alongside failing ones throughout.
			if i%2 == 1 {
				if _, err := host.Call(ctx, spec, appplugin.Request{
					Method: "GET", Path: "boom",
				}); err == nil || !strings.Contains(err.Error(), "panicked") {
					failures <- fmt.Sprintf("caller %d: boom returned %v", i, err)
				}
				return
			}

			marker := fmt.Sprintf("caller-%d", i)
			response, err := host.Call(ctx, spec, appplugin.Request{
				Method: "POST",
				Path:   "pid",
				Query:  map[string][]string{"q": {marker}},
				Body:   []byte(marker),
				Caller: appplugin.Caller{Email: marker + "@example.com"},
			})
			if err != nil {
				failures <- fmt.Sprintf("caller %d: %v", i, err)
				return
			}
			// The echoed body, query, and caller must all be this caller's.
			for _, field := range []string{
				fmt.Sprintf(`"body":%q`, marker),
				fmt.Sprintf(`"query":%q`, marker),
				fmt.Sprintf(`"caller":"%s@example.com"`, marker),
			} {
				if !strings.Contains(string(response.Body), field) {
					failures <- fmt.Sprintf(
						"caller %d got another caller's answer: wanted %s in %s",
						i, field, response.Body)
				}
			}
		}()
	}
	group.Wait()
	close(failures)
	for failure := range failures {
		t.Error(failure)
	}
}

// Serving a request must not touch the builder. The catalog is embedded, so
// nothing about an image can change while the server runs — re-reading and
// re-hashing the plugin per call would be pure overhead, and it would funnel
// concurrent calls to one image through the builder's lock.
func TestServingARequestDoesNotRebuild(t *testing.T) {
	host := newTestHost(t, fakeCatalog{"test-image": sourceFS(testPluginSource)})
	spec := testSpec("test-image", "instance-nobuild")

	call(t, host, spec, appplugin.Request{Method: "GET", Path: "pid"})
	afterLaunch := host.builder.calls.Load()

	for range 25 {
		call(t, host, spec, appplugin.Request{Method: "GET", Path: "pid"})
	}
	if got := host.builder.calls.Load(); got != afterLaunch {
		t.Errorf("the builder ran %d times while serving requests, want 0",
			got-afterLaunch)
	}

	// A crashed plugin must still be rebuilt-and-relaunched on the next call,
	// so the fast path cannot be a blanket skip.
	if err := host.Stop(context.Background(), spec.Instance.ID); err != nil {
		t.Fatalf("stop: %v", err)
	}
	call(t, host, spec, appplugin.Request{Method: "GET", Path: "pid"})
	if got := host.builder.calls.Load(); got != afterLaunch+1 {
		t.Errorf("relaunch consulted the builder %d times, want 1", got-afterLaunch)
	}
}
