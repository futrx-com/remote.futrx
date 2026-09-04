package webpush

import (
	"time"

	protocol "github.com/SherClockHolmes/webpush-go"
)

// Subscription is the browser-minted PushSubscription: where to deliver, and
// the keys the payload is encrypted to.
type Subscription struct {
	Endpoint string
	P256dh   string
	Auth     string
}

// Urgency lets a device defer low-value messages while on battery saver.
type Urgency = protocol.Urgency

const (
	UrgencyNormal = protocol.UrgencyNormal
	UrgencyHigh   = protocol.UrgencyHigh
)

// Options tune a single delivery.
type Options struct {
	// TTL is how long the push service may hold an undelivered message.
	TTL time.Duration
	// Urgency defaults to normal.
	Urgency Urgency
	// Topic collapses undelivered messages: a newer message with the same
	// topic replaces an older one still queued at the push service.
	Topic string
}
