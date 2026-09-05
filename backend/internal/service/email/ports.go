package email

import "context"

// Store persists the single server-wide Gmail credential. Credentials
// returns (nil, nil) when nothing has ever been saved - that absence is the
// correct "not configured" state, not an error, mirroring
// service/auth.TwoFactorStore.Get.
type Store interface {
	Credentials(ctx context.Context) (*Credentials, error)
	Save(ctx context.Context, creds Credentials) error
	Delete(ctx context.Context) error
}

// Sender speaks the outbound protocol. The composition layer supplies the
// real SMTP client; tests supply a recorder.
type Sender interface {
	Verify(ctx context.Context, creds Credentials) error
	Send(ctx context.Context, creds Credentials, msg Message) error
}

// Directory resolves an application user key to a deliverable address, so a
// calling service can address mail to "the user who owns this run" without
// carrying an address around. The composition layer supplies an adapter over
// user.Service; a Mailer built without one simply rejects Mail.ToUser.
//
// It returns ErrUnknownRecipient when the key names nobody we may mail, which
// is the natural place a per-user email opt-out would later live.
type Directory interface {
	Address(ctx context.Context, userKey string) (string, error)
}
