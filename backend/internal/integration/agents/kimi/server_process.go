package kimi

import (
	"context"
	"errors"
	"os/exec"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

// executeCommand owns transport startup, cancellation persistence and draining
// shutdown before the provider collects credentials or publishes an outcome.
func (r *serverRun) executeCommand(ctx context.Context, cmd *exec.Cmd) error {
	transport, err := startServerTransport(ctx, cmd)
	if err == nil {
		transport.onEvent = r.onEvent
		err = r.execute(ctx, transport)
		if err != nil || ctx.Err() != nil || r.interrupted {
			r.abort(transport)
		}
		if closeErr := transport.close(); err == nil {
			err = closeErr
		}
		if err == nil && r.failure != "" {
			err = errors.New(r.failure)
		}
	}
	return err
}

func (r *serverRun) finish(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		r.publish(agent.Event{Type: agent.EventRunInterrupted, Status: "interrupted", Usage: r.usage.raw()})
		if errors.Is(ctx.Err(), context.Canceled) {
			return nil
		}
		return ctx.Err()
	}
	if errors.Is(err, agent.ErrSessionNotFound) {
		return err
	}
	if err != nil {
		r.publish(agent.Event{Type: agent.EventRunFailed, Message: "Kimi run failed: " + err.Error(), IsError: true, Usage: r.usage.raw()})
		return agent.ErrRunFailed
	}
	if r.interrupted {
		r.publish(agent.Event{Type: agent.EventRunInterrupted, Status: "interrupted", Usage: r.usage.raw()})
		return nil
	}
	r.publish(agent.Event{Type: agent.EventRunCompleted, Usage: r.usage.raw()})
	return nil
}
