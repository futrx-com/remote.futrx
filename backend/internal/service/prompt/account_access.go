package prompt

import (
	"context"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	"github.com/futrx-com/remote.futrx.com/internal/rbac"
)

type AccountAccess interface {
	Resolve(context.Context, agent.ProviderID, string) (string, error)
}

func WithAccountAccess(access AccountAccess) Option {
	return func(s *Service) { s.accountAccess = access }
}

// Actor is supplied by the authenticated socket or the stored schedule owner.
// Never elevate scheduled runs to system or trust Actor.IsAdmin for RBAC.
func accountActorContext(ctx context.Context, input StartInput) context.Context {
	return rbac.ContextWithActor(ctx, rbac.UserActor(input.Actor.Email))
}
