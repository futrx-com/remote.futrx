package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"

	appLifecycle "futrx.local/catalog/applications/hello-remote/backend/lifecycle"
	"github.com/futrx-com/remote.futrx.com/pkg/applications"
)

type recordingEventEmitter struct {
	mu           sync.Mutex
	publications []applications.Publication
	err          error
}

func (p *recordingEventEmitter) Emit(publication applications.Publication) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	publication.Payload = append(json.RawMessage(nil), publication.Payload...)
	p.publications = append(p.publications, publication)
	return p.err
}

func (p *recordingEventEmitter) recorded() []applications.Publication {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]applications.Publication(nil), p.publications...)
}

// A backend is ordinary Go in the repository's module, so it is tested like
// ordinary Go: build the backend, hand it an Instance the way the host would,
// and call Handle. Nothing here needs a server, a container, or the backend
// host.
func newTestBackend(t *testing.T, dataDir string, env map[string]string) *api {
	return newTestBackendWithEvents(t, dataDir, env, &recordingEventEmitter{})
}

func newTestBackendWithEvents(
	t *testing.T,
	dataDir string,
	env map[string]string,
	events applications.EventEmitter,
) *api {
	t.Helper()
	b := handler(
		appLifecycle.NewGreetings(events),
		appLifecycle.NewInspections(events),
	)
	if err := b.Init(applications.Instance{
		ID: "test", ApplicationID: "hello-remote", Scope: "global",
		DataDir: dataDir, Env: env,
	}); err != nil {
		t.Fatalf("init: %v", err)
	}
	return b
}

func TestContainerReportsTheInstalledContainer(t *testing.T) {
	emitter := &recordingEventEmitter{}
	b := newTestBackendWithEvents(t, t.TempDir(), nil, emitter)
	b.instance.ContainerName = "futrx-app-test"
	b.inspectContainer = func(name string) (containerFacts, error) {
		if name != "futrx-app-test" {
			t.Fatalf("container name = %q, want futrx-app-test", name)
		}
		return containerFacts{Hostname: "hello", OperatingSystem: "Ubuntu 24.04 LTS", CPUCount: 4}, nil
	}

	body := call(t, b, "GET", "container")
	if got := body["name"]; got != "futrx-app-test" {
		t.Errorf("name = %v, want futrx-app-test", got)
	}
	if got := body["operatingSystem"]; got != "Ubuntu 24.04 LTS" {
		t.Errorf("operatingSystem = %v, want Ubuntu 24.04 LTS", got)
	}
	if got := body["cpuCount"]; got != float64(4) {
		t.Errorf("cpuCount = %v, want 4", got)
	}
	publications := emitter.recorded()
	if len(publications) != 1 {
		t.Fatalf("publications = %d, want 1", len(publications))
	}
	publication := publications[0]
	if publication.Publisher != "inspections" ||
		publication.Event != "container-inspected" || publication.Version != 1 {
		t.Fatalf("publication identity = %+v", publication)
	}
	var payload map[string]string
	if err := json.Unmarshal(publication.Payload, &payload); err != nil {
		t.Fatalf("decode publication payload: %v", err)
	}
	if payload["container"] != "futrx-app-test" || payload["hostname"] != "hello" {
		t.Fatalf("publication payload = %v", payload)
	}
}

func TestServiceReportsTheSupervisedContainerService(t *testing.T) {
	b := newTestBackend(t, t.TempDir(), nil)
	b.instance.Service = "hello-remote"
	b.instance.InternalPort = 4780
	b.instance.ExternalPort = 4781
	b.inspectService = func(port int) (serviceHealth, error) {
		if port != 4781 {
			t.Fatalf("service port = %d, want 4781", port)
		}
		return serviceHealth{
			Status:             "ok",
			Message:            "Hello from the container service.",
			Version:            "build-id",
			ProvisionedVersion: "14",
		}, nil
	}

	body := call(t, b, "GET", "service")
	if got := body["service"]; got != "hello-remote" {
		t.Errorf("service = %v, want hello-remote", got)
	}
	if got := body["internalPort"]; got != float64(4780) {
		t.Errorf("internal port = %v, want 4780", got)
	}
	if got := body["externalPort"]; got != float64(4781) {
		t.Errorf("external port = %v, want 4781", got)
	}
	if got := body["provisionedVersion"]; got != "14" {
		t.Errorf("provisionedVersion = %v, want 14", got)
	}
}

func call(t *testing.T, b *api, method, path string) map[string]any {
	t.Helper()
	response, err := b.Handle(applications.Request{
		Method: method,
		Path:   path,
		Caller: applications.Caller{Email: "user@example.com"},
	})
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	if response.Status != http.StatusOK {
		t.Fatalf("%s %s status = %d, want 200: %s", method, path, response.Status, response.Body)
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body, &body); err != nil {
		t.Fatalf("decode %s %s: %v", method, path, err)
	}
	return body
}

func TestHelloUsesInstallGreetingAndCaller(t *testing.T) {
	b := newTestBackend(t, t.TempDir(), map[string]string{"HELLO_GREETING": "Good morning"})

	if got, want := call(t, b, "GET", "hello")["message"], "Good morning, user@example.com."; got != want {
		t.Errorf("message = %v, want %v", got, want)
	}
}

func TestEchoShowsFrontendRequestOptions(t *testing.T) {
	b := newTestBackend(t, t.TempDir(), nil)
	response, err := b.Handle(applications.Request{
		Method:  http.MethodPost,
		Path:    "echo",
		Query:   map[string][]string{"source": {"frontend-showcase"}},
		Headers: map[string][]string{"X-Hello-Remote": {"frontend-showcase"}},
		Body:    []byte(`{"message":"hello"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		Method  string              `json:"method"`
		Query   map[string][]string `json:"query"`
		Headers map[string][]string `json:"headers"`
		Body    map[string]string   `json:"body"`
	}
	if err := json.Unmarshal(response.Body, &body); err != nil {
		t.Fatal(err)
	}
	if body.Method != http.MethodPost || body.Query["source"][0] != "frontend-showcase" ||
		body.Headers["X-Hello-Remote"][0] != "frontend-showcase" || body.Body["message"] != "hello" {
		t.Fatalf("echo response = %+v", body)
	}
}

// The counter is what shows DataDir doing its job: the process a user's first
// call started is not the process their next one reaches, because a stop, a
// crash, or a server restart is repaired lazily. Only what reached the
// directory survives that.
func TestVisitsSurviveANewProcess(t *testing.T) {
	dataDir := t.TempDir()
	first := newTestBackend(t, dataDir, nil)
	call(t, first, "POST", "visits")
	call(t, first, "POST", "visits")

	restarted := newTestBackend(t, dataDir, nil)
	if got := call(t, restarted, "GET", "visits")["visits"]; got != float64(2) {
		t.Errorf("visits after restart = %v, want 2", got)
	}
}

func TestVisitsWithoutADataDirStillAnswer(t *testing.T) {
	b := newTestBackend(t, "", nil)

	body := call(t, b, "POST", "visits")
	if got := body["visits"]; got != float64(1) {
		t.Errorf("visits = %v, want 1", got)
	}
	if body["warning"] == nil {
		t.Error("expected a warning explaining the count was not persisted")
	}
}

func TestGreetingVisitEmitsTheDeclaredEvent(t *testing.T) {
	emitter := &recordingEventEmitter{}
	b := newTestBackendWithEvents(t, t.TempDir(), nil, emitter)

	body := call(t, b, http.MethodPost, "visits")
	if body["warning"] != nil {
		t.Fatalf("successful greeting warning = %v", body["warning"])
	}
	publications := emitter.recorded()
	if len(publications) != 1 {
		t.Fatalf("publications = %d, want 1", len(publications))
	}
	publication := publications[0]
	if publication.Publisher != "greetings" || publication.Event != "greeted" ||
		publication.Version != 1 {
		t.Fatalf("publication identity = %+v", publication)
	}
	var payload map[string]int
	if err := json.Unmarshal(publication.Payload, &payload); err != nil {
		t.Fatalf("decode publication payload: %v", err)
	}
	if payload["visits"] != 1 {
		t.Fatalf("publication payload = %v, want visits 1", payload)
	}
}

func TestGreetingVisitSucceedsWhenPublicationFails(t *testing.T) {
	b := newTestBackendWithEvents(
		t,
		t.TempDir(),
		nil,
		&recordingEventEmitter{err: errors.New("bus unavailable")},
	)

	body := call(t, b, http.MethodPost, "visits")
	if body["visits"] != float64(1) {
		t.Fatalf("visits = %v, want 1", body["visits"])
	}
	warning, _ := body["warning"].(string)
	if !strings.Contains(warning, "event not published: bus unavailable") {
		t.Fatalf("warning = %q", warning)
	}
}

func TestBackendDoesNotOwnEventRuntimeContracts(t *testing.T) {
	b := newTestBackend(t, t.TempDir(), nil)
	if _, ok := any(b).(applications.EventEmitter); ok {
		t.Fatal("backend unexpectedly implements applications.EventEmitter")
	}
	if _, ok := any(b).(applications.EventSubscriber); ok {
		t.Fatal("backend unexpectedly implements applications.EventSubscriber")
	}
}
