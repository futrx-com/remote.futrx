package applications

import (
	"context"
	"errors"
	"testing"
)

func TestRequestCancellationDefaultsToBackground(t *testing.T) {
	request := Request{}
	if request.Done() != nil {
		t.Fatal("plain request has a cancellation channel")
	}
	if request.Err() != nil {
		t.Fatalf("plain request error = %v", request.Err())
	}
}

func TestRequestCancellationContextPreservesCause(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	request := (Request{}).WithCancellation(ctx)
	cancel(context.DeadlineExceeded)

	select {
	case <-request.Done():
	default:
		t.Fatal("request cancellation was not observable")
	}
	if !errors.Is(request.Err(), context.DeadlineExceeded) {
		t.Fatalf("request error = %v, want deadline exceeded", request.Err())
	}
	if !errors.Is(request.CancellationContext().Err(), context.Canceled) {
		t.Fatalf("context error = %v, want canceled", request.CancellationContext().Err())
	}
}
