package push

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"sync"
	"time"
)

// deliveryTimeout bounds one fan-out. Push services are third parties on the
// public internet; a wedged one must not pin a goroutine forever.
const deliveryTimeout = 30 * time.Second

// notificationDispatcher owns notification encoding, fan-out, endpoint
// retirement, and the lifecycle of background deliveries.
type notificationDispatcher struct {
	repo   Repository
	sender Sender
	now    func() time.Time

	// wg lets tests (and a future graceful shutdown) wait for in-flight
	// deliveries that were started in the background.
	wg sync.WaitGroup
}

func newNotificationDispatcher(repo Repository, sender Sender) notificationDispatcher {
	return notificationDispatcher{repo: repo, sender: sender, now: time.Now}
}

func (d *notificationDispatcher) notify(
	ctx context.Context,
	recipients []string,
	notification Notification,
) {
	payload, err := json.Marshal(notification)
	if err != nil {
		log.Printf("push: encode notification: %v", err)
		return
	}

	for _, email := range dedupeEmails(recipients) {
		subscriptions, err := d.repo.List(ctx, email)
		if err != nil {
			log.Printf("push: list subscriptions: %v", err)
			continue
		}
		for _, subscription := range subscriptions {
			d.deliver(ctx, email, subscription, payload, notification.Urgent)
		}
	}
}

func (d *notificationDispatcher) notifyAsync(recipients []string, notification Notification) {
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), deliveryTimeout)
		defer cancel()
		d.notify(ctx, recipients, notification)
	}()
}

func (d *notificationDispatcher) wait() {
	d.wg.Wait()
}

func (d *notificationDispatcher) deliver(
	ctx context.Context,
	email string,
	subscription Subscription,
	payload []byte,
	urgent bool,
) {
	err := d.sender.Send(ctx, subscription, payload, urgent)
	switch {
	case err == nil:
		subscription.LastSentAt = d.now().UnixMilli()
		if saveErr := d.repo.Save(ctx, email, subscription); saveErr != nil {
			log.Printf("push: record delivery: %v", saveErr)
		}
	case errors.Is(err, ErrGone):
		// The browser dropped this registration. Forget it so the next
		// fan-out is not slowed down by a dead endpoint.
		if deleteErr := d.repo.Delete(ctx, email, subscription.Endpoint); deleteErr != nil {
			log.Printf("push: prune retired subscription: %v", deleteErr)
		}
	default:
		log.Printf("push: deliver to %s: %v", endpointHost(subscription.Endpoint), err)
	}
}

func dedupeEmails(emails []string) []string {
	seen := make(map[string]struct{}, len(emails))
	out := make([]string, 0, len(emails))
	for _, email := range emails {
		email = NormalizeEmail(email)
		if email == "" {
			continue
		}
		if _, ok := seen[email]; ok {
			continue
		}
		seen[email] = struct{}{}
		out = append(out, email)
	}
	return out
}

// endpointHost keeps push-service hostnames in logs without the subscription
// id, which is a bearer capability to notify that device.
func endpointHost(endpoint string) string {
	trimmed := strings.TrimPrefix(endpoint, "https://")
	if index := strings.IndexByte(trimmed, '/'); index >= 0 {
		return trimmed[:index]
	}
	return trimmed
}
