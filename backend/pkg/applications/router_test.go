package applications

import (
	"net/http"
	"testing"
)

func newTestRouter() *Router {
	router := NewRouter()
	router.GET("health", "", func(Request) Response { return Text(200, "health") })
	router.GET("kv/*", "", func(request Request) Response {
		return Text(200, "get:"+request.Tail("kv/"))
	})
	router.POST("kv/*", "", func(request Request) Response {
		return Text(200, "put:"+request.Tail("kv/"))
	})
	router.Handle("*", "any", "", func(request Request) Response {
		return Text(200, "any:"+request.Method)
	})
	return router
}

func TestRouterRouting(t *testing.T) {
	router := newTestRouter()
	cases := []struct {
		method, path string
		status       int
		body         string
	}{
		{http.MethodGet, "health", 200, "health"},
		{http.MethodGet, "/health", 200, "health"},
		{http.MethodGet, "kv/greeting", 200, "get:greeting"},
		{http.MethodPost, "kv/greeting", 200, "put:greeting"},
		{http.MethodDelete, "kv/greeting", http.StatusMethodNotAllowed, ""},
		{http.MethodPost, "health", http.StatusMethodNotAllowed, ""},
		{http.MethodGet, "nothing", http.StatusNotFound, ""},
		{http.MethodPatch, "any", 200, "any:PATCH"},
	}
	for _, tc := range cases {
		response := router.Serve(Request{Method: tc.method, Path: tc.path})
		if response.Status != tc.status {
			t.Errorf("%s %s: status = %d, want %d", tc.method, tc.path, response.Status, tc.status)
		}
		if tc.body != "" && string(response.Body) != tc.body {
			t.Errorf("%s %s: body = %q, want %q", tc.method, tc.path, response.Body, tc.body)
		}
	}
}

// An exact route and a prefix route can both match; the exact one has to win,
// or a backend could never special-case one key of a wildcard collection.
func TestRouterPrefersExactOverPrefixAndLongerPrefix(t *testing.T) {
	router := NewRouter()
	router.GET("kv/*", "", func(Request) Response { return Text(200, "short") })
	router.GET("kv/system/*", "", func(Request) Response { return Text(200, "long") })
	router.GET("kv/system/version", "", func(Request) Response { return Text(200, "exact") })

	for path, want := range map[string]string{
		"kv/a":              "short",
		"kv/system/other":   "long",
		"kv/system/version": "exact",
	} {
		if got := string(router.Serve(Request{Method: http.MethodGet, Path: path}).Body); got != want {
			t.Errorf("%s = %q, want %q", path, got, want)
		}
	}
}

func TestRouterRoutesAreStableAndDeduplicated(t *testing.T) {
	router := newTestRouter()
	router.GET("health", "replaced", func(Request) Response { return Text(200, "again") })

	routes := router.Routes()
	if len(routes) != 4 {
		t.Fatalf("routes = %d, want 4: %+v", len(routes), routes)
	}
	if routes[0].Path != "any" || routes[1].Path != "health" {
		t.Errorf("routes are not sorted by path: %+v", routes)
	}
	if routes[1].Description != "replaced" {
		t.Errorf("re-registering did not replace the route: %+v", routes[1])
	}
}

func TestRequestHelpers(t *testing.T) {
	request := Request{
		Path:    "kv/greeting",
		Query:   map[string][]string{"n": {"7"}},
		Headers: map[string][]string{"Content-Type": {"application/json"}},
		Body:    []byte(`{"value":"hi"}`),
	}
	if got := request.Tail("kv/"); got != "greeting" {
		t.Errorf("Tail = %q", got)
	}
	if got := request.Tail("other/"); got != "" {
		t.Errorf("Tail on a non-match = %q, want empty", got)
	}
	if got := request.QueryValue("n"); got != "7" {
		t.Errorf("QueryValue = %q", got)
	}
	if got := request.Header("content-type"); got != "application/json" {
		t.Errorf("Header is not case-insensitive: %q", got)
	}
	var decoded struct {
		Value string `json:"value"`
	}
	if err := request.DecodeJSON(&decoded); err != nil || decoded.Value != "hi" {
		t.Errorf("DecodeJSON = %+v, %v", decoded, err)
	}
	if err := (Request{}).DecodeJSON(&decoded); err == nil {
		t.Error("DecodeJSON on an empty body should fail")
	}
}

// A bare "*" is the catch-all a backend registers as its fallback: it is a
// prefix route whose prefix is empty, and reading an empty prefix as "no
// prefix" turned it into a pattern that matched nothing at all.
func TestRouterCatchAllPatternMatchesEveryPath(t *testing.T) {
	router := NewRouter()
	router.GET("health", "", func(Request) Response { return Text(200, "health") })
	router.Handle("*", "*", "", func(request Request) Response {
		return Text(200, "fallback:"+request.Path)
	})

	for path, want := range map[string]string{
		"health":      "health",
		"anything":    "fallback:anything",
		"deep/path/x": "fallback:deep/path/x",
		"":            "fallback:",
	} {
		response := router.Serve(Request{Method: http.MethodGet, Path: path})
		if response.Status != 200 {
			t.Errorf("GET %q: status = %d, want 200", path, response.Status)
		}
		if string(response.Body) != want {
			t.Errorf("GET %q: body = %q, want %q", path, response.Body, want)
		}
	}
}
