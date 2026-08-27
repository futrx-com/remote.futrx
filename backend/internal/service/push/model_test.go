package push

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func TestSubscriptionValidateAcceptsBrowserSubscription(t *testing.T) {
	subscription := validBrowserSubscription(t)
	subscription.Endpoint = "  https://fcm.googleapis.com/fcm/send/device-token  "
	subscription.UserAgent = strings.Repeat("x", 300)

	if err := subscription.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if subscription.Endpoint != "https://fcm.googleapis.com/fcm/send/device-token" {
		t.Fatalf("Endpoint = %q", subscription.Endpoint)
	}
	if len(subscription.UserAgent) != 256 {
		t.Fatalf("UserAgent length = %d, want 256", len(subscription.UserAgent))
	}
}

func TestSubscriptionValidateRejectsUnsafeEndpoints(t *testing.T) {
	for _, endpoint := range []string{
		"http://push.example.com/device",
		"https://localhost/device",
		"https://push.localhost/device",
		"https://127.0.0.1/device",
		"https://10.0.0.1/device",
		"https://[::1]/device",
		"https://user:password@push.example.com/device",
		"https://push.example.com/device#fragment",
	} {
		t.Run(endpoint, func(t *testing.T) {
			subscription := validBrowserSubscription(t)
			subscription.Endpoint = endpoint
			if err := subscription.Validate(); !errors.Is(err, ErrInvalidEndpoint) {
				t.Fatalf("Validate() error = %v, want ErrInvalidEndpoint", err)
			}
		})
	}
}

func TestSubscriptionValidateRejectsMalformedKeys(t *testing.T) {
	tests := []func(*Subscription){
		func(subscription *Subscription) { subscription.P256dh = "not-base64" },
		func(subscription *Subscription) {
			subscription.P256dh = base64.RawURLEncoding.EncodeToString(make([]byte, 65))
		},
		func(subscription *Subscription) {
			subscription.Auth = base64.RawURLEncoding.EncodeToString(make([]byte, 15))
		},
		func(subscription *Subscription) { subscription.Auth = "not-base64" },
	}
	for index, mutate := range tests {
		t.Run(string(rune('a'+index)), func(t *testing.T) {
			subscription := validBrowserSubscription(t)
			mutate(&subscription)
			if err := subscription.Validate(); !errors.Is(err, ErrInvalidKeys) {
				t.Fatalf("Validate() error = %v, want ErrInvalidKeys", err)
			}
		})
	}
}

func validBrowserSubscription(t *testing.T) Subscription {
	t.Helper()
	privateKey, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate subscription key: %v", err)
	}
	return Subscription{
		Endpoint: "https://push.example.com/device",
		P256dh:   base64.RawURLEncoding.EncodeToString(privateKey.PublicKey().Bytes()),
		Auth:     base64.RawURLEncoding.EncodeToString(make([]byte, 16)),
	}
}
