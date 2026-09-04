package capability

import (
	"context"
	"sync"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

// catalogFlights coalesces simultaneous discovery requests for the same
// execution environment. Completed results are deliberately removed from the
// flight map: Service retains them in catalogCache according to the backend's
// freshness policy, while this guard only prevents duplicate CLI processes
// when several requests overlap.
type catalogFlights struct {
	mu      sync.Mutex
	entries map[string]*catalogFlight
}

type catalogFlight struct {
	done   chan struct{}
	result []agent.Capabilities
	err    error
}

func newCatalogFlights() *catalogFlights {
	return &catalogFlights{entries: make(map[string]*catalogFlight)}
}

func (f *catalogFlights) do(
	ctx context.Context,
	key string,
	discover func(context.Context) ([]agent.Capabilities, error),
) ([]agent.Capabilities, error) {
	f.mu.Lock()
	if running, ok := f.entries[key]; ok {
		f.mu.Unlock()
		select {
		case <-running.done:
			return cloneCapabilities(running.result), running.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	running := &catalogFlight{done: make(chan struct{})}
	f.entries[key] = running
	f.mu.Unlock()

	// A browser navigating away should stop waiting, but it should not cancel
	// discovery for other callers that joined the same flight. Service applies
	// the configured global deadline to each provider probe.
	running.result, running.err = discover(context.WithoutCancel(ctx))

	f.mu.Lock()
	delete(f.entries, key)
	close(running.done)
	f.mu.Unlock()

	return cloneCapabilities(running.result), running.err
}

func cloneCapabilities(input []agent.Capabilities) []agent.Capabilities {
	output := make([]agent.Capabilities, len(input))
	for index, caps := range input {
		output[index] = caps.Clone()
	}
	return output
}
