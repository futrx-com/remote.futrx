package applications

import (
	"context"
	"errors"
	"fmt"

	goplugin "github.com/hashicorp/go-plugin"

	"github.com/futrx-com/remote.futrx.com/pkg/applications"
)

// backendProcess is one live backend. Its fields are fixed after launch and the
// host publishes the process only after the handshake and initialization have
// completed.
type backendProcess struct {
	applicationID string
	binary        string
	configuration [32]byte
	generation    uint64
	client        *goplugin.Client
	backend       applications.Backend
	descriptor    applications.Descriptor
}

func (p *backendProcess) running() bool {
	return !p.client.Exited()
}

func (p *backendProcess) stop() {
	p.client.Kill()
}

// call enforces the service's timeout on a transport that has no notion of
// one. net/rpc calls cannot be cancelled, so a timed-out call is abandoned
// rather than interrupted; the backend keeps running and the next request
// finds it healthy.
func (p *backendProcess) call(
	ctx context.Context,
	request applications.Request,
) (applications.Response, error) {
	type result struct {
		response applications.Response
		err      error
	}
	done := make(chan result, 1)
	go func() {
		response, err := p.backend.Handle(request)
		done <- result{response: response, err: err}
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
