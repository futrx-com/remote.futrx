package rpc

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/rpc"
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

func TestCancellationRegistryCoalescesCompletionsBehindSlowRequest(t *testing.T) {
	var requests requestCancellations
	_, finishSlow := requests.begin(1)
	defer finishSlow()

	const completedRequests = 10_000
	for id := uint64(2); id <= completedRequests+1; id++ {
		_, finish := requests.begin(id)
		finish()
	}

	requests.mu.Lock()
	if got := len(requests.completed); got != 1 {
		requests.mu.Unlock()
		t.Fatalf("completed ranges = %d, want 1", got)
	}
	completed := requests.completed[0]
	requests.mu.Unlock()
	if completed != (requestIDRange{first: 2, last: completedRequests + 1}) {
		t.Fatalf("completed range = %+v, want [2,%d]", completed, completedRequests+1)
	}

	// A late cancel inside the coalesced range must still be ignored, while a
	// cancel for a request that has not begun must remain available to begin.
	requests.cancel(completedRequests, context.DeadlineExceeded)
	earlyID := uint64(completedRequests + 2)
	requests.cancel(earlyID, context.DeadlineExceeded)
	earlyContext, finishEarly := requests.begin(earlyID)
	defer finishEarly()
	if !errors.Is(context.Cause(earlyContext), context.DeadlineExceeded) {
		t.Fatalf("early cancellation cause = %v, want deadline exceeded", context.Cause(earlyContext))
	}

	requests.mu.Lock()
	defer requests.mu.Unlock()
	if len(requests.pending) != 0 {
		t.Fatalf("pending cancellations = %d, want 0", len(requests.pending))
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

func TestHandleContextDeadlineDoesNotWaitForBlockedRPCWrite(t *testing.T) {
	codec := newBlockingWriteClientCodec()
	client := rpc.NewClientWithCodec(codec)
	transport := &Client{client: client}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := transport.HandleContext(ctx, applications.Request{Path: "blocked-write"})
		result <- err
	}()

	select {
	case method := <-codec.writeStarted:
		if method != "Plugin.Handle" {
			t.Fatalf("first blocked write = %q, want Plugin.Handle", method)
		}
	case <-time.After(time.Second):
		t.Fatal("Handle RPC did not begin its transport write")
	}

	select {
	case err := <-result:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("HandleContext error = %v, want deadline exceeded", err)
		}
		if !errors.Is(err, ErrRequestWriteBlocked) {
			t.Fatalf("HandleContext error = %v, want blocked-write marker", err)
		}
	case <-time.After(time.Second):
		t.Fatal("HandleContext waited for a blocked transport write after its deadline")
	}

	if err := client.Close(); err != nil {
		t.Fatalf("close stalled client: %v", err)
	}
	select {
	case <-codec.writeFinished:
	case <-time.After(time.Second):
		t.Fatal("closing the stalled client did not release its blocked writer")
	}
}

type blockingWriteClientCodec struct {
	writeStarted  chan string
	writeFinished chan struct{}
	closed        chan struct{}
	closeOnce     sync.Once
	finishOnce    sync.Once
}

func newBlockingWriteClientCodec() *blockingWriteClientCodec {
	return &blockingWriteClientCodec{
		writeStarted:  make(chan string, 1),
		writeFinished: make(chan struct{}),
		closed:        make(chan struct{}),
	}
}

func (c *blockingWriteClientCodec) WriteRequest(request *rpc.Request, _ any) error {
	select {
	case c.writeStarted <- request.ServiceMethod:
	default:
	}
	<-c.closed
	c.finishOnce.Do(func() { close(c.writeFinished) })
	return io.ErrClosedPipe
}

func (c *blockingWriteClientCodec) ReadResponseHeader(*rpc.Response) error {
	<-c.closed
	return io.EOF
}

func (*blockingWriteClientCodec) ReadResponseBody(any) error { return nil }

func (c *blockingWriteClientCodec) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return nil
}
