package custom

// command.go wires the custom provider's Run to an OpenAI-compatible
// chat-completions endpoint. The admin-supplied base URL is expected to be the
// API root (e.g. https://api.example.com/v1); the completions endpoint is
// {baseURL}/chat/completions. Streaming is requested via "stream": true and
// the SSE response is parsed line-by-line; if the server ignores streaming or
// returns a single JSON object, the full content is emitted as one delta.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	agentauth "github.com/futrx-com/remote.futrx.com/internal/service/agent/auth"
)

const (
	customHTTPTimeout  = 30 * time.Minute
	customMaxLineBytes = 1 << 20
)

// chatMessage is the OpenAI chat-completions message shape we send.
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatRequest is the body posted to {baseURL}/chat/completions.
type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

// chatChoice mirrors one entry in a non-streaming completions response.
type chatChoice struct {
	Message chatMessage `json:"message"`
}

// chatResponse is the non-streaming fallback shape.
type chatResponse struct {
	Choices []chatChoice `json:"choices"`
}

// streamDelta mirrors one SSE chunk's first choice.
type streamDelta struct {
	Delta struct {
		Content string `json:"content"`
	} `json:"delta"`
}

// streamChunk is the JSON payload of one SSE `data:` line.
type streamChunk struct {
	Choices []streamDelta `json:"choices"`
}

func runCompletion(
	ctx context.Context,
	req agent.RunRequest,
	cfg agentauth.APIKeyConfig,
	emit func(agent.Event),
) error {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		emitError(req, emit, "custom provider base url is empty")
		return ErrRunNotConfigured
	}
	if strings.TrimSpace(cfg.Model) == "" {
		emitError(req, emit, "custom provider model is empty")
		return ErrRunNotConfigured
	}

	endpoint := strings.TrimRight(cfg.BaseURL, "/") + "/chat/completions"
	body := chatRequest{
		Model: cfg.Model,
		// The prompt service already embeds prior visible transcript into the
		// prompt string, so a single user message is the right shape here.
		Messages: []chatMessage{{Role: "user", Content: req.Prompt}},
		Stream:   true,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		emitError(req, emit, "build custom provider request: "+err.Error())
		return err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		emitError(req, emit, "build custom provider request: "+err.Error())
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	httpReq.Header.Set("Accept", "text/event-stream")

	client := &http.Client{Timeout: customHTTPTimeout}
	resp, err := client.Do(httpReq)
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return nil
		}
		emitError(req, emit, "custom provider request failed: "+err.Error())
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message := fmt.Sprintf("custom provider returned HTTP %d", resp.StatusCode)
		if tail, readErr := io.ReadAll(io.LimitReader(resp.Body, 4096)); readErr == nil && len(tail) > 0 {
			message += ": " + strings.TrimSpace(string(tail))
		}
		emitError(req, emit, message)
		return fmt.Errorf("%s", message)
	}

	streamed, err := streamOrRead(ctx, resp.Body, req, emit)
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return nil
		}
		return err
	}
	_ = streamed

	emit(agent.Event{
		T:              time.Now().UnixMilli(),
		Type:           agent.EventRunCompleted,
		Provider:       agent.ProviderCustom,
		ConversationID: req.ConversationID,
	})
	return nil
}

// streamOrRead consumes the response body with a single buffered reader so the
// fallback path sees every byte. It peeks the first non-empty line: if it
// begins with an SSE field (`data:`, `event:`, `:`, `id:`, `retry:`) the body
// is treated as a Server-Sent-Events stream; otherwise the full body (peeked
// line included) is parsed as a single chat-completions JSON object.
func streamOrRead(
	ctx context.Context,
	body io.Reader,
	req agent.RunRequest,
	emit func(agent.Event),
) (bool, error) {
	reader := bufio.NewReaderSize(body, customMaxLineBytes)

	// Peek the first non-empty line without consuming it so the fallback can
	// still read it. PeekSize covers one reasonable SSE/JSON line.
	peek, err := reader.Peek(customMaxLineBytes)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, bufio.ErrBufferFull) {
		return false, err
	}
	firstLine := firstNonEmptyLine(peek)
	if isSSEField(firstLine) {
		return true, streamSSEFromReader(ctx, reader, req, emit)
	}
	return false, readFullCompletionFromReader(reader, req, emit)
}

func firstNonEmptyLine(data []byte) string {
	for _, line := range bytes.Split(data, []byte("\n")) {
		if trimmed := bytes.TrimSpace(line); len(trimmed) > 0 {
			return string(trimmed)
		}
	}
	return ""
}

func isSSEField(line string) bool {
	return strings.HasPrefix(line, "data:") ||
		strings.HasPrefix(line, "event:") ||
		strings.HasPrefix(line, "id:") ||
		strings.HasPrefix(line, "retry:") ||
		strings.HasPrefix(line, ":")
}

// streamSSEFromReader parses a Server-Sent-Events stream. Each `data: {...}`
// line whose JSON carries a non-empty choices[0].delta.content is emitted as an
// assistant text delta. A terminal `data: [DONE]` line ends the stream.
func streamSSEFromReader(
	ctx context.Context,
	reader *bufio.Reader,
	req agent.RunRequest,
	emit func(agent.Event),
) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			trimmed := bytes.TrimSpace(line)
			if bytes.HasPrefix(trimmed, []byte("data:")) {
				payload := bytes.TrimSpace(trimmed[5:])
				if bytes.Equal(payload, []byte("[DONE]")) {
					return nil
				}
				if len(payload) > 0 {
					if text := decodeStreamDelta(payload); text != "" {
						emit(agent.Event{
							T:              time.Now().UnixMilli(),
							Type:           agent.EventAssistantTextDelta,
							Provider:       agent.ProviderCustom,
							ConversationID: req.ConversationID,
							ItemKind:       agent.ItemMessage,
							Text:           text,
						})
					}
				}
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

// decodeStreamDelta extracts the assistant text from one SSE `data:` JSON
// payload. Returns "" for keep-alive frames (no choices / empty content) and
// for unparseable lines so the stream loop can keep going.
func decodeStreamDelta(payload []byte) string {
	var chunk streamChunk
	if err := json.Unmarshal(payload, &chunk); err != nil {
		return ""
	}
	if len(chunk.Choices) == 0 {
		return ""
	}
	return chunk.Choices[0].Delta.Content
}

// readFullCompletionFromReader handles a non-streaming JSON response by
// emitting the concatenated choice contents as a single assistant text delta.
// The reader has only been Peek'd (not consumed), so the full body is still
// available to read from the start.
func readFullCompletionFromReader(reader *bufio.Reader, req agent.RunRequest, emit func(agent.Event)) error {
	var buf bytes.Buffer
	chunk := make([]byte, 32*1024)
	for {
		n, err := reader.Read(chunk)
		if n > 0 {
			if buf.Len()+n > 16<<20 {
				return fmt.Errorf("custom provider response exceeds 16MiB")
			}
			buf.Write(chunk[:n])
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}
	}
	raw := bytes.TrimSpace(buf.Bytes())
	if len(raw) == 0 {
		return nil
	}
	var resp chatResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return err
	}
	var text strings.Builder
	for _, choice := range resp.Choices {
		text.WriteString(choice.Message.Content)
	}
	if out := text.String(); out != "" {
		emit(agent.Event{
			T:              time.Now().UnixMilli(),
			Type:           agent.EventAssistantTextDelta,
			Provider:       agent.ProviderCustom,
			ConversationID: req.ConversationID,
			ItemKind:       agent.ItemMessage,
			Text:           out,
		})
	}
	return nil
}

func emitError(req agent.RunRequest, emit func(agent.Event), message string) {
	emit(agent.Event{
		T:              time.Now().UnixMilli(),
		Type:           agent.EventError,
		Provider:       agent.ProviderCustom,
		ConversationID: req.ConversationID,
		Message:        message,
	})
}
