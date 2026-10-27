package httphandlers

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	serviceapplications "github.com/futrx-com/remote.futrx.com/internal/service/applications"
	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileapplications"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileauth"
	httptransport "github.com/futrx-com/remote.futrx.com/internal/transport/http"
	httpmiddleware "github.com/futrx-com/remote.futrx.com/internal/transport/http/middleware"
	"github.com/gorilla/websocket"
)

const webTestID = "abcdef123456"
const webTestHost = "editor--project.remote.test"

type webTestRegistry struct {
	application serviceapplications.Application
}

func (r *webTestRegistry) List() []serviceapplications.Application {
	return []serviceapplications.Application{r.application}
}
func (r *webTestRegistry) Get(id string) (serviceapplications.Application, bool) {
	return r.application, id == r.application.ID
}
func (*webTestRegistry) UIAsset(string, string) ([]byte, bool) { return nil, false }

type webTestUsers struct{}

func (webTestUsers) IsAdmin(_ context.Context, email string) (bool, error) {
	return email == "admin@example.test", nil
}
func (webTestUsers) IsRegistered(_ context.Context, email string) (bool, error) {
	return email != "removed@example.test", nil
}
func (webTestUsers) AddBootstrapAdmin(context.Context, string) error { return nil }
func (webTestUsers) FirstAdmin(context.Context) (*serviceauth.UserDirectoryEntry, error) {
	return &serviceauth.UserDirectoryEntry{Email: "admin@example.test"}, nil
}

type webTestProjects struct{ project serviceproject.Meta }

func (p webTestProjects) ListVisible(_ context.Context, email string, isAdmin bool) ([]serviceproject.Meta, error) {
	if isAdmin || email == "member@example.test" {
		return []serviceproject.Meta{p.project}, nil
	}
	return nil, nil
}

type webFixture struct {
	apps     *ApplicationsHandler
	auth     *serviceauth.Service
	store    *fileapplications.Store
	registry *webTestRegistry
	instance serviceapplications.Instance
	projects webTestProjects
}

func newWebFixture(t *testing.T, baseURL string) *webFixture {
	t.Helper()
	auth, err := serviceauth.New(context.Background(), fileauth.New(t.TempDir()), webTestUsers{},
		func(string, string, string) serviceauth.OAuthProvider { return verifyOAuthProvider{} },
		baseURL, []byte("web-fixture-signing-key"), twoFactorStoreForTest(t), sessionRegistryStoreForTest(t), testAuthOptions())
	if err != nil {
		t.Fatal(err)
	}
	store, err := fileapplications.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	registry := &webTestRegistry{application: serviceapplications.Application{ID: "editor", Scopes: []serviceapplications.Scope{serviceapplications.ScopeProject}, Web: &serviceapplications.ApplicationWeb{Port: 8400, Subdomain: "editor"}}}
	instance := serviceapplications.Instance{ID: webTestID, ApplicationID: "editor", Scope: serviceapplications.ScopeProject, ProjectID: "aaaa1111", Status: serviceapplications.StatusRunning}
	if err := store.Put(context.Background(), instance); err != nil {
		t.Fatal(err)
	}
	projects := webTestProjects{project: serviceproject.Meta{ID: "aaaa1111", Slug: "project"}}
	base, err := url.Parse(baseURL)
	if err != nil {
		t.Fatal(err)
	}
	apps := NewApplicationsHandler(serviceapplications.New(registry, store, nil, nil, nil), auth, projects).WithWebHost(base.Host)
	return &webFixture{apps: apps, auth: auth, store: store, registry: registry, instance: instance, projects: projects}
}

func (f *webFixture) cookie(t *testing.T, email string) *http.Cookie {
	t.Helper()
	value, err := f.auth.IssueSession(context.Background(), serviceauth.User{Email: email, Sub: email}, serviceauth.SignInMethodGoogle, "", "")
	if err != nil {
		t.Fatal(err)
	}
	return &http.Cookie{Name: serviceauth.SessionCookieName, Value: value}
}

func (f *webFixture) handler() http.Handler {
	platform := http.NewServeMux()
	f.apps.RegisterRoutes(platform)
	platform.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "PLATFORM") })
	return httpmiddleware.NewBrowserProtection(f.auth.BaseURL()).Wrap(f.apps.WebHandler(platform))
}

func TestApplicationWebAuthorizationAndLaunch(t *testing.T) {
	f := newWebFixture(t, "https://remote.test")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
			t.Error("credentials reached application")
		}
		if r.Host != webTestHost || r.Header.Get("X-Forwarded-Proto") != "https" {
			t.Error("public origin lost")
		}
		fmt.Fprint(w, "APPLICATION "+r.URL.RequestURI())
	}))
	defer upstream.Close()
	f.apps.webTransport = &http.Transport{DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
		if addr != "project.lxd:8400" {
			t.Errorf("unexpected target %q", addr)
		}
		return (&net.Dialer{}).DialContext(ctx, network, strings.TrimPrefix(upstream.URL, "http://"))
	}}
	member := f.cookie(t, "member@example.test")
	for _, tc := range []struct {
		name, method, target string
		cookie               *http.Cookie
		status               int
		location, body       string
	}{
		{"launch", "GET", "https://remote.test/apps/project/editor/src/a%2Fb?line=7", member, 302, "https://" + webTestHost + "/src/a%2Fb?line=7", ""},
		{"bare launch", "GET", "https://remote.test/apps/project/editor", member, 302, "https://" + webTestHost + "/", ""},
		{"launch is read only", "POST", "https://remote.test/apps/project/editor/", member, 405, "", ""},
		{"app asset", "GET", "https://" + webTestHost + "/src/a%2Fb?line=7", member, 200, "", "APPLICATION /src/a%2Fb?line=7"},
		{"no platform API on app host", "GET", "https://" + webTestHost + "/api/projects", member, 200, "", "APPLICATION /api/projects"},
		{"no platform auth on app host", "GET", "https://" + webTestHost + "/auth/me", member, 200, "", "APPLICATION /auth/me"},
		{"no platform internal route", "GET", "https://" + webTestHost + "/internal/tls-ask", member, 200, "", "APPLICATION /internal/tls-ask"},
		{"removed installation origin", "GET", "https://" + webTestID + ".apps.remote.test/", member, 404, "", ""},
		{"malformed app host", "GET", "https://bad.remote.test/", member, 404, "", ""},
		{"unknown install", "GET", "https://unknown--project.remote.test/", member, 404, "", ""},
		{"nonmember", "GET", "https://" + webTestHost + "/", f.cookie(t, "other@example.test"), 404, "", ""},
		{"removed user", "GET", "https://" + webTestHost + "/", f.cookie(t, "removed@example.test"), 403, "", ""},
		{"admin", "GET", "https://" + webTestHost + "/", f.cookie(t, "admin@example.test"), 200, "", "APPLICATION /"},
		{"anonymous navigation", "GET", "https://" + webTestHost + "/", nil, 302, "https://remote.test/?return_to=" + url.QueryEscape("https://"+webTestHost+"/"), ""},
		{"anonymous write", "POST", "https://" + webTestHost + "/", nil, 401, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.target, nil)
			if tc.cookie != nil {
				req.AddCookie(tc.cookie)
			}
			req.Header.Set("Authorization", "Bearer should-not-leak")
			req.Header.Set("X-Forwarded-Host", "attacker.invalid")
			rec := httptest.NewRecorder()
			f.handler().ServeHTTP(rec, req)
			if rec.Code != tc.status || rec.Header().Get("Location") != tc.location || (tc.body != "" && rec.Body.String() != tc.body) {
				t.Fatalf("status=%d location=%q body=%q", rec.Code, rec.Header().Get("Location"), rec.Body.String())
			}
		})
	}
	for _, state := range []serviceapplications.InstanceStatus{serviceapplications.StatusStopped, serviceapplications.StatusError} {
		f.instance.Status = state
		if err := f.store.Put(context.Background(), f.instance); err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest("GET", "https://"+webTestHost+"/", nil)
		req.AddCookie(member)
		rec := httptest.NewRecorder()
		f.handler().ServeHTTP(rec, req)
		if rec.Code != 404 {
			t.Fatalf("%s install returned %d", state, rec.Code)
		}
	}
}

func TestApplicationWebSocketOriginAndProxy(t *testing.T) {
	f := newWebFixture(t, "https://remote.test")
	upgrader := httptransport.NewUpgrader()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "" {
			t.Error("cookie reached socket server")
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		kind, data, err := conn.ReadMessage()
		if err == nil {
			_ = conn.WriteMessage(kind, data)
		}
	}))
	defer upstream.Close()
	f.apps.webTransport = &http.Transport{DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, strings.TrimPrefix(upstream.URL, "http://"))
	}}
	gateway := httptest.NewServer(f.handler())
	defer gateway.Close()
	dialer := websocket.Dialer{NetDial: func(network, _ string) (net.Conn, error) {
		return net.Dial(network, strings.TrimPrefix(gateway.URL, "http://"))
	}}
	for _, origin := range []string{"https://" + webTestHost, "https://other--project.remote.test"} {
		header := http.Header{"Origin": {origin}, "Cookie": {f.cookie(t, "member@example.test").String()}}
		conn, response, err := dialer.Dial("ws://"+webTestHost+"/socket?x=1", header)
		if origin != "https://"+webTestHost {
			if err == nil {
				conn.Close()
				t.Fatal("cross-application socket accepted")
			}
			if response == nil || response.StatusCode != 403 {
				t.Fatalf("cross-origin response=%v err=%v", response, err)
			}
			response.Body.Close()
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if err := conn.WriteMessage(websocket.TextMessage, []byte("hello")); err != nil {
			t.Fatal(err)
		}
		_, data, err := conn.ReadMessage()
		conn.Close()
		if err != nil || string(data) != "hello" {
			t.Fatalf("echo=%q err=%v", data, err)
		}
	}
}

func TestApplicationWebCertificatesRequireRunningProjectInstall(t *testing.T) {
	handler, project := newTLSAskProjectHandler(t, "remote.test")
	f := newWebFixture(t, "https://remote.test")
	if err := f.store.Delete(context.Background(), f.instance.ID); err != nil {
		t.Fatal(err)
	}
	f.instance.ProjectID = string(project.ID)
	handler.apps = f.apps
	webHost := "editor--" + project.Slug + ".remote.test"
	for _, tc := range []struct {
		name, host string
		state      serviceapplications.InstanceStatus
		scope      serviceapplications.Scope
		projectID  string
		web        bool
		want       int
	}{
		{"removed installation origin", webTestID + ".apps.remote.test", serviceapplications.StatusRunning, serviceapplications.ScopeProject, string(project.ID), true, 404},
		{"running", webHost, serviceapplications.StatusRunning, serviceapplications.ScopeProject, string(project.ID), true, 200},
		{"unknown", "unknown--project.remote.test", serviceapplications.StatusRunning, serviceapplications.ScopeProject, string(project.ID), true, 404},
		{"malformed", "bad.remote.test", serviceapplications.StatusRunning, serviceapplications.ScopeProject, string(project.ID), true, 404},
		{"foreign suffix", webTestHost + ".evil.test", serviceapplications.StatusRunning, serviceapplications.ScopeProject, string(project.ID), true, 404},
		{"stopped", webHost, serviceapplications.StatusStopped, serviceapplications.ScopeProject, string(project.ID), true, 404},
		{"global", webHost, serviceapplications.StatusRunning, serviceapplications.ScopeGlobal, "", true, 404},
		{"missing project", webHost, serviceapplications.StatusRunning, serviceapplications.ScopeProject, "bbbb2222", true, 404},
		{"no web declaration", webHost, serviceapplications.StatusRunning, serviceapplications.ScopeProject, string(project.ID), false, 404},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := f.store.Delete(context.Background(), f.instance.ID); err != nil {
				t.Fatal(err)
			}
			f.instance.Status, f.instance.Scope, f.instance.ProjectID = tc.state, tc.scope, tc.projectID
			f.registry.application.Web = nil
			if tc.web {
				f.registry.application.Web = &serviceapplications.ApplicationWeb{Port: 8400, Subdomain: "editor"}
			}
			if err := f.store.Put(context.Background(), f.instance); err != nil {
				t.Fatal(err)
			}
			rec := httptest.NewRecorder()
			handler.HandleTLSAsk(rec, httptest.NewRequest("GET", "/internal/tls-ask?domain="+url.QueryEscape(tc.host), nil))
			if rec.Code != tc.want {
				t.Fatalf("status=%d want=%d", rec.Code, tc.want)
			}
		})
	}
}

func TestNamedApplicationWebHostUsesWildcard(t *testing.T) {
	for _, label := range []string{"code", "editor-2"} {
		t.Run(label, func(t *testing.T) {
			f := newWebFixture(t, "https://remote.test")
			f.registry.application.Web.Subdomain = label
			canonical := label + "--project.remote.test"
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "APP") }))
			defer upstream.Close()
			f.apps.webTransport = &http.Transport{DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, network, strings.TrimPrefix(upstream.URL, "http://"))
			}}
			for _, host := range []string{canonical, "wrong--project.remote.test", webTestHost, "extra." + canonical} {
				request := httptest.NewRequest("GET", "https://"+host+"/api/", nil)
				request.AddCookie(f.cookie(t, "member@example.test"))
				response := httptest.NewRecorder()
				f.handler().ServeHTTP(response, request)
				want := 404
				if host == canonical {
					want = 200
				}
				if response.Code != want {
					t.Fatalf("host %s: status %d, want %d", host, response.Code, want)
				}
			}
			// Project origins retain authorization and follow the current installation.
			for _, email := range []string{"other@example.test", "removed@example.test"} {
				req := httptest.NewRequest("GET", "https://"+canonical+"/", nil)
				req.AddCookie(f.cookie(t, email))
				rec := httptest.NewRecorder()
				f.handler().ServeHTTP(rec, req)
				if rec.Code == 200 {
					t.Fatalf("unauthorized access: %s", email)
				}
			}
			if err := f.store.Delete(context.Background(), f.instance.ID); err != nil {
				t.Fatal(err)
			}
			f.instance.ID = "123456abcdef"
			if err := f.store.Put(context.Background(), f.instance); err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest("GET", "https://"+canonical+"/", nil)
			req.AddCookie(f.cookie(t, "member@example.test"))
			rec := httptest.NewRecorder()
			f.handler().ServeHTTP(rec, req)
			if rec.Code != 200 {
				t.Fatalf("reinstall changed address: %d", rec.Code)
			}

			request := httptest.NewRequest("GET", "https://remote.test/apps/project/editor/?folder=%2Fworkspace", nil)
			request.AddCookie(f.cookie(t, "member@example.test"))
			response := httptest.NewRecorder()
			f.handler().ServeHTTP(response, request)
			if response.Header().Get("Location") != "https://"+canonical+"/?folder=%2Fworkspace" {
				t.Fatal(response.Header())
			}
			handler, project := newTLSAskProjectHandler(t, "remote.test")
			f.instance.ProjectID = string(project.ID)
			if err := f.store.Put(context.Background(), f.instance); err != nil {
				t.Fatal(err)
			}
			handler.apps = f.apps
			canonical = label + "--" + project.Slug + ".remote.test"
			// Caddy asks before issuing per host: only the canonical host of the
			// running installation is admitted.
			for host, want := range map[string]int{
				canonical: http.StatusOK,
				"wrong--" + project.Slug + ".remote.test": http.StatusNotFound,
				label + "--missing.remote.test":           http.StatusNotFound,
				"extra." + canonical:                      http.StatusNotFound,
				webTestHost:                               http.StatusNotFound,
			} {
				response := httptest.NewRecorder()
				handler.HandleTLSAsk(response, httptest.NewRequest("GET", "/internal/tls-ask?domain="+host, nil))
				if response.Code != want {
					t.Fatalf("TLS %s: status %d, want %d", host, response.Code, want)
				}
			}
			f.instance.Status = serviceapplications.StatusStopped
			if err := f.store.Put(context.Background(), f.instance); err != nil {
				t.Fatal(err)
			}
			response = httptest.NewRecorder()
			handler.HandleTLSAsk(response, httptest.NewRequest("GET", "/internal/tls-ask?domain="+canonical, nil))
			if response.Code != http.StatusNotFound {
				t.Fatalf("TLS %s: stopped installation admitted, got %d", canonical, response.Code)
			}
		})
	}
}
