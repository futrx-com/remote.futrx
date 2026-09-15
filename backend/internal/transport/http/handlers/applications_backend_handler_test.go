package httphandlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/futrx-com/remote.futrx.com/pkg/appplugin"
)

// The plugin prefix has to be recognised exactly: too loose and an instance
// named "backendish" routes to a plugin, too strict and nested routes break.
func TestIsBackendPath(t *testing.T) {
	for _, tc := range []struct {
		action string
		path   string
		want   bool
	}{
		{"backend", "", true},
		{"backend/", "", true},
		{"backend/health", "health", true},
		{"backend/kv/greeting", "kv/greeting", true},
		{"backendish", "", false},
		{"credentials", "", false},
		{"start", "", false},
		{"", "", false},
	} {
		path, ok := isBackendPath(tc.action)
		if ok != tc.want || path != tc.path {
			t.Errorf("isBackendPath(%q) = %q, %v; want %q, %v", tc.action, path, ok, tc.path, tc.want)
		}
	}
}

// A plugin is told who the caller is; it is not given the means to become
// them. Forwarding the session cookie would hand every plugin the ability to
// act as the signed-in user against the rest of the API.
func TestForwardableHeadersWithholdsCredentials(t *testing.T) {
	forwarded := forwardableHeaders(http.Header{
		"Cookie":            {"session=secret"},
		"Authorization":     {"Bearer token"},
		"Connection":        {"keep-alive"},
		"Transfer-Encoding": {"chunked"},
		"Content-Type":      {"application/json"},
		"X-Request-Id":      {"abc"},
	})
	for _, withheld := range []string{"Cookie", "Authorization", "Connection", "Transfer-Encoding"} {
		if _, present := forwarded[withheld]; present {
			t.Errorf("%s was forwarded to the plugin", withheld)
		}
	}
	if got := forwarded["Content-Type"]; len(got) != 1 || got[0] != "application/json" {
		t.Errorf("Content-Type = %v, want it forwarded", got)
	}
	if got := forwarded["X-Request-Id"]; len(got) != 1 {
		t.Errorf("X-Request-Id = %v, want it forwarded", got)
	}
}

// A plugin's response is same-origin with the SPA, so it must not be able to
// write the session or have its body reinterpreted by the browser.
func TestWriteBackendResponseSanitizesHeaders(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeBackendResponse(recorder, appplugin.Response{
		Status: http.StatusCreated,
		Headers: map[string][]string{
			"Content-Type":     {"application/json; charset=utf-8"},
			"Set-Cookie":       {"session=hijacked"},
			"Connection":       {"close"},
			"Content-Length":   {"999"},
			"Cache-Control":    {"no-store"},
			"X-Plugin-Verdict": {"ok"},
		},
		Body: []byte(`{"ok":true}`),
	})
	result := recorder.Result()
	defer result.Body.Close()

	if result.StatusCode != http.StatusCreated {
		t.Errorf("status = %d", result.StatusCode)
	}
	for _, dropped := range []string{"Set-Cookie", "Connection", "Content-Length"} {
		if value := result.Header.Get(dropped); value != "" {
			t.Errorf("%s survived as %q", dropped, value)
		}
	}
	if result.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Error("the response is sniffable")
	}
	for header, want := range map[string]string{
		"Content-Type":     "application/json; charset=utf-8",
		"Cache-Control":    "no-store",
		"X-Plugin-Verdict": "ok",
	} {
		if got := result.Header.Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
}

// A plugin that answers with nothing still has to produce a valid response,
// and one that cannot be executed by the browser.
func TestWriteBackendResponseDefaults(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		want   int
	}{
		{"unset status", 0, http.StatusOK},
		{"nonsense status", 42, http.StatusOK},
		{"plugin error", http.StatusBadGateway, http.StatusBadGateway},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			writeBackendResponse(recorder, appplugin.Response{Status: tc.status})
			if recorder.Code != tc.want {
				t.Errorf("status = %d, want %d", recorder.Code, tc.want)
			}
			if got := recorder.Header().Get("Content-Type"); got != "application/octet-stream" {
				t.Errorf("Content-Type = %q, want a non-executable default", got)
			}
		})
	}
}
