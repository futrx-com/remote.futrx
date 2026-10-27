package main

import (
	"context"
	"log"
)

// chatIndexWarmer is the startup-only capability needed by the executable.
// The store remains synchronous; this composition layer owns the goroutine.
type chatIndexWarmer interface {
	WarmRecentChatIndexes(context.Context, int) error
}

// startChatIndexWarmup launches the one-shot startup task without delaying the
// HTTP server. The returned channel closes when the task finishes, keeping its
// lifecycle observable without moving scheduling into the persistence layer.
func startChatIndexWarmup(
	ctx context.Context,
	warmer chatIndexWarmer,
	chatLimit int,
	logger *log.Logger,
) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := warmer.WarmRecentChatIndexes(ctx, chatLimit); err != nil {
			logger.Printf("chat event index warmup warning: %v", err)
		}
	}()
	return done
}
