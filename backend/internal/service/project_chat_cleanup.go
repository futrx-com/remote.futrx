package service

import (
	"context"
	"errors"
	"fmt"

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
)

// projectChatCleanup adapts the chat service's persistence and run controls to
// the narrow cleanup contract owned by the project service.
type projectChatCleanup struct {
	chats  servicechat.Repository
	cancel func(context.Context, servicechat.ID) error
}

func (cleanup projectChatCleanup) DeleteProjectChats(
	ctx context.Context,
	projectID serviceproject.ID,
) error {
	chats, err := cleanup.chats.List(ctx)
	if err != nil {
		return fmt.Errorf("list chats: %w", err)
	}

	var failures []error
	for _, chat := range chats {
		if chat.ProjectID != servicechat.ProjectID(projectID) {
			continue
		}
		if cleanup.cancel == nil {
			failures = append(failures, fmt.Errorf("cancel chat %s: cancellation unavailable", chat.ID))
			continue
		}
		if err := cleanup.cancel(ctx, chat.ID); err != nil {
			failures = append(failures, fmt.Errorf("cancel chat %s: %w", chat.ID, err))
			continue
		}
		if err := cleanup.chats.Delete(ctx, chat.ID); err != nil {
			failures = append(failures, fmt.Errorf("delete chat %s: %w", chat.ID, err))
		}
	}
	return errors.Join(failures...)
}
