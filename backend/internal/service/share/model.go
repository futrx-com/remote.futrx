// Package share owns public preview links: time-boxed, revocable grants that
// let someone without a platform account open one project's
// <slug>--<port>.dev.<host> preview. A share never widens access to the IDE,
// the agent browser, or the main application.
package share

// ID identifies one share link inside a project.
type ID string

// CreateInput is the application input for a new share link.
type CreateInput struct {
	Port     int
	TTLHours int
	Label    string
}

// Metadata is the management view of an active share link. It intentionally
// excludes the token digest and revocation state kept by the repository.
type Metadata struct {
	ID        ID
	Port      int
	Label     string
	CreatedBy string
	CreatedAt int64
	ExpiresAt int64
}

// AuthorizationGrant is the minimum information the edge needs after a
// plaintext token has been validated.
type AuthorizationGrant struct {
	ID        ID
	ExpiresAt int64
}

// Created carries the one and only copy of the plaintext token alongside the
// management metadata and the project slug the link belongs to.
type Created struct {
	Metadata Metadata
	Token    string
	Slug     string
}
