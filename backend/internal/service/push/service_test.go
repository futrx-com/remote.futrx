package push

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
)

type memoryRepo struct {
	mu   sync.Mutex
	rows map[string][]Subscription
	err  error
}

func newMemoryRepo() *memoryRepo {
	return &memoryRepo{rows: map[string][]Subscription{}}
}

func (r *memoryRepo) List(_ context.Context, email string) ([]Subscription, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return nil, r.err
	}
	return append([]Subscription(nil), r.rows[email]...), nil
}

func (r *memoryRepo) Save(_ context.Context, email string, subscription Subscription) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return r.err
	}
	for index, existing := range r.rows[email] {
		if existing.Endpoint == subscription.Endpoint {
			r.rows[email][index] = subscription
			return nil
		}
	}
	r.rows[email] = append(r.rows[email], subscription)
	return nil
}

func (r *memoryRepo) Delete(_ context.Context, email, endpoint string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	kept := r.rows[email][:0]
	for _, subscription := range r.rows[email] {
		if subscription.Endpoint != endpoint {
			kept = append(kept, subscription)
		}
	}
	r.rows[email] = kept
	return nil
}

type recordingSender struct {
	mu       sync.Mutex
	sent     []Subscription
	payloads [][]byte
	urgent   []bool
	fail     map[string]error
}

func (s *recordingSender) PublicKey() string { return "test-public-key" }

func (s *recordingSender) Send(_ context.Context, sub Subscription, payload []byte, urgent bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err, ok := s.fail[sub.Endpoint]; ok {
		return err
	}
	s.sent = append(s.sent, sub)
	s.payloads = append(s.payloads, append([]byte(nil), payload...))
	s.urgent = append(s.urgent, urgent)
	return nil
}

func (s *recordingSender) endpoints() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.sent))
	for _, sub := range s.sent {
		out = append(out, sub.Endpoint)
	}
	return out
}

func validSubscription(endpoint string) Subscription {
	return Subscription{
		Endpoint: endpoint,
		// The encoded P-256 generator is a valid browser public key. The auth
		// value is a 16-byte test secret.
		P256dh: "BGsX0fLhLEJH-Lzm5WOkQPJ3A32BLeszoPShOUXYmMKWT-NC4v4af5uO5-tKfA-eFivOM1drMV7Oy7ZAaDe_UfU",
		Auth:   "AAAAAAAAAAAAAAAAAAAAAA",
	}
}

func newTestService() (*Service, *memoryRepo, *recordingSender) {
	repo := newMemoryRepo()
	sender := &recordingSender{fail: map[string]error{}}
	return New(repo, sender), repo, sender
}

func TestSubscribeStoresADeviceAgainstTheNormalizedEmail(t *testing.T) {
	service, repo, _ := newTestService()

	if err := service.Subscribe(context.Background(), "  Ops@Example.COM ", validSubscription("https://push.example.com/a")); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	stored, err := repo.List(context.Background(), "ops@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 {
		t.Fatalf("stored %d subscriptions, want 1", len(stored))
	}
	if stored[0].CreatedAt == 0 {
		t.Fatal("subscription was stored without a creation timestamp")
	}
}

func TestSubscribeRejectsMalformedRegistrations(t *testing.T) {
	service, _, _ := newTestService()
	ctx := context.Background()

	insecure := validSubscription("http://push.example.com/a")
	if err := service.Subscribe(ctx, "ops@example.com", insecure); !errors.Is(err, ErrInvalidEndpoint) {
		t.Fatalf("http endpoint: err = %v, want ErrInvalidEndpoint", err)
	}

	shortKey := validSubscription("https://push.example.com/a")
	shortKey.P256dh = "too-short"
	if err := service.Subscribe(ctx, "ops@example.com", shortKey); !errors.Is(err, ErrInvalidKeys) {
		t.Fatalf("short p256dh: err = %v, want ErrInvalidKeys", err)
	}

	if err := service.Subscribe(ctx, "  ", validSubscription("https://push.example.com/a")); !errors.Is(err, ErrInvalidIdentity) {
		t.Fatalf("blank email: err = %v, want ErrInvalidIdentity", err)
	}
}

func TestSubscribeTreatsAKnownEndpointAsARefreshNotANewDevice(t *testing.T) {
	service, repo, _ := newTestService()
	ctx := context.Background()

	for i := 0; i < MaxSubscriptionsPerUser; i++ {
		endpoint := "https://push.example.com/" + string(rune('a'+i))
		if err := service.Subscribe(ctx, "ops@example.com", validSubscription(endpoint)); err != nil {
			t.Fatalf("subscribe %d: %v", i, err)
		}
	}

	// At the cap: a fresh device is refused...
	err := service.Subscribe(ctx, "ops@example.com", validSubscription("https://push.example.com/overflow"))
	if !errors.Is(err, ErrTooManySubscription) {
		t.Fatalf("err = %v, want ErrTooManySubscription", err)
	}
	// ...but re-registering one we already hold still succeeds.
	if err := service.Subscribe(ctx, "ops@example.com", validSubscription("https://push.example.com/a")); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	stored, _ := repo.List(ctx, "ops@example.com")
	if len(stored) != MaxSubscriptionsPerUser {
		t.Fatalf("stored %d subscriptions, want %d", len(stored), MaxSubscriptionsPerUser)
	}
}

func TestNotifyReachesEveryDeviceOfEveryRecipientOnce(t *testing.T) {
	service, _, sender := newTestService()
	ctx := context.Background()

	_ = service.Subscribe(ctx, "a@example.com", validSubscription("https://push.example.com/a1"))
	_ = service.Subscribe(ctx, "a@example.com", validSubscription("https://push.example.com/a2"))
	_ = service.Subscribe(ctx, "b@example.com", validSubscription("https://push.example.com/b1"))

	// The same recipient listed twice — a project member who is also an
	// admin — must not be notified twice.
	service.Notify(ctx, []string{"a@example.com", "B@example.com", "a@example.com"}, Notification{
		Kind:   KindQuestion,
		ChatID: "beefcafe",
		Title:  "The agent is asking a question",
		Urgent: true,
	})

	got := sender.endpoints()
	if len(got) != 3 {
		t.Fatalf("delivered to %v, want three endpoints", got)
	}
	if !sender.urgent[0] {
		t.Fatal("an urgent notification was sent without the urgent flag")
	}

	var payload Notification
	if err := json.Unmarshal(sender.payloads[0], &payload); err != nil {
		t.Fatalf("payload is not valid json: %v", err)
	}
	if payload.ChatID != "beefcafe" || payload.Kind != KindQuestion {
		t.Fatalf("payload = %+v", payload)
	}
}

func TestNotifyPrunesSubscriptionsThePushServiceRetired(t *testing.T) {
	service, repo, sender := newTestService()
	ctx := context.Background()

	_ = service.Subscribe(ctx, "ops@example.com", validSubscription("https://push.example.com/live"))
	_ = service.Subscribe(ctx, "ops@example.com", validSubscription("https://push.example.com/dead"))
	sender.fail["https://push.example.com/dead"] = ErrGone

	service.Notify(ctx, []string{"ops@example.com"}, Notification{Title: "hi"})

	stored, _ := repo.List(ctx, "ops@example.com")
	if len(stored) != 1 || stored[0].Endpoint != "https://push.example.com/live" {
		t.Fatalf("stored = %+v, want only the live endpoint", stored)
	}
}

func TestNotifyKeepsSubscriptionsAfterATransientFailure(t *testing.T) {
	service, repo, sender := newTestService()
	ctx := context.Background()

	_ = service.Subscribe(ctx, "ops@example.com", validSubscription("https://push.example.com/a"))
	sender.fail["https://push.example.com/a"] = errors.New("503 service unavailable")

	service.Notify(ctx, []string{"ops@example.com"}, Notification{Title: "hi"})

	stored, _ := repo.List(ctx, "ops@example.com")
	if len(stored) != 1 {
		t.Fatal("a transient push failure dropped the subscription")
	}
}

func TestNotifyAsyncDeliversWithoutBlockingTheCaller(t *testing.T) {
	service, _, sender := newTestService()
	_ = service.Subscribe(context.Background(), "ops@example.com", validSubscription("https://push.example.com/a"))

	service.NotifyAsync([]string{"ops@example.com"}, Notification{Title: "hi"})
	service.Wait()

	if len(sender.endpoints()) != 1 {
		t.Fatal("async notification was not delivered")
	}
}

func TestADisabledServiceIsInertRatherThanFatal(t *testing.T) {
	service := New(nil, nil)
	ctx := context.Background()

	if service.Enabled() {
		t.Fatal("service without a sender reported itself enabled")
	}
	if service.PublicKey() != "" {
		t.Fatal("disabled service returned a public key")
	}
	if err := service.Subscribe(ctx, "ops@example.com", validSubscription("https://push.example.com/a")); !errors.Is(err, ErrDisabled) {
		t.Fatalf("err = %v, want ErrDisabled", err)
	}
	// Fan-out is a no-op rather than a panic, so callers need no guard.
	service.Notify(ctx, []string{"ops@example.com"}, Notification{Title: "hi"})
	service.NotifyAsync([]string{"ops@example.com"}, Notification{Title: "hi"})
	service.Wait()
}

func TestHasSubscriptionsReportsWhetherThisAccountIsReachable(t *testing.T) {
	service, _, _ := newTestService()
	ctx := context.Background()

	if reachable, err := service.HasSubscriptions(ctx, "ops@example.com"); err != nil || reachable {
		t.Fatalf("reachable = %v, %v; want false", reachable, err)
	}
	_ = service.Subscribe(ctx, "ops@example.com", validSubscription("https://push.example.com/a"))
	if reachable, err := service.HasSubscriptions(ctx, "OPS@example.com"); err != nil || !reachable {
		t.Fatalf("reachable = %v, %v; want true", reachable, err)
	}
}

func TestUnsubscribeIsIdempotent(t *testing.T) {
	service, _, _ := newTestService()
	ctx := context.Background()

	_ = service.Subscribe(ctx, "ops@example.com", validSubscription("https://push.example.com/a"))
	for i := 0; i < 2; i++ {
		if err := service.Unsubscribe(ctx, "ops@example.com", "https://push.example.com/a"); err != nil {
			t.Fatalf("unsubscribe %d: %v", i, err)
		}
	}
	if reachable, _ := service.HasSubscriptions(ctx, "ops@example.com"); reachable {
		t.Fatal("subscription survived unsubscribe")
	}
}

func TestOwnsSubscriptionChecksTheExactEndpointForThisAccount(t *testing.T) {
	service, _, _ := newTestService()
	ctx := context.Background()
	_ = service.Subscribe(ctx, "first@example.com", validSubscription("https://push.example.com/device"))

	if owns, err := service.OwnsSubscription(ctx, "first@example.com", "https://push.example.com/device"); err != nil || !owns {
		t.Fatalf("first account ownership = %v, %v; want true", owns, err)
	}
	if owns, err := service.OwnsSubscription(ctx, "second@example.com", "https://push.example.com/device"); err != nil || owns {
		t.Fatalf("second account ownership = %v, %v; want false", owns, err)
	}
}
