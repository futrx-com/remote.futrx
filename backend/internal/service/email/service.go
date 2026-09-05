package email

import (
	"context"
	"fmt"
)

// Service owns the policy for the server's single Gmail sender identity:
// normalise, validate, verify, persist, send. The protocol itself is
// delegated to a Sender.
type Service struct {
	store  Store
	sender Sender
}

// New builds a Service. It is total: a nil store or sender simply makes the
// feature report ErrNotConfigured everywhere instead of panicking, matching
// how the rest of this repository degrades an unavailable dependency instead
// of refusing to boot.
func New(store Store, sender Sender) *Service {
	return &Service{store: store, sender: sender}
}

func (s *Service) configured() bool {
	return s != nil && s.store != nil && s.sender != nil
}

// Settings reports whether a credential is stored, and its address. A store
// holding nothing is reported as Settings{Configured: false} with a nil
// error; only a real I/O failure is an error.
func (s *Service) Settings(ctx context.Context) (Settings, error) {
	if !s.configured() {
		return Settings{}, ErrNotConfigured
	}
	creds, err := s.store.Credentials(ctx)
	if err != nil {
		return Settings{}, err
	}
	if creds == nil {
		return Settings{Configured: false}, nil
	}
	return Settings{Configured: true, Address: creds.Address}, nil
}

// Configure normalises and verifies creds against the live server, and only
// on success persists them. A failed verification leaves any previously
// stored credential untouched, so a bad edit cannot break a working
// configuration.
func (s *Service) Configure(ctx context.Context, creds Credentials) (Settings, error) {
	if !s.configured() {
		return Settings{}, ErrNotConfigured
	}
	normalized, err := normalize(creds)
	if err != nil {
		return Settings{}, err
	}
	if err := s.sender.Verify(ctx, normalized); err != nil {
		return Settings{}, fmt.Errorf("%w: %v", ErrVerificationFailed, err)
	}
	if err := s.store.Save(ctx, normalized); err != nil {
		return Settings{}, err
	}
	return Settings{Configured: true, Address: normalized.Address}, nil
}

// send delivers one already-composed message with the stored credentials. It
// is the single point where a Message meets the sender, used by SendTest and
// by Mailer; composition and recipient policy belong to the caller.
func (s *Service) send(ctx context.Context, msg Message) error {
	if !s.configured() {
		return ErrNotConfigured
	}
	creds, err := s.store.Credentials(ctx)
	if err != nil {
		return err
	}
	if creds == nil {
		return ErrNotConfigured
	}
	if err := s.sender.Send(ctx, *creds, msg); err != nil {
		return fmt.Errorf("%w: %v", ErrSendFailed, err)
	}
	return nil
}

// mail starts composing a message this package sends itself (SendTest), bound
// to this Service alone. It has no Directory - ToUser is not used here - and
// no concurrency limit of its own, since it never overlaps with Mailer's
// bounded background sends.
func (s *Service) mail() *Mail {
	return &Mail{mailer: &Mailer{svc: s}}
}

// SendTest sends a fixed test message to recipient using the stored
// credentials. It is marked Required: unlike mail a feature raises for a
// user, an unconfigured server here is an error, not a silent no-op, because
// an administrator asked directly and must be told.
func (s *Service) SendTest(ctx context.Context, recipient string) error {
	if !s.configured() {
		return ErrNotConfigured
	}
	return s.mail().
		To(recipient).
		Subject("Remote test email").
		Heading("Your test email worked!").
		Text("This confirms that your Remote server\u2019s email settings are configured correctly.").
		Required().
		Send(ctx)
}

// Disable removes any stored credential. It is idempotent.
func (s *Service) Disable(ctx context.Context) error {
	if !s.configured() {
		return ErrNotConfigured
	}
	return s.store.Delete(ctx)
}
