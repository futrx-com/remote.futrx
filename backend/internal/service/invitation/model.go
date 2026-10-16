// Package invitation orchestrates creating, resetting and removing accounts
// that sign in with an emailed temporary password: the user directory, the
// credential, session revocation, mail and the compensation between them, all
// serialized per email address.
package invitation

import (
	"errors"

	serviceuser "github.com/futrx-com/remote.futrx.com/internal/service/user"
)

// Method is how an invited account signs in.
type Method string

const (
	MethodGoogle   Method = "google"
	MethodPassword Method = "password"
)

// Delivery is what happened to the credentials email.
type Delivery string

const (
	// DeliverySent: the SMTP server accepted the message.
	DeliverySent Delivery = "sent"
	// DeliveryFailed: an email was attempted and not delivered; the account and
	// its password exist, so the caller must show the password.
	DeliveryFailed Delivery = "failed"
	// DeliveryNotConfigured: the server has no SMTP configuration.
	DeliveryNotConfigured Delivery = "not_configured"
	// DeliveryNotRequested: the administrator chose not to email.
	DeliveryNotRequested Delivery = "not_requested"
)

// Fixed, safe descriptions of a failed delivery. The raw transport error is
// never shown: it can carry server banners, addresses and internal paths.
const (
	deliveryErrorRejected    = "The SMTP server rejected the message"
	deliveryErrorUnreachable = "Could not reach the SMTP server"
	deliveryErrorGeneric     = "The email could not be sent"
)

var (
	// ErrInvalidMethod reports a sign-in method that is neither google nor
	// password.
	ErrInvalidMethod = errors.New("method must be 'google' or 'password'")
	// ErrResetSelf refuses an administrator resetting their own account; they
	// change their password instead.
	ErrResetSelf = errors.New("you cannot reset your own password; use Change password")
	// ErrResetLocalAdmin refuses to reset the local administrator, whose
	// password is never issued by anyone else.
	ErrResetLocalAdmin = errors.New("the local administrator's password cannot be reset")
	// ErrAccountNotCreated reports a password invite that failed after the user
	// was added, and was rolled back.
	ErrAccountNotCreated = errors.New("account was not created")
	// ErrAccountNotRolledBack reports a failed invite whose rollback also
	// failed: the user exists without a usable credential and must be removed
	// by hand.
	ErrAccountNotRolledBack = errors.New("account was not created and could not be rolled back")
)

// InviteRequest asks for a new account. A zero Method means Google.
type InviteRequest struct {
	Email     string
	Role      serviceuser.Role
	Method    Method
	SendEmail bool
	// InvitedBy is the administrator's email, recorded as the user's AddedBy.
	InvitedBy string
}

// ResetRequest asks for a new temporary password for an existing user.
type ResetRequest struct {
	Email string
	// Actor is the administrator asking; resetting oneself is refused.
	Actor     string
	SendEmail bool
}

// Result is the outcome of an invite or a reset. TemporaryPassword is set only
// for the password method and must reach the caller's response and nowhere
// else. DeliveryError is a fixed safe description, set only when Delivery is
// DeliveryFailed.
type Result struct {
	User              serviceuser.User
	Method            Method
	Delivery          Delivery
	DeliveryError     string
	TemporaryPassword string
}
