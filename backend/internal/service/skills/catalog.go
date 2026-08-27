package skills

import (
	"context"
	"errors"
	"strings"

	agentmodule "github.com/futrx-com/remote.futrx.com/internal/service/agent/module"
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
}

func NewCatalog(skills *Service, projects ProjectCatalog, auth Authorizer) *Catalog {
	return &Catalog{skills: skills, projects: projects, auth: auth}
}

func (c *Catalog) List(ctx context.Context, query ListQuery) ([]Skill, error) {
	scope := agentmodule.ScopeHost
	if query.ProjectID != "" {
		scope = agentmodule.ScopeProject
	}
	provider := query.Provider
	if provider == "" {
		provider = c.skills.DefaultProvider(scope)
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

	return c.skills.List(ctx, provider, workspacePath)
}
