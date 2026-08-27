// Package push keeps per-user Web Push registrations and fans notifications
// out to them. It knows nothing about why a notification was raised: callers
// decide the audience and the copy.
package push

import "context"

// Service is the stable Push entry point. Registration policy and notification
// delivery are delegated to separate collaborators because they have different
// lifecycles and reasons to change.
type Service struct {
	repo          Repository
	sender        Sender
	subscriptions subscriptionRegistry
	deliveries    notificationDispatcher
}

func New(repo Repository, sender Sender) *Service {
	return &Service{
		repo:          repo,
		sender:        sender,
		subscriptions: newSubscriptionRegistry(repo),
		deliveries:    newNotificationDispatcher(repo, sender),
	}
}

// Enabled reports whether push is configured. It is false when the deployment
// could not build a VAPID key, and every entry point degrades to a no-op.
func (s *Service) Enabled() bool {
	return s != nil && s.repo != nil && s.sender != nil
}

// PublicKey is the applicationServerKey the browser needs to subscribe.
func (s *Service) PublicKey() string {
	if !s.Enabled() {
		return ""
	}
	return s.sender.PublicKey()
}

// Subscribe registers (or refreshes) one device for a user.
func (s *Service) Subscribe(ctx context.Context, email string, subscription Subscription) error {
	if !s.Enabled() {
		return ErrDisabled
	}
	return s.subscriptions.subscribe(ctx, email, subscription)
}

// Unsubscribe drops one device. Removing an endpoint that was never stored is
// not an error: the browser may be retrying a cleanup.
func (s *Service) Unsubscribe(ctx context.Context, email, endpoint string) error {
	if !s.Enabled() {
		return ErrDisabled
	}
	return s.subscriptions.unsubscribe(ctx, email, endpoint)
}

// HasSubscriptions reports whether a user has any device registered, so the UI
// can show whether this account is reachable at all.
func (s *Service) HasSubscriptions(ctx context.Context, email string) (bool, error) {
	if !s.Enabled() {
		return false, nil
	}
	return s.subscriptions.has(ctx, email)
}

// OwnsSubscription reports whether this exact browser endpoint belongs to the
// signed-in account. Browser push subscriptions are origin-wide, so merely
// finding one locally is not proof that it belongs to the current user.
func (s *Service) OwnsSubscription(ctx context.Context, email, endpoint string) (bool, error) {
	if s == nil || s.repo == nil {
		return false, nil
	}
	return s.subscriptions.owns(ctx, email, endpoint)
}

// Notify delivers to every device of every recipient, pruning subscriptions
// the push service reports as retired. It blocks; use NotifyAsync from
// latency-sensitive paths.
func (s *Service) Notify(ctx context.Context, recipients []string, notification Notification) {
	if !s.Enabled() || len(recipients) == 0 {
		return
	}
	s.deliveries.notify(ctx, recipients, notification)
}

// NotifyAsync runs Notify on its own goroutine with an independent deadline.
// Chat events are appended on the streaming hot path, and a slow push service
// must never hold that up.
func (s *Service) NotifyAsync(recipients []string, notification Notification) {
	if !s.Enabled() || len(recipients) == 0 {
		return
	}
	s.deliveries.notifyAsync(recipients, notification)
}

// Wait blocks until background deliveries finish. Used by tests.
func (s *Service) Wait() {
	if s == nil {
		return
	}
	s.deliveries.wait()
}
