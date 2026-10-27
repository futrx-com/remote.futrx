// Package webpush adapts the application's push model to the standards-based
// Web Push implementation provided by github.com/SherClockHolmes/webpush-go.
// Protocol cryptography and VAPID signing stay in that dependency; this package
// owns Remote-specific endpoint safety, timeouts, and response handling.
package webpush

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	protocol "github.com/SherClockHolmes/webpush-go"
)

// ErrSubscriptionGone reports that the push service has permanently retired an
// endpoint. The caller should drop the stored subscription rather than retry.
var ErrSubscriptionGone = errors.New("push subscription is gone")

// ErrPayloadTooLarge is returned by the protocol implementation when a payload
// cannot fit in one Web Push record.
var ErrPayloadTooLarge = protocol.ErrMaxPadExceeded

// Client sends encrypted notifications to push services on behalf of one
// application server identity.
type Client struct {
	key        VAPIDKey
	subscriber string
	http       httpDoer
}

// NewClient binds a VAPID key pair to a contact subject (an email address or
// HTTPS URL identifying whoever runs this server).
func NewClient(key VAPIDKey, subject string) (*Client, error) {
	return newClient(key, subject, newSafeHTTPClient())
}

func newClient(key VAPIDKey, subject string, httpClient httpDoer) (*Client, error) {
	if !key.valid() {
		return nil, errors.New("vapid key is not initialized")
	}
	subscriber, err := normalizeSubscriber(subject)
	if err != nil {
		return nil, err
	}
	if httpClient == nil {
		return nil, errors.New("web push http client is required")
	}
	return &Client{key: key, subscriber: subscriber, http: httpClient}, nil
}

// PublicKey is the applicationServerKey browsers need to subscribe.
func (c *Client) PublicKey() string { return c.key.PublicKeyBase64() }

// Send delegates encryption and VAPID signing to webpush-go, then translates
// the push service response into the errors understood by the push service.
func (c *Client) Send(ctx context.Context, sub Subscription, payload []byte, opts Options) error {
	endpoint := strings.TrimSpace(sub.Endpoint)
	if err := validateEndpointURL(endpoint); err != nil {
		return err
	}

	ttl := opts.TTL
	if ttl <= 0 {
		ttl = 12 * time.Hour
	}
	urgency := opts.Urgency
	if urgency == "" {
		urgency = UrgencyNormal
	}

	response, err := protocol.SendNotificationWithContext(
		ctx,
		payload,
		&protocol.Subscription{
			Endpoint: endpoint,
			Keys: protocol.Keys{
				P256dh: strings.TrimSpace(sub.P256dh),
				Auth:   strings.TrimSpace(sub.Auth),
			},
		},
		&protocol.Options{
			HTTPClient:      c.http,
			Subscriber:      c.subscriber,
			Topic:           opts.Topic,
			TTL:             int(ttl.Seconds()),
			Urgency:         urgency,
			VAPIDPublicKey:  c.key.PublicKeyBase64(),
			VAPIDPrivateKey: c.key.PrivateKeyBase64(),
		},
	)
	if err != nil {
		return fmt.Errorf("send push: %w", err)
	}
	defer response.Body.Close()
	// Drain enough to let the connection be reused, and to quote the failure
	// without logging an unbounded response from a third party.
	detail, _ := io.ReadAll(io.LimitReader(response.Body, 512))

	switch {
	case response.StatusCode >= 200 && response.StatusCode < 300:
		return nil
	case response.StatusCode == http.StatusNotFound, response.StatusCode == http.StatusGone:
		return ErrSubscriptionGone
	default:
		return fmt.Errorf(
			"push service returned %s: %s",
			response.Status,
			strings.TrimSpace(string(detail)),
		)
	}
}
