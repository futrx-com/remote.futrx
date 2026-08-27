package service

import (
	"context"
	"errors"
	"fmt"

	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
)

type removedUserProjectAccess interface {
	List(ctx context.Context) ([]serviceproject.Meta, error)
	RemoveAccess(ctx context.Context, projectID serviceproject.ID, email string) error
}

type removedUserSubscriptions interface {
	DeleteAll(ctx context.Context, email string) error
}

// userRemovalCleanup removes identity-keyed authorization and delivery state
// before the user record disappears. Each operation is idempotent, allowing a
// failed removal to be retried without restoring already-revoked access.
type userRemovalCleanup struct {
	projects      removedUserProjectAccess
	subscriptions removedUserSubscriptions
}

func (c userRemovalCleanup) CleanupRemovedUser(ctx context.Context, email string) error {
	var cleanupErrors []error
	if c.projects != nil {
		projects, err := c.projects.List(ctx)
		if err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("list projects for user cleanup: %w", err))
		} else {
			for _, project := range projects {
				if err := c.projects.RemoveAccess(ctx, project.ID, email); err != nil {
					cleanupErrors = append(cleanupErrors, fmt.Errorf("remove access to project %s: %w", project.ID, err))
				}
			}
		}
	}
	if c.subscriptions != nil {
		if err := c.subscriptions.DeleteAll(ctx, email); err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("remove push subscriptions: %w", err))
		}
	}
	return errors.Join(cleanupErrors...)
}
