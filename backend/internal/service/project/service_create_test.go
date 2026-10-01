package project

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestCreateRefusesBeforeRecordingWhenTheHostIsFull(t *testing.T) {
	full := fmt.Errorf("%w: 1.0 GiB free", ErrInsufficientStorage)
	repo := &startTestRepository{}
	lifecycle := &startTestLifecycle{state: ContainerStateMissing, capacityErr: full}
	service := New(repo, ContainerDependencies{Lifecycle: lifecycle}, nil, nil, WithAuthorizer(allowAllAuthorizer{}))

	_, err := service.Create(context.Background(), CreateInput{Name: "demo"}, "owner@example.com")
	if !errors.Is(err, ErrInsufficientStorage) {
		t.Fatalf("Create() error = %v, want ErrInsufficientStorage", err)
	}
	if repo.meta.Name != "" {
		t.Fatalf("repository meta = %#v, want nothing recorded", repo.meta)
	}
	if lifecycle.launchCalls != 0 {
		t.Fatalf("Ensure calls = %d, want none", lifecycle.launchCalls)
	}
}

func TestCreateValidatesTheNameBeforeCheckingCapacity(t *testing.T) {
	lifecycle := &startTestLifecycle{capacityErr: ErrInsufficientStorage}
	service := New(&startTestRepository{}, ContainerDependencies{Lifecycle: lifecycle}, nil, nil, WithAuthorizer(allowAllAuthorizer{}))

	if _, err := service.Create(context.Background(), CreateInput{Name: "  "}, ""); !errors.Is(err, ErrNameRequired) {
		t.Fatalf("Create() error = %v, want ErrNameRequired", err)
	}
}
