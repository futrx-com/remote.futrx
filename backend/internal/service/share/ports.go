package share

import (
	"context"

	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
)

// Repository is the storage port for per-project share links. Implementations
// persist atomically and serialize concurrent callers per project.
type Repository interface {
	List(ctx context.Context, projectID serviceproject.ID) ([]Share, error)
	// Update hands the project's stored links to fn and persists whatever fn
	// returns. fn runs while the project's records are locked, so callers can
	// read-modify-write without racing another request.
	Update(
		ctx context.Context,
		projectID serviceproject.ID,
		fn func([]Share) ([]Share, error),
	) ([]Share, error)
}

// Projects is the lookup the share service needs into the project directory:
// creation resolves an ID to its slug, and edge authorization resolves a
// preview hostname's slug back to the project holding the links.
type Projects interface {
	Get(ctx context.Context, id serviceproject.ID) (serviceproject.Meta, error)
	GetBySlug(ctx context.Context, slug string) (serviceproject.Meta, error)
}
