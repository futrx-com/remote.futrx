package notify

import "testing"

func TestMaskSecret(t *testing.T) {
	tests := []struct {
		name   string
		secret string
		want   string
	}{
		{name: "empty stays empty", secret: "", want: ""},
		{name: "whitespace only stays empty", secret: "   ", want: ""},
		{name: "short secret exposes nothing", secret: "abcd", want: "••••"},
		{name: "long secret exposes the last four", secret: "1234567890:AAstuv", want: "••••stuv"},
		{name: "surrounding whitespace is ignored", secret: "  token1234  ", want: "••••1234"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := MaskSecret(test.secret); got != test.want {
				t.Fatalf("MaskSecret(%q) = %q, want %q", test.secret, got, test.want)
			}
		})
	}
}

func TestPublicNeverExposesSecrets(t *testing.T) {
	cfg := Config{
		Enabled:  true,
		Telegram: TelegramConfig{BotToken: "123456:ABCDEFsecrettail", ChatID: "-1001234567890"},
		Webhook:  WebhookConfig{URL: "https://hooks.example.com/remote", Secret: "shared-secret-9876"},
		Events:   EventToggles{RunFinished: true},
	}

	public := cfg.Public()

	if public.Telegram.BotTokenMasked != "••••tail" {
		t.Fatalf("bot token mask = %q", public.Telegram.BotTokenMasked)
	}
	if public.Webhook.SecretMasked != "••••9876" {
		t.Fatalf("webhook secret mask = %q", public.Webhook.SecretMasked)
	}
	if !public.Telegram.Configured || !public.Webhook.Configured {
		t.Fatalf("expected both sinks configured: %+v", public)
	}
	if public.Telegram.ChatID != "-1001234567890" {
		t.Fatalf("chat id = %q, want it echoed back", public.Telegram.ChatID)
	}
	if public.Webhook.URL != "https://hooks.example.com/remote" {
		t.Fatalf("webhook url = %q, want it echoed back", public.Webhook.URL)
	}
}

func TestConfigApplyPreservesSecretsUnlessResubmittedOrCleared(t *testing.T) {
	stored := Config{
		Telegram: TelegramConfig{BotToken: "stored-token", ChatID: "111"},
		Webhook:  WebhookConfig{URL: "https://old.example.com", Secret: "stored-secret"},
	}

	tests := []struct {
		name       string
		input      UpdateInput
		wantToken  string
		wantSecret string
	}{
		{
			name:       "blank secrets keep the stored values",
			input:      UpdateInput{Telegram: TelegramInput{ChatID: "222"}, Webhook: WebhookInput{URL: "https://new.example.com"}},
			wantToken:  "stored-token",
			wantSecret: "stored-secret",
		},
		{
			name: "submitted secrets replace the stored values",
			input: UpdateInput{
				Telegram: TelegramInput{BotToken: " fresh-token ", ChatID: "222"},
				Webhook:  WebhookInput{URL: "https://new.example.com", Secret: "fresh-secret"},
			},
			wantToken:  "fresh-token",
			wantSecret: "fresh-secret",
		},
		{
			name: "clear flags remove the stored values",
			input: UpdateInput{
				Telegram: TelegramInput{ClearBotToken: true, BotToken: "ignored"},
				Webhook:  WebhookInput{URL: "https://new.example.com", ClearSecret: true, Secret: "ignored"},
			},
			wantToken:  "",
			wantSecret: "",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := stored.Apply(test.input)
			if got.Telegram.BotToken != test.wantToken {
				t.Fatalf("bot token = %q, want %q", got.Telegram.BotToken, test.wantToken)
			}
			if got.Webhook.Secret != test.wantSecret {
				t.Fatalf("webhook secret = %q, want %q", got.Webhook.Secret, test.wantSecret)
			}
		})
	}
}

func TestWantsEvent(t *testing.T) {
	all := EventToggles{RunFinished: true, RunFailed: true, NeedsAttention: true, ScheduledRun: true}

	tests := []struct {
		name string
		cfg  Config
		kind Kind
		want bool
	}{
		{name: "disabled blocks everything", cfg: Config{Enabled: false, Events: all}, kind: KindRunFinished, want: false},
		{name: "disabled still allows a test", cfg: Config{Enabled: false}, kind: KindTest, want: true},
		{name: "enabled honours the toggle", cfg: Config{Enabled: true, Events: all}, kind: KindNeedsAttention, want: true},
		{
			name: "an unselected event is skipped",
			cfg:  Config{Enabled: true, Events: EventToggles{RunFinished: true}},
			kind: KindRunFailed,
			want: false,
		},
		{name: "unknown kinds are skipped", cfg: Config{Enabled: true, Events: all}, kind: Kind("nonsense"), want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.cfg.WantsEvent(test.kind); got != test.want {
				t.Fatalf("WantsEvent(%q) = %t, want %t", test.kind, got, test.want)
			}
		})
	}
}
