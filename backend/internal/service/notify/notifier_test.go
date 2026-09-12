package notify

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// recordingSink counts deliveries and can fail a configurable number of times
// before succeeding, which is how the retry behaviour is exercised without a
// real network.
type recordingSink struct {
	name       string
	configured bool

	mu           sync.Mutex
	failures     int
	attempts     int
	delivered    []Event
	deliveryDone chan struct{}
}

func newRecordingSink(name string, failures int) *recordingSink {
	return &recordingSink{
		name:         name,
		configured:   true,
		failures:     failures,
		deliveryDone: make(chan struct{}, 16),
	}
}

func (s *recordingSink) Name() string { return s.name }

func (s *recordingSink) Configured(Config) bool { return s.configured }

func (s *recordingSink) Send(_ context.Context, _ Config, event Event) error {
	s.mu.Lock()
	s.attempts++
	remaining := s.failures
	if remaining > 0 {
		s.failures--
	} else {
		s.delivered = append(s.delivered, event)
	}
	s.mu.Unlock()
	if remaining > 0 {
		return errors.New("sink temporarily unavailable")
	}
	s.deliveryDone <- struct{}{}
	return nil
}

func (s *recordingSink) counts() (attempts int, delivered []Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.attempts, append([]Event(nil), s.delivered...)
}

func (s *recordingSink) waitForDelivery(t *testing.T) {
	t.Helper()
	select {
	case <-s.deliveryDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for a delivery")
	}
}

func enabledConfig() Config {
	return Config{
		Enabled:  true,
		Telegram: TelegramConfig{BotToken: "token", ChatID: "chat"},
		Webhook:  WebhookConfig{URL: "https://hooks.example.com/remote"},
		Events:   EventToggles{RunFinished: true, RunFailed: true, NeedsAttention: true, ScheduledRun: true},
	}
}

func startNotifier(t *testing.T, cfg Config, sinks ...Sink) *Notifier {
	t.Helper()
	notifier := NewNotifier(
		func() Config { return cfg },
		WithSinks(sinks...),
		WithBackoff(time.Millisecond),
	)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		notifier.Stop()
	})
	notifier.Start(ctx)
	return notifier
}

func TestNotifierRetriesUntilTheSinkAccepts(t *testing.T) {
	sink := newRecordingSink("flaky", 2)
	notifier := startNotifier(t, enabledConfig(), sink)

	if !notifier.Publish(Event{Event: KindRunFinished, Summary: "done"}) {
		t.Fatal("Publish should have queued the event")
	}
	sink.waitForDelivery(t)

	attempts, delivered := sink.counts()
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3 (two failures then success)", attempts)
	}
	if len(delivered) != 1 || delivered[0].Summary != "done" {
		t.Fatalf("delivered = %+v, want one event", delivered)
	}
}

func TestNotifierGivesUpAfterThreeAttempts(t *testing.T) {
	sink := newRecordingSink("broken", 99)
	notifier := startNotifier(t, enabledConfig(), sink)

	notifier.Publish(Event{Event: KindRunFailed})

	deadline := time.Now().Add(5 * time.Second)
	for {
		attempts, _ := sink.counts()
		if attempts >= deliveryAttempts {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("attempts = %d, want %d", attempts, deliveryAttempts)
		}
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)

	if attempts, _ := sink.counts(); attempts != deliveryAttempts {
		t.Fatalf("attempts = %d, want exactly %d", attempts, deliveryAttempts)
	}
}

func TestNotifierDeliversOneEventPerDedupeKey(t *testing.T) {
	sink := newRecordingSink("telegram", 0)
	notifier := startNotifier(t, enabledConfig(), sink)

	first := notifier.Publish(Event{Event: KindRunFinished, DedupeKey: "run:chat1:7", Summary: "first"})
	second := notifier.Publish(Event{Event: KindRunFinished, DedupeKey: "run:chat1:7", Summary: "duplicate"})
	third := notifier.Publish(Event{Event: KindRunFinished, DedupeKey: "run:chat1:8", Summary: "next run"})

	if !first || second || !third {
		t.Fatalf("publish results = (%t, %t, %t), want (true, false, true)", first, second, third)
	}
	sink.waitForDelivery(t)
	sink.waitForDelivery(t)

	_, delivered := sink.counts()
	if len(delivered) != 2 {
		t.Fatalf("delivered %d events, want 2: %+v", len(delivered), delivered)
	}
}

func TestNotifierSkipsEventsTheConfigurationDoesNotWant(t *testing.T) {
	sink := newRecordingSink("telegram", 0)
	cfg := enabledConfig()
	cfg.Events.RunFailed = false
	notifier := startNotifier(t, cfg, sink)

	if notifier.Publish(Event{Event: KindRunFailed}) {
		t.Fatal("a deselected event should not be queued")
	}

	disabled := enabledConfig()
	disabled.Enabled = false
	offSink := newRecordingSink("telegram", 0)
	offNotifier := startNotifier(t, disabled, offSink)
	if offNotifier.Publish(Event{Event: KindRunFinished}) {
		t.Fatal("no event should be queued while notifications are disabled")
	}
}

func TestNotifierDropsEventsWhenTheQueueIsFull(t *testing.T) {
	release := make(chan struct{})
	blocking := &blockingSink{release: release}
	notifier := startNotifier(t, enabledConfig(), blocking)

	queued := 0
	for i := 0; i < queueCapacity+50; i++ {
		if notifier.Publish(Event{Event: KindRunFinished}) {
			queued++
		}
	}
	close(release)

	if queued > queueCapacity+1 {
		t.Fatalf("queued %d events, want at most %d", queued, queueCapacity+1)
	}
	if queued < queueCapacity {
		t.Fatalf("queued %d events, want the queue to fill to %d", queued, queueCapacity)
	}
}

type blockingSink struct {
	release chan struct{}
}

func (s *blockingSink) Name() string { return "blocking" }

func (s *blockingSink) Configured(Config) bool { return true }

func (s *blockingSink) Send(ctx context.Context, _ Config, _ Event) error {
	select {
	case <-s.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestWebhookSinkSignsTheExactBody(t *testing.T) {
	type captured struct {
		signature string
		body      []byte
	}
	received := make(chan captured, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received <- captured{signature: r.Header.Get(SignatureHeader), body: body}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	cfg := Config{Webhook: WebhookConfig{URL: server.URL, Secret: "shared"}}
	sink := NewWebhookSink(server.Client())
	event := Event{
		Event:       KindRunFinished,
		ProjectID:   "p1",
		ProjectSlug: "demo",
		ProjectName: "Demo",
		ChatID:      "c1",
		ChatTitle:   "Fix login",
		Provider:    "claude",
		Status:      StatusFinished,
		Summary:     "all green",
		URL:         "https://remote.example.com/?chat=c1",
		At:          1700000000000,
	}

	if err := sink.Send(context.Background(), cfg, event); err != nil {
		t.Fatalf("Send: %v", err)
	}

	got := <-received
	if want := SignBody("shared", got.body); got.signature != want {
		t.Fatalf("signature = %q, want %q", got.signature, want)
	}

	var payload map[string]any
	if err := json.Unmarshal(got.body, &payload); err != nil {
		t.Fatalf("payload is not JSON: %v", err)
	}
	for _, key := range []string{
		"event", "projectId", "projectSlug", "projectName",
		"chatId", "chatTitle", "provider", "status", "summary", "url", "at",
	} {
		if _, ok := payload[key]; !ok {
			t.Fatalf("payload missing %q: %v", key, payload)
		}
	}
	if _, leaked := payload["dedupeKey"]; leaked {
		t.Fatalf("payload leaked the internal dedupe key: %v", payload)
	}
}

func TestWebhookSinkOmitsTheSignatureWithoutASecret(t *testing.T) {
	signatures := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		signatures <- r.Header.Get(SignatureHeader)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	sink := NewWebhookSink(server.Client())
	if err := sink.Send(context.Background(), Config{Webhook: WebhookConfig{URL: server.URL}}, Event{Event: KindTest}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if signature := <-signatures; signature != "" {
		t.Fatalf("signature = %q, want none", signature)
	}
}

func TestWebhookSinkReportsNonSuccessStatuses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("nope"))
	}))
	defer server.Close()

	sink := NewWebhookSink(server.Client())
	err := sink.Send(context.Background(), Config{Webhook: WebhookConfig{URL: server.URL}}, Event{Event: KindTest})
	if err == nil {
		t.Fatal("expected an error for a 418 response")
	}
}

func TestTelegramSinkPostsToTheBotAPIAndHidesTheToken(t *testing.T) {
	type captured struct {
		path string
		body map[string]any
	}
	received := make(chan captured, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		received <- captured{path: r.URL.Path, body: body}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"description":"bad token 987654:SECRETTOKEN"}`))
	}))
	defer server.Close()

	cfg := Config{Telegram: TelegramConfig{BotToken: "987654:SECRETTOKEN", ChatID: "-100200"}}
	sink := NewTelegramSink(server.Client()).WithBaseURL(server.URL)

	err := sink.Send(context.Background(), cfg, Event{Event: KindRunFinished, Summary: "done"})
	if err == nil {
		t.Fatal("expected an error for a 400 response")
	}
	if got := err.Error(); strings.Contains(got, "SECRETTOKEN") {
		t.Fatalf("error leaked the bot token: %s", got)
	}

	got := <-received
	if got.path != "/bot987654:SECRETTOKEN/sendMessage" {
		t.Fatalf("path = %q", got.path)
	}
	if got.body["chat_id"] != "-100200" || got.body["parse_mode"] != "HTML" {
		t.Fatalf("body = %+v", got.body)
	}
}

func TestSendTestReportsEverySinkOutcome(t *testing.T) {
	working := newRecordingSink("telegram", 0)
	broken := newRecordingSink("webhook", 99)
	unconfigured := newRecordingSink("other", 0)
	unconfigured.configured = false

	notifier := NewNotifier(
		func() Config { return enabledConfig() },
		WithSinks(working, broken, unconfigured),
		WithBackoff(time.Millisecond),
	)

	results := notifier.SendTest(context.Background(), Event{Event: KindTest})

	if len(results) != 3 {
		t.Fatalf("results = %+v, want one per sink", results)
	}
	if !results[0].Delivered || results[0].Error != "" {
		t.Fatalf("working sink result = %+v", results[0])
	}
	if results[1].Delivered || results[1].Error == "" {
		t.Fatalf("broken sink result = %+v", results[1])
	}
	if results[2].Configured || results[2].Error != "not configured" {
		t.Fatalf("unconfigured sink result = %+v", results[2])
	}
}
