package chat

import (
	"context"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

// AccountAccess is implemented by the existing-policy adapter. No account ACL
// or credential material is stored in the chat service.
type AccountAccess interface {
	Resolve(context.Context, agent.ProviderID, string) (string, error)
}

func WithAccountAccess(access AccountAccess) Option {
	return func(s *Service) { s.accountAccess = access }
}
func (s *Service) requireAccount(ctx context.Context, provider Provider, id string) error {
	if s.accountAccess == nil {
		return nil
	}
	_, err := s.accountAccess.Resolve(ctx, agent.ProviderID(provider), id)
	return err
}
