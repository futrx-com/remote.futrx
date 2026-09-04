// Package notify delivers agent-run lifecycle events to outbound sinks
// (Telegram and a generic webhook) so an operator who left an agent working
// gets pinged when it finishes, fails, or needs a human.
//
// Delivery is best effort by design: events are queued and retried on a worker
// goroutine so a slow or broken sink can never block or fail a run.
package notify

import (
	"strings"
)

// Kind identifies a lifecycle event. The values double as the JSON keys of the
// per-event toggles in the stored configuration.
type Kind string

const (
	KindRunFinished    Kind = "runFinished"
	KindRunFailed      Kind = "runFailed"
	KindNeedsAttention Kind = "needsAttention"
	KindScheduledRun   Kind = "scheduledRun"
	// KindTest is only produced by the admin "send test" action. It ignores
	// the per-event toggles and the global enable switch.
	KindTest Kind = "test"
)

// Event is the provider-neutral payload delivered to every sink. Its JSON
// shape is the documented generic-webhook contract, so the field names and
// omitempty behaviour are load bearing.
type Event struct {
	Event       Kind   `json:"event"`
	ProjectID   string `json:"projectId,omitempty"`
	ProjectSlug string `json:"projectSlug,omitempty"`
	ProjectName string `json:"projectName,omitempty"`
	ChatID      string `json:"chatId,omitempty"`
	ChatTitle   string `json:"chatTitle,omitempty"`
	Provider    string `json:"provider,omitempty"`
	Status      string `json:"status,omitempty"`
	Summary     string `json:"summary,omitempty"`
	URL         string `json:"url,omitempty"`
	At          int64  `json:"at"`

	// DedupeKey collapses repeated deliveries of the same logical event (for
	// example one "finished" per run). It never reaches the wire.
	DedupeKey string `json:"-"`
}

// Config is the global notification configuration persisted at
// DATA_DIR/notifications.json.
type Config struct {
	Enabled   bool           `json:"enabled"`
	Telegram  TelegramConfig `json:"telegram"`
	Webhook   WebhookConfig  `json:"webhook"`
	Events    EventToggles   `json:"events"`
	UpdatedAt int64          `json:"updatedAt,omitempty"`
}

type TelegramConfig struct {
	BotToken string `json:"botToken,omitempty"`
	ChatID   string `json:"chatId,omitempty"`
}

type WebhookConfig struct {
	URL    string `json:"url,omitempty"`
	Secret string `json:"secret,omitempty"`
}

// EventToggles selects which lifecycle events are delivered. Defaults are all
// true so enabling notifications does not also require picking events.
type EventToggles struct {
	RunFinished    bool `json:"runFinished"`
	RunFailed      bool `json:"runFailed"`
	NeedsAttention bool `json:"needsAttention"`
	ScheduledRun   bool `json:"scheduledRun"`
}

// DefaultConfig is the configuration a server starts with: off, no sinks, all
// events selected.
func DefaultConfig() Config {
	return Config{
		Enabled: false,
		Events: EventToggles{
			RunFinished:    true,
			RunFailed:      true,
			NeedsAttention: true,
			ScheduledRun:   true,
		},
	}
}

// Normalize trims user-entered values so comparisons and "is configured"
// checks do not depend on stray whitespace.
func (c Config) Normalize() Config {
	c.Telegram.BotToken = strings.TrimSpace(c.Telegram.BotToken)
	c.Telegram.ChatID = strings.TrimSpace(c.Telegram.ChatID)
	c.Webhook.URL = strings.TrimSpace(c.Webhook.URL)
	c.Webhook.Secret = strings.TrimSpace(c.Webhook.Secret)
	return c
}

// TelegramConfigured reports whether the Telegram sink has everything it needs.
func (c Config) TelegramConfigured() bool {
	return strings.TrimSpace(c.Telegram.BotToken) != "" && strings.TrimSpace(c.Telegram.ChatID) != ""
}

// WebhookConfigured reports whether the generic webhook sink has a target.
func (c Config) WebhookConfigured() bool {
	return strings.TrimSpace(c.Webhook.URL) != ""
}

// WantsEvent reports whether kind should be delivered under this
// configuration. Test events bypass both the master switch and the toggles so
// an operator can always debug a sink.
func (c Config) WantsEvent(kind Kind) bool {
	if kind == KindTest {
		return true
	}
	if !c.Enabled {
		return false
	}
	switch kind {
	case KindRunFinished:
		return c.Events.RunFinished
	case KindRunFailed:
		return c.Events.RunFailed
	case KindNeedsAttention:
		return c.Events.NeedsAttention
	case KindScheduledRun:
		return c.Events.ScheduledRun
	default:
		return false
	}
}

// PublicConfig is the admin-facing view. Secrets are never echoed back: the
// caller sees only whether a secret is stored and a masked tail.
type PublicConfig struct {
	Enabled   bool           `json:"enabled"`
	Telegram  PublicTelegram `json:"telegram"`
	Webhook   PublicWebhook  `json:"webhook"`
	Events    EventToggles   `json:"events"`
	UpdatedAt int64          `json:"updatedAt,omitempty"`
}

type PublicTelegram struct {
	Configured     bool   `json:"configured"`
	BotTokenMasked string `json:"botTokenMasked,omitempty"`
	ChatID         string `json:"chatId,omitempty"`
}

type PublicWebhook struct {
	Configured   bool   `json:"configured"`
	URL          string `json:"url,omitempty"`
	SecretMasked string `json:"secretMasked,omitempty"`
}

// Public renders the admin-facing view of a configuration.
func (c Config) Public() PublicConfig {
	c = c.Normalize()
	return PublicConfig{
		Enabled: c.Enabled,
		Telegram: PublicTelegram{
			Configured:     c.TelegramConfigured(),
			BotTokenMasked: MaskSecret(c.Telegram.BotToken),
			ChatID:         c.Telegram.ChatID,
		},
		Webhook: PublicWebhook{
			Configured:   c.WebhookConfigured(),
			URL:          c.Webhook.URL,
			SecretMasked: MaskSecret(c.Webhook.Secret),
		},
		Events:    c.Events,
		UpdatedAt: c.UpdatedAt,
	}
}

const maskPrefix = "••••"

// MaskSecret renders a stored secret as the mask prefix plus its last four
// characters: enough for an operator to recognize which credential is
// installed, never enough to reuse it.
func MaskSecret(secret string) string {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return ""
	}
	runes := []rune(secret)
	if len(runes) <= 4 {
		return maskPrefix
	}
	return maskPrefix + string(runes[len(runes)-4:])
}

// UpdateInput is the admin PUT body. Secrets follow write-only semantics: an
// empty string keeps whatever is stored (the client only ever saw a mask), and
// the explicit clear flags remove it.
type UpdateInput struct {
	Enabled  bool          `json:"enabled"`
	Telegram TelegramInput `json:"telegram"`
	Webhook  WebhookInput  `json:"webhook"`
	Events   EventToggles  `json:"events"`
}

type TelegramInput struct {
	BotToken      string `json:"botToken"`
	ClearBotToken bool   `json:"clearBotToken"`
	ChatID        string `json:"chatId"`
}

type WebhookInput struct {
	URL         string `json:"url"`
	Secret      string `json:"secret"`
	ClearSecret bool   `json:"clearSecret"`
}

// Apply folds an update onto the stored configuration, preserving secrets the
// caller did not resubmit.
func (c Config) Apply(input UpdateInput) Config {
	current := c.Normalize()
	next := Config{
		Enabled: input.Enabled,
		Events:  input.Events,
		Telegram: TelegramConfig{
			BotToken: current.Telegram.BotToken,
			ChatID:   strings.TrimSpace(input.Telegram.ChatID),
		},
		Webhook: WebhookConfig{
			URL:    strings.TrimSpace(input.Webhook.URL),
			Secret: current.Webhook.Secret,
		},
	}
	switch {
	case input.Telegram.ClearBotToken:
		next.Telegram.BotToken = ""
	case strings.TrimSpace(input.Telegram.BotToken) != "":
		next.Telegram.BotToken = strings.TrimSpace(input.Telegram.BotToken)
	}
	switch {
	case input.Webhook.ClearSecret:
		next.Webhook.Secret = ""
	case strings.TrimSpace(input.Webhook.Secret) != "":
		next.Webhook.Secret = strings.TrimSpace(input.Webhook.Secret)
	}
	// A retained secret is meaningless without its destination, and would
	// otherwise make "clear the webhook URL" fail validation forever.
	if next.Webhook.URL == "" && strings.TrimSpace(input.Webhook.Secret) == "" {
		next.Webhook.Secret = ""
	}
	if next.Telegram.ChatID == "" && strings.TrimSpace(input.Telegram.BotToken) == "" {
		next.Telegram.BotToken = ""
	}
	return next
}
