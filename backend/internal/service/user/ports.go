package user

import "context"

// Repository is the storage port for the users table. Implementations must
// persist atomically. Emails are stored case-normalized (lowercase) but
// callers may pass any case to lookups.
type Repository interface {
	List(ctx context.Context) ([]User, error)
	Get(ctx context.Context, email string) (*User, error)
	Add(ctx context.Context, u User) error
	Remove(ctx context.Context, email string) error
	SetRole(ctx context.Context, email string, role Role) error
	Count(ctx context.Context) (int, error)
}

// RemovalCleanup revokes resources that are keyed by a user's identity before
// the identity itself disappears. Implementations must be idempotent because
// a partially completed removal can be retried.
type RemovalCleanup interface {
	CleanupRemovedUser(ctx context.Context, email string) error
}
