package archives

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestFailedBuildReleasesItsSlot(t *testing.T) {
	downloads := NewDownloads(1, time.Second)
	failure := errors.New("build failed")
	if _, err := downloads.Build(func(context.Context) ([]byte, error) {
		return nil, failure
	}); !errors.Is(err, failure) {
		t.Fatalf("build failure = %v", err)
	}
	content, err := downloads.Build(func(ctx context.Context) ([]byte, error) {
		if _, ok := ctx.Deadline(); !ok {
			t.Error("the build has no deadline")
		}
		return []byte("zip"), nil
	})
	if err != nil || string(content) != "zip" {
		t.Fatalf("next build = %q, %v", content, err)
	}
}

func TestWaitingForASlotTimesOutWithoutStartingABuild(t *testing.T) {
	downloads := NewDownloads(1, 50*time.Millisecond)
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = downloads.Build(func(context.Context) ([]byte, error) {
			close(started)
			<-release
			return nil, nil
		})
	}()
	defer func() { close(release); <-done }()
	<-started
	_, err := downloads.Build(func(context.Context) ([]byte, error) {
		t.Error("a second build started with no available slot")
		return nil, nil
	})
	if !errors.Is(err, ErrBusy) {
		t.Fatalf("waiting for admission = %v", err)
	}
}
