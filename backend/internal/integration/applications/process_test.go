package applications

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	applicationapi "github.com/futrx-com/remote.futrx.com/pkg/applications"
	applicationrpc "github.com/futrx-com/remote.futrx.com/pkg/applications/rpc"
)

func TestBlockedRequestWriteRetiresProcessWithoutExtendingDeadline(t *testing.T) {
	client := &blockingKillProcessClient{
		killStarted:  make(chan struct{}),
		releaseKill:  make(chan struct{}),
		killFinished: make(chan struct{}),
		forceKilled:  make(chan struct{}),
	}
	process := &backendProcess{
		client:  client,
		backend: blockedWriteBackend{},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result := make(chan error, 1)
	go func() {
		_, err := process.call(ctx, applicationapi.Request{})
		result <- err
	}()

	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("call error = %v, want context canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("call waited for process shutdown after its context ended")
	}
	select {
	case <-client.forceKilled:
	case <-time.After(time.Second):
		t.Fatal("blocked request write did not force-kill the child")
	}
	if process.running() {
		t.Fatal("process remained eligible for reuse while shutdown was blocked")
	}

	stopped := make(chan struct{})
	go func() {
		process.stop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("subsequent lifecycle stop remained blocked after force-kill")
	}
}

type blockedWriteBackend struct{}

func (blockedWriteBackend) Describe() (applicationapi.Descriptor, error) {
	return applicationapi.Descriptor{}, nil
}

func (blockedWriteBackend) Init(applicationapi.Instance) error { return nil }

func (blockedWriteBackend) Handle(applicationapi.Request) (applicationapi.Response, error) {
	return applicationapi.Response{}, nil
}

func (blockedWriteBackend) HandleContext(
	ctx context.Context,
	_ applicationapi.Request,
) (applicationapi.Response, error) {
	return applicationapi.Response{}, fmt.Errorf(
		"handle: %w: %w", applicationrpc.ErrRequestWriteBlocked, ctx.Err())
}

type blockingKillProcessClient struct {
	killStarted  chan struct{}
	releaseKill  chan struct{}
	killFinished chan struct{}
	forceKilled  chan struct{}
	startOnce    sync.Once
	finishOnce   sync.Once
	forceOnce    sync.Once
	exited       atomic.Bool
}

func (c *blockingKillProcessClient) Exited() bool { return c.exited.Load() }

func (c *blockingKillProcessClient) Kill() {
	c.startOnce.Do(func() {
		close(c.killStarted)
	})
	<-c.releaseKill
	c.exited.Store(true)
	c.finishOnce.Do(func() { close(c.killFinished) })
}

func (c *blockingKillProcessClient) ForceKill() {
	c.forceOnce.Do(func() {
		c.exited.Store(true)
		close(c.forceKilled)
		close(c.releaseKill)
	})
}
