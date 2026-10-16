package auth

import (
	"context"
	"fmt"
)

// DeleteAccountCredential removes email's invited password credential, from
// the store first and then from memory so a failed delete never leaves the
// cache ahead of the disk. It is idempotent. Removing a user and re-inviting
// them as a Google user must go through this rather than the store: a
// credential left in memory would sign them in with the old password again.
func (s *Service) DeleteAccountCredential(ctx context.Context, email string) error {
	email = normalizeEmail(email)
	defer s.accounts.lock(email)()
	if err := s.accounts.store.Delete(ctx, email); err != nil {
		return fmt.Errorf("delete account credential: %w", err)
	}
	s.accounts.mu.Lock()
	delete(s.accounts.cache, email)
	s.accounts.mu.Unlock()
	return nil
}
