package applications

import (
	"context"
	"github.com/futrx-com/remote.futrx.com/internal/rbac"
	"github.com/futrx-com/remote.futrx.com/internal/service/audit"
	"github.com/futrx-com/remote.futrx.com/internal/service/workspaceaccess"
	"github.com/futrx-com/remote.futrx.com/internal/service/workspaceide"
)

func WithWorkspaceAuthorizer(a workspaceaccess.Authorizer) Option {
	return func(s *Service) { s.workspaceAuthorizer = a }
}

// RequireWebAccess guards current application launch, direct HTTP and WebSocket
// admission. Instance identity comes from the store, never a forwarded header.
func (s *Service) RequireWebAccess(ctx context.Context, instanceID string) (resultErr error) {
	instance, ok, err := s.store.Get(ctx, instanceID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}
	if s.audit != nil && (instance.ApplicationID == "code-server" || instance.ApplicationID == "terminal") {
		defer func() {
			entry := audit.Success("workspace.application.open", audit.Target{Type: audit.TargetProject, ID: instance.ProjectID}, audit.Meta{"applicationId": instance.ApplicationID, "instanceId": instance.ID})
			entry.OK = resultErr == nil
			if resultErr != nil {
				entry.Error = "workspace access denied"
			}
			if actor, ok := rbac.ActorFromContext(ctx); ok {
				entry.Actor = audit.Actor{Email: actor.Email}
			}
			s.audit.Record(ctx, entry)
		}()
	}
	switch instance.ApplicationID {
	case "code-server":
		if instance.Scope != ScopeProject {
			return rbac.ErrDenied
		}
		return workspaceide.RequireAccess(ctx, s.workspaceAuthorizer, instance.ProjectID)
	case "terminal":
		if instance.Scope != ScopeProject {
			return rbac.ErrDenied
		}
		return workspaceaccess.Require(ctx, s.workspaceAuthorizer, "terminal", instance.ProjectID)
	}
	return nil
}

func WithAudit(recorder audit.Recorder) Option { return func(s *Service) { s.audit = recorder } }
