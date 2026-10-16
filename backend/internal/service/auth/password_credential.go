package auth

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ErrPasswordChangeRequired is returned for a correct temporary password:
// the account may not hold a session until its owner has chosen a new one.
var ErrPasswordChangeRequired = errors.New("password change required")

// ErrTemporaryPasswordLocalAdmin refuses to give the local administrator an
// invited credential, which could never be used and would only confuse
// whoever reads the credentials file.
var ErrTemporaryPasswordLocalAdmin = errors.New("the local administrator cannot be given a temporary password")

type passwordCredentialKind int

const (
	// passwordCredentialNone is a miss: hash is a real one to verify against
	// anyway, so a miss costs the same as a hit.
	passwordCredentialNone passwordCredentialKind = iota
	passwordCredentialLocalAdmin
	passwordCredentialInvited
)

// passwordCredential is the outcome of resolving which credential, if any,
// owns an email for password sign-in.
type passwordCredential struct {
	kind    passwordCredentialKind
	email   string
	hash    string
	account AccountCredential // invited only
}

func (c passwordCredential) mustChange() bool {
	return c.kind == passwordCredentialInvited && c.account.MustChangePassword
}

func (c passwordCredential) user() User {
	if c.kind == passwordCredentialLocalAdmin {
		return localAdminUser(c.email)
	}
	return invitedUser(c.email)
}

func invitedUser(email string) User {
	return User{Email: normalizeEmail(email), Sub: subjectPassword}
}

// resolvePasswordCredential is the one place that decides whose password an
// email is checked against: the local administrator's, else an invited
// account's if the email is still registered, else nothing. An invited
// credential outliving its user (a reset racing a removal) is ignored, so it
// can never sign anyone in, including after a later Google re-invite. A
// directory failure is returned together with a verifiable miss, so the
// caller still performs exactly one password verification.
func (s *Service) resolvePasswordCredential(ctx context.Context, email string) (passwordCredential, error) {
	local, hash := s.local.resolveCredential(email)
	if local != nil {
		return passwordCredential{kind: passwordCredentialLocalAdmin, email: local.Email, hash: hash}, nil
	}
	miss := passwordCredential{kind: passwordCredentialNone, hash: hash}
	account, ok := s.accounts.credential(email)
	if !ok || s.users == nil {
		return miss, nil
	}
	registered, err := s.users.IsRegistered(ctx, email)
	if err != nil {
		return miss, err
	}
	if !registered {
		return miss, nil
	}
	return passwordCredential{
		kind:    passwordCredentialInvited,
		email:   normalizeEmail(email),
		hash:    account.PasswordHash,
		account: account,
	}, nil
}

// authenticatePassword resolves email's credential and verifies password
// against it with exactly one password-hash verification on every path.
func (s *Service) authenticatePassword(ctx context.Context, email, password string) (passwordCredential, error) {
	credential, resolveErr := s.resolvePasswordCredential(ctx, email)
	ok, verifyErr := s.verifyPassword(credential.hash, password)
	if resolveErr != nil {
		return passwordCredential{}, errors.Join(ErrInvalidCredentials, resolveErr)
	}
	if verifyErr != nil || !ok || credential.kind == passwordCredentialNone {
		return passwordCredential{}, ErrInvalidCredentials
	}
	return credential, nil
}

// IssueTemporaryPassword gives email a new temporary password and returns its
// plaintext, which exists nowhere else once the caller drops it. It replaces
// any earlier password, so a reset also invalidates the old temporary one,
// and signs out every session issued before now. Revocation comes first so
// that a failure never leaves a new credential next to live old sessions.
func (s *Service) IssueTemporaryPassword(ctx context.Context, email string) (string, error) {
	email = normalizeEmail(email)
	if email == "" {
		return "", errors.New("email is required")
	}
	if s.local.isLocalAdmin(email) {
		return "", ErrTemporaryPasswordLocalAdmin
	}
	password, err := s.temporaryPassword()
	if err != nil {
		return "", err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return "", fmt.Errorf("hash temporary password: %w", err)
	}

	defer s.accounts.lock(email)()
	if err := s.registry.RevokeSessionsBefore(ctx, email, time.Now()); err != nil {
		return "", fmt.Errorf("revoke sessions: %w", err)
	}
	if err := s.accounts.issueLocked(ctx, email, hash); err != nil {
		return "", err
	}
	return password, nil
}

// PasswordStates reports, by normalized email, every invited account that
// holds a password and whether it still has to be changed. It is read from
// memory; the caller joins it with the user list.
func (s *Service) PasswordStates(context.Context) map[string]PasswordState {
	return s.accounts.states()
}
