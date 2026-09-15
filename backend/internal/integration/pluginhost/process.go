package pluginhost

import (
	"context"
	"errors"
	"fmt"

	goplugin "github.com/hashicorp/go-plugin"

	"github.com/futrx-com/remote.futrx.com/pkg/appplugin"
)

// pluginProcess is one live plugin. Its fields are fixed after launch and the
// host publishes the process only after the handshake and initialization have
// completed.
type pluginProcess struct {
	binary     string
	client     *goplugin.Client
	backend    appplugin.Backend
	descriptor appplugin.Descriptor
}

func (p *pluginProcess) running() bool {
	return !p.client.Exited()
}

func (p *pluginProcess) stop() {
	p.client.Kill()
}

// call enforces the service's timeout on a transport that has no notion of
// one. net/rpc calls cannot be cancelled, so a timed-out call is abandoned
// rather than interrupted; the plugin keeps running and the next request
// finds it healthy.
func (p *pluginProcess) call(
	ctx context.Context,
	request appplugin.Request,
) (appplugin.Response, error) {
	type result struct {
		response appplugin.Response
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
			return appplugin.Response{}, fmt.Errorf("plugin call failed: %w", outcome.err)
		}
		return outcome.response, nil
	case <-ctx.Done():
		if !p.running() {
			return appplugin.Response{}, errors.New("plugin exited while handling the request")
		}
		return appplugin.Response{}, fmt.Errorf("plugin call timed out: %w", ctx.Err())
	}
}
