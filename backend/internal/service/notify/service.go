package notify

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strings"
	"sync"
	"time"
)

// ErrInvalidConfig is returned for configurations the operator can fix.
var ErrInvalidConfig = errors.New("invalid notification settings")

// testTimeout bounds the synchronous admin "send test" probe.
const testTimeout = 30 * time.Second

// Store persists the single global configuration document.
type Store interface {
	Load(ctx context.Context) (Config, error)
	Save(ctx context.Context, cfg Config) error
}

// Service owns the configuration cache and the notifier. Handlers talk to it;
// the run and schedule pipelines publish through Publish, which never blocks.
type Service struct {
	store    Store
	notifier *Notifier
	baseURL  string
	now      func() time.Time

	mu     sync.RWMutex
	config Config
}

type Option func(*Service)

// WithNotifier replaces the notifier. Tests use it to install fake sinks.
func WithNotifier(notifier *Notifier) Option {
	return func(s *Service) {
		if notifier != nil {
			s.notifier = notifier
		}
	}
}

// WithClock replaces the clock used for UpdatedAt and event timestamps.
func WithClock(now func() time.Time) Option {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}

// New loads the stored configuration and starts the delivery worker. A missing
// or unreadable document degrades to defaults so notification problems can
// never keep the server from booting.
func New(ctx context.Context, store Store, baseURL string, options ...Option) *Service {
	service := &Service{
		store:   store,
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		now:     time.Now,
		config:  DefaultConfig(),
	}
	if store != nil {
		loaded, err := store.Load(ctx)
		if err != nil {
			log.Printf("notify: reading stored settings failed, notifications stay off: %v", err)
		} else {
			service.config = loaded.Normalize()
		}
	}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	if service.notifier == nil {
		service.notifier = NewNotifier(service.Config)
	} else {
		service.notifier.config = service.Config
	}
	service.notifier.Start(ctx)
	return service
}

// Stop shuts the delivery worker down.
func (s *Service) Stop() {
	if s == nil {
		return
	}
	s.notifier.Stop()
}

// Config returns the live configuration, secrets included. It is the notifier's
// config source and must not be exposed over HTTP.
func (s *Service) Config() Config {
	if s == nil {
		return Config{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config
}

// PublicConfig returns the admin-facing, secret-masked view.
func (s *Service) PublicConfig() PublicConfig {
	return s.Config().Public()
}

// Save validates and persists an update, then arms the new configuration.
func (s *Service) Save(ctx context.Context, input UpdateInput) (PublicConfig, error) {
	if s == nil {
		return PublicConfig{}, errors.New("notifications are unavailable")
	}
	next := s.Config().Apply(input)
	if err := validate(next); err != nil {
		return PublicConfig{}, err
	}
	next.UpdatedAt = s.now().UnixMilli()

	if s.store != nil {
		if err := s.store.Save(ctx, next); err != nil {
			return PublicConfig{}, fmt.Errorf("save notification settings: %w", err)
		}
	}
	s.mu.Lock()
	s.config = next
	s.mu.Unlock()
	return next.Public(), nil
}

// Test delivers a synthetic event to every configured sink and reports each
// sink's outcome so an operator can debug credentials without waiting for a
// real agent run.
func (s *Service) Test(ctx context.Context) []SinkResult {
	if s == nil {
		return nil
	}
	// Bound the whole probe: retries across two sinks could otherwise hold an
	// admin request open for more than a minute.
	ctx, cancel := context.WithTimeout(ctx, testTimeout)
	defer cancel()
	return s.notifier.SendTest(ctx, Event{
		Event:   KindTest,
		Status:  StatusFinished,
		Summary: "Notifications from Remote are working. You will be pinged when an agent run finishes, fails, or needs you.",
		URL:     ChatURL(s.baseURL, ""),
		At:      s.now().UnixMilli(),
	})
}

// Publish queues an event for delivery. It fills in the timestamp and the deep
// link, and returns immediately.
func (s *Service) Publish(event Event) {
	if s == nil {
		return
	}
	if event.At == 0 {
		event.At = s.now().UnixMilli()
	}
	if event.URL == "" {
		event.URL = ChatURL(s.baseURL, event.ChatID)
	}
	s.notifier.Publish(event)
}

func validate(cfg Config) error {
	cfg = cfg.Normalize()
	if cfg.Telegram.BotToken != "" && cfg.Telegram.ChatID == "" {
		return fmt.Errorf("%w: a Telegram chat ID is required alongside the bot token", ErrInvalidConfig)
	}
	if cfg.Telegram.ChatID != "" && cfg.Telegram.BotToken != "" && strings.ContainsAny(cfg.Telegram.ChatID, " \t") {
		return fmt.Errorf("%w: the Telegram chat ID must not contain spaces", ErrInvalidConfig)
	}
	if cfg.Webhook.URL != "" {
		parsed, err := url.Parse(cfg.Webhook.URL)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return fmt.Errorf("%w: the webhook URL must be an absolute http(s) URL", ErrInvalidConfig)
		}
	}
	if cfg.Webhook.Secret != "" && cfg.Webhook.URL == "" {
		return fmt.Errorf("%w: a webhook URL is required alongside the shared secret", ErrInvalidConfig)
	}
	if cfg.Enabled && !cfg.TelegramConfigured() && !cfg.WebhookConfigured() {
		return fmt.Errorf("%w: configure Telegram or a webhook before enabling notifications", ErrInvalidConfig)
	}
	return nil
}
