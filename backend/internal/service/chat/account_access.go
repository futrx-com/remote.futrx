package chat

import (
	"context"

	"github.com/futrx-com/remote.futrx.com/internal/harness"
)

// AccountAccess is implemented by the existing-policy adapter. No account ACL
// or credential material is stored in the chat service.
type AccountAccess interface {
	Resolve(context.Context, harness.ProviderID, string) (string, error)
}

func WithAccountAccess(access AccountAccess) Option {
	return func(s *Service) { s.accountAccess = access }
}
func (s *Service) requireAccount(ctx context.Context, provider Provider, id string) error {
	if s.accountAccess == nil {
		return nil
	}
	_, err := s.accountAccess.Resolve(ctx, harness.ProviderID(provider), id)
	return err
}
