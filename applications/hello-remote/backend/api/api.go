// Package api implements Hello Remote's host API. The executable composition
// root lives at backend/main.go; this package owns only request handling.
package api

import (
	"sync"

	appLifecycle "futrx.local/catalog/applications/hello-remote/backend/lifecycle"
	"github.com/futrx-com/remote.futrx.com/pkg/applications"
)

type api struct {
	router           *applications.Router
	inspectContainer func(string) (containerFacts, error)
	inspectService   func(int) (serviceHealth, error)
	greetings        appLifecycle.GreetingEvents
	inspections      appLifecycle.InspectionEvents

	mu       sync.Mutex
	instance applications.Instance
	visits   int
}

var (
	_ applications.Backend = (*api)(nil)
)

// REQUIRED — Describe runs once when the host connects. APIVersion must use
// applications.APIVersion or the host refuses the process. Routes() reports what
// handler registered, so the route table the SPA discovers through
// remote.backend.describe() cannot drift from the one actually served.
func (b *api) Describe() (applications.Descriptor, error) {
	return applications.Descriptor{
		APIVersion: applications.APIVersion,
		Routes:     b.router.Routes(),
	}, nil
}

// REQUIRED — Init runs once before the first request, and is where the process learns
// which installed copy it belongs to. A global install and two project
// installs are three processes, each with its own Instance and its own
// DataDir.
func (b *api) Init(instance applications.Instance) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.instance = instance
	b.visits = readVisits(instance.DataDir)
	return nil
}

// REQUIRED — Handle may be called concurrently.
func (b *api) Handle(request applications.Request) (applications.Response, error) {
	return b.router.Serve(request), nil
}

// New constructs the request API from lifecycle dependencies supplied by the
// backend composition root.
func New(
	greetings appLifecycle.GreetingEvents,
	inspections appLifecycle.InspectionEvents,
) applications.Backend {
	return handler(greetings, inspections)
}

func handler(
	greetings appLifecycle.GreetingEvents,
	inspections appLifecycle.InspectionEvents,
) *api {
	h := &api{
		router:           applications.NewRouter(),
		inspectContainer: readContainerFacts,
		inspectService:   readServiceHealth,
		greetings:        greetings,
		inspections:      inspections,
	}

	h.router.GET("hello", "Greet the calling user", h.hello)
	h.router.POST("echo", "Echo JSON, query, and headers from the frontend API explorer", h.echo)
	h.router.GET("container", "Report safe facts about this install's LXD container", h.container)
	h.router.GET("service", "Report the supervised container service and its safe configuration", h.service)
	h.router.GET("visits", "Report how many greetings this install has served", h.readVisits)
	h.router.POST("visits", "Count one greeting", h.countVisit)
	return h
}
