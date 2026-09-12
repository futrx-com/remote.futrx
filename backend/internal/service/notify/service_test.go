package notify

import (
	"context"
	"errors"
	"testing"
	"time"
)

type memoryStore struct {
	config  Config
	saved   []Config
	loadErr error
}

func (s *memoryStore) Load(context.Context) (Config, error) {
	if s.loadErr != nil {
		return Config{}, s.loadErr
	}
	return s.config, nil
}

func (s *memoryStore) Save(_ context.Context, cfg Config) error {
	s.config = cfg
	s.saved = append(s.saved, cfg)
	return nil
}

func fixedClock() func() time.Time {
	return func() time.Time { return time.UnixMilli(1700000000000) }
}

func newTestService(t *testing.T, store Store, sinks ...Sink) *Service {
	t.Helper()
	var options []Option
	if len(sinks) > 0 {
		options = append(options, WithNotifier(NewNotifier(nil, WithSinks(sinks...), WithBackoff(time.Millisecond))))
	}
	options = append(options, WithClock(fixedClock()))
	service := New(context.Background(), store, "https://remote.example.com", options...)
	t.Cleanup(service.Stop)
	return service
}

func TestNewFallsBackToDefaultsWhenTheStoreFails(t *testing.T) {
	service := newTestService(t, &memoryStore{loadErr: errors.New("disk on fire")})

	if got := service.Config(); got.Enabled || !got.Events.RunFinished {
		t.Fatalf("config = %+v, want defaults", got)
	}
}

func TestSaveValidatesAndPersists(t *testing.T) {
	tests := []struct {
		name      string
		input     UpdateInput
		wantError bool
	}{
		{
			name: "telegram and webhook together",
			input: UpdateInput{
				Enabled:  true,
				Telegram: TelegramInput{BotToken: "123:abc", ChatID: "-100"},
				Webhook:  WebhookInput{URL: "https://hooks.example.com/x", Secret: "s"},
				Events:   EventToggles{RunFinished: true},
			},
		},
		{
			name:      "enabling without a sink is rejected",
			input:     UpdateInput{Enabled: true, Events: EventToggles{RunFinished: true}},
			wantError: true,
		},
		{
			name:      "a bot token without a chat id is rejected",
			input:     UpdateInput{Telegram: TelegramInput{BotToken: "123:abc"}},
			wantError: true,
		},
		{
			name:      "a relative webhook URL is rejected",
			input:     UpdateInput{Webhook: WebhookInput{URL: "/hooks/remote"}},
			wantError: true,
		},
		{
			name:      "a webhook secret without a URL is rejected",
			input:     UpdateInput{Webhook: WebhookInput{Secret: "s"}},
			wantError: true,
		},
		{
			name:  "disabled with no sinks is allowed",
			input: UpdateInput{Events: EventToggles{RunFinished: true}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &memoryStore{config: DefaultConfig()}
			service := newTestService(t, store)

			public, err := service.Save(context.Background(), test.input)
			if test.wantError {
				if err == nil {
					t.Fatal("expected a validation error")
				}
				if !errors.Is(err, ErrInvalidConfig) {
					t.Fatalf("error = %v, want ErrInvalidConfig", err)
				}
				if len(store.saved) != 0 {
					t.Fatalf("a rejected configuration was persisted: %+v", store.saved)
				}
				return
			}
			if err != nil {
				t.Fatalf("Save: %v", err)
			}
			if len(store.saved) != 1 {
				t.Fatalf("saved %d documents, want 1", len(store.saved))
			}
			if public.UpdatedAt != 1700000000000 {
				t.Fatalf("UpdatedAt = %d, want the fixed clock value", public.UpdatedAt)
			}
		})
	}
}

func TestSaveKeepsTheStoredTokenWhenTheAdminResubmitsAMask(t *testing.T) {
	store := &memoryStore{config: Config{
		Telegram: TelegramConfig{BotToken: "123:originaltoken", ChatID: "-100"},
		Events:   EventToggles{RunFinished: true},
	}}
	service := newTestService(t, store)

	public, err := service.Save(context.Background(), UpdateInput{
		Enabled:  true,
		Telegram: TelegramInput{ChatID: "-100"},
		Events:   EventToggles{RunFinished: true},
	})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if public.Telegram.BotTokenMasked != "••••oken" {
		t.Fatalf("masked token = %q", public.Telegram.BotTokenMasked)
	}
	if got := service.Config().Telegram.BotToken; got != "123:originaltoken" {
		t.Fatalf("stored token = %q, want it preserved", got)
	}
}

func TestSaveArmsTheNewConfigurationImmediately(t *testing.T) {
	sink := newRecordingSink("telegram", 0)
	service := newTestService(t, &memoryStore{config: DefaultConfig()}, sink)

	service.Publish(Event{Event: KindRunFinished, DedupeKey: "before"})
	if attempts, _ := sink.counts(); attempts != 0 {
		t.Fatalf("attempts before enabling = %d, want 0", attempts)
	}

	if _, err := service.Save(context.Background(), UpdateInput{
		Enabled:  true,
		Telegram: TelegramInput{BotToken: "123:abc", ChatID: "-100"},
		Events:   EventToggles{RunFinished: true},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	service.Publish(Event{Event: KindRunFinished, DedupeKey: "after"})
	sink.waitForDelivery(t)

	_, delivered := sink.counts()
	if len(delivered) != 1 {
		t.Fatalf("delivered = %+v, want exactly the post-save event", delivered)
	}
	if delivered[0].URL != "https://remote.example.com/" {
		t.Fatalf("deep link = %q", delivered[0].URL)
	}
}

func TestPublishFillsTheChatDeepLinkAndTimestamp(t *testing.T) {
	sink := newRecordingSink("telegram", 0)
	store := &memoryStore{config: Config{
		Enabled:  true,
		Telegram: TelegramConfig{BotToken: "123:abc", ChatID: "-100"},
		Events:   EventToggles{RunFinished: true},
	}}
	service := newTestService(t, store, sink)

	service.Publish(Event{Event: KindRunFinished, ChatID: "abc123"})
	sink.waitForDelivery(t)

	_, delivered := sink.counts()
	if len(delivered) != 1 {
		t.Fatalf("delivered = %+v", delivered)
	}
	if delivered[0].URL != "https://remote.example.com/?chat=abc123" {
		t.Fatalf("deep link = %q", delivered[0].URL)
	}
	if delivered[0].At != 1700000000000 {
		t.Fatalf("timestamp = %d, want the fixed clock value", delivered[0].At)
	}
}

func TestTestReportsPerSinkResults(t *testing.T) {
	working := newRecordingSink("telegram", 0)
	service := newTestService(t, &memoryStore{config: DefaultConfig()}, working)

	results := service.Test(context.Background())

	if len(results) != 1 || !results[0].Delivered {
		t.Fatalf("results = %+v, want one delivered sink", results)
	}
	_, delivered := working.counts()
	if len(delivered) != 1 || delivered[0].Event != KindTest {
		t.Fatalf("delivered = %+v, want a single test event", delivered)
	}
}
