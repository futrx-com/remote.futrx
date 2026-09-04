package webpush

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type doerFunc func(*http.Request) (*http.Response, error)

func (function doerFunc) Do(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestSendDelegatesProtocolAndBuildsExpectedRequest(t *testing.T) {
	key := testVAPIDKey(t)
	subscription := testSubscription(t, "https://push.example.com/device/abc")
	payload := []byte(`{"title":"Agent needs you"}`)

	var delivered *http.Request
	var encrypted []byte
	client, err := newClient(key, "ops@example.com", doerFunc(func(request *http.Request) (*http.Response, error) {
		delivered = request
		encrypted, _ = io.ReadAll(request.Body)
		return response(http.StatusCreated, ""), nil
	}))
	if err != nil {
		t.Fatal(err)
	}

	err = client.Send(
		context.Background(),
		subscription,
		payload,
		Options{TTL: 90 * time.Second, Urgency: UrgencyHigh, Topic: "chat-beefcafe"},
	)
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if delivered == nil {
		t.Fatal("protocol implementation did not execute the HTTP request")
	}
	if delivered.Method != http.MethodPost || delivered.URL.String() != subscription.Endpoint {
		t.Fatalf("request = %s %s", delivered.Method, delivered.URL)
	}
	if len(encrypted) == 0 || bytes.Contains(encrypted, payload) {
		t.Fatal("request body was empty or exposed the plaintext payload")
	}
	for name, want := range map[string]string{
		"Content-Encoding": "aes128gcm",
		"Content-Type":     "application/octet-stream",
		"TTL":              "90",
		"Urgency":          "high",
		"Topic":            "chat-beefcafe",
	} {
		if got := delivered.Header.Get(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
	assertVAPIDSubject(t, delivered.Header.Get("Authorization"), "mailto:ops@example.com")
}

func TestSendReportsRetiredSubscriptionsAsGone(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusGone} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			client, err := newClient(testVAPIDKey(t), "ops@example.com", doerFunc(func(*http.Request) (*http.Response, error) {
				return response(status, ""), nil
			}))
			if err != nil {
				t.Fatal(err)
			}
			err = client.Send(context.Background(), testSubscription(t, "https://push.example.com/device"), []byte("{}"), Options{})
			if !errors.Is(err, ErrSubscriptionGone) {
				t.Fatalf("Send() error = %v, want ErrSubscriptionGone", err)
			}
		})
	}
}

func TestSendSurfacesOtherPushServiceFailures(t *testing.T) {
	client, err := newClient(testVAPIDKey(t), "ops@example.com", doerFunc(func(*http.Request) (*http.Response, error) {
		return response(http.StatusTooManyRequests, "slow down"), nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	err = client.Send(context.Background(), testSubscription(t, "https://push.example.com/device"), []byte("{}"), Options{})
	if err == nil || errors.Is(err, ErrSubscriptionGone) || !strings.Contains(err.Error(), "slow down") {
		t.Fatalf("Send() error = %v, want ordinary failure with response detail", err)
	}
}

func TestSendRejectsOversizedPayloadBeforeHTTP(t *testing.T) {
	called := false
	client, err := newClient(testVAPIDKey(t), "ops@example.com", doerFunc(func(*http.Request) (*http.Response, error) {
		called = true
		return response(http.StatusCreated, ""), nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	err = client.Send(
		context.Background(),
		testSubscription(t, "https://push.example.com/device"),
		bytes.Repeat([]byte("x"), 5000),
		Options{},
	)
	if !errors.Is(err, ErrPayloadTooLarge) {
		t.Fatalf("Send() error = %v, want ErrPayloadTooLarge", err)
	}
	if called {
		t.Fatal("oversized payload reached HTTP client")
	}
}

func TestSendRejectsUnsafeEndpointBeforeHTTP(t *testing.T) {
	for _, endpoint := range []string{
		"http://push.example.com/device",
		"https://localhost/device",
		"https://127.0.0.1/device",
		"https://10.0.0.1/device",
		"https://user:password@push.example.com/device",
	} {
		t.Run(endpoint, func(t *testing.T) {
			called := false
			client, err := newClient(testVAPIDKey(t), "ops@example.com", doerFunc(func(*http.Request) (*http.Response, error) {
				called = true
				return response(http.StatusCreated, ""), nil
			}))
			if err != nil {
				t.Fatal(err)
			}
			err = client.Send(context.Background(), testSubscription(t, endpoint), []byte("{}"), Options{})
			if err == nil {
				t.Fatal("Send() accepted an unsafe endpoint")
			}
			if called {
				t.Fatal("unsafe endpoint reached HTTP client")
			}
		})
	}
}

func testVAPIDKey(t *testing.T) VAPIDKey {
	t.Helper()
	key, err := GenerateVAPIDKey()
	if err != nil {
		t.Fatalf("GenerateVAPIDKey() error = %v", err)
	}
	return key
}

func testSubscription(t *testing.T, endpoint string) Subscription {
	t.Helper()
	privateKey, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate subscription key: %v", err)
	}
	return Subscription{
		Endpoint: endpoint,
		P256dh:   base64.RawURLEncoding.EncodeToString(privateKey.PublicKey().Bytes()),
		Auth:     base64.RawURLEncoding.EncodeToString(make([]byte, 16)),
	}
}

func response(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

func assertVAPIDSubject(t *testing.T, authorization, want string) {
	t.Helper()
	token, ok := strings.CutPrefix(authorization, "vapid t=")
	if !ok {
		t.Fatalf("Authorization = %q", authorization)
	}
	token, _, ok = strings.Cut(token, ", k=")
	if !ok {
		t.Fatalf("Authorization = %q", authorization)
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("VAPID token has %d segments", len(parts))
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode VAPID claims: %v", err)
	}
	claims := map[string]any{}
	if err := json.Unmarshal(raw, &claims); err != nil {
		t.Fatalf("parse VAPID claims: %v", err)
	}
	if claims["sub"] != want {
		t.Fatalf("VAPID subject = %v, want %q", claims["sub"], want)
	}
}
