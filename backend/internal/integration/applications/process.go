package applications

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/futrx-com/remote.futrx.com/pkg/applications"
	applicationrpc "github.com/futrx-com/remote.futrx.com/pkg/applications/rpc"
)

// backendProcess is one live backend. Its fields are fixed after launch and the
// host publishes the process only after the handshake and initialization have
// completed.
type backendProcess struct {
	applicationID string
	binary        string
	configuration [32]byte
	generation    uint64
	client        processClient
	backend       applications.Backend
	descriptor    applications.Descriptor
	unhealthy     atomic.Bool
}

type processClient interface {
	Exited() bool
	Kill()
	ForceKill()
}

type backendCallResult struct {
	response applications.Response
	err      error
}

type contextBackend interface {
	HandleContext(context.Context, applications.Request) (applications.Response, error)
}

func (p *backendProcess) running() bool {
	return !p.unhealthy.Load() && !p.client.Exited()
}

func (p *backendProcess) stop() {
	p.unhealthy.Store(true)
	p.client.Kill()
}

// stopAsync immediately removes a wedged process from consideration without
// extending the request's deadline by go-plugin's graceful shutdown window.
// The next call sees running=false and relaunches it; Stop/Remove may safely
// call Kill concurrently because go-plugin supports repeated calls.
func (p *backendProcess) stopAsync() {
	if p.unhealthy.CompareAndSwap(false, true) {
		// The transport has already failed its request deadline while writing.
		// Kill the exact child immediately so both its data and control sockets
		// close; go-plugin cleanup can then finish without holding this caller.
		p.client.ForceKill()
		go p.client.Kill()
	}
}

// call enforces the service's timeout and, for the current RPC transport,
// signals that cancellation to only this Handle invocation. Older/custom
// Backend implementations retain the compatibility path below.
func (p *backendProcess) call(
	ctx context.Context,
	request applications.Request,
) (applications.Response, error) {
	if backend, ok := p.backend.(contextBackend); ok {
		response, err := backend.HandleContext(ctx, request)
		writeBlocked := errors.Is(err, applicationrpc.ErrRequestWriteBlocked)
		if writeBlocked {
			p.stopAsync()
		}
		if ctx.Err() != nil {
			// Completion and cancellation can become ready together. If the
			// transport handed us a stream while the caller's context ended,
			// ownership cannot escape through the timeout path.
			if content, _, _, streaming := response.ResponseStream(); streaming && content != nil {
				_ = content.Close()
			}
			if !p.running() && !writeBlocked {
				return applications.Response{}, errors.New("backend exited while handling the request")
			}
			return applications.Response{}, fmt.Errorf("backend call timed out: %w", ctx.Err())
		}
		if err != nil {
			return applications.Response{}, fmt.Errorf("backend call failed: %w", err)
		}
		return response, nil
	}

	done := make(chan backendCallResult)
	go func() {
		response, err := p.backend.Handle(request)
		deliverBackendCall(ctx, done, backendCallResult{response: response, err: err})
	}()

	select {
	case outcome := <-done:
		if outcome.err != nil {
			return applications.Response{}, fmt.Errorf("backend call failed: %w", outcome.err)
		}
		return outcome.response, nil
	case <-ctx.Done():
		if !p.running() {
			return applications.Response{}, errors.New("backend exited while handling the request")
		}
		return applications.Response{}, fmt.Errorf("backend call timed out: %w", ctx.Err())
	}
}

// deliverBackendCall closes a streamed response that arrives after its caller
// has timed out. A buffered response owns no resource, but abandoning a stream
// without this handoff would leave both the broker connection and the
// application-owned reader open until the process exits.
func deliverBackendCall(ctx context.Context, done chan<- backendCallResult, outcome backendCallResult) {
	select {
	case done <- outcome:
	case <-ctx.Done():
		if content, _, _, ok := outcome.response.ResponseStream(); ok && content != nil {
			_ = content.Close()
		}
	}
}

// notify bounds an event handler on an RPC transport that cannot cancel an
// individual call. A timeout terminates the process so neither the host
// goroutine nor the child handler can accumulate behind later events.
func (p *backendProcess) notify(ctx context.Context, event applications.Event) error {
	subscriber, ok := p.backend.(applications.EventSubscriber)
	if !ok {
		return errors.New("backend event subscriber transport is unavailable")
	}
	done := make(chan error, 1)
	go func() {
		done <- subscriber.OnEvent(event)
	}()

	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("backend event handler failed: %w", err)
		}
		return nil
	case <-ctx.Done():
		wasRunning := p.running()
		// Unlike an interactive request, event delivery is driven by a single
		// bounded worker. Leaving an uncancellable OnEvent RPC running would leak
		// one host goroutine and one child handler per later event. Terminate the
		// unhealthy process; the next delivery or request restores it lazily.
		p.stop()
		if !wasRunning {
			return errors.New("backend exited while handling an event")
		}
		return fmt.Errorf("backend event handler timed out: %w", ctx.Err())
	}
}
