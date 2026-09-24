package applications

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// WithCancellation attaches the lifetime of one host call to a request. It is
// transport plumbing exposed because the RPC adapter lives in a child package;
// backend code normally consumes CancellationContext, Done, or Err instead.
// A nil context is treated as context.Background.
func (r Request) WithCancellation(ctx context.Context) Request {
	if ctx == nil {
		ctx = context.Background()
	}
	r.cancellation = ctx
	return r
}

// CancellationContext returns a context that is cancelled when the HTTP
// caller disconnects or the backend's setup deadline expires. A request used
// directly in a unit test or outside the RPC transport returns Background.
func (r Request) CancellationContext() context.Context {
	if r.cancellation == nil {
		return context.Background()
	}
	return r.cancellation
}

// Done is closed when the host no longer needs this request. It is nil for a
// request that did not arrive through the RPC transport.
func (r Request) Done() <-chan struct{} {
	return r.CancellationContext().Done()
}

// Err reports context.Canceled for a disconnected caller and
// context.DeadlineExceeded when the backend setup deadline elapsed.
func (r Request) Err() error {
	return context.Cause(r.CancellationContext())
}

// Tail returns the part of Path after prefix, or "" when Path does not start
// with it. It is the companion to a "kv/*" route pattern.
func (r Request) Tail(prefix string) string {
	if !strings.HasPrefix(r.Path, prefix) {
		return ""
	}
	return strings.TrimPrefix(r.Path, prefix)
}

// QueryValue returns the first value for a query parameter, or "".
func (r Request) QueryValue(key string) string {
	values := r.Query[key]
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

// Header returns the first value for a header, matched case-insensitively.
func (r Request) Header(name string) string {
	for key, values := range r.Headers {
		if strings.EqualFold(key, name) && len(values) > 0 {
			return values[0]
		}
	}
	return ""
}

// DecodeJSON unmarshals the request body into target.
func (r Request) DecodeJSON(target any) error {
	if len(r.Body) == 0 {
		return fmt.Errorf("applications: empty request body")
	}
	return json.Unmarshal(r.Body, target)
}
