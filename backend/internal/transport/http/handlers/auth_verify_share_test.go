package httphandlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
	serviceshare "github.com/futrx-com/remote.futrx.com/internal/service/share"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileauth"
)

const (
	verifyBaseHost    = "remote.example.test"
	verifyProjectSlug = "alpha"
	verifyPreviewHost = verifyProjectSlug + "--3000.dev." + verifyBaseHost
)

// TestVerifyAcceptsShareTokenAndIssuesScopedCookie pins the first-visit
// exchange: a valid token becomes a redirect that both sets the share cookie
// and drops the token from the URL.
func TestVerifyAcceptsShareTokenAndIssuesScopedCookie(t *testing.T) {
	handler, shares := newVerifyHandler(t)
	shares.share = serviceshare.Share{
		ID:        "1f2e3d4c",
		Port:      3000,
		ExpiresAt: time.Now().Add(24 * time.Hour).UnixMilli(),
	}
	shares.validToken = "good-token"

	rec := verifyRequest(t, handler, verifyPreviewHost, "/dashboard?share=good-token&keep=1", nil)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d (forward_auth only relays non-2xx responses)", rec.Code, http.StatusFound)
	}
	location := rec.Header().Get("Location")
	if strings.Contains(location, "share=") {
		t.Fatalf("Location = %q, want the token stripped", location)
	}
	if want := "https://" + verifyPreviewHost + "/dashboard?keep=1"; location != want {
		t.Fatalf("Location = %q, want %q", location, want)
	}

	cookie := responseCookie(rec, serviceauth.ShareCookieName)
	if cookie == nil {
		t.Fatal("no share cookie was set")
	}
	if cookie.Domain != "" {
		t.Fatalf("cookie Domain = %q, want host-only so it cannot reach other hosts", cookie.Domain)
	}
	if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("cookie flags = %#v, want HttpOnly, Secure, SameSite=Lax", cookie)
	}
	pass, err := handler.auth.VerifySharePass(cookie.Value)
	if err != nil {
		t.Fatalf("VerifySharePass: %v", err)
	}
	if pass.Slug != verifyProjectSlug || pass.Port != 3000 || pass.ShareID != "1f2e3d4c" {
		t.Fatalf("pass = %#v, want it bound to this slug, port, and share", pass)
	}
}

// TestVerifyAcceptsShareCookieOnLaterRequests covers the steady state, where
// the visitor no longer carries a token.
func TestVerifyAcceptsShareCookieOnLaterRequests(t *testing.T) {
	handler, shares := newVerifyHandler(t)
	shares.allows = true
	pass := handler.auth.SignSharePass(serviceauth.SharePass{
		Slug:    verifyProjectSlug,
		Port:    3000,
		ShareID: "1f2e3d4c",
		Exp:     time.Now().Add(time.Hour).Unix(),
	})

	rec := verifyRequest(t, handler, verifyPreviewHost, "/assets/app.js", &http.Cookie{
		Name: serviceauth.ShareCookieName, Value: pass,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if responseCookie(rec, serviceauth.ShareCookieName) != nil {
		t.Fatal("the steady-state path re-issued the share cookie")
	}
}

func TestVerifyRejectsShareAttempts(t *testing.T) {
	validPass := func(slug string, port int) *http.Cookie {
		handler, _ := newVerifyHandler(t)
		return &http.Cookie{
			Name: serviceauth.ShareCookieName,
			Value: handler.auth.SignSharePass(serviceauth.SharePass{
				Slug: slug, Port: port, ShareID: "1f2e3d4c",
				Exp: time.Now().Add(time.Hour).Unix(),
			}),
		}
	}

	tests := []struct {
		name       string
		host       string
		uri        string
		cookie     *http.Cookie
		validToken string
		allows     bool
	}{
		{
			name: "unknown token",
			host: verifyPreviewHost, uri: "/?share=wrong", validToken: "good-token",
		},
		{
			name: "no token and no cookie",
			host: verifyPreviewHost, uri: "/",
		},
		{
			name: "agent browser port is never shareable",
			host: verifyProjectSlug + "--6080.dev." + verifyBaseHost,
			uri:  "/vnc.html?share=good-token", validToken: "good-token",
		},
		{
			name: "IDE host ignores share tokens",
			host: verifyProjectSlug + ".code." + verifyBaseHost,
			uri:  "/?share=good-token", validToken: "good-token",
		},
		{
			name: "main application ignores share tokens",
			host: verifyBaseHost, uri: "/?share=good-token", validToken: "good-token",
		},
		{
			name: "preview host on another base domain",
			host: verifyProjectSlug + "--3000.dev.attacker.test",
			uri:  "/?share=good-token", validToken: "good-token",
		},
		{
			name: "cookie minted for another project",
			host: verifyPreviewHost, uri: "/",
			cookie: validPass("beta", 3000), allows: true,
		},
		{
			name: "cookie minted for another port",
			host: verifyPreviewHost, uri: "/",
			cookie: validPass(verifyProjectSlug, 5173), allows: true,
		},
		{
			name: "cookie for a revoked link",
			host: verifyPreviewHost, uri: "/",
			cookie: validPass(verifyProjectSlug, 3000), allows: false,
		},
		{
			name: "forged cookie",
			host: verifyPreviewHost, uri: "/",
			cookie: &http.Cookie{Name: serviceauth.ShareCookieName, Value: "bogus.signature"},
			allows: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler, shares := newVerifyHandler(t)
			shares.validToken = test.validToken
			shares.allows = test.allows
			shares.share = serviceshare.Share{
				ID:        "1f2e3d4c",
				Port:      3000,
				ExpiresAt: time.Now().Add(time.Hour).UnixMilli(),
			}

			rec := verifyRequest(t, handler, test.host, test.uri, test.cookie)

			if rec.Code == http.StatusOK {
				t.Fatal("status = 200, want the request refused")
			}
			if responseCookie(rec, serviceauth.ShareCookieName) != nil {
				t.Fatal("a share cookie was issued for a refused request")
			}
			if rec.Code == http.StatusFound {
				location := rec.Header().Get("Location")
				if !strings.HasPrefix(location, "https://"+verifyBaseHost+"/") {
					t.Fatalf("Location = %q, want the platform login page", location)
				}
			}
		})
	}
}

func verifyRequest(
	t *testing.T,
	handler *authVerifyHandler,
	host, uri string,
	cookie *http.Cookie,
) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/auth/verify", nil)
	req.Header.Set("X-Forwarded-Host", host)
	req.Header.Set("X-Forwarded-Uri", uri)
	req.Header.Set("X-Forwarded-Proto", "https")
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	handler.verify(rec, req)
	return rec
}

func responseCookie(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, cookie := range (&http.Response{Header: rec.Header()}).Cookies() {
		if cookie.Name == name {
			return cookie
		}
	}
	return nil
}

func newVerifyHandler(t *testing.T) (*authVerifyHandler, *shareAuthorizerStub) {
	t.Helper()
	auth, err := serviceauth.New(
		context.Background(),
		fileauth.New(t.TempDir()),
		verifyUserDirectory{},
		func(string, string, string) serviceauth.OAuthProvider { return verifyOAuthProvider{} },
		"https://"+verifyBaseHost,
		[]byte("verify-handler-test-key"),
	)
	if err != nil {
		t.Fatalf("New auth service: %v", err)
	}
	shares := &shareAuthorizerStub{}
	return &authVerifyHandler{
		auth:   auth,
		access: serviceauth.NewAccessVerifier(auth, verifyProjectDirectory{}),
		shares: shares,
	}, shares
}

type shareAuthorizerStub struct {
	validToken string
	share      serviceshare.Share
	allows     bool
}

func (s *shareAuthorizerStub) Validate(
	_ context.Context, _ string, _ int, token string,
) (serviceshare.Share, bool) {
	if s.validToken == "" || token != s.validToken {
		return serviceshare.Share{}, false
	}
	return s.share, true
}

func (s *shareAuthorizerStub) Allows(context.Context, string, int, serviceshare.ID) bool {
	return s.allows
}

type verifyProjectDirectory struct{}

func (verifyProjectDirectory) GetBySlug(_ context.Context, slug string) (serviceproject.Meta, error) {
	if slug != verifyProjectSlug {
		return serviceproject.Meta{}, serviceproject.ErrNotFound
	}
	return serviceproject.Meta{ID: "aaaa1111", Slug: slug}, nil
}

func (verifyProjectDirectory) HasAccess(context.Context, serviceproject.ID, string) (bool, error) {
	return false, nil
}

type verifyUserDirectory struct{}

func (verifyUserDirectory) IsAdmin(context.Context, string) (bool, error)      { return false, nil }
func (verifyUserDirectory) IsRegistered(context.Context, string) (bool, error) { return false, nil }
func (verifyUserDirectory) AddBootstrapAdmin(context.Context, string) error    { return nil }
func (verifyUserDirectory) FirstAdmin(context.Context) (*serviceauth.UserDirectoryEntry, error) {
	return nil, nil
}

type verifyOAuthProvider struct{}

func (verifyOAuthProvider) AuthCodeURL(string) string { return "https://accounts.example.test" }
func (verifyOAuthProvider) ExchangeUser(context.Context, string) (serviceauth.User, error) {
	return serviceauth.User{}, nil
}
