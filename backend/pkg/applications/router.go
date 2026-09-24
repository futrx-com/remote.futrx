package applications

import (
	"net/http"
	"sort"
	"strings"
	"sync"
)

// Handler serves one request matched by a Router.
type Handler func(Request) Response

// Router is the small request router most backends want instead of a switch statement. It
// also builds the Routes half of a Descriptor, so a backend's advertised
// surface cannot drift from the one it actually serves.
//
// Patterns are either exact ("health") or a prefix ending in "*" ("kv/*", which
// matches "kv/greeting" and "kv/"; a bare "*" matches every path). Longer
// prefixes win over shorter ones; an exact route always wins over a prefix. "*"
// as a method matches any method.
type Router struct {
	mu     sync.RWMutex
	routes []registeredRoute
}

type registeredRoute struct {
	method  string
	pattern string
	// prefix is what a wildcard pattern matches on; wildcard says whether the
	// pattern had one at all. The two are separate because a bare "*" is a
	// wildcard whose prefix is empty — reading an empty prefix as "not a
	// wildcard" would make the catch-all every backend's fallback route the one
	// pattern that matches nothing.
	prefix      string
	wildcard    bool
	description string
	handler     Handler
}

// NewRouter returns an empty Router.
func NewRouter() *Router { return &Router{} }

// Handle registers a handler. Registering the same method and pattern twice
// replaces the first, which keeps a backend's route table honest when its
// registration is built from a loop.
func (router *Router) Handle(method, pattern, description string, handler Handler) {
	pattern = strings.TrimPrefix(strings.TrimSpace(pattern), "/")
	route := registeredRoute{
		method:      strings.ToUpper(strings.TrimSpace(method)),
		pattern:     pattern,
		description: description,
		handler:     handler,
	}
	if strings.HasSuffix(pattern, "*") {
		route.prefix = strings.TrimSuffix(pattern, "*")
		route.wildcard = true
	}
	router.mu.Lock()
	defer router.mu.Unlock()
	for i, existing := range router.routes {
		if existing.method == route.method && existing.pattern == route.pattern {
			router.routes[i] = route
			return
		}
	}
	router.routes = append(router.routes, route)
}

// GET and POST are the two shorthands worth having; anything else is rare
// enough to spell out with Handle.
func (router *Router) GET(pattern, description string, handler Handler) {
	router.Handle(http.MethodGet, pattern, description, handler)
}

func (router *Router) POST(pattern, description string, handler Handler) {
	router.Handle(http.MethodPost, pattern, description, handler)
}

// Routes reports the registered routes in a stable order, for a Descriptor.
func (router *Router) Routes() []Route {
	router.mu.RLock()
	defer router.mu.RUnlock()
	routes := make([]Route, 0, len(router.routes))
	for _, route := range router.routes {
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
func (router *Router) Serve(request Request) Response {
	path := strings.TrimPrefix(request.Path, "/")
	method := strings.ToUpper(request.Method)

	router.mu.RLock()
	defer router.mu.RUnlock()

	var (
		best      *registeredRoute
		bestScore = -1
		pathKnown bool
	)
	for i := range router.routes {
		route := &router.routes[i]
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
	// HEAD has the same representation metadata as GET. Give an explicitly
	// registered HEAD route (or an any-method route) the first chance above,
	// then reuse the most-specific GET route when neither exists. Keep the
	// original method on request so a handler can still avoid GET-only work.
	if best == nil && method == http.MethodHead {
		for i := range router.routes {
			route := &router.routes[i]
			if route.method != http.MethodGet {
				continue
			}
			score, matches := route.match(path)
			if matches && score > bestScore {
				best, bestScore = route, score
			}
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
func (r registeredRoute) match(path string) (int, bool) {
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
