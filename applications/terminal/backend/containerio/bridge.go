//go:build linux

// Package containerio relays Terminal's requests into the project container.
// One `lxc exec <container> -- remote-terminal bridge` process carries every
// request for an installation as JSON lines, and the container's
// remote-terminal service answers them. The shells belong to that service, so
// this process, and the bridge with it, can restart without ending any.
package containerio

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sync"
	"sync/atomic"

	"futrx.local/catalog/applications/terminal/backend/config"
)

// maxFrameBytes matches the container bridge's line limit.
const maxFrameBytes = 4 << 20

// Request and Response are the bridge's line format, shared with
// backend/container/internal/bridge.
type Request struct {
	ID     uint64 `json:"id"`
	Method string `json:"method"`
	Path   string `json:"path"`
	Query  string `json:"query,omitempty"`
	Body   []byte `json:"body,omitempty"`
}

type Response struct {
	ID     uint64 `json:"id"`
	Status int    `json:"status"`
	Body   []byte `json:"body,omitempty"`
}

var errBridgeExited = errors.New("the terminal bridge into the project container exited")

// BridgeCommand returns how to start the bridge into container. The command
// is fixed; browser input travels only inside its JSON lines.
func BridgeCommand(container string) func() *exec.Cmd {
	return func() *exec.Cmd {
		return exec.Command("lxc", "exec", container, "--", config.ContainerCommand, "bridge")
	}
}

// Bridge sends requests over one bridge process, starting another when the
// last one has exited.
type Bridge struct {
	start  func() *exec.Cmd
	nextID atomic.Uint64

	mu      sync.Mutex
	current *process
}

func NewBridge(start func() *exec.Cmd) *Bridge {
	return &Bridge{start: start}
}

// Do sends request and waits for its answer, the bridge exiting, or ctx.
func (b *Bridge) Do(ctx context.Context, request Request) (Response, error) {
	current, err := b.process()
	if err != nil {
		return Response{}, err
	}
	request.ID = b.nextID.Add(1)
	answer, err := current.send(request)
	if err != nil {
		return Response{}, err
	}
	defer current.forget(request.ID)
	select {
	case response := <-answer:
		return response, nil
	case <-current.exited:
		return Response{}, current.failure()
	case <-ctx.Done():
		return Response{}, ctx.Err()
	}
}

// Warm starts the bridge ahead of the first request, so opening a terminal
// does not wait for `lxc exec`. A failure is retried by the next request.
func (b *Bridge) Warm() {
	_, _ = b.process()
}

// Close stops the current bridge process.
func (b *Bridge) Close() {
	b.mu.Lock()
	current := b.current
	b.current = nil
	b.mu.Unlock()
	if current != nil {
		current.stop()
	}
}

func (b *Bridge) process() (*process, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.current != nil && !b.current.done() {
		return b.current, nil
	}
	started, err := startProcess(b.start())
	if err != nil {
		return nil, fmt.Errorf("start the terminal bridge: %w", err)
	}
	b.current = started
	return started, nil
}
