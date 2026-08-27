package service

import (
	"context"
	"errors"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/integration/webpush"
	servicepush "github.com/futrx-com/remote.futrx.com/internal/service/push"
)

// webPushSender adapts the Web Push integration to the push service's port,
// translating the transport-level "endpoint retired" signal into the domain
// error the service prunes on.
type webPushSender struct {
	client *webpush.Client
}

func (s webPushSender) PublicKey() string { return s.client.PublicKey() }

func (s webPushSender) Send(
	ctx context.Context,
	subscription servicepush.Subscription,
	payload []byte,
	urgent bool,
) error {
	urgency := webpush.UrgencyNormal
	ttl := 12 * time.Hour
	if urgent {
		urgency = webpush.UrgencyHigh
		// A question only matters while the run is still parked on it; there
		// is no point waking a device about it hours later.
		ttl = time.Hour
	}
	err := s.client.Send(ctx, webpush.Subscription{
		Endpoint: subscription.Endpoint,
		P256dh:   subscription.P256dh,
		Auth:     subscription.Auth,
	}, payload, webpush.Options{TTL: ttl, Urgency: urgency})
	if errors.Is(err, webpush.ErrSubscriptionGone) {
		return servicepush.ErrGone
	}
	return err
}
