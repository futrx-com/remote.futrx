package service

import (
	"context"
	"errors"
	"fmt"

	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
)

type removedUserProjectAccess interface {
	List(ctx context.Context) ([]serviceproject.Meta, error)
	RemoveAccess(ctx context.Context, projectID serviceproject.ID, email string) error
}

type removedUserSubscriptions interface {
	DeleteAll(ctx context.Context, email string) error
}

// removedUserSecurityState is per-account security state keyed by email, held
// in its own store and discarded wholesale with the account. Both the
// two-factor enrollment (TOTP secret and recovery-code hashes) and the session
// registry entry (active session id, sign-in history, pending recovery-code
// alert) expose exactly this shape.
type removedUserSecurityState interface {
	Delete(ctx context.Context, email string) error
}

// accountCredentialRemoval deletes a removed user's invited password
// credential through the auth service, which owns the in-memory copy a plain
// store delete would leave behind. The auth service is built after the user
// service that holds this cleanup, so it is bound afterwards; until then, and
// without auth, there is nothing to delete.
type accountCredentialRemoval struct {
	auth *serviceauth.Service
}

func (r *accountCredentialRemoval) Delete(ctx context.Context, email string) error {
	if r == nil || r.auth == nil {
		return nil
	}
	return r.auth.DeleteAccountCredential(ctx, email)
}

// userRemovalCleanup removes identity-keyed authorization, delivery, and
// security state before the user record disappears. Each operation is
// idempotent, allowing a failed removal to be retried without restoring
// already-revoked access.
type userRemovalCleanup struct {
	projects        removedUserProjectAccess
	subscriptions   removedUserSubscriptions
	twoFactor       removedUserSecurityState
	sessionRegistry removedUserSecurityState
	// credentials is the invited password credential. Removing it with the
	// user means a later re-invite starts without the old password.
	credentials removedUserSecurityState
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
	if c.twoFactor != nil {
		if err := c.twoFactor.Delete(ctx, email); err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("remove two-factor enrollment: %w", err))
		}
	}
	if c.sessionRegistry != nil {
		if err := c.sessionRegistry.Delete(ctx, email); err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("remove session registry entry: %w", err))
		}
	}
	if c.credentials != nil {
		if err := c.credentials.Delete(ctx, email); err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("remove account credential: %w", err))
		}
	}
	return errors.Join(cleanupErrors...)
}
