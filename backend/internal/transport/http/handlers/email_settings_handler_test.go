package httphandlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	serviceemail "github.com/futrx-com/remote.futrx.com/internal/service/email"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileauth"
	"github.com/futrx-com/remote.futrx.com/internal/stores/filesessions"
	"github.com/futrx-com/remote.futrx.com/internal/stores/filetwofactor"
)

// emailTestDirectory reports admin status per email so tests can exercise
// both an admin and a non-admin caller.
type emailTestDirectory struct {
	admins map[string]bool
}

func (d emailTestDirectory) IsAdmin(_ context.Context, email string) (bool, error) {
	return d.admins[email], nil
}
func (emailTestDirectory) IsRegistered(context.Context, string) (bool, error) { return true, nil }
func (emailTestDirectory) AddBootstrapAdmin(context.Context, string) error    { return nil }
func (emailTestDirectory) FirstAdmin(context.Context) (*serviceauth.UserDirectoryEntry, error) {
	return nil, nil
}

type emailTestOAuth struct{}

func (emailTestOAuth) AuthCodeURL(string) string { return "https://accounts.example.com" }
func (emailTestOAuth) ExchangeUser(context.Context, string) (serviceauth.User, error) {
	return serviceauth.User{}, nil
}

type fakeEmailStore struct {
	creds *serviceemail.Credentials
}

func (f *fakeEmailStore) Credentials(context.Context) (*serviceemail.Credentials, error) {
	return f.creds, nil
}
func (f *fakeEmailStore) Save(_ context.Context, creds serviceemail.Credentials) error {
	f.creds = &creds
	return nil
}
func (f *fakeEmailStore) Delete(context.Context) error {
	f.creds = nil
	return nil
}

type fakeEmailSender struct {
	verifyErr error
	sendErr   error
}

func (f *fakeEmailSender) Verify(context.Context, serviceemail.Credentials) error {
	return f.verifyErr
}
func (f *fakeEmailSender) Send(context.Context, serviceemail.Credentials, serviceemail.Message) error {
	return f.sendErr
}

// newEmailTestHandler builds an EmailSettingsHandler over a real auth
// service (so IsAdmin/session gating is exercised for real) and a fake
// email store/sender (so no test reaches a network).
func newEmailTestHandler(t *testing.T, admins map[string]bool) (*EmailSettingsHandler, *serviceauth.Service, *fakeEmailStore, *fakeEmailSender) {
	t.Helper()
	twoFactorStore, err := filetwofactor.New(t.TempDir())
	if err != nil {
		t.Fatalf("init two-factor store: %v", err)
	}
	sessionRegistryStore, err := filesessions.New(t.TempDir())
	if err != nil {
		t.Fatalf("init session registry store: %v", err)
	}
	auth, err := serviceauth.New(
		context.Background(),
		fileauth.New(t.TempDir()),
		emailTestDirectory{admins: admins},
		func(string, string, string) serviceauth.OAuthProvider { return emailTestOAuth{} },
		"https://remote.example.com",
		[]byte("test-session-key"),
		twoFactorStore,
		sessionRegistryStore,
		serviceauth.Options{
			PendingLoginTTL:     5 * time.Minute,
			EnrollmentTTL:       10 * time.Minute,
			RecoveryCodeCount:   10,
			SessionHistoryLimit: 20,
			SetupTokenTTL:       30 * time.Minute,
		},
	)
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}

	store := &fakeEmailStore{}
	sender := &fakeEmailSender{}
	email := serviceemail.New(store, sender)
	return NewEmailSettingsHandler(email, auth), auth, store, sender
}

func sessionRequest(t *testing.T, auth *serviceauth.Service, email, method, path string, body any) *http.Request {
	t.Helper()
	cookieValue, err := auth.IssueSession(context.Background(), serviceauth.User{Email: email}, serviceauth.SignInMethodPassword, "127.0.0.1", "test-agent")
	if err != nil {
		t.Fatalf("issue session: %v", err)
	}
	var reader *strings.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = strings.NewReader(string(raw))
	} else {
		reader = strings.NewReader("")
	}
	request := httptest.NewRequest(method, path, reader)
	request.AddCookie(&http.Cookie{Name: serviceauth.SessionCookieName, Value: cookieValue})
	return request
}

func TestEmailSettingsHandlerAuthorization(t *testing.T) {
	handler, auth, _, _ := newEmailTestHandler(t, map[string]bool{"admin@example.com": true, "member@example.com": false})
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	t.Run("no session returns 401", func(t *testing.T) {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/admin/email", nil))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
	})

	t.Run("signed-in non-admin returns 403", func(t *testing.T) {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, sessionRequest(t, auth, "member@example.com", http.MethodGet, "/api/admin/email", nil))
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403, body = %s", rec.Code, rec.Body.String())
		}
	})
}

func TestEmailSettingsHandlerValidation(t *testing.T) {
	handler, auth, store, _ := newEmailTestHandler(t, map[string]bool{"admin@example.com": true})
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, sessionRequest(t, auth, "admin@example.com", http.MethodPut, "/api/admin/email", map[string]string{
		"address": "admin@example.com", "appPassword": "abcd efgh ijkl mno",
	}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
	if store.creds != nil {
		t.Fatalf("smtp credentials were saved despite invalid input: %+v", store.creds)
	}
}

func TestEmailSettingsHandlerVerificationFailure(t *testing.T) {
	handler, auth, store, sender := newEmailTestHandler(t, map[string]bool{"admin@example.com": true})
	sender.verifyErr = errors.New("535 authentication failed")
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, sessionRequest(t, auth, "admin@example.com", http.MethodPut, "/api/admin/email", map[string]string{
		"address": "admin@example.com", "appPassword": "abcd efgh ijkl mnop",
	}))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502, body = %s", rec.Code, rec.Body.String())
	}
	if store.creds != nil {
		t.Fatalf("smtp credentials were saved despite failed verification: %+v", store.creds)
	}
}

func TestEmailSettingsHandlerDelete(t *testing.T) {
	handler, auth, _, _ := newEmailTestHandler(t, map[string]bool{"admin@example.com": true})
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, sessionRequest(t, auth, "admin@example.com", http.MethodDelete, "/api/admin/email", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204, body = %s", rec.Code, rec.Body.String())
	}
}

func TestEmailSettingsHandlerMethodNotAllowed(t *testing.T) {
	handler, auth, _, _ := newEmailTestHandler(t, map[string]bool{"admin@example.com": true})
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, sessionRequest(t, auth, "admin@example.com", http.MethodPatch, "/api/admin/email", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405, body = %s", rec.Code, rec.Body.String())
	}
}

func TestEmailSettingsHandlerGetOmitsAppPassword(t *testing.T) {
	handler, auth, store, _ := newEmailTestHandler(t, map[string]bool{"admin@example.com": true})
	store.creds = &serviceemail.Credentials{Address: "admin@example.com", AppPassword: "abcdefghijklmnop"}
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, sessionRequest(t, auth, "admin@example.com", http.MethodGet, "/api/admin/email", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "address") {
		t.Errorf("response missing address field: %s", body)
	}
	if strings.Contains(body, "appPassword") || strings.Contains(body, "abcdefghijklmnop") {
		t.Errorf("response leaks the app password: %s", body)
	}
}
