package project

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type deleteTestRepository struct {
	Repository
	meta    Meta
	deleted bool
}

func (repository *deleteTestRepository) Get(context.Context, ID) (Meta, error) {
	return repository.meta, nil
}

func (repository *deleteTestRepository) Delete(context.Context, ID) error {
	repository.deleted = true
	return nil
}

type deleteTestChatCleanup struct {
	projectID ID
	err       error
}

func (cleanup *deleteTestChatCleanup) DeleteProjectChats(_ context.Context, projectID ID) error {
	cleanup.projectID = projectID
	return cleanup.err
}

func TestDeleteRemovesAssociatedChats(t *testing.T) {
	repository := &deleteTestRepository{meta: Meta{ID: "deadbeef"}}
	chats := &deleteTestChatCleanup{}
	service := New(
		repository,
		ContainerDependencies{},
		nil,
		nil,
		WithChatCleanup(chats),
	)

	if err := service.Delete(context.Background(), repository.meta.ID); err != nil {
		t.Fatal(err)
	}
	if chats.projectID != repository.meta.ID {
		t.Fatalf("chat cleanup project = %q, want %q", chats.projectID, repository.meta.ID)
	}
	if !repository.deleted {
		t.Fatal("project record was not deleted after chat cleanup")
	}
}

func TestDeletePreservesProjectWhenChatCleanupFails(t *testing.T) {
	repository := &deleteTestRepository{meta: Meta{ID: "deadbeef"}}
	wantErr := errors.New("disk full")
	service := New(
		repository,
		ContainerDependencies{},
		nil,
		nil,
		WithChatCleanup(&deleteTestChatCleanup{err: wantErr}),
	)

	err := service.Delete(context.Background(), repository.meta.ID)
	if !errors.Is(err, wantErr) || !strings.Contains(err.Error(), "delete project chats") {
		t.Fatalf("Delete() error = %v, want wrapped chat cleanup error", err)
	}
	if repository.deleted {
		t.Fatal("project record was deleted after chat cleanup failed")
	}
}
