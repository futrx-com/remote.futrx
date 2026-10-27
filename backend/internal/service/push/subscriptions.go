package push

import (
	"context"
	"strings"
	"time"
)

// subscriptionRegistry owns device-registration validation and capacity
// policy. Persistence remains behind Repository.
type subscriptionRegistry struct {
	repo Repository
	now  func() time.Time
}

func newSubscriptionRegistry(repo Repository) subscriptionRegistry {
	return subscriptionRegistry{repo: repo, now: time.Now}
}

func (r subscriptionRegistry) subscribe(
	ctx context.Context,
	email string,
	subscription Subscription,
) error {
	email = NormalizeEmail(email)
	if email == "" {
		return ErrInvalidIdentity
	}
	if err := subscription.Validate(); err != nil {
		return err
	}

	existing, err := r.repo.List(ctx, email)
	if err != nil {
		return err
	}
	// Re-subscribing the same endpoint is a refresh, not a new device, so it
	// never counts against the cap.
	known := false
	for _, candidate := range existing {
		if candidate.Endpoint == subscription.Endpoint {
			known = true
			subscription.CreatedAt = candidate.CreatedAt
			break
		}
	}
	if !known && len(existing) >= MaxSubscriptionsPerUser {
		return ErrTooManySubscription
	}
	if subscription.CreatedAt == 0 {
		subscription.CreatedAt = r.now().UnixMilli()
	}
	return r.repo.Save(ctx, email, subscription)
}

func (r subscriptionRegistry) unsubscribe(ctx context.Context, email, endpoint string) error {
	email = NormalizeEmail(email)
	if email == "" {
		return ErrInvalidIdentity
	}
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return ErrInvalidEndpoint
	}
	return r.repo.Delete(ctx, email, endpoint)
}

func (r subscriptionRegistry) has(ctx context.Context, email string) (bool, error) {
	subscriptions, err := r.repo.List(ctx, NormalizeEmail(email))
	if err != nil {
		return false, err
	}
	return len(subscriptions) > 0, nil
}

func (r subscriptionRegistry) owns(ctx context.Context, email, endpoint string) (bool, error) {
	email = NormalizeEmail(email)
	if email == "" {
		return false, ErrInvalidIdentity
	}
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return false, ErrInvalidEndpoint
	}
	subscriptions, err := r.repo.List(ctx, email)
	if err != nil {
		return false, err
	}
	for _, subscription := range subscriptions {
		if subscription.Endpoint == endpoint {
			return true, nil
		}
	}
	return false, nil
}
