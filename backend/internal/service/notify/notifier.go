package notify

import (
	"context"
	"log"
	"net/http"
	"sync"
	"time"
)

// Sink is one outbound delivery target. Implementations must respect the
// context deadline and return a descriptive error the operator can act on.
type Sink interface {
	// Name is the stable identifier reported back by the test endpoint.
	Name() string
	// Configured reports whether cfg carries everything this sink needs.
	Configured(cfg Config) bool
	// Send delivers one event. It is called from the worker goroutine only.
	Send(ctx context.Context, cfg Config, event Event) error
}

const (
	// queueCapacity bounds the backlog. A full queue drops the newest event
	// rather than blocking the run pipeline that produced it.
	queueCapacity = 256
	// dedupeCapacity bounds how many recent dedupe keys are remembered.
	dedupeCapacity = 512
	// deliveryAttempts is the total number of tries per sink per event.
	deliveryAttempts = 3
	// requestTimeout caps every outbound HTTP call.
	requestTimeout = 10 * time.Second
)

var defaultBackoff = []time.Duration{time.Second, 3 * time.Second}

// ConfigSource reads the live configuration at delivery time so a save takes
// effect for events already sitting in the queue.
type ConfigSource func() Config

// Notifier owns the bounded queue, the single worker goroutine, per-event
// dedupe, and retry with backoff. Publish never blocks.
type Notifier struct {
	config  ConfigSource
	sinks   []Sink
	queue   chan Event
	backoff []time.Duration

	mu       sync.Mutex
	seen     map[string]struct{}
	seenRing []string

	startOnce sync.Once
	stopOnce  sync.Once
	done      chan struct{}
	wg        sync.WaitGroup
}

type NotifierOption func(*Notifier)

// WithBackoff replaces the retry delays. Tests use it to keep runs fast; the
// slice length does not have to match the attempt count (the last entry is
// reused for any further retries).
func WithBackoff(delays ...time.Duration) NotifierOption {
	return func(n *Notifier) {
		if len(delays) > 0 {
			n.backoff = delays
		}
	}
}

// WithSinks replaces the default Telegram + webhook sinks. Used by tests.
func WithSinks(sinks ...Sink) NotifierOption {
	return func(n *Notifier) {
		n.sinks = sinks
	}
}

// NewNotifier builds a notifier over the default sinks. Nothing is delivered
// until Start is called.
func NewNotifier(config ConfigSource, options ...NotifierOption) *Notifier {
	client := &http.Client{Timeout: requestTimeout}
	notifier := &Notifier{
		config:  config,
		sinks:   []Sink{NewTelegramSink(client), NewWebhookSink(client)},
		queue:   make(chan Event, queueCapacity),
		backoff: defaultBackoff,
		seen:    map[string]struct{}{},
		done:    make(chan struct{}),
	}
	for _, option := range options {
		if option != nil {
			option(notifier)
		}
	}
	return notifier
}

// Start launches the worker goroutine. It is idempotent, and the worker stops
// when ctx is cancelled or Stop is called.
func (n *Notifier) Start(ctx context.Context) {
	if n == nil {
		return
	}
	n.startOnce.Do(func() {
		n.wg.Add(1)
		go n.run(ctx)
	})
}

// Stop drains nothing: it signals the worker to exit and waits for the event
// currently in flight. Safe to call more than once.
func (n *Notifier) Stop() {
	if n == nil {
		return
	}
	n.stopOnce.Do(func() { close(n.done) })
	n.wg.Wait()
}

// Publish enqueues an event. It returns false when the event was filtered by
// configuration, suppressed by dedupe, or dropped because the queue is full —
// callers never need to care, and must never block on the result.
func (n *Notifier) Publish(event Event) bool {
	if n == nil {
		return false
	}
	if event.At == 0 {
		event.At = time.Now().UnixMilli()
	}
	cfg := n.currentConfig()
	if !cfg.WantsEvent(event.Event) {
		return false
	}
	if !n.claimDedupeKey(event.DedupeKey) {
		return false
	}
	select {
	case n.queue <- event:
		return true
	default:
		log.Printf("notify: queue full, dropped %s event", event.Event)
		n.releaseDedupeKey(event.DedupeKey)
		return false
	}
}

func (n *Notifier) currentConfig() Config {
	if n.config == nil {
		return Config{}
	}
	return n.config().Normalize()
}

// claimDedupeKey records key and reports whether this is its first sighting.
// An empty key is always accepted.
func (n *Notifier) claimDedupeKey(key string) bool {
	if key == "" {
		return true
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if _, seen := n.seen[key]; seen {
		return false
	}
	n.seen[key] = struct{}{}
	n.seenRing = append(n.seenRing, key)
	if len(n.seenRing) > dedupeCapacity {
		evicted := n.seenRing[0]
		n.seenRing = n.seenRing[1:]
		delete(n.seen, evicted)
	}
	return true
}

func (n *Notifier) releaseDedupeKey(key string) {
	if key == "" {
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if _, seen := n.seen[key]; !seen {
		return
	}
	delete(n.seen, key)
	for i, candidate := range n.seenRing {
		if candidate == key {
			n.seenRing = append(n.seenRing[:i], n.seenRing[i+1:]...)
			break
		}
	}
}

func (n *Notifier) run(ctx context.Context) {
	defer n.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case <-n.done:
			return
		case event := <-n.queue:
			n.deliver(ctx, event)
		}
	}
}

func (n *Notifier) deliver(ctx context.Context, event Event) {
	cfg := n.currentConfig()
	for _, sink := range n.sinks {
		if !sink.Configured(cfg) {
			continue
		}
		if err := n.sendWithRetry(ctx, sink, cfg, event); err != nil {
			log.Printf("notify: %s delivery of %s failed: %v", sink.Name(), event.Event, err)
		}
	}
}

// sendWithRetry makes up to deliveryAttempts tries, sleeping the configured
// backoff between them. It aborts early when the process is shutting down.
func (n *Notifier) sendWithRetry(ctx context.Context, sink Sink, cfg Config, event Event) error {
	var lastErr error
	for attempt := 0; attempt < deliveryAttempts; attempt++ {
		if attempt > 0 && !n.wait(ctx, n.backoffFor(attempt-1)) {
			return lastErr
		}
		attemptCtx, cancel := context.WithTimeout(ctx, requestTimeout)
		err := sink.Send(attemptCtx, cfg, event)
		cancel()
		if err == nil {
			return nil
		}
		lastErr = err
	}
	return lastErr
}

func (n *Notifier) backoffFor(index int) time.Duration {
	if len(n.backoff) == 0 {
		return 0
	}
	if index >= len(n.backoff) {
		index = len(n.backoff) - 1
	}
	return n.backoff[index]
}

// wait sleeps for delay and reports whether the caller should keep going.
func (n *Notifier) wait(ctx context.Context, delay time.Duration) bool {
	if delay <= 0 {
		return true
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-n.done:
		return false
	case <-timer.C:
		return true
	}
}

// SinkResult reports one sink's outcome for the admin test action.
type SinkResult struct {
	Sink       string `json:"sink"`
	Configured bool   `json:"configured"`
	Delivered  bool   `json:"delivered"`
	Error      string `json:"error,omitempty"`
}

// SendTest delivers event to every configured sink synchronously and reports
// per-sink results so an operator can debug credentials. It bypasses the queue
// and the dedupe cache but keeps the same retry and timeout behaviour.
func (n *Notifier) SendTest(ctx context.Context, event Event) []SinkResult {
	if n == nil {
		return nil
	}
	cfg := n.currentConfig()
	results := make([]SinkResult, 0, len(n.sinks))
	for _, sink := range n.sinks {
		result := SinkResult{Sink: sink.Name(), Configured: sink.Configured(cfg)}
		if !result.Configured {
			result.Error = "not configured"
			results = append(results, result)
			continue
		}
		if err := n.sendWithRetry(ctx, sink, cfg, event); err != nil {
			result.Error = err.Error()
		} else {
			result.Delivered = true
		}
		results = append(results, result)
	}
	return results
}
