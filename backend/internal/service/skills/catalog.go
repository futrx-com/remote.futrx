package skills

import (
	"context"
	"errors"
	"strings"

	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
)

var (
	ErrProjectLookupUnavailable = errors.New("project lookup unavailable")
	ErrProjectNotFound          = errors.New("project not found")
	ErrAuthenticationRequired   = errors.New("authentication required")
	ErrProjectAccessDenied      = errors.New("project access denied")
)

type ProjectCatalog interface {
	Get(ctx context.Context, id serviceproject.ID) (serviceproject.Meta, error)
	HasAccess(ctx context.Context, id serviceproject.ID, email string) (bool, error)
}

type Authorizer interface {
	CurrentSession(cookieValue string) (*serviceauth.Session, error)
	IsAdmin(ctx context.Context, email string) (bool, error)
}

type ListQuery struct {
	Provider      Provider
	ProjectID     serviceproject.ID
	SessionCookie string
}

type Catalog struct {
	skills   *Service
	projects ProjectCatalog
	auth     Authorizer
	global   *GlobalService
}

func NewCatalog(skills *Service, projects ProjectCatalog, auth Authorizer) *Catalog {
	return &Catalog{skills: skills, projects: projects, auth: auth}
}

// WithGlobalLibrary merges the platform-wide skill library into every project
// listing. Loose chats have no container to link global skills into, so they
// keep seeing host and built-in skills only.
func (c *Catalog) WithGlobalLibrary(global *GlobalService) *Catalog {
	if c == nil {
		return nil
	}
	c.global = global
	return c
}

func (c *Catalog) List(ctx context.Context, query ListQuery) ([]Skill, error) {
	provider := query.Provider
	if provider == "" {
		provider = ProviderCodex
	}

	workspacePath := ""
	if query.ProjectID != "" {
		if c.projects == nil || c.auth == nil {
			return nil, ErrProjectLookupUnavailable
		}
		project, err := c.projects.Get(ctx, query.ProjectID)
		if err != nil {
			if errors.Is(err, serviceproject.ErrNotFound) {
				return nil, ErrProjectNotFound
			}
			return nil, err
		}
		session, err := c.auth.CurrentSession(query.SessionCookie)
		if err != nil || session == nil {
			return nil, ErrAuthenticationRequired
		}
		email := strings.ToLower(strings.TrimSpace(session.Email))
		if email == "" {
			return nil, ErrAuthenticationRequired
		}
		isAdmin, _ := c.auth.IsAdmin(ctx, email)
		if !isAdmin {
			hasAccess, _ := c.projects.HasAccess(ctx, project.ID, email)
			if !hasAccess {
				return nil, ErrProjectAccessDenied
			}
		}
		workspacePath = project.Cwd
	}

	listed, err := c.skills.List(ctx, provider, workspacePath)
	if err != nil {
		return nil, err
	}
	if query.ProjectID == "" || c.global == nil {
		return listed, nil
	}
	if global := c.global.CatalogEntries(ctx, provider, listed); len(global) > 0 {
		listed = append(listed, global...)
		SortSkills(listed)
	}
	return listed, nil
}
