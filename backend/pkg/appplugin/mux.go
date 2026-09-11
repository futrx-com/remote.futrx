package appplugin

import (
	"net/http"
	"sort"
	"strings"
	"sync"
)

// Handler serves one request matched by a Mux.
type Handler func(Request) Response

// Mux is the small router most plugins want instead of a switch statement. It
// also builds the Routes half of a Descriptor, so a plugin's advertised
// surface cannot drift from the one it actually serves.
//
// Patterns are either exact ("health") or a prefix ending in "*" ("kv/*", which
// matches "kv/greeting" and "kv/"; a bare "*" matches every path). Longer
// prefixes win over shorter ones; an exact route always wins over a prefix. "*"
// as a method matches any method.
type Mux struct {
	mu     sync.RWMutex
	routes []muxRoute
}

type muxRoute struct {
	method  string
	pattern string
	// prefix is what a wildcard pattern matches on; wildcard says whether the
	// pattern had one at all. The two are separate because a bare "*" is a
	// wildcard whose prefix is empty — reading an empty prefix as "not a
	// wildcard" would make the catch-all every plugin's fallback route the one
	// pattern that matches nothing.
	prefix      string
	wildcard    bool
	description string
	handler     Handler
}

// NewMux returns an empty Mux.
func NewMux() *Mux { return &Mux{} }

// Handle registers a handler. Registering the same method and pattern twice
// replaces the first, which keeps a plugin's route table honest when its
// registration is built from a loop.
func (m *Mux) Handle(method, pattern, description string, handler Handler) {
	pattern = strings.TrimPrefix(strings.TrimSpace(pattern), "/")
	route := muxRoute{
		method:      strings.ToUpper(strings.TrimSpace(method)),
		pattern:     pattern,
		description: description,
		handler:     handler,
	}
	if strings.HasSuffix(pattern, "*") {
		route.prefix = strings.TrimSuffix(pattern, "*")
		route.wildcard = true
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, existing := range m.routes {
		if existing.method == route.method && existing.pattern == route.pattern {
			m.routes[i] = route
			return
		}
	}
	m.routes = append(m.routes, route)
}

// GET and POST are the two shorthands worth having; anything else is rare
// enough to spell out with Handle.
func (m *Mux) GET(pattern, description string, handler Handler) {
	m.Handle(http.MethodGet, pattern, description, handler)
}

func (m *Mux) POST(pattern, description string, handler Handler) {
	m.Handle(http.MethodPost, pattern, description, handler)
}

// Routes reports the registered routes in a stable order, for a Descriptor.
func (m *Mux) Routes() []Route {
	m.mu.RLock()
	defer m.mu.RUnlock()
	routes := make([]Route, 0, len(m.routes))
	for _, route := range m.routes {
		routes = append(routes, Route{
			Method:      route.method,
			Path:        route.pattern,
			Description: route.description,
		})
	}
	sort.SliceStable(routes, func(i, j int) bool {
		if routes[i].Path != routes[j].Path {
			return routes[i].Path < routes[j].Path
		}
		return routes[i].Method < routes[j].Method
	})
	return routes
}

// Serve dispatches a request, answering 404 when nothing matches and 405 when
// only the method is wrong — the distinction a caller needs to tell "wrong
// URL" from "wrong verb".
func (m *Mux) Serve(request Request) Response {
	path := strings.TrimPrefix(request.Path, "/")
	method := strings.ToUpper(request.Method)

	m.mu.RLock()
	defer m.mu.RUnlock()

	var (
		best      *muxRoute
		bestScore = -1
		pathKnown bool
	)
	for i := range m.routes {
		route := &m.routes[i]
		score, matches := route.match(path)
		if !matches {
			continue
		}
		pathKnown = true
		if route.method != "*" && route.method != method {
			continue
		}
		if score > bestScore {
			best, bestScore = route, score
		}
	}
	switch {
	case best != nil:
		return best.handler(request)
	case pathKnown:
		return Errorf(http.StatusMethodNotAllowed, "method %s not allowed for %q", method, path)
	default:
		return Errorf(http.StatusNotFound, "no route for %q", path)
	}
}

// match scores a route against a path: an exact hit outranks every prefix, and
// a longer prefix outranks a shorter one.
func (r muxRoute) match(path string) (int, bool) {
	if !r.wildcard {
		if r.pattern == path {
			return 1 << 30, true
		}
		return 0, false
	}
	if strings.HasPrefix(path, r.prefix) {
		return len(r.prefix), true
	}
	return 0, false
}
