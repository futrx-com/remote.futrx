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
	service := New(repo, ContainerDependencies{Lifecycle: lifecycle}, nil, nil)

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
	service := New(&startTestRepository{}, ContainerDependencies{Lifecycle: lifecycle}, nil, nil)

	if _, err := service.Create(context.Background(), CreateInput{Name: "  "}, ""); !errors.Is(err, ErrNameRequired) {
		t.Fatalf("Create() error = %v, want ErrNameRequired", err)
	}
}

func TestProjectNameRejectsReservedSeparator(t *testing.T) {
	for _, name := range []string{"game--head", "--start", "end--", "a---b"} {
		repo := &startTestRepository{meta: Meta{ID: "abcd", Name: "original"}}
		lifecycle := &startTestLifecycle{capacityErr: ErrInsufficientStorage}
		service := New(repo, ContainerDependencies{Lifecycle: lifecycle}, nil, nil)
		if _, err := service.Create(context.Background(), CreateInput{Name: name}, ""); !errors.Is(err, ErrReservedNameSeparator) {
			t.Fatalf("Create(%q): %v", name, err)
		}
		if _, err := service.Update(context.Background(), "abcd", UpdateInput{Name: &name}); !errors.Is(err, ErrReservedNameSeparator) {
			t.Fatalf("Update(%q): %v", name, err)
		}
		if repo.meta.Name != "original" || lifecycle.launchCalls != 0 {
			t.Fatal("invalid name caused a mutation")
		}
	}
}

func TestCreateInstallsGlobalApplicationsWithoutFailingOnTheirErrors(t *testing.T) {
	repo := &startTestRepository{}
	service := New(repo, ContainerDependencies{}, nil, nil)
	var installedIn []string
	service.SetApplicationInstaller(func(_ context.Context, projectID string) error {
		installedIn = append(installedIn, projectID)
		return errors.New("one application failed")
	})

	created, err := service.Create(context.Background(), CreateInput{Name: "demo"}, "owner@example.com")
	if err != nil {
		t.Fatalf("Create() error = %v, want the project despite the application failure", err)
	}
	if len(installedIn) != 1 || installedIn[0] != string(created.ID) {
		t.Fatalf("installed in %v, want only the new project %q", installedIn, created.ID)
	}
	if created.Status != StatusRunning {
		t.Fatalf("status = %s, want running", created.Status)
	}
}
