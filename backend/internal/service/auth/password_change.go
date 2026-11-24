package auth

import (
	"context"
	"errors"
	"time"
)

var (
	ErrPasswordSignInUnavailable         = errors.New("this account does not sign in with a password")
	ErrCurrentPasswordIncorrect          = errors.New("current password is incorrect")
	ErrTwoFactorCodeRequired             = errors.New("two-factor code required")
	ErrPasswordReuse                     = errors.New("new password must differ from the current password")
	ErrPasswordNotChanged                = errors.New("password not changed")
	ErrPasswordNotChangedSessionsRevoked = errors.New("password not changed; other sessions were signed out")
	ErrPasswordChangedSessionFailed      = errors.New("password changed; sign in again with your new password")
)

// PasswordChange carries the current password, replacement and optional 2FA
// proof only for the lifetime of a password-change request.
type PasswordChange struct {
	Current string
	New     string
	Code    string
}

// ChangePassword proves account ownership, invalidates older sessions, then
// replaces the credential and issues a new session for the current browser.
func (s *Service) ChangePassword(ctx context.Context, user User, change PasswordChange, ip, userAgent string) (string, error) {
	// Hold one lock across the complete workflow: a losing concurrent request
	// must verify against the winning credential before it can revoke anything.
	s.local.claimMu.Lock()
	defer s.local.claimMu.Unlock()

	expected, err := s.local.verifyPassword(user.Email, change.Current)
	if err != nil {
		return "", err
	}
	record, err := s.twoFactor.load(ctx, expected.Email)
	if err != nil {
		return "", ErrPasswordNotChanged
	}
	if record != nil {
		if change.Code == "" {
			return "", ErrTwoFactorCodeRequired
		}
		if _, err := s.twoFactor.VerifyChallenge(ctx, expected.Email, change.Code); err != nil {
			if errors.Is(err, ErrInvalidTwoFactorCode) || errors.Is(err, ErrTwoFactorCodeReused) {
				return "", err
			}
			return "", ErrPasswordNotChanged
		}
	}
	if change.New == change.Current {
		return "", ErrPasswordReuse
	}
	hash, err := HashPassword(change.New)
	if err != nil {
		if errors.Is(err, ErrPasswordTooShort) || errors.Is(err, ErrPasswordTooLong) {
			return "", err
		}
		return "", ErrPasswordNotChanged
	}
	if err := s.registry.RevokeSessionsBefore(ctx, expected.Email, time.Now()); err != nil {
		return "", ErrPasswordNotChanged
	}
	if err := s.local.replacePassword(ctx, expected, hash); err != nil {
		if errors.Is(err, ErrLocalAdminCredentialChanged) {
			return "", ErrLocalAdminCredentialChanged
		}
		return "", ErrPasswordNotChangedSessionsRevoked
	}
	// Use the credential's canonical account identity, rather than propagating
	// arbitrary caller metadata into the replacement password session.
	cookie, err := s.IssueSession(ctx, localAdminUser(expected.Email), SignInMethodPassword, ip, userAgent)
	if err != nil {
		return "", ErrPasswordChangedSessionFailed
	}
	return cookie, nil
}
