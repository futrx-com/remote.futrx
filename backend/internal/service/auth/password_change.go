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

// passwordOwner is the credential a password change targets, verified and
// locked: the local administrator's, or an invited account's.
type passwordOwner struct {
	email string
	// user is the canonical identity of the replacement session, rather than
	// arbitrary caller metadata.
	user    User
	replace func(ctx context.Context, hash string) error
}

// lockPasswordOwner locks and verifies the credential owning user's email and
// returns it with the func that releases the lock. Hold one lock across the
// complete workflow: a losing concurrent request must verify against the
// winning credential before it can revoke anything.
func (s *Service) lockPasswordOwner(ctx context.Context, user User, current string) (passwordOwner, func(), error) {
	if s.local.isLocalAdmin(user.Email) {
		s.local.claimMu.Lock()
		expected, err := s.local.verifyPassword(user.Email, current)
		if err != nil {
			s.local.claimMu.Unlock()
			return passwordOwner{}, nil, err
		}
		return passwordOwner{
			email: expected.Email,
			user:  localAdminUser(expected.Email),
			replace: func(ctx context.Context, hash string) error {
				return s.local.replacePassword(ctx, expected, hash)
			},
		}, s.local.claimMu.Unlock, nil
	}

	unlock := s.accounts.lock(user.Email)
	credential, err := s.resolvePasswordCredential(ctx, user.Email)
	if err != nil {
		unlock()
		return passwordOwner{}, nil, ErrPasswordNotChanged
	}
	if credential.kind != passwordCredentialInvited {
		unlock()
		return passwordOwner{}, nil, ErrPasswordSignInUnavailable
	}
	if ok, err := s.verifyPassword(credential.hash, current); err != nil || !ok {
		unlock()
		return passwordOwner{}, nil, ErrCurrentPasswordIncorrect
	}
	return passwordOwner{
		email: credential.email,
		user:  credential.user(),
		replace: func(ctx context.Context, hash string) error {
			return s.accounts.replaceLocked(ctx, credential.email, credential.account.Version, hash)
		},
	}, unlock, nil
}

// ChangePassword proves account ownership, invalidates older sessions, then
// replaces the credential and issues a new session for the current browser.
func (s *Service) ChangePassword(ctx context.Context, user User, change PasswordChange, ip, userAgent string) (string, error) {
	owner, unlock, err := s.lockPasswordOwner(ctx, user, change.Current)
	if err != nil {
		return "", err
	}
	defer unlock()

	record, err := s.twoFactor.load(ctx, owner.email)
	if err != nil {
		return "", ErrPasswordNotChanged
	}
	if record != nil {
		if change.Code == "" {
			return "", ErrTwoFactorCodeRequired
		}
		if _, err := s.twoFactor.VerifyChallenge(ctx, owner.email, change.Code); err != nil {
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
	if err := s.registry.RevokeSessionsBefore(ctx, owner.email, time.Now()); err != nil {
		return "", ErrPasswordNotChanged
	}
	if err := owner.replace(ctx, hash); err != nil {
		if errors.Is(err, ErrLocalAdminCredentialChanged) || errors.Is(err, ErrAccountCredentialChanged) {
			return "", err
		}
		return "", ErrPasswordNotChangedSessionsRevoked
	}
	cookie, err := s.IssueSession(ctx, owner.user, SignInMethodPassword, ip, userAgent)
	if err != nil {
		return "", ErrPasswordChangedSessionFailed
	}
	return cookie, nil
}
