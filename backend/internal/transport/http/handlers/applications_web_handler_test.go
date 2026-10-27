package httphandlers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestApplicationWebProxyStripsPlatformCookiesAndKeepsRoute(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/src/file" || r.URL.RawQuery != "line=7" || r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
			t.Errorf("upstream request: path=%q query=%q cookie=%q", r.URL.Path, r.URL.RawQuery, r.Header.Get("Cookie"))
		}
		w.Header().Set("Set-Cookie", "remote_session=stolen")
		w.Header().Set("Clear-Site-Data", `"cookies"`)
		w.Header().Set("Location", "/signin")
		w.Header().Set("Service-Worker-Allowed", "/")
		w.WriteHeader(http.StatusFound)
	}))
	defer upstream.Close()
	target, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "https://editor--project.remote.test/src/file?line=7", nil)
	req.Header.Add("Cookie", "remote_session=secret; remote_2fa_pending=pending; remote_oauth_state=state; return_to=target")
	req.Header.Add("Cookie", "remote_share=grant; application_cookie=value")
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	newWebProxy(target, "https").ServeHTTP(rec, req)
	response := rec.Result()
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)
	if response.StatusCode != http.StatusFound || response.Header.Get("Set-Cookie") != "" ||
		response.Header.Get("Location") != "/signin" || response.Header.Get("Clear-Site-Data") != "" ||
		response.Header.Get("Service-Worker-Allowed") != "/" {
		t.Fatalf("proxy response: status=%d cookie=%q location=%q workerScope=%q", response.StatusCode,
			response.Header.Get("Set-Cookie"), response.Header.Get("Location"),
			response.Header.Get("Service-Worker-Allowed"))
	}
}

func TestApplicationWebRouteRejectsMalformedNames(t *testing.T) {
	handler := &ApplicationsHandler{}
	for _, path := range []string{"/apps/../editor/", "/apps/project/../", "/apps/project/@editor/"} {
		rec := httptest.NewRecorder()
		handler.serveWeb(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: status %d", path, rec.Code)
		}
	}
}
