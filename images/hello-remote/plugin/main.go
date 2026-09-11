// Hello Remote is the catalog's worked example: the smallest image that still
// exercises both halves of the feature. It ships a plugin/ the server compiles
// and runs as a child process, and a ui/ that calls it from the browser.
//
// It deliberately installs nothing. A backend image gets no container, no port
// and no proxy device, so this app installs on a laptop with no LXD at all —
// which is what makes it usable as the thing you install first to see whether
// the plumbing works end to end.
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/futrx-com/remote.futrx.com/pkg/appplugin"
	"github.com/futrx-com/remote.futrx.com/pkg/appplugin/pluginrpc"
)

// visitsFile is where the greeting counter lives inside the instance's
// DataDir. That directory is the only storage a plugin can rely on: the
// process is killed on stop, uninstall, and server restart, and restarted
// lazily by the next call, so anything kept in memory is gone by then. The
// counter surviving a restart is the whole point of the example.
const visitsFile = "visits.json"

type backend struct {
	mux *appplugin.Mux

	mu       sync.Mutex
	instance appplugin.Instance
	visits   int
}

func main() {
	b := &backend{mux: appplugin.NewMux()}

	b.mux.GET("hello", "Greet the calling user", b.hello)
	b.mux.GET("visits", "Report how many greetings this install has served", b.readVisits)
	b.mux.POST("visits", "Count one greeting", b.countVisit)

	pluginrpc.Serve(b)
}

// Describe runs once, when the host connects. Routes() reports exactly what
// was registered above, so the route table the SPA discovers through
// remote.backend.describe() cannot drift from the one actually served.
func (b *backend) Describe() (appplugin.Descriptor, error) {
	return appplugin.Descriptor{
		Name:       "Hello Remote",
		Version:    "1",
		APIVersion: appplugin.APIVersion,
		Routes:     b.mux.Routes(),
	}, nil
}

// Init runs once before the first request, and is where the process learns
// which installed copy it belongs to. A global install and two project
// installs are three processes, each with its own Instance and its own
// DataDir.
func (b *backend) Init(instance appplugin.Instance) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.instance = instance
	b.visits = readVisits(instance.DataDir)
	return nil
}

// Handle may be called concurrently, so everything below takes the lock.
func (b *backend) Handle(request appplugin.Request) (appplugin.Response, error) {
	return b.mux.Serve(request), nil
}

func (b *backend) hello(request appplugin.Request) appplugin.Response {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Env carries the install's resolved inputs — here the greeting the user
	// typed into the install dialog, declared as env[] in image.json.
	greeting := b.instance.Env["HELLO_GREETING"]
	if greeting == "" {
		greeting = "Hello"
	}
	// Caller is stamped by the server from the session, never sent by the
	// browser, so a plugin may trust it. The cookies that authenticated it are
	// withheld: this plugin can tell who is asking, and cannot act as them.
	who := request.Caller.Email
	if who == "" {
		who = "there"
	}

	return appplugin.JSON(http.StatusOK, map[string]any{
		"message": fmt.Sprintf("%s, %s.", greeting, who),
		"scope":   b.instance.Scope,
		"project": b.instance.ProjectID,
		"admin":   request.Caller.IsAdmin,
		"visits":  b.visits,
	})
}

func (b *backend) readVisits(appplugin.Request) appplugin.Response {
	b.mu.Lock()
	defer b.mu.Unlock()
	return appplugin.JSON(http.StatusOK, map[string]int{"visits": b.visits})
}

func (b *backend) countVisit(appplugin.Request) appplugin.Response {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.visits++
	if err := writeVisits(b.instance.DataDir, b.visits); err != nil {
		// The count is still correct in memory, so the call succeeds and the
		// browser sees it; only its survival across a restart is lost. A
		// plugin's errors are its own to grade — the host only forwards them.
		return appplugin.JSON(http.StatusOK, map[string]any{
			"visits":  b.visits,
			"warning": fmt.Sprintf("not persisted: %v", err),
		})
	}
	return appplugin.JSON(http.StatusOK, map[string]int{"visits": b.visits})
}

// readVisits tolerates every kind of missing: no DataDir, no file, or a file
// this version cannot read. A fresh install and an unreadable one both start
// at zero rather than failing Init, which would fail the app's start.
func readVisits(dataDir string) int {
	if dataDir == "" {
		return 0
	}
	raw, err := os.ReadFile(filepath.Join(dataDir, visitsFile))
	if err != nil {
		return 0
	}
	var state struct {
		Visits int `json:"visits"`
	}
	if err := json.Unmarshal(raw, &state); err != nil {
		return 0
	}
	return state.Visits
}

func writeVisits(dataDir string, visits int) error {
	if dataDir == "" {
		return fmt.Errorf("no data directory")
	}
	raw, err := json.Marshal(map[string]int{"visits": visits})
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dataDir, visitsFile), raw, 0o600)
}
