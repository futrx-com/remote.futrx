package applications

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	svc "github.com/futrx-com/remote.futrx.com/internal/service/applications"
	"github.com/futrx-com/remote.futrx.com/pkg/applications"
)

// These tests compile and run a real backend process, because the parts most
// likely to break — the generated module, the handshake, the RPC shapes — are
// exactly the parts a mock would replace. One build is shared by every test in
// the package through the builder's fingerprint cache.

// testBackendSource is a complete backend, held as source because that is what
// the catalog ships and what the host consumes.
const testBackendSource = `package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/futrx-com/remote.futrx.com/pkg/applications"
	"github.com/futrx-com/remote.futrx.com/pkg/applications/rpc"
)

type backend struct{ instance applications.Instance }

type responseStream struct {
	*strings.Reader
	marker string
}

func (s *responseStream) Close() error {
	if s.marker == "" {
		return nil
	}
	return os.WriteFile(s.marker, []byte("closed"), 0o600)
}

func (b *backend) Describe() (applications.Descriptor, error) {
	return applications.Descriptor{
		Name:       "test",
		Version:    "1",
		APIVersion: applications.APIVersion,
		Routes:     []applications.Route{{Method: "GET", Path: "pid"}},
	}, nil
}

func (b *backend) Init(instance applications.Instance) error {
	b.instance = instance
	return nil
}

func (b *backend) Handle(request applications.Request) (applications.Response, error) {
	switch request.Path {
	case "pid":
		return applications.JSON(http.StatusOK, map[string]any{
			"pid":              os.Getpid(),
			"instance":         b.instance.ID,
			"project":          b.instance.ProjectID,
			"env":              b.instance.Env,
			"dataDir":          b.instance.DataDir,
			"sharedRuntimeDir": b.instance.SharedRuntimeDir,
			"caller":           request.Caller.Email,
			"admin":            request.Caller.IsAdmin,
			"query":            request.QueryValue("q"),
			"body":             string(request.Body),
		}), nil
	case "write":
		path := filepath.Join(b.instance.DataDir, "kept.txt")
		if err := os.WriteFile(path, request.Body, 0o600); err != nil {
			return applications.Errorf(http.StatusInternalServerError, "%v", err), nil
		}
		return applications.Text(http.StatusOK, path), nil
	case "read":
		data, err := os.ReadFile(filepath.Join(b.instance.DataDir, "kept.txt"))
		if err != nil {
			return applications.Errorf(http.StatusNotFound, "%v", err), nil
		}
		return applications.Text(http.StatusOK, string(data)), nil
	case "boom":
		panic("deliberate")
	case "stream":
		data := strings.Repeat("0123456789abcdef", (2 << 20) / 16)
		return applications.Stream(
			&responseStream{Reader: strings.NewReader(data)},
			int64(len(data)),
			time.Unix(1700000000, 0),
			map[string][]string{"Content-Type": {"application/test-stream"}},
		), nil
	case "late-stream":
		time.Sleep(250 * time.Millisecond)
		data := "late"
		return applications.Stream(
			&responseStream{
				Reader: strings.NewReader(data),
				marker: filepath.Join(b.instance.DataDir, "late-stream-closed"),
			},
			int64(len(data)),
			time.Time{},
			nil,
		), nil
	case "cancel":
		if err := os.WriteFile(filepath.Join(b.instance.DataDir, "cancel-started"), []byte("started"), 0o600); err != nil {
			return applications.Response{}, err
		}
		<-request.Done()
		if err := os.WriteFile(filepath.Join(b.instance.DataDir, "cancel-finished"), []byte(request.Err().Error()), 0o600); err != nil {
			return applications.Response{}, err
		}
		return applications.Text(http.StatusRequestTimeout, request.Err().Error()), nil
	case "slow":
		select {}
	}
	return applications.Errorf(http.StatusNotFound, "no route %q", request.Path), nil
}

func main() { rpc.Serve(&backend{}) }
`

// wrongVersionSource reports a contract version the host does not speak.
const wrongVersionSource = `package main

import (
	"github.com/futrx-com/remote.futrx.com/pkg/applications"
	"github.com/futrx-com/remote.futrx.com/pkg/applications/rpc"
)

type backend struct{}

func (backend) Describe() (applications.Descriptor, error) {
	return applications.Descriptor{Name: "old", APIVersion: applications.APIVersion + 1}, nil
}
func (backend) Init(applications.Instance) error { return nil }
func (backend) Handle(applications.Request) (applications.Response, error) {
	return applications.Response{}, nil
}

func main() { rpc.Serve(backend{}) }
`

const hangingInitSource = `package main

import (
	"github.com/futrx-com/remote.futrx.com/pkg/applications"
	"github.com/futrx-com/remote.futrx.com/pkg/applications/rpc"
)

type backend struct{}

func (backend) Describe() (applications.Descriptor, error) {
	return applications.Descriptor{APIVersion: applications.APIVersion}, nil
}
func (backend) Init(applications.Instance) error { select {} }
func (backend) Handle(applications.Request) (applications.Response, error) {
	return applications.Response{}, nil
}

func main() { rpc.Serve(backend{}) }
`

type fakeCatalog map[string]fs.FS

func (c fakeCatalog) BackendSource(applicationID string) (fs.FS, bool) {
	source, ok := c[applicationID]
	return source, ok
}

func sourceFS(main string) fs.FS {
	return fstest.MapFS{"main.go": &fstest.MapFile{Data: []byte(main)}}
}

// testGoToolOverride mirrors the application edge's configuration injection.
// It keeps the tests able to select a wrapper toolchain without teaching the
// production integration how to read environment variables.
func testGoToolOverride() string {
	return os.Getenv("REMOTE_APPLICATION_GO")
}

func testInstance(applicationID, instanceID string) applications.Instance {
	return applications.Instance{
		ID:                 instanceID,
		ApplicationID:      applicationID,
		ApplicationName:    "Test Application",
		ApplicationVersion: "2.4.0",
		Scope:              string(svc.ScopeProject),
		ProjectID:          "project-1",
		Env:                map[string]string{"TOKEN": "secret-value"},
	}
}

// newTestHost builds a host over a temporary root. The build cache is shared
// across tests in one run via t.TempDir's parent, so only the first test in a
// package pays for compilation.
func newTestHost(t *testing.T, catalog fakeCatalog) *Host {
	t.Helper()
	if testing.Short() {
		t.Skip("compiles a backend with the Go toolchain")
	}
	if _, err := findGoTool(testGoToolOverride()); err != nil {
		t.Skipf("no Go toolchain available: %v", err)
	}
	host := New(sharedRoot(t), catalog, Options{GoTool: testGoToolOverride()})
	t.Cleanup(host.Shutdown)
	return host
}

// sharedRoot keeps one build cache for the whole package so the backend is
// compiled once rather than once per test.
var packageRoot string

func sharedRoot(t *testing.T) string {
	t.Helper()
	if packageRoot == "" {
		root, err := os.MkdirTemp("", "application-backend-test-")
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

func call(t *testing.T, host *Host, instance applications.Instance, request applications.Request) applications.Response {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	response, err := host.Call(ctx, instance, request)
	if err != nil {
		t.Fatalf("call %q: %v", request.Path, err)
	}
	return response
}

func TestHostCompilesAndServesABackend(t *testing.T) {
	host := newTestHost(t, fakeCatalog{"test-application": sourceFS(testBackendSource)})
	spec := testInstance("test-application", "instance-1")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	descriptor, err := host.Ensure(ctx, spec)
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if descriptor.Name != "Test Application" || descriptor.Version != "2.4.0" || descriptor.APIVersion != applications.APIVersion {
		t.Fatalf("descriptor = %+v", descriptor)
	}
	if len(descriptor.Routes) != 1 || descriptor.Routes[0].Path != "pid" {
		t.Errorf("routes = %+v", descriptor.Routes)
	}

	response := call(t, host, spec, applications.Request{
		Method: "POST",
		Path:   "pid",
		Query:  map[string][]string{"q": {"asked"}},
		Body:   []byte("payload"),
		Caller: applications.Caller{Email: "admin@example.com", IsAdmin: true},
	})
	if response.Status != 200 {
		t.Fatalf("status = %d, body %s", response.Status, response.Body)
	}
	// The backend sees the instance it was initialized with and the caller the
	// service stamped, including the install's secret env — that combination is
	// the whole reason a backend backend can do anything useful.
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

func TestHostProvidesIsolatedDataAndSharedApplicationRuntimeDirectories(t *testing.T) {
	host := newTestHost(t, fakeCatalog{
		"test-application":  sourceFS(testBackendSource),
		"other-application": sourceFS(testBackendSource),
	})
	first := testInstance("test-application", "instance-directory-first")
	second := testInstance("test-application", "instance-directory-second")
	other := testInstance("other-application", "instance-directory-other")

	firstDirectories := responseDirectories(t, call(t, host, first, applications.Request{Method: "GET", Path: "pid"}))
	secondDirectories := responseDirectories(t, call(t, host, second, applications.Request{Method: "GET", Path: "pid"}))
	otherDirectories := responseDirectories(t, call(t, host, other, applications.Request{Method: "GET", Path: "pid"}))

	if firstDirectories.DataDir == secondDirectories.DataDir {
		t.Fatalf("instances share DataDir %q", firstDirectories.DataDir)
	}
	if firstDirectories.SharedRuntimeDir != secondDirectories.SharedRuntimeDir {
		t.Fatalf(
			"same application runtime directories = %q, %q",
			firstDirectories.SharedRuntimeDir,
			secondDirectories.SharedRuntimeDir,
		)
	}
	if firstDirectories.SharedRuntimeDir == otherDirectories.SharedRuntimeDir {
		t.Fatalf("different applications share runtime directory %q", firstDirectories.SharedRuntimeDir)
	}
	for _, directory := range []string{
		firstDirectories.DataDir,
		secondDirectories.DataDir,
		firstDirectories.SharedRuntimeDir,
		otherDirectories.SharedRuntimeDir,
	} {
		info, err := os.Stat(directory)
		if err != nil || !info.IsDir() {
			t.Errorf("host directory %q = (%v, %v), want directory", directory, info, err)
		}
	}

	host.Shutdown()
	if _, err := os.Stat(host.sharedRuntimeRoot()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("shared runtime root survived shutdown: %v", err)
	}
	for _, directory := range []string{firstDirectories.DataDir, secondDirectories.DataDir} {
		if _, err := os.Stat(directory); err != nil {
			t.Errorf("durable data directory %q removed on shutdown: %v", directory, err)
		}
	}
}

type backendDirectories struct {
	DataDir          string `json:"dataDir"`
	SharedRuntimeDir string `json:"sharedRuntimeDir"`
}

func responseDirectories(t *testing.T, response applications.Response) backendDirectories {
	t.Helper()
	var directories backendDirectories
	if err := json.Unmarshal(response.Body, &directories); err != nil {
		t.Fatalf("decode backend directories %q: %v", response.Body, err)
	}
	if directories.DataDir == "" || directories.SharedRuntimeDir == "" {
		t.Fatalf("backend directories = %+v", directories)
	}
	return directories
}

func TestHostStreamsASeekableResponseWithoutBuffering(t *testing.T) {
	host := newTestHost(t, fakeCatalog{"test-application": sourceFS(testBackendSource)})
	spec := testInstance("test-application", "instance-stream")
	response := call(t, host, spec, applications.Request{Method: "GET", Path: "stream"})

	if response.Body != nil {
		t.Fatalf("buffered body has %d bytes, want nil", len(response.Body))
	}
	content, size, modTime, ok := response.ResponseStream()
	if !ok {
		t.Fatal("response has no stream")
	}
	defer content.Close()
	if size != 2<<20 {
		t.Fatalf("stream size = %d", size)
	}
	if !modTime.Equal(time.Unix(1_700_000_000, 0)) {
		t.Errorf("mod time = %s", modTime)
	}
	if got := response.Headers["Content-Type"]; len(got) != 1 || got[0] != "application/test-stream" {
		t.Errorf("Content-Type = %v", got)
	}
	first := make([]byte, 32)
	if _, err := io.ReadFull(content, first); err != nil || string(first) != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("first read = %q, %v", first, err)
	}
	position, err := content.Seek(-16, io.SeekEnd)
	if err != nil || position != size-16 {
		t.Fatalf("SeekEnd() = %d, %v", position, err)
	}
	last, err := io.ReadAll(content)
	if err != nil || string(last) != "0123456789abcdef" {
		t.Fatalf("last read = %q, %v", last, err)
	}
}

func TestLateStreamIsClosedAfterCallerTimeout(t *testing.T) {
	host := newTestHost(t, fakeCatalog{"test-application": sourceFS(testBackendSource)})
	spec := testInstance("test-application", "instance-late-stream")
	call(t, host, spec, applications.Request{Method: "GET", Path: "pid"})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := host.Call(ctx, spec, applications.Request{Method: "GET", Path: "late-stream"}); err == nil {
		t.Fatal("late stream reported success")
	} else if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("call error = %v, want timeout", err)
	}

	marker := filepath.Join(host.dataDir(spec.ID), "late-stream-closed")
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(marker); err == nil {
			break
		} else if !os.IsNotExist(err) {
			t.Fatalf("stat close marker: %v", err)
		}
		if time.Now().After(deadline) {
			t.Fatal("stream returned after timeout was not closed")
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func TestCallerCancellationStopsOnlyThatBackendHandle(t *testing.T) {
	host := newTestHost(t, fakeCatalog{"test-application": sourceFS(testBackendSource)})
	spec := testInstance("test-application", "instance-cancel")
	before := string(call(t, host, spec, applications.Request{Method: "GET", Path: "pid"}).Body)

	ctx, cancel := context.WithCancel(context.Background())
	called := make(chan error, 1)
	go func() {
		_, err := host.Call(ctx, spec, applications.Request{Method: "GET", Path: "cancel"})
		called <- err
	}()
	waitForFile := func(name string) string {
		t.Helper()
		path := filepath.Join(host.dataDir(spec.ID), name)
		deadline := time.Now().Add(5 * time.Second)
		for {
			body, err := os.ReadFile(path)
			if err == nil {
				return string(body)
			}
			if !os.IsNotExist(err) {
				t.Fatalf("read %s: %v", name, err)
			}
			if time.Now().After(deadline) {
				t.Fatalf("timed out waiting for %s", name)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	waitForFile("cancel-started")
	cancel()
	select {
	case err := <-called:
		if err == nil || !strings.Contains(err.Error(), "context canceled") {
			t.Fatalf("canceled call error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("host call did not return after cancellation")
	}
	if got := waitForFile("cancel-finished"); got != context.Canceled.Error() {
		t.Fatalf("backend observed cancellation %q", got)
	}
	after := string(call(t, host, spec, applications.Request{Method: "GET", Path: "pid"}).Body)
	if before != after {
		t.Fatalf("canceling one Handle restarted or changed the backend:\nbefore=%s\nafter=%s", before, after)
	}
}

// One process per instance is the contract the whole feature rests on: state a
// backend keeps between requests is only meaningful if the process is the same.
func TestHostReusesOneProcessPerInstance(t *testing.T) {
	host := newTestHost(t, fakeCatalog{"test-application": sourceFS(testBackendSource)})
	spec := testInstance("test-application", "instance-reuse")

	first := string(call(t, host, spec, applications.Request{Method: "GET", Path: "pid"}).Body)
	second := string(call(t, host, spec, applications.Request{Method: "GET", Path: "pid"}).Body)
	if first != second {
		t.Errorf("two calls hit different processes:\n%s\n%s", first, second)
	}

	other := testInstance("test-application", "instance-other")
	third := string(call(t, host, other, applications.Request{Method: "GET", Path: "pid"}).Body)
	if third == first {
		t.Error("two instances share one process; they must not")
	}
}

func TestHostStopEndsTheProcessAndCallRestartsIt(t *testing.T) {
	host := newTestHost(t, fakeCatalog{"test-application": sourceFS(testBackendSource)})
	spec := testInstance("test-application", "instance-restart")

	before := string(call(t, host, spec, applications.Request{Method: "GET", Path: "pid"}).Body)
	if err := host.Stop(context.Background(), spec.ID); err != nil {
		t.Fatalf("stop: %v", err)
	}
	// A call after a stop starts the backend again rather than failing, which is
	// what lets installed backends survive a server restart with no sweep.
	after := string(call(t, host, spec, applications.Request{Method: "GET", Path: "pid"}).Body)
	if before == after {
		t.Errorf("stop did not end the process: %s", after)
	}
}

func TestHostInvalidatesOnlyProcessesFromTheReplacedApplication(t *testing.T) {
	host := newTestHost(t, fakeCatalog{
		"test-application":  sourceFS(testBackendSource),
		"other-application": sourceFS(testBackendSource),
	})
	replaced := testInstance("test-application", "instance-invalidated")
	other := testInstance("other-application", "instance-unrelated")

	before := backendPID(t, call(t, host, replaced, applications.Request{Method: "GET", Path: "pid"}))
	otherBefore := backendPID(t, call(t, host, other, applications.Request{Method: "GET", Path: "pid"}))
	runtimeMarker := filepath.Join(host.sharedRuntimeDir(replaced.ApplicationID), "old-contract.lock")
	if err := os.WriteFile(runtimeMarker, nil, 0o600); err != nil {
		t.Fatalf("write runtime marker: %v", err)
	}

	host.InvalidateApplication(replaced.ApplicationID)
	if current := host.lookup(replaced.ID); current != nil {
		t.Fatal("invalidated process remains published by the host")
	}
	if _, err := os.Stat(runtimeMarker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("invalidated runtime marker survived: %v", err)
	}
	if current := host.lookup(other.ID); current == nil || !current.running() {
		t.Fatal("unrelated application process was invalidated")
	}

	after := backendPID(t, call(t, host, replaced, applications.Request{Method: "GET", Path: "pid"}))
	otherAfter := backendPID(t, call(t, host, other, applications.Request{Method: "GET", Path: "pid"}))
	if after == before {
		t.Fatalf("invalidated application reused pid %d", after)
	}
	if otherAfter != otherBefore {
		t.Fatalf("unrelated application moved from pid %d to %d", otherBefore, otherAfter)
	}
}

func TestApplicationInvalidationWaitsForConcurrentTerminationBeforeRemovingSharedRuntime(t *testing.T) {
	operations := []struct {
		name string
		run  func(*Host, string) error
	}{
		{name: "stop", run: func(host *Host, instanceID string) error {
			return host.Stop(context.Background(), instanceID)
		}},
		{name: "remove", run: func(host *Host, instanceID string) error {
			return host.Remove(context.Background(), instanceID)
		}},
	}

	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			host := newTestHost(t, fakeCatalog{"test-application": sourceFS(testBackendSource)})
			spec := testInstance("test-application", "instance-"+operation.name+"-invalidation")
			call(t, host, spec, applications.Request{Method: "GET", Path: "pid"})

			runtimeMarker := filepath.Join(host.sharedRuntimeDir(spec.ApplicationID), "active.lock")
			if err := os.WriteFile(runtimeMarker, nil, 0o600); err != nil {
				t.Fatalf("write runtime marker: %v", err)
			}

			terminationEntered := make(chan struct{})
			allowTermination := make(chan struct{})
			realTerminate := host.terminateProcess
			host.terminateProcess = func(process *backendProcess) {
				close(terminationEntered)
				<-allowTermination
				realTerminate(process)
			}

			terminationDone := make(chan error, 1)
			go func() {
				terminationDone <- operation.run(host, spec.ID)
			}()
			<-terminationEntered

			// Stop/Remove has removed the process from the running map but must
			// retain the application lifecycle boundary until termination completes.
			// This deterministic assertion prevents invalidation from unlinking the
			// shared lock files while the old process still uses them.
			host.applicationChanges.mu.Lock()
			applicationLock := host.applicationChanges.locks[spec.ApplicationID]
			host.applicationChanges.mu.Unlock()
			if applicationLock == nil {
				close(allowTermination)
				<-terminationDone
				t.Fatal("termination did not create an application lifecycle lock")
			}
			if applicationLock.TryLock() {
				applicationLock.Unlock()
				close(allowTermination)
				<-terminationDone
				t.Fatal("application lifecycle lock was released before process termination")
			}

			invalidateDone := make(chan struct{})
			go func() {
				host.InvalidateApplication(spec.ApplicationID)
				close(invalidateDone)
			}()

			if _, err := os.Stat(runtimeMarker); err != nil {
				close(allowTermination)
				<-terminationDone
				<-invalidateDone
				t.Fatalf("shared runtime marker removed before termination completed: %v", err)
			}

			close(allowTermination)
			if err := <-terminationDone; err != nil {
				t.Fatalf("%s: %v", operation.name, err)
			}
			<-invalidateDone
			if _, err := os.Stat(runtimeMarker); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("shared runtime marker survived invalidation: %v", err)
			}
		})
	}
}

func TestConcurrentLifecycleWaitsForApplicationInvalidationToFinishTermination(t *testing.T) {
	operations := []struct {
		name string
		run  func(*Host, string) error
	}{
		{name: "stop", run: func(host *Host, instanceID string) error {
			return host.Stop(context.Background(), instanceID)
		}},
		{name: "remove", run: func(host *Host, instanceID string) error {
			return host.Remove(context.Background(), instanceID)
		}},
	}

	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			host := newTestHost(t, fakeCatalog{"test-application": sourceFS(testBackendSource)})
			spec := testInstance("test-application", "instance-invalidation-before-"+operation.name)
			call(t, host, spec, applications.Request{Method: "GET", Path: "pid"})

			dataMarker := filepath.Join(host.dataDir(spec.ID), "state")
			if err := os.WriteFile(dataMarker, nil, 0o600); err != nil {
				t.Fatalf("write data marker: %v", err)
			}

			terminationEntered := make(chan struct{})
			allowTermination := make(chan struct{})
			realTerminate := host.terminateProcess
			host.terminateProcess = func(process *backendProcess) {
				close(terminationEntered)
				<-allowTermination
				realTerminate(process)
			}

			invalidateDone := make(chan struct{})
			go func() {
				host.InvalidateApplication(spec.ApplicationID)
				close(invalidateDone)
			}()
			<-terminationEntered

			lifecycleDone := make(chan error, 1)
			go func() {
				lifecycleDone <- operation.run(host, spec.ID)
			}()

			select {
			case err := <-lifecycleDone:
				close(allowTermination)
				<-invalidateDone
				t.Fatalf("%s returned before invalidation terminated the process: %v", operation.name, err)
			case <-time.After(50 * time.Millisecond):
			}
			if _, err := os.Stat(dataMarker); err != nil {
				close(allowTermination)
				<-invalidateDone
				<-lifecycleDone
				t.Fatalf("data changed before process termination completed: %v", err)
			}

			close(allowTermination)
			<-invalidateDone
			if err := <-lifecycleDone; err != nil {
				t.Fatalf("%s: %v", operation.name, err)
			}
			if operation.name == "remove" {
				if _, err := os.Stat(dataMarker); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("data marker survived remove: %v", err)
				}
			}
		})
	}
}

func TestLaunchCannotSurviveApplicationInvalidation(t *testing.T) {
	host := newTestHost(t, fakeCatalog{"test-application": sourceFS(testBackendSource)})
	spec := testInstance("test-application", "instance-stale-launch")
	binary, err := host.builder.Build(context.Background(), spec.ApplicationID, sourceFS(testBackendSource))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	configuration, err := backendConfiguration(spec)
	if err != nil {
		t.Fatalf("configuration: %v", err)
	}

	// This launch began under generation zero; package replacement advances the
	// application before its handshake can be published.
	host.InvalidateApplication(spec.ApplicationID)
	started, err := host.launch(context.Background(), spec, binary, configuration, 0)
	if started != nil || !errors.Is(err, errBackendInvalidated) {
		t.Fatalf("stale launch = (%v, %v), want invalidated", started, err)
	}
	if current := host.lookup(spec.ID); current != nil {
		t.Fatal("stale launch was published after invalidation")
	}

	response := call(t, host, spec, applications.Request{Method: "GET", Path: "pid"})
	if backendPID(t, response) == 0 {
		t.Fatal("current generation did not launch")
	}
}

func TestHostRestartsWhenInstanceConfigurationChanges(t *testing.T) {
	host := newTestHost(t, fakeCatalog{"test-application": sourceFS(testBackendSource)})
	spec := testInstance("test-application", "instance-reconfigured")

	first := call(t, host, spec, applications.Request{Method: "GET", Path: "pid"})
	before := backendPID(t, first)
	spec.ProjectID = "project-2"
	second := call(t, host, spec, applications.Request{Method: "GET", Path: "pid"})
	after := backendPID(t, second)

	if after == before {
		t.Fatalf("changed configuration reused pid %d", after)
	}
	if !strings.Contains(string(second.Body), `"project":"project-2"`) {
		t.Fatalf("backend kept stale initialization: %s", second.Body)
	}
}

func backendPID(t *testing.T, response applications.Response) int {
	t.Helper()
	var body struct {
		PID int `json:"pid"`
	}
	if err := json.Unmarshal(response.Body, &body); err != nil {
		t.Fatalf("decode pid response %q: %v", response.Body, err)
	}
	if body.PID == 0 {
		t.Fatalf("pid response = %s", response.Body)
	}
	return body.PID
}

// Stop keeps a backend's data; only Remove discards it. That split is what
// makes stop and start safe to use freely on an app someone relies on.
func TestStopKeepsBackendDataAndRemoveDiscardsIt(t *testing.T) {
	host := newTestHost(t, fakeCatalog{"test-application": sourceFS(testBackendSource)})
	spec := testInstance("test-application", "instance-data")

	written := call(t, host, spec, applications.Request{
		Method: "POST", Path: "write", Body: []byte("durable"),
	})
	if written.Status != 200 {
		t.Fatalf("write: %s", written.Body)
	}
	if err := host.Stop(context.Background(), spec.ID); err != nil {
		t.Fatalf("stop: %v", err)
	}
	read := call(t, host, spec, applications.Request{Method: "GET", Path: "read"})
	if string(read.Body) != "durable" {
		t.Errorf("after stop, read = %q (%d)", read.Body, read.Status)
	}

	dataDir := host.dataDir(spec.ID)
	if err := host.Remove(context.Background(), spec.ID); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := os.Stat(dataDir); !os.IsNotExist(err) {
		t.Errorf("data directory %s survived uninstall", dataDir)
	}
	gone := call(t, host, spec, applications.Request{Method: "GET", Path: "read"})
	if gone.Status != 404 {
		t.Errorf("after remove, read = %d %s", gone.Status, gone.Body)
	}
}

// A panicking route must cost one request, not the process. Every other
// request in flight on that backend depends on it.
func TestPanickingRouteFailsOneCallAndKeepsTheProcess(t *testing.T) {
	host := newTestHost(t, fakeCatalog{"test-application": sourceFS(testBackendSource)})
	spec := testInstance("test-application", "instance-panic")

	before := string(call(t, host, spec, applications.Request{Method: "GET", Path: "pid"}).Body)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if _, err := host.Call(ctx, spec, applications.Request{Method: "GET", Path: "boom"}); err == nil {
		t.Fatal("a panicking route reported success")
	} else if !strings.Contains(err.Error(), "panicked") {
		t.Errorf("error = %v, want it to mention the panic", err)
	}

	after := string(call(t, host, spec, applications.Request{Method: "GET", Path: "pid"}).Body)
	if before != after {
		t.Errorf("the backend restarted after a panic:\n%s\n%s", before, after)
	}
}

// A backend that never answers must not hold the caller's connection: the
// deadline belongs to the host, since the transport has no notion of one.
func TestCallRespectsTheCallerDeadline(t *testing.T) {
	host := newTestHost(t, fakeCatalog{"test-application": sourceFS(testBackendSource)})
	spec := testInstance("test-application", "instance-timeout")

	// Start the backend first so the deadline covers only the call.
	call(t, host, spec, applications.Request{Method: "GET", Path: "pid"})

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	started := time.Now()
	if _, err := host.Call(ctx, spec, applications.Request{Method: "GET", Path: "slow"}); err == nil {
		t.Fatal("a call that never answers reported success")
	} else if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("error = %v, want a timeout", err)
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Errorf("the deadline was not enforced: waited %s", elapsed)
	}
}

func TestBackendInitializationRespectsTheCallerDeadline(t *testing.T) {
	host := newTestHost(t, fakeCatalog{"hanging-init": sourceFS(hangingInitSource)})
	spec := testInstance("hanging-init", "instance-hanging-init")

	// Populate the build cache under a generous deadline so this assertion
	// exercises the uncancellable Init RPC rather than compiler cancellation.
	if _, err := host.builder.Build(context.Background(), spec.ApplicationID, sourceFS(hangingInitSource)); err != nil {
		t.Fatalf("prebuild: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := host.Ensure(ctx, spec)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("ensure error = %v, want timeout", err)
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("backend initialization ignored deadline for %s", elapsed)
	}
	if current := host.lookup(spec.ID); current != nil {
		t.Fatal("timed-out backend initialization was published")
	}
}

// A backend built against a different contract is refused at connect time, so
// the mismatch is one clear error instead of an unreadable failure later.
func TestBackendWithAWrongContractVersionIsRefused(t *testing.T) {
	host := newTestHost(t, fakeCatalog{"old-application": sourceFS(wrongVersionSource)})
	spec := testInstance("old-application", "instance-old")

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

func TestEnsureRejectsAnImageWithNoBackendSource(t *testing.T) {
	host := newTestHost(t, fakeCatalog{})
	_, err := host.Ensure(context.Background(), testInstance("missing", "instance-missing"))
	if err == nil {
		t.Fatal("an application with no backend source was accepted")
	}
}

// The compiled binary is cached by a fingerprint of its inputs, so editing a
// backend produces a new binary and leaves no stale one behind.
func TestBuildIsCachedByFingerprintAndPrunesStaleBinaries(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles a backend with the Go toolchain")
	}
	if _, err := findGoTool(testGoToolOverride()); err != nil {
		t.Skipf("no Go toolchain available: %v", err)
	}
	// Share the package build cache: a cold GOCACHE would dominate the runtime.
	builder := NewBuilder(sharedRoot(t), testGoToolOverride())
	ctx := context.Background()

	first, err := builder.Build(ctx, "img", sourceFS(testBackendSource))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	again, err := builder.Build(ctx, "img", sourceFS(testBackendSource))
	if err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	if first != again {
		t.Errorf("identical source produced two binaries:\n%s\n%s", first, again)
	}

	edited, err := builder.Build(ctx, "img", sourceFS(
		strings.Replace(testBackendSource, `Name:       "test"`, `Name:       "edited"`, 1)))
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
		t.Skip("compiles a backend with the Go toolchain")
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
// some of them are panicking. A backend serves one instance for every user who
// can reach it, so a crossed response would be one user's data handed to
// another — the one failure here that would be worse than an outage.
func TestConcurrentCallsDoNotCrossResponses(t *testing.T) {
	host := newTestHost(t, fakeCatalog{"test-application": sourceFS(testBackendSource)})
	spec := testInstance("test-application", "instance-concurrent")

	// Start the backend once so every goroutine below races on calling, not on
	// launching.
	call(t, host, spec, applications.Request{Method: "GET", Path: "pid"})

	const callers = 40
	var group sync.WaitGroup
	failures := make(chan string, callers*2)

	for i := range callers {
		group.Add(1)
		go func() {
			defer group.Done()
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()

			// Every other caller asks the backend to panic, so the well-behaved
			// callers are answering alongside failing ones throughout.
			if i%2 == 1 {
				if _, err := host.Call(ctx, spec, applications.Request{
					Method: "GET", Path: "boom",
				}); err == nil || !strings.Contains(err.Error(), "panicked") {
					failures <- fmt.Sprintf("caller %d: boom returned %v", i, err)
				}
				return
			}

			marker := fmt.Sprintf("caller-%d", i)
			response, err := host.Call(ctx, spec, applications.Request{
				Method: "POST",
				Path:   "pid",
				Query:  map[string][]string{"q": {marker}},
				Body:   []byte(marker),
				Caller: applications.Caller{Email: marker + "@example.com"},
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
// nothing about an application can change while the server runs — re-reading and
// re-hashing the backend per call would be pure overhead, and it would funnel
// concurrent calls to one application through the builder's lock.
func TestServingARequestDoesNotRebuild(t *testing.T) {
	host := newTestHost(t, fakeCatalog{"test-application": sourceFS(testBackendSource)})
	spec := testInstance("test-application", "instance-nobuild")

	call(t, host, spec, applications.Request{Method: "GET", Path: "pid"})
	afterLaunch := host.builder.calls.Load()

	for range 25 {
		call(t, host, spec, applications.Request{Method: "GET", Path: "pid"})
	}
	if got := host.builder.calls.Load(); got != afterLaunch {
		t.Errorf("the builder ran %d times while serving requests, want 0",
			got-afterLaunch)
	}

	// A crashed backend must still be rebuilt-and-relaunched on the next call,
	// so the fast path cannot be a blanket skip.
	if err := host.Stop(context.Background(), spec.ID); err != nil {
		t.Fatalf("stop: %v", err)
	}
	call(t, host, spec, applications.Request{Method: "GET", Path: "pid"})
	if got := host.builder.calls.Load(); got != afterLaunch+1 {
		t.Errorf("relaunch consulted the builder %d times, want 1", got-afterLaunch)
	}
}
