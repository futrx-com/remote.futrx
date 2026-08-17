package notify

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// SinkWebhook is the stable identifier of the generic webhook sink.
const SinkWebhook = "webhook"

// SignatureHeader carries the HMAC-SHA256 of the raw request body when a
// shared secret is configured.
const SignatureHeader = "X-Remote-Signature"

// WebhookSink POSTs the event JSON to an operator-supplied URL.
type WebhookSink struct {
	client *http.Client
}

func NewWebhookSink(client *http.Client) *WebhookSink {
	return &WebhookSink{client: client}
}

func (s *WebhookSink) Name() string { return SinkWebhook }

func (s *WebhookSink) Configured(cfg Config) bool { return cfg.WebhookConfigured() }

func (s *WebhookSink) Send(ctx context.Context, cfg Config, event Event) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("encode webhook payload: %w", err)
	}

	endpoint := strings.TrimSpace(cfg.Webhook.URL)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build webhook request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	if secret := strings.TrimSpace(cfg.Webhook.Secret); secret != "" {
		request.Header.Set(SignatureHeader, SignBody(secret, body))
	}

	response, err := s.client.Do(request)
	if err != nil {
		return fmt.Errorf("webhook request failed: %w", err)
	}
	defer response.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(response.Body, 4<<10))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf(
			"webhook responded %d: %s",
			response.StatusCode,
			truncate(strings.TrimSpace(string(payload)), 200),
		)
	}
	return nil
}

// SignBody returns the value of the signature header for body: the lowercase
// hex HMAC-SHA256 of the exact bytes sent, prefixed with the algorithm.
func SignBody(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}
