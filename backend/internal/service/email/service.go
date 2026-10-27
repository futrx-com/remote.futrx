package email

import (
	"context"
	"fmt"

	emailapplication "github.com/futrx-com/remote.futrx.com/internal/model/email/application"
	emaildomain "github.com/futrx-com/remote.futrx.com/internal/model/email/domain"
	emailoutbound "github.com/futrx-com/remote.futrx.com/internal/port/email/outbound"
)

// Service owns the policy for the server's single SMTP configuration:
// validate, verify, persist, send. The protocol itself is delegated to a
// Sender.
type Service struct {
	store  emailoutbound.ConfigurationStore
	sender emailoutbound.Sender
}

// New builds a Service. It is total: a nil store or sender simply makes the
// feature report ErrNotConfigured everywhere instead of panicking, matching
// how the rest of this repository degrades an unavailable dependency instead
// of refusing to boot.
func New(store emailoutbound.ConfigurationStore, sender emailoutbound.Sender) *Service {
	return &Service{store: store, sender: sender}
}

func (s *Service) configured() bool {
	return s != nil && s.store != nil && s.sender != nil
}

// Settings reports the stored configuration's secret-free projection. A
// store holding nothing is reported as a nil Configuration with a nil
// error; only a real I/O failure is an error.
func (s *Service) Settings(ctx context.Context) (emailapplication.SMTPSettings, error) {
	if !s.configured() {
		return emailapplication.SMTPSettings{}, ErrNotConfigured
	}
	cfg, err := s.store.Configuration(ctx)
	if err != nil {
		return emailapplication.SMTPSettings{}, err
	}
	return emailapplication.SMTPSettings{Configuration: emailapplication.Public(cfg)}, nil
}

// Configure validates req against the stored configuration, verifies the
// resulting candidate against the live server, and only on success persists
// it. A failed validation or verification leaves any previously stored
// configuration untouched, so a bad edit cannot break a working setup.
func (s *Service) Configure(ctx context.Context, req ConfigureRequest) (emailapplication.SMTPSettings, error) {
	if !s.configured() {
		return emailapplication.SMTPSettings{}, ErrNotConfigured
	}
	current, err := s.store.Configuration(ctx)
	if err != nil {
		return emailapplication.SMTPSettings{}, err
	}
	candidate, err := buildCandidate(current, req)
	if err != nil {
		return emailapplication.SMTPSettings{}, err
	}
	if err := s.sender.Verify(ctx, candidate); err != nil {
		return emailapplication.SMTPSettings{}, fmt.Errorf("%w: %v", ErrVerificationFailed, err)
	}
	if err := s.store.Save(ctx, candidate); err != nil {
		return emailapplication.SMTPSettings{}, err
	}
	return emailapplication.SMTPSettings{Configuration: emailapplication.Public(&candidate)}, nil
}

// send delivers one already-composed message with the stored configuration.
// It is the single point where a Message meets the sender, used by SendTest
// and by Mailer; composition and recipient policy belong to the caller.
func (s *Service) send(ctx context.Context, msg emaildomain.Message) error {
	if !s.configured() {
		return ErrNotConfigured
	}
	cfg, err := s.store.Configuration(ctx)
	if err != nil {
		return err
	}
	if cfg == nil {
		return ErrNotConfigured
	}
	if err := s.sender.Send(ctx, *cfg, msg); err != nil {
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
// configuration. It is marked Required: unlike mail a feature raises for a
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
		Text("This confirms that your Remote server’s email settings are configured correctly.").
		Required().
		Send(ctx)
}

// Disable removes any stored configuration. It is idempotent.
func (s *Service) Disable(ctx context.Context) error {
	if !s.configured() {
		return ErrNotConfigured
	}
	return s.store.Delete(ctx)
}
