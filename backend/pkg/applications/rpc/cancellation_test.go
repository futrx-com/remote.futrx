package rpc

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/futrx-com/remote.futrx.com/pkg/applications"
)

type cancellationObservation struct {
	path string
	err  error
}

type cancellationBackend struct {
	started  chan string
	observed chan cancellationObservation
	release  chan struct{}
	once     sync.Once
}

func TestCancellationWireUsesProtocolFive(t *testing.T) {
	if Handshake.ProtocolVersion != 5 {
		t.Fatalf("handshake protocol = %d, want 5", Handshake.ProtocolVersion)
	}
}

func (*cancellationBackend) Describe() (applications.Descriptor, error) {
	return applications.Descriptor{APIVersion: applications.APIVersion}, nil
}

func (*cancellationBackend) Init(applications.Instance) error { return nil }

func (b *cancellationBackend) Handle(request applications.Request) (applications.Response, error) {
	b.started <- request.Path
	switch request.Path {
	case "cancel":
		<-request.Done()
		b.observed <- cancellationObservation{path: request.Path, err: request.Err()}
		return applications.Text(http.StatusRequestTimeout, request.Err().Error()), nil
	case "other":
		select {
		case <-request.Done():
			b.observed <- cancellationObservation{path: request.Path, err: request.Err()}
			return applications.Response{}, request.Err()
		case <-b.release:
			return applications.Text(http.StatusOK, "released"), nil
		}
	default:
		return applications.Text(http.StatusOK, "ok"), nil
	}
}

func TestCancelBeforeHandleRegistersIsApplied(t *testing.T) {
	backend := &cancellationBackend{
		started: make(chan string, 1), observed: make(chan cancellationObservation, 1), release: make(chan struct{}),
	}
	transport := &server{impl: backend}
	var cancelReply CancelReply
	if err := transport.Cancel(CancelArgs{RequestID: 7, DeadlineExceeded: true}, &cancelReply); err != nil {
		t.Fatal(err)
	}
	if !cancelReply.Acknowledged {
		t.Fatal("early cancellation was not acknowledged")
	}

	done := make(chan HandleReply, 1)
	go func() {
		var reply HandleReply
		_ = transport.Handle(HandleArgs{RequestID: 7, Request: applications.Request{Path: "cancel"}}, &reply)
		done <- reply
	}()

	select {
	case observation := <-backend.observed:
		if !errors.Is(observation.err, context.DeadlineExceeded) {
			t.Fatalf("backend cancellation = %v, want deadline exceeded", observation.err)
		}
	case <-time.After(time.Second):
		t.Fatal("Handle did not observe cancellation registered before it")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("canceled Handle did not return")
	}
	transport.requests.mu.Lock()
	defer transport.requests.mu.Unlock()
	if len(transport.requests.active) != 0 || len(transport.requests.pending) != 0 {
		t.Fatalf("request cancellation state leaked: active=%d pending=%d", len(transport.requests.active), len(transport.requests.pending))
	}
}

func TestRequestCancellationRegistryIsolatesConcurrentCalls(t *testing.T) {
	var requests requestCancellations
	first, finishFirst := requests.begin(1)
	second, finishSecond := requests.begin(2)
	defer finishFirst()
	defer finishSecond()

	requests.cancel(1, context.Canceled)
	select {
	case <-first.Done():
	default:
		t.Fatal("selected request was not canceled")
	}
	select {
	case <-second.Done():
		t.Fatal("canceling one request canceled another in-flight request")
	default:
	}
}

func TestCancelAfterHandleFinishesIsNotRetained(t *testing.T) {
	var requests requestCancellations
	requestContext, finish := requests.begin(1)
	finish()
	if !errors.Is(requestContext.Err(), context.Canceled) {
		t.Fatalf("finished request context error = %v", requestContext.Err())
	}

	requests.cancel(1, context.DeadlineExceeded)
	requests.mu.Lock()
	defer requests.mu.Unlock()
	if len(requests.pending) != 0 || len(requests.active) != 0 {
		t.Fatalf("late cancellation was retained: active=%d pending=%d", len(requests.active), len(requests.pending))
	}
	if requests.completedThrough != 1 {
		t.Fatalf("completed watermark = %d, want 1", requests.completedThrough)
	}
}

func TestHandleContextCancelsOnlyItsOwnRPC(t *testing.T) {
	backend := &cancellationBackend{
		started: make(chan string, 2), observed: make(chan cancellationObservation, 2), release: make(chan struct{}),
	}
	client, closeClient := rpcClientFor(t, &server{impl: backend})
	defer closeClient()
	transport := &Client{client: client}

	cancelCtx, cancel := context.WithCancel(context.Background())
	canceled := make(chan error, 1)
	go func() {
		_, err := transport.HandleContext(cancelCtx, applications.Request{Path: "cancel"})
		canceled <- err
	}()
	other := make(chan struct {
		response applications.Response
		err      error
	}, 1)
	go func() {
		response, err := transport.HandleContext(context.Background(), applications.Request{Path: "other"})
		other <- struct {
			response applications.Response
			err      error
		}{response: response, err: err}
	}()

	started := map[string]bool{}
	for len(started) < 2 {
		select {
		case path := <-backend.started:
			started[path] = true
		case <-time.After(time.Second):
			t.Fatal("backend calls did not start")
		}
	}
	cancel()
	select {
	case err := <-canceled:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled call error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled RPC did not return promptly")
	}
	select {
	case observation := <-backend.observed:
		if observation.path != "cancel" || !errors.Is(observation.err, context.Canceled) {
			t.Fatalf("cancellation observation = %+v", observation)
		}
	case <-time.After(time.Second):
		t.Fatal("backend did not observe caller cancellation")
	}
	select {
	case result := <-other:
		t.Fatalf("other call ended with canceled request: response=%+v err=%v", result.response, result.err)
	default:
	}
	backend.once.Do(func() { close(backend.release) })
	select {
	case result := <-other:
		if result.err != nil || string(result.response.Body) != "released" {
			t.Fatalf("other call = response %+v, error %v", result.response, result.err)
		}
	case <-time.After(time.Second):
		t.Fatal("uncanceled RPC did not finish")
	}
}
