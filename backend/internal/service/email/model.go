package email

import "errors"

var (
	// ErrNotConfigured is returned by every operation that needs a stored
	// SMTP configuration when none has been saved yet.
	ErrNotConfigured = errors.New("email: not configured")
	// ErrInvalidConfiguration wraps every reason a candidate SMTP
	// configuration fails validation before any network operation is
	// attempted: a malformed host, an out-of-range port, an unrecognized
	// TLS or authentication mode, an invalid sender address, a plaintext +
	// authenticated combination, or a missing username/password for an
	// authenticated mode.
	ErrInvalidConfiguration = errors.New("email: invalid configuration")
	// ErrInvalidRecipient means the given recipient (a test-send address or
	// a Mail.To address) failed address validation.
	ErrInvalidRecipient = errors.New("email: invalid recipient")
	// ErrVerificationFailed wraps the cause returned by the sender's Verify
	// call.
	ErrVerificationFailed = errors.New("email: verification failed")
	// ErrSendFailed wraps the cause returned by the sender's Send call.
	ErrSendFailed = errors.New("email: send failed")
	// ErrIncompleteMail means a Mail was sent before it was fully composed:
	// no recipient, no subject, no content, or a block given an unusable
	// value. It is a programming error in the calling service, reported at
	// Build or Send rather than mid-chain so the builder stays fluent.
	ErrIncompleteMail = errors.New("email: incomplete mail")
	// ErrUnknownRecipient means a Directory could not resolve a user key to a
	// deliverable address - the user is not registered, or has no address on
	// file.
	ErrUnknownRecipient = errors.New("email: unknown recipient")
)
