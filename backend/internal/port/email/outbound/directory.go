package emailoutbound

import "context"

// Directory resolves an application user key to a deliverable address, so a
// calling service can address mail to "the user who owns this run" without
// carrying an address around. The composition layer supplies an adapter over
// user.Service; a Mailer built without one simply rejects Mail.ToUser.
//
// It returns email.ErrUnknownRecipient when the key names nobody we may
// mail, which is the natural place a per-user email opt-out would later
// live.
type Directory interface {
	Address(ctx context.Context, userKey string) (string, error)
}
