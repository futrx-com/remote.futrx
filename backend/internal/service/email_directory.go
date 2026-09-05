package service

import (
	"context"
	"errors"
	"fmt"

	serviceemail "github.com/futrx-com/remote.futrx.com/internal/service/email"
	serviceuser "github.com/futrx-com/remote.futrx.com/internal/service/user"
)

// emailDirectory wraps *user.Service to satisfy email.Directory, the same way
// emailSender wraps the SMTP client and userDirectoryAdapter wraps it for auth.
//
// Users are already keyed by email in this codebase, so resolving a key to an
// address is mostly a "may we mail this person" gate: the lookup fails for
// anyone who is not registered, which is where a per-user email opt-out would
// naturally go.
type emailDirectory struct {
	users *serviceuser.Service
}

var _ serviceemail.Directory = emailDirectory{}

// Address resolves userKey via the user directory. Only "this key names
// nobody we may mail" becomes ErrUnknownRecipient - a real lookup failure
// (a store error, a cancelled context) is returned as-is, so a caller can
// tell "the recipient does not exist" from "we could not check", which
// callers of Required mail (2FA, invitations) need to handle differently: an
// unknown recipient is a caller bug, an infrastructure failure is not.
func (d emailDirectory) Address(ctx context.Context, userKey string) (string, error) {
	if d.users == nil {
		return "", fmt.Errorf("%w: %q: no user directory", serviceemail.ErrUnknownRecipient, userKey)
	}
	user, err := d.users.Get(ctx, userKey)
	switch {
	case errors.Is(err, serviceuser.ErrUserNotFound), errors.Is(err, serviceuser.ErrInvalidEmail):
		return "", fmt.Errorf("%w: %q", serviceemail.ErrUnknownRecipient, userKey)
	case err != nil:
		return "", fmt.Errorf("resolve mail recipient %q: %w", userKey, err)
	case user == nil || user.Email == "":
		return "", fmt.Errorf("%w: %q", serviceemail.ErrUnknownRecipient, userKey)
	}
	return user.Email, nil
}
