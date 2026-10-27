package httphandlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	servicepresence "github.com/futrx-com/remote.futrx.com/internal/service/presence"
	servicepush "github.com/futrx-com/remote.futrx.com/internal/service/push"
)

type pushRepoStub struct {
	mu   sync.Mutex
	rows map[string][]servicepush.Subscription
}

func (r *pushRepoStub) List(_ context.Context, email string) ([]servicepush.Subscription, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]servicepush.Subscription(nil), r.rows[email]...), nil
}

func (r *pushRepoStub) Save(_ context.Context, email string, sub servicepush.Subscription) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for index, existing := range r.rows[email] {
		if existing.Endpoint == sub.Endpoint {
			r.rows[email][index] = sub
			return nil
		}
	}
	r.rows[email] = append(r.rows[email], sub)
	return nil
}

func (r *pushRepoStub) Delete(_ context.Context, email, endpoint string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	kept := r.rows[email][:0]
	for _, sub := range r.rows[email] {
		if sub.Endpoint != endpoint {
			kept = append(kept, sub)
		}
	}
	r.rows[email] = kept
	return nil
}

type pushSenderStub struct {
	mu       sync.Mutex
	payloads [][]byte
}

func (s *pushSenderStub) PublicKey() string { return "BPublicKey" }

func (s *pushSenderStub) Send(_ context.Context, _ servicepush.Subscription, payload []byte, _ bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.payloads = append(s.payloads, payload)
	return nil
}

// browserSubscriptionJSON is exactly what PushSubscription.toJSON() serializes
// in a browser, including the expirationTime field the backend ignores.
const browserSubscriptionJSON = `{
  "endpoint": "https://fcm.googleapis.com/fcm/send/abc123",
  "expirationTime": null,
  "keys": {
    "p256dh": "BGsX0fLhLEJH-Lzm5WOkQPJ3A32BLeszoPShOUXYmMKWT-NC4v4af5uO5-tKfA-eFivOM1drMV7Oy7ZAaDe_UfU",
    "auth": "AAAAAAAAAAAAAAAAAAAAAA"
  }
}`

func newPushHandler() (*PushHandler, *pushRepoStub, *pushSenderStub) {
	repo := &pushRepoStub{rows: map[string][]servicepush.Subscription{}}
	sender := &pushSenderStub{}
	// A nil auth service means the handler falls back to the local-admin
	// identity, which is how a single-operator box runs.
	return NewPushHandler(servicepush.New(repo, sender), nil, servicepresence.New()), repo, sender
}

func TestPushConfigAdvertisesTheServerKey(t *testing.T) {
	handler, _, _ := newPushHandler()

	response := httptest.NewRecorder()
	handler.HandleConfig(response, httptest.NewRequest(http.MethodGet, "/api/push/config", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	var body pushConfigResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if !body.Enabled || body.PublicKey != "BPublicKey" {
		t.Fatalf("body = %+v", body)
	}
	if body.Subscribed {
		t.Fatal("reported a subscription before any device registered")
	}
}

func TestPushConfigReportsADisabledServerWithoutFailing(t *testing.T) {
	handler := NewPushHandler(servicepush.New(nil, nil), nil, servicepresence.New())

	response := httptest.NewRecorder()
	handler.HandleConfig(response, httptest.NewRequest(http.MethodGet, "/api/push/config", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 so the UI can explain why", response.Code)
	}
	var body pushConfigResponse
	_ = json.NewDecoder(response.Body).Decode(&body)
	if body.Enabled || body.PublicKey != "" {
		t.Fatalf("body = %+v", body)
	}
}

func TestSubscribeAcceptsABrowserSubscriptionVerbatim(t *testing.T) {
	handler, repo, _ := newPushHandler()

	request := httptest.NewRequest(http.MethodPost, "/api/push/subscriptions", strings.NewReader(browserSubscriptionJSON))
	request.Header.Set("User-Agent", "Mozilla/5.0 (iPhone)")
	response := httptest.NewRecorder()
	handler.HandleSubscriptions(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body)
	}
	stored, _ := repo.List(context.Background(), "local-admin")
	if len(stored) != 1 {
		t.Fatalf("stored %d subscriptions", len(stored))
	}
	if stored[0].Endpoint != "https://fcm.googleapis.com/fcm/send/abc123" {
		t.Fatalf("endpoint = %q", stored[0].Endpoint)
	}
	// The nested keys object is where browsers put these; flattening it wrong
	// would store empty keys and every push would fail to encrypt.
	if stored[0].P256dh == "" || stored[0].Auth == "" {
		t.Fatalf("keys were not read out of the nested object: %+v", stored[0])
	}
	if stored[0].UserAgent != "Mozilla/5.0 (iPhone)" {
		t.Fatalf("user agent = %q", stored[0].UserAgent)
	}

	// Config now reports this account as reachable.
	config := httptest.NewRecorder()
	handler.HandleConfig(config, httptest.NewRequest(http.MethodGet, "/api/push/config", nil))
	var body pushConfigResponse
	_ = json.NewDecoder(config.Body).Decode(&body)
	if !body.Subscribed {
		t.Fatal("config did not report the registered device")
	}
}

func TestSubscribeRejectsAMalformedRegistration(t *testing.T) {
	handler, _, _ := newPushHandler()

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/push/subscriptions",
		strings.NewReader(`{"endpoint":"http://insecure.example.com/x","keys":{"p256dh":"x","auth":"y"}}`),
	)
	response := httptest.NewRecorder()
	handler.HandleSubscriptions(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", response.Code)
	}
}

func TestUnsubscribeRemovesTheDevice(t *testing.T) {
	handler, repo, _ := newPushHandler()

	post := httptest.NewRequest(http.MethodPost, "/api/push/subscriptions", strings.NewReader(browserSubscriptionJSON))
	handler.HandleSubscriptions(httptest.NewRecorder(), post)

	del := httptest.NewRequest(
		http.MethodDelete,
		"/api/push/subscriptions",
		strings.NewReader(`{"endpoint":"https://fcm.googleapis.com/fcm/send/abc123"}`),
	)
	response := httptest.NewRecorder()
	handler.HandleSubscriptions(response, del)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body)
	}
	stored, _ := repo.List(context.Background(), "local-admin")
	if len(stored) != 0 {
		t.Fatalf("stored = %+v", stored)
	}
}

func TestSubscriptionStatusChecksThisAccountsExactEndpoint(t *testing.T) {
	handler, _, _ := newPushHandler()
	post := httptest.NewRequest(http.MethodPost, "/api/push/subscriptions", strings.NewReader(browserSubscriptionJSON))
	handler.HandleSubscriptions(httptest.NewRecorder(), post)

	response := httptest.NewRecorder()
	handler.HandleSubscriptionOwnership(
		response,
		httptest.NewRequest(
			http.MethodPost,
			"/api/push/subscriptions/status",
			strings.NewReader(`{"endpoint":"https://fcm.googleapis.com/fcm/send/abc123"}`),
		),
	)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body)
	}
	var body subscriptionOwnershipResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if !body.Owned {
		t.Fatal("the endpoint registered to this account was not recognized")
	}

	response = httptest.NewRecorder()
	handler.HandleSubscriptionOwnership(
		response,
		httptest.NewRequest(
			http.MethodPost,
			"/api/push/subscriptions/status",
			strings.NewReader(`{"endpoint":"https://push.example.com/someone-elses-device"}`),
		),
	)
	_ = json.NewDecoder(response.Body).Decode(&body)
	if body.Owned {
		t.Fatal("an endpoint not registered to this account was reported as owned")
	}
}

func TestTestNotificationGoesOnlyToTheCaller(t *testing.T) {
	handler, _, sender := newPushHandler()

	post := httptest.NewRequest(http.MethodPost, "/api/push/subscriptions", strings.NewReader(browserSubscriptionJSON))
	handler.HandleSubscriptions(httptest.NewRecorder(), post)

	response := httptest.NewRecorder()
	handler.HandleTest(response, httptest.NewRequest(http.MethodPost, "/api/push/test", nil))

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body)
	}
	if len(sender.payloads) != 1 {
		t.Fatalf("sent %d notifications, want 1", len(sender.payloads))
	}
	var payload servicepush.Notification
	if err := json.Unmarshal(sender.payloads[0], &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Kind != servicepush.KindTest || payload.Title == "" {
		t.Fatalf("payload = %+v", payload)
	}
}

func TestPushRoutesRejectTheWrongMethod(t *testing.T) {
	handler, _, _ := newPushHandler()

	for _, tc := range []struct {
		path   string
		method string
		serve  func(http.ResponseWriter, *http.Request)
	}{
		{"/api/push/config", http.MethodPost, handler.HandleConfig},
		{"/api/push/subscriptions", http.MethodGet, handler.HandleSubscriptions},
		{"/api/push/test", http.MethodGet, handler.HandleTest},
	} {
		response := httptest.NewRecorder()
		tc.serve(response, httptest.NewRequest(tc.method, tc.path, nil))
		if response.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s %s: status = %d, want 405", tc.method, tc.path, response.Code)
		}
	}
}

func TestPresenceRecordsAndWithdrawsAClaim(t *testing.T) {
	handler, _, _ := newPushHandler()

	post := func(body string) int {
		response := httptest.NewRecorder()
		handler.HandlePresence(
			response,
			httptest.NewRequest(http.MethodPost, "/api/push/presence", strings.NewReader(body)),
		)
		return response.Code
	}

	if code := post(`{"chatId":"beefcafe","clientId":"tab-1","revision":1}`); code != http.StatusNoContent {
		t.Fatalf("status = %d", code)
	}
	// local-admin is the identity a nil auth service falls back to.
	if !handler.presence.IsWatching("local-admin", "beefcafe") {
		t.Fatal("posting a chat id should mark the caller as watching it")
	}

	if code := post(`{"chatId":"","clientId":"tab-1","revision":2}`); code != http.StatusNoContent {
		t.Fatalf("status = %d", code)
	}
	if handler.presence.IsWatching("local-admin", "beefcafe") {
		t.Fatal("an empty chat id should withdraw the claim")
	}
}

func TestPresenceRejectsANonPost(t *testing.T) {
	handler, _, _ := newPushHandler()

	response := httptest.NewRecorder()
	handler.HandlePresence(response, httptest.NewRequest(http.MethodGet, "/api/push/presence", nil))

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d", response.Code)
	}
}

// Whitespace is not a chat id: a client sending it is signing off, and must
// not be left holding a claim that silences its owner until the TTL runs out.
func TestPresenceTreatsABlankChatIDAsSigningOff(t *testing.T) {
	handler, _, _ := newPushHandler()

	post := func(body string) {
		handler.HandlePresence(
			httptest.NewRecorder(),
			httptest.NewRequest(http.MethodPost, "/api/push/presence", strings.NewReader(body)),
		)
	}

	post(`{"chatId":"beefcafe","clientId":"tab-1","revision":1}`)
	post(`{"chatId":"   ","clientId":"tab-1","revision":2}`)

	if handler.presence.IsWatching("local-admin", "beefcafe") {
		t.Fatal("a whitespace chat id should withdraw the claim, not be ignored")
	}
}

func TestPresenceRejectsARequestWithoutAnOrderingRevision(t *testing.T) {
	handler, _, _ := newPushHandler()
	response := httptest.NewRecorder()
	handler.HandlePresence(
		response,
		httptest.NewRequest(
			http.MethodPost,
			"/api/push/presence",
			strings.NewReader(`{"chatId":"beefcafe","clientId":"tab-1"}`),
		),
	)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", response.Code)
	}
}
