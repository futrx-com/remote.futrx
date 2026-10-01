package lifecycle

import (
	"context"
	"errors"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
	"testing"
)

type rejectedPersistentStorage struct{}

func (rejectedPersistentStorage) Ensure(context.Context, serviceproject.Meta) error {
	return errors.New("persistent quota unavailable")
}
func TestPersistentQuotaFailurePreventsContainerMutation(t *testing.T) {
	var events []string
	runtime := &recordingRuntime{events: &events}
	service := newTestService(runtime, &events).WithPersistentStorage(rejectedPersistentStorage{})
	if err := service.Ensure(context.Background(), testProject(t)); err == nil {
		t.Fatal("launched with failed persistent quota")
	}
	if len(events) != 1 || events[0] != "runtime available" {
		t.Fatalf("mutated container before quota: %v", events)
	}
}
