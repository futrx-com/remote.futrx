package service

import (
	"context"
	"errors"
	"slices"
	"testing"

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
)

type cleanupChatRepository struct {
	servicechat.Repository
	chats      []servicechat.Meta
	listErr    error
	deleteErr  map[servicechat.ID]error
	deleted    []servicechat.ID
	operations *[]string
}

func (repository *cleanupChatRepository) List(context.Context) ([]servicechat.Meta, error) {
	return repository.chats, repository.listErr
}

func (repository *cleanupChatRepository) Delete(_ context.Context, id servicechat.ID) error {
	if err := repository.deleteErr[id]; err != nil {
		return err
	}
	if repository.operations != nil {
		*repository.operations = append(*repository.operations, "delete:"+string(id))
	}
	repository.deleted = append(repository.deleted, id)
	return nil
}

func TestProjectChatCleanupCancelsThenDeletesOnlyMatchingChats(t *testing.T) {
	var operations []string
	repository := &cleanupChatRepository{chats: []servicechat.Meta{
		{ID: "aaaaaaaa", ProjectID: "deadbeef"},
		{ID: "bbbbbbbb", ProjectID: "feedface"},
		{ID: "cccccccc", ProjectID: "deadbeef"},
	}, operations: &operations}
	var cancelled []servicechat.ID
	cleanup := projectChatCleanup{
		chats: repository,
		cancel: func(_ context.Context, id servicechat.ID) error {
			cancelled = append(cancelled, id)
			operations = append(operations, "cancel:"+string(id))
			return nil
		},
	}

	if err := cleanup.DeleteProjectChats(context.Background(), serviceproject.ID("deadbeef")); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(repository.deleted, []servicechat.ID{"aaaaaaaa", "cccccccc"}) {
		t.Fatalf("deleted chats = %v", repository.deleted)
	}
	if !slices.Equal(cancelled, []servicechat.ID{"aaaaaaaa", "cccccccc"}) {
		t.Fatalf("cancelled chats = %v", cancelled)
	}
	wantOperations := []string{
		"cancel:aaaaaaaa",
		"delete:aaaaaaaa",
		"cancel:cccccccc",
		"delete:cccccccc",
	}
	if !slices.Equal(operations, wantOperations) {
		t.Fatalf("operations = %v, want %v", operations, wantOperations)
	}
}

func TestProjectChatCleanupReportsFailuresAndContinues(t *testing.T) {
	wantDeleteErr := errors.New("delete failed")
	wantCancelErr := errors.New("cancel failed")
	repository := &cleanupChatRepository{
		chats: []servicechat.Meta{
			{ID: "aaaaaaaa", ProjectID: "deadbeef"},
			{ID: "bbbbbbbb", ProjectID: "deadbeef"},
			{ID: "cccccccc", ProjectID: "deadbeef"},
		},
		deleteErr: map[servicechat.ID]error{"aaaaaaaa": wantDeleteErr},
	}
	cleanup := projectChatCleanup{
		chats: repository,
		cancel: func(_ context.Context, id servicechat.ID) error {
			if id == "bbbbbbbb" {
				return wantCancelErr
			}
			return nil
		},
	}

	err := cleanup.DeleteProjectChats(context.Background(), "deadbeef")
	if !errors.Is(err, wantDeleteErr) || !errors.Is(err, wantCancelErr) {
		t.Fatalf("cleanup error = %v, want both failures", err)
	}
	if !slices.Equal(repository.deleted, []servicechat.ID{"cccccccc"}) {
		t.Fatalf("successful deletions = %v, want cleanup to continue", repository.deleted)
	}
}

func TestProjectChatCleanupDoesNotDeleteWithoutCancellation(t *testing.T) {
	repository := &cleanupChatRepository{chats: []servicechat.Meta{
		{ID: "aaaaaaaa", ProjectID: "deadbeef"},
	}}
	cleanup := projectChatCleanup{chats: repository}

	err := cleanup.DeleteProjectChats(context.Background(), "deadbeef")
	if err == nil {
		t.Fatal("cleanup succeeded without a cancellation dependency")
	}
	if len(repository.deleted) != 0 {
		t.Fatalf("deleted chats = %v, want none", repository.deleted)
	}
}

func TestProjectChatCleanupStopsWhenChatsCannotBeListed(t *testing.T) {
	wantErr := errors.New("read failed")
	cleanup := projectChatCleanup{chats: &cleanupChatRepository{listErr: wantErr}}

	err := cleanup.DeleteProjectChats(context.Background(), "deadbeef")
	if !errors.Is(err, wantErr) {
		t.Fatalf("cleanup error = %v, want %v", err, wantErr)
	}
}
