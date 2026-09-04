// Backend Playground: the reference implementation of an image's Go plugin.
//
// It is to the backend API what ui-playground is to the extension API — every
// mechanism the contract offers, exercised once, in the smallest program that
// still demonstrates it:
//
//	health     the process is alive, and it is the same process as last time
//	echo       what a plugin receives from a browser, and what it does not
//	instance   the install the host handed over, redacted by caller
//	kv         state in memory, proving the process outlives a request
//	notes      state on disk, proving DataDir outlives the process
//	compute    real Go running on the server rather than in the browser
//	slow       the per-call timeout, from the plugin's side
//	boom       a panic, and the fact that the server keeps working
//	admin      a route the plugin authorizes itself
//
// Nothing here is specific to what any real image would do. It is the thing to
// run after changing the backend plugin contract.
package main

import (
	"net/http"
	"os"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/futrx-com/remote.futrx.com/pkg/appplugin"
	"github.com/futrx-com/remote.futrx.com/pkg/appplugin/pluginrpc"
)

func main() { pluginrpc.Serve(newPlayground()) }

type playground struct {
	mux     *appplugin.Mux
	started time.Time

	mu       sync.Mutex
	instance appplugin.Instance
	requests int
	values   map[string]string
}

func newPlayground() *playground {
	p := &playground{
		mux:     appplugin.NewMux(),
		started: time.Now(),
		values:  map[string]string{},
	}
	p.mux.GET("health", "Process identity and uptime", p.health)
	p.mux.Handle("*", "echo", "Echo the request as the plugin received it", p.echo)
	p.mux.GET("instance", "The install this process serves", p.describeInstance)
	p.mux.GET("kv", "List in-memory keys", p.listValues)
	p.mux.GET("kv/*", "Read one in-memory key", p.readValue)
	p.mux.POST("kv/*", "Write one in-memory key", p.writeValue)
	p.mux.GET("notes", "Read the note stored in the plugin's data directory", p.readNotes)
	p.mux.POST("notes", "Write the note stored in the plugin's data directory", p.writeNotes)
	p.mux.POST("compute", "Run real Go work on the server", p.compute)
	p.mux.GET("slow", "Sleep for ?ms, to exercise the call timeout", p.slow)
	p.mux.GET("boom", "Panic on purpose", p.boom)
	p.mux.GET("admin", "A route the plugin restricts to administrators", p.adminOnly)
	return p
}

// ---- contract --------------------------------------------------------------

func (p *playground) Describe() (appplugin.Descriptor, error) {
	return appplugin.Descriptor{
		Name:       "Backend Playground",
		Version:    "1",
		APIVersion: appplugin.APIVersion,
		Routes:     p.mux.Routes(),
	}, nil
}

// Init is where a real plugin would open its database or dial the service its
// image installed. Here it only records what it was given, because showing
// what a plugin receives is the fixture's whole point.
func (p *playground) Init(instance appplugin.Instance) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.instance = instance
	return nil
}

func (p *playground) Handle(request appplugin.Request) (appplugin.Response, error) {
	p.mu.Lock()
	p.requests++
	p.mu.Unlock()
	return p.mux.Serve(request), nil
}

// ---- routes ----------------------------------------------------------------

// health is the route to watch when asking "is this the same process?". The
// pid and uptime only reset when the host restarts the plugin, which is what a
// stop, a start, or a crash looks like from the outside.
func (p *playground) health(appplugin.Request) appplugin.Response {
	p.mu.Lock()
	requests := p.requests
	instanceID := p.instance.ID
	p.mu.Unlock()

	return appplugin.JSON(http.StatusOK, map[string]any{
		"message":       "Hello from GO!",
		"ok":            true,
		"pid":           os.Getpid(),
		"instanceId":    instanceID,
		"goVersion":     runtime.Version(),
		"os":            runtime.GOOS,
		"arch":          runtime.GOARCH,
		"uptimeSeconds": int(time.Since(p.started).Seconds()),
		"requests":      requests,
	})
}

// echo shows a plugin author exactly what crosses the boundary — including
// what does not: the caller's cookies are withheld by the transport, and the
// caller identity is stamped by the server rather than sent by the browser.
func (p *playground) echo(request appplugin.Request) appplugin.Response {
	return appplugin.JSON(http.StatusOK, map[string]any{
		"method":  request.Method,
		"path":    request.Path,
		"query":   request.Query,
		"headers": request.Headers,
		"body":    string(request.Body),
		"caller":  request.Caller,
	})
}

// describeInstance redacts by caller rather than by field: the env of an
// install carries the secrets its own install script generated, so a plugin
// that surfaces it has to decide who may see it. This is the pattern every
// plugin handling anything sensitive should copy.
func (p *playground) describeInstance(request appplugin.Request) appplugin.Response {
	p.mu.Lock()
	instance := p.instance
	p.mu.Unlock()

	keys := make([]string, 0, len(instance.Env))
	for key := range instance.Env {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	payload := map[string]any{
		"id":            instance.ID,
		"imageId":       instance.ImageID,
		"scope":         instance.Scope,
		"projectId":     instance.ProjectID,
		"containerName": instance.ContainerName,
		"dataDir":       instance.DataDir,
		"envKeys":       keys,
	}
	if request.Caller.IsAdmin {
		payload["env"] = instance.Env
	}
	return appplugin.JSON(http.StatusOK, payload)
}
