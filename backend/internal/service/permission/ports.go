package permission

import "context"

// Repository persists the policy. Mutate is the only write path: it hands the
// change function a snapshot while holding the store's exclusive lock, so any
// authorization the function performs cannot go stale before the write lands.
//
// Returning no audit events means "nothing changed": the store must not write
// the returned state or append to the audit log. Otherwise the store records
// the events durably before publishing the new state, and fails the mutation
// if it cannot; an unaudited authorization change is worse than a rejected one.
type Repository interface {
	Load(ctx context.Context) (State, error)
	Mutate(ctx context.Context, change func(State) (State, []AuditEvent, error)) error
}

// IdentityDirectory answers questions about existing users. The permission
// layer references users by email and owns no user model.
type IdentityDirectory interface {
	// IsAdmin reports whether email is a current administrator, the root
	// policy that mutable assignments cannot remove.
	IsAdmin(ctx context.Context, email string) (bool, error)
	IsRegistered(ctx context.Context, email string) (bool, error)
}

// ProjectMembership answers project-visibility questions for the
// project-member compatibility baseline.
type ProjectMembership interface {
	HasAccess(ctx context.Context, projectID string, email string) (bool, error)
}
