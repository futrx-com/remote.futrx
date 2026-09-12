package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// SinkTelegram is the stable identifier of the Telegram sink.
const SinkTelegram = "telegram"

const telegramAPIBase = "https://api.telegram.org"

// telegramMessageLimit is the Bot API sendMessage ceiling. Messages are
// truncated well below it so a long agent summary cannot fail a delivery.
const telegramMessageLimit = 3500

// TelegramSink posts to the Bot API sendMessage method.
type TelegramSink struct {
	client  *http.Client
	baseURL string
}

func NewTelegramSink(client *http.Client) *TelegramSink {
	return &TelegramSink{client: client, baseURL: telegramAPIBase}
}

// WithBaseURL points the sink at a different API origin. Tests use it to aim
// at an httptest server.
func (s *TelegramSink) WithBaseURL(baseURL string) *TelegramSink {
	s.baseURL = strings.TrimRight(baseURL, "/")
	return s
}

func (s *TelegramSink) Name() string { return SinkTelegram }

func (s *TelegramSink) Configured(cfg Config) bool { return cfg.TelegramConfigured() }

func (s *TelegramSink) Send(ctx context.Context, cfg Config, event Event) error {
	token := strings.TrimSpace(cfg.Telegram.BotToken)
	body, err := json.Marshal(map[string]any{
		"chat_id":                  strings.TrimSpace(cfg.Telegram.ChatID),
		"text":                     TelegramMessage(event),
		"parse_mode":               "HTML",
		"disable_web_page_preview": true,
	})
	if err != nil {
		return fmt.Errorf("encode telegram payload: %w", err)
	}

	endpoint := s.baseURL + "/bot" + url.PathEscape(token) + "/sendMessage"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build telegram request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := s.client.Do(request)
	if err != nil {
		// Never surface the URL: it embeds the bot token.
		return fmt.Errorf("telegram request failed: %s", redactToken(err.Error(), token))
	}
	defer response.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(response.Body, 4<<10))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf(
			"telegram responded %d: %s",
			response.StatusCode,
			redactToken(strings.TrimSpace(string(payload)), token),
		)
	}
	return nil
}

// redactToken keeps a bot token out of error strings that reach the admin UI
// and the server log.
func redactToken(text, token string) string {
	if token == "" {
		return text
	}
	return strings.ReplaceAll(text, token, MaskSecret(token))
}

// TelegramMessage renders an event as Telegram-flavoured HTML. Every
// interpolated value is escaped, so agent output cannot inject markup.
func TelegramMessage(event Event) string {
	var out strings.Builder
	out.WriteString(telegramIcon(event))
	out.WriteString(" <b>")
	out.WriteString(escapeTelegramHTML(EventHeadline(event)))
	out.WriteString("</b>")

	writeField := func(label, value string) {
		if strings.TrimSpace(value) == "" {
			return
		}
		out.WriteString("\n<b>")
		out.WriteString(label)
		out.WriteString(":</b> ")
		out.WriteString(escapeTelegramHTML(value))
	}
	writeField("Project", event.ProjectName)
	writeField("Chat", event.ChatTitle)
	writeField("Agent", event.Provider)
	writeField("Status", event.Status)

	if summary := strings.TrimSpace(event.Summary); summary != "" {
		out.WriteString("\n\n")
		out.WriteString(escapeTelegramHTML(truncate(summary, 900)))
	}
	if link := strings.TrimSpace(event.URL); link != "" {
		out.WriteString("\n\n<a href=\"")
		out.WriteString(escapeTelegramHTML(link))
		out.WriteString("\">Open in Remote</a>")
	}
	return truncate(out.String(), telegramMessageLimit)
}

func telegramIcon(event Event) string {
	switch event.Event {
	case KindRunFinished:
		return "✅"
	case KindRunFailed:
		return "❌"
	case KindNeedsAttention:
		return "❗"
	case KindScheduledRun:
		if event.Status == StatusFailed {
			return "❌"
		}
		return "⏰"
	default:
		return "\U0001f514"
	}
}

// escapeTelegramHTML escapes the three characters the Bot API treats as
// markup in HTML parse mode.
func escapeTelegramHTML(value string) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return replacer.Replace(value)
}

func truncate(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	if limit <= 1 {
		return string(runes[:limit])
	}
	return string(runes[:limit-1]) + "…"
}
