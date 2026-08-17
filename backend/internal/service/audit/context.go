package audit

import (
	"context"
	"strings"
)

// Caller is the request-scoped identity the audit log stamps onto every entry
// recorded while handling that request. Transport puts it in the context so
// service code can record an action without taking an *http.Request.
type Caller struct {
	Actor     Actor
	IP        string
	UserAgent string
}

type callerContextKey struct{}

// WithCaller always replaces any caller already on ctx.
func WithCaller(ctx context.Context, caller Caller) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	caller.Actor.Email = NormalizeActorEmail(caller.Actor.Email)
	return context.WithValue(ctx, callerContextKey{}, caller)
}

// EnsureCaller attaches caller only when ctx does not already carry one. The
// authentication middleware runs first and resolves the richest identity it
// can; inner layers use this so a coarser guess never overwrites it.
func EnsureCaller(ctx context.Context, caller Caller) context.Context {
	if _, ok := CallerFrom(ctx); ok {
		return ctx
	}
	return WithCaller(ctx, caller)
}

// CallerFrom returns the caller stashed by the transport layer, if any.
func CallerFrom(ctx context.Context) (Caller, bool) {
	if ctx == nil {
		return Caller{}, false
	}
	caller, ok := ctx.Value(callerContextKey{}).(Caller)
	return caller, ok
}

func truncate(value string, max int) string {
	value = strings.TrimSpace(value)
	if len(value) <= max {
		return value
	}
	return value[:max]
}
