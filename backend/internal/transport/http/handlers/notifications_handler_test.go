package httphandlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	servicenotify "github.com/futrx-com/remote.futrx.com/internal/service/notify"
)

type notificationsServiceStub struct {
	config    servicenotify.PublicConfig
	saveInput servicenotify.UpdateInput
	saveErr   error
	tested    bool
}

func (s *notificationsServiceStub) PublicConfig() servicenotify.PublicConfig {
	return s.config
}

func (s *notificationsServiceStub) Save(
	_ context.Context,
	input servicenotify.UpdateInput,
) (servicenotify.PublicConfig, error) {
	s.saveInput = input
	if s.saveErr != nil {
		return servicenotify.PublicConfig{}, s.saveErr
	}
	return s.config, nil
}

func (s *notificationsServiceStub) Test(context.Context) []servicenotify.SinkResult {
	s.tested = true
	return []servicenotify.SinkResult{
		{Sink: servicenotify.SinkTelegram, Configured: true, Delivered: true},
		{Sink: servicenotify.SinkWebhook, Configured: false, Error: "not configured"},
	}
}

type callerStub struct {
	email   string
	isAdmin bool
	err     error
}

func (c callerStub) EmailAndAdmin(context.Context, *http.Request) (string, bool, error) {
	return c.email, c.isAdmin, c.err
}

func newNotificationsHandler(
	service NotificationsService,
	caller notificationsCaller,
) *NotificationsHandler {
	return &NotificationsHandler{notifications: service, caller: caller}
}

func TestNotificationsHandlerRejectsNonAdmins(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		target     string
		caller     callerStub
		wantStatus int
	}{
		{
			name:       "anonymous GET",
			method:     http.MethodGet,
			target:     "/api/admin/notifications",
			caller:     callerStub{},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "member GET",
			method:     http.MethodGet,
			target:     "/api/admin/notifications",
			caller:     callerStub{email: "member@example.com"},
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "member PUT",
			method:     http.MethodPut,
			target:     "/api/admin/notifications",
			caller:     callerStub{email: "member@example.com"},
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "member test",
			method:     http.MethodPost,
			target:     "/api/admin/notifications/test",
			caller:     callerStub{email: "member@example.com"},
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "admin DELETE is not a supported method",
			method:     http.MethodDelete,
			target:     "/api/admin/notifications",
			caller:     callerStub{email: "admin@example.com", isAdmin: true},
			wantStatus: http.StatusMethodNotAllowed,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &notificationsServiceStub{}
			mux := http.NewServeMux()
			newNotificationsHandler(service, test.caller).RegisterRoutes(mux)

			recorder := httptest.NewRecorder()
			mux.ServeHTTP(recorder, httptest.NewRequest(test.method, test.target, strings.NewReader("{}")))

			if recorder.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", recorder.Code, test.wantStatus, recorder.Body)
			}
			if service.tested {
				t.Fatal("an unauthorized caller reached the test sink")
			}
		})
	}
}

func TestNotificationsHandlerGetReturnsMaskedSecretsOnly(t *testing.T) {
	stored := servicenotify.Config{
		Enabled:  true,
		Telegram: servicenotify.TelegramConfig{BotToken: "123456:SUPERSECRET", ChatID: "-100200"},
		Webhook:  servicenotify.WebhookConfig{URL: "https://hooks.example.com/x", Secret: "hook-secret"},
		Events:   servicenotify.EventToggles{RunFinished: true},
	}
	service := &notificationsServiceStub{config: stored.Public()}
	mux := http.NewServeMux()
	newNotificationsHandler(service, callerStub{email: "admin@example.com", isAdmin: true}).RegisterRoutes(mux)

	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/admin/notifications", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	body := recorder.Body.String()
	for _, secret := range []string{"123456:SUPERSECRET", "SUPERSECRET", "hook-secret"} {
		if strings.Contains(body, secret) {
			t.Fatalf("response leaked %q: %s", secret, body)
		}
	}

	var response servicenotify.PublicConfig
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if response.Telegram.BotTokenMasked != "••••CRET" || !response.Telegram.Configured {
		t.Fatalf("telegram view = %+v", response.Telegram)
	}
	if response.Webhook.URL != "https://hooks.example.com/x" {
		t.Fatalf("webhook view = %+v", response.Webhook)
	}
}

func TestNotificationsHandlerPutForwardsTheUpdate(t *testing.T) {
	service := &notificationsServiceStub{}
	mux := http.NewServeMux()
	newNotificationsHandler(service, callerStub{email: "admin@example.com", isAdmin: true}).RegisterRoutes(mux)

	body := `{"enabled":true,"telegram":{"botToken":"123:abc","chatId":"-100"},` +
		`"webhook":{"url":"https://hooks.example.com/x","clearSecret":true},` +
		`"events":{"runFinished":true,"runFailed":false,"needsAttention":true,"scheduledRun":false}}`
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, "/api/admin/notifications", strings.NewReader(body)))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", recorder.Code, recorder.Body)
	}
	if !service.saveInput.Enabled ||
		service.saveInput.Telegram.BotToken != "123:abc" ||
		service.saveInput.Telegram.ChatID != "-100" ||
		!service.saveInput.Webhook.ClearSecret ||
		service.saveInput.Events != (servicenotify.EventToggles{RunFinished: true, NeedsAttention: true}) {
		t.Fatalf("forwarded input = %+v", service.saveInput)
	}
}

func TestNotificationsHandlerPutReportsValidationErrorsAsBadRequest(t *testing.T) {
	service := &notificationsServiceStub{saveErr: servicenotify.ErrInvalidConfig}
	mux := http.NewServeMux()
	newNotificationsHandler(service, callerStub{email: "admin@example.com", isAdmin: true}).RegisterRoutes(mux)

	recorder := httptest.NewRecorder()
	mux.ServeHTTP(
		recorder,
		httptest.NewRequest(http.MethodPut, "/api/admin/notifications", strings.NewReader(`{"enabled":true}`)),
	)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
}

func TestNotificationsHandlerTestReturnsPerSinkResults(t *testing.T) {
	service := &notificationsServiceStub{}
	mux := http.NewServeMux()
	newNotificationsHandler(service, callerStub{email: "admin@example.com", isAdmin: true}).RegisterRoutes(mux)

	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/admin/notifications/test", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	var response struct {
		Results []servicenotify.SinkResult `json:"results"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if len(response.Results) != 2 || !response.Results[0].Delivered || response.Results[1].Error == "" {
		t.Fatalf("results = %+v", response.Results)
	}
	if !service.tested {
		t.Fatal("the handler did not reach the service")
	}
}
