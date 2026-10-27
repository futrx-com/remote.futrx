package push

import (
	"context"
	"errors"
)

// ErrGone is what a Sender returns when the push service has permanently
// retired an endpoint. The service prunes those subscriptions.
var ErrGone = errors.New("push subscription is gone")

// Repository stores each user's registered devices, keyed by lowercased email.
type Repository interface {
	List(ctx context.Context, email string) ([]Subscription, error)
	Save(ctx context.Context, email string, subscription Subscription) error
	Delete(ctx context.Context, email, endpoint string) error
}

// Sender delivers one encrypted notification. The composition layer supplies
// the Web Push implementation; tests supply a recorder.
type Sender interface {
	// PublicKey is the VAPID application server key browsers subscribe with.
	PublicKey() string
	Send(ctx context.Context, subscription Subscription, payload []byte, urgent bool) error
}
