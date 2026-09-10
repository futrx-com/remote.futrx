package main

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/futrx-com/remote.futrx.com/pkg/appplugin"
)

// A plugin is ordinary Go in the repository's module, so it is tested like
// ordinary Go: build the backend, hand it an Instance the way the host would,
// and call Handle. Nothing here needs a server, a container, or the plugin
// host.
func newTestBackend(t *testing.T, dataDir string, env map[string]string) *backend {
	t.Helper()
	b := &backend{mux: appplugin.NewMux()}
	b.mux.GET("hello", "", b.hello)
	b.mux.GET("visits", "", b.readVisits)
	b.mux.POST("visits", "", b.countVisit)
	if err := b.Init(appplugin.Instance{
		ID: "test", ImageID: "hello-remote", Scope: "global",
		DataDir: dataDir, Env: env,
	}); err != nil {
		t.Fatalf("init: %v", err)
	}
	return b
}

func call(t *testing.T, b *backend, method, path string) map[string]any {
	t.Helper()
	response, err := b.Handle(appplugin.Request{
		Method: method,
		Path:   path,
		Caller: appplugin.Caller{Email: "user@example.com"},
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
