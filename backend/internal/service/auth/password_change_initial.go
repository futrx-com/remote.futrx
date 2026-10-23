package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"
)

// ErrInvalidPasswordChange reports that the first-login password-change token
// is missing, forged, expired, or was superseded by a reset or a change, so
// the owner must sign in again.
var ErrInvalidPasswordChange = errors.New("invalid or expired password change")

// passwordChangeTTL is how long a correct temporary password may wait for the
// new one. The change cookie lives exactly as long.
const passwordChangeTTL = 15 * time.Minute

// pendingPasswordChange is the signed, stateless payload carried between a
// sign-in that proved a temporary password (by password or by Google) and
// CompleteInitialPasswordChange. Version makes it single-use: any set of the
// credential bumps it. Fingerprint additionally binds it to the exact stored
// hash, because removing an account deletes its credential and with it the
// version counter, so a re-invite restarts at version 1 and would otherwise
// accept a token minted for the previous incarnation.
type pendingPasswordChange struct {
	Email       string       `json:"email"`
	Sub         string       `json:"sub"`
	Method      SignInMethod `json:"method"`
	Version     int64        `json:"version"`
	Fingerprint string       `json:"fp"`
	Exp         int64        `json:"exp"`
}

func (p pendingPasswordChange) expired(now time.Time) bool {
	return now.Unix() > p.Exp
}

// passwordHashFingerprint identifies a stored hash without revealing it. The
// hash embeds a random salt, so two issues never share a fingerprint.
func passwordHashFingerprint(hash string) string {
	sum := sha256.Sum256([]byte(hash))
	return hex.EncodeToString(sum[:16])
}

// PasswordChangeDuration is how long a password-change token stays valid.
func (s *Service) PasswordChangeDuration() time.Duration {
	return passwordChangeTTL
}

// passwordChangeLogin is the outcome of a sign-in that may not become a
// session yet: the token the browser must present to choose a new password.
func (s *Service) passwordChangeLogin(user User, method SignInMethod, account AccountCredential) LoginResult {
	token := s.passwordChangeCodec.sign(pendingPasswordChange{
		Email:       user.Email,
		Sub:         user.Sub,
		Method:      method,
		Version:     account.Version,
		Fingerprint: passwordHashFingerprint(account.PasswordHash),
		Exp:         time.Now().Add(passwordChangeTTL).Unix(),
	})
	return LoginResult{PasswordChangeRequired: true, PasswordChangeToken: token}
}

// CompleteInitialPasswordChange lets the owner of a temporary password choose
// a new one, then finishes the sign-in that led here with its original method,
// so an account with two-factor enabled still gets the second-factor step
// instead of a session.
//
// The account lock is held from verifying the token to saving the password: a
// reset or another change can neither slip in between nor be overwritten. The
// token stays valid after a rejected password (too short, the temporary one),
// so the owner can try again; it dies with the first change that is saved.
func (s *Service) CompleteInitialPasswordChange(ctx context.Context, token, newPassword, ip, userAgent string) (LoginResult, error) {
	pending, err := s.passwordChangeCodec.verify(token)
	if err != nil {
		return LoginResult{}, ErrInvalidPasswordChange
	}

	defer s.accounts.lock(pending.Email)()
	credential, err := s.resolvePasswordCredential(ctx, pending.Email)
	if err != nil {
		return LoginResult{}, ErrPasswordNotChanged
	}
	if credential.kind != passwordCredentialInvited || !credential.mustChange() ||
		credential.account.Version != pending.Version ||
		passwordHashFingerprint(credential.account.PasswordHash) != pending.Fingerprint {
		return LoginResult{}, ErrInvalidPasswordChange
	}

	same, err := s.verifyPassword(credential.hash, newPassword)
	if err != nil {
		return LoginResult{}, ErrPasswordNotChanged
	}
	if same {
		return LoginResult{}, ErrPasswordReuse
	}
	hash, err := HashPassword(newPassword)
	if err != nil {
		if errors.Is(err, ErrPasswordTooShort) || errors.Is(err, ErrPasswordTooLong) {
			return LoginResult{}, err
		}
		return LoginResult{}, ErrPasswordNotChanged
	}
	if err := s.registry.RevokeSessionsBefore(ctx, credential.email, time.Now()); err != nil {
		return LoginResult{}, ErrPasswordNotChanged
	}
	if err := s.accounts.replaceLocked(ctx, credential.email, pending.Version, hash); err != nil {
		if errors.Is(err, ErrAccountCredentialChanged) {
			return LoginResult{}, ErrInvalidPasswordChange
		}
		return LoginResult{}, ErrPasswordNotChangedSessionsRevoked
	}
	result, err := s.completeLogin(ctx, User{Email: pending.Email, Sub: pending.Sub}, pending.Method, ip, userAgent)
	if err != nil {
		return LoginResult{}, ErrPasswordChangedSessionFailed
	}
	return result, nil
}
