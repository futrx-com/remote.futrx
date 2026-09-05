package email

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"
)

// deliveryTimeout bounds a background send. It matches push's delivery budget:
// long enough for a TLS handshake and an SMTP conversation, short enough that
// a hung server cannot pin a goroutine forever.
const deliveryTimeout = 30 * time.Second

// maxConcurrentSends bounds how many SMTP sessions this process holds open at
// once. Each recipient of each Mail is its own session (see Mail.Build), so a
// mail with many recipients - or many mails in flight together - could
// otherwise open unbounded connections against Gmail and pile up unbounded
// goroutines. A background send blocks here before dialing, so the goroutine
// count can grow but the concurrent SMTP work cannot.
const maxConcurrentSends = 20

// Mailer is the stable entry point every other service uses to send email. It
// is the only thing above this package that needs to exist: callers compose a
// Mail through it and never touch credentials, MIME, net/smtp, or HTML email
// markup.
//
// It aggregates the credential-owning Service with a Directory for addressing
// users by key, following the same shape as skills.Catalog and
// chat.AccessService - a core service plus narrow, consumer-defined
// collaborators.
//
// Like the rest of this repository, an unavailable dependency degrades rather
// than refusing to boot: a nil Mailer, a nil Service, or a server with no
// Gmail credential saved turns every send into a logged no-op (or, for mail
// marked Required, into ErrNotConfigured). Every exported method on *Mailer
// is safe to call on a nil receiver, and so is every method reachable from
// Mailer.Mail() - Mail, Send and SendAsync included - so a caller never has
// to nil-check before composing.
type Mailer struct {
	svc  *Service
	dir  Directory
	wg   sync.WaitGroup
	sema chan struct{}
}

// NewMailer builds the facade. dir may be nil, in which case Mail.ToUser is
// rejected but literal addresses still work.
func NewMailer(svc *Service, dir Directory) *Mailer {
	return &Mailer{svc: svc, dir: dir, sema: make(chan struct{}, maxConcurrentSends)}
}

// Mail starts composing an email. It is safe on a nil Mailer: the resulting
// Mail composes normally and drops at send time, so a caller in a deployment
// without email never has to nil-check.
func (m *Mailer) Mail() *Mail {
	return &Mail{mailer: m}
}

// Enabled reports whether a send would actually reach Gmail: the service is
// wired and an administrator has saved a credential. Callers do not need to
// consult it before sending - it exists so a feature can tell a user whether
// email notifications are available at all.
func (m *Mailer) Enabled(ctx context.Context) bool {
	if m == nil || m.svc == nil {
		return false
	}
	settings, err := m.svc.Settings(ctx)
	return err == nil && settings.Configured
}

// Wait blocks until every background send started by SendAsync has finished.
// It exists so tests are deterministic; production code does not call it.
func (m *Mailer) Wait() {
	if m == nil {
		return
	}
	m.wg.Wait()
}

// acquire reserves one of the bounded concurrent-send slots, blocking if the
// process is already holding the maximum. A nil or zero-value Mailer (as
// Service.mail builds internally) has no semaphore and never blocks.
func (m *Mailer) acquire() {
	if m == nil || m.sema == nil {
		return
	}
	m.sema <- struct{}{}
}

func (m *Mailer) release() {
	if m == nil || m.sema == nil {
		return
	}
	<-m.sema
}

// deliver sends every message, attempting all recipients before reporting.
// When no credential is configured, it is a logged no-op unless required is
// set, in which case it reports ErrNotConfigured.
func (m *Mailer) deliver(ctx context.Context, messages []Message, required bool) error {
	if m == nil || m.svc == nil {
		if required {
			return ErrNotConfigured
		}
		logSkipped(messages, "email is not wired in this deployment")
		return nil
	}
	settings, err := m.svc.Settings(ctx)
	if err != nil {
		return err
	}
	if !settings.Configured {
		if required {
			return ErrNotConfigured
		}
		logSkipped(messages, "no Gmail credential is configured")
		return nil
	}
	m.acquire()
	defer m.release()
	var errs []error
	for _, msg := range messages {
		if err := m.svc.send(ctx, msg); err != nil {
			errs = append(errs, err)
		}
	}
	return joinSendErrors(errs)
}

// dispatchAsync delivers already-built messages on their own goroutine. It
// takes rendered Messages rather than the Mail itself so the goroutine never
// reads the builder's mutable state - only the immutable values captured
// here - even if the caller keeps mutating or reusing the Mail after
// SendAsync returns.
//
// The background context is deliberate: the request that raised this email
// is usually already answered, and its cancellation must not abort the send.
// context.WithoutCancel keeps request-scoped values (trace/log fields) while
// detaching from that cancellation.
func (m *Mailer) dispatchAsync(ctx context.Context, messages []Message, required bool) {
	if m == nil {
		return
	}
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		sendCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), deliveryTimeout)
		defer cancel()
		if err := m.deliver(sendCtx, messages, required); err != nil {
			log.Printf("email: async send failed: %v", err)
		}
	}()
}

// logSkipped records mail that was composed but never sent, so a silently
// undelivered notification is still traceable in the server log. Addresses
// are masked: the log is diagnostic, not a mailing list.
func logSkipped(messages []Message, reason string) {
	for _, msg := range messages {
		log.Printf("email: skipped %q to %s: %s", msg.Subject, maskAddress(msg.To), reason)
	}
}

// maskAddress redacts everything but the first character of the local part,
// so a log line can show that a send was attempted without recording a full
// address: "j***@example.com" rather than "jane.doe@example.com".
func maskAddress(addr string) string {
	at := strings.IndexByte(addr, '@')
	if at <= 0 {
		return "***"
	}
	return addr[:1] + "***" + addr[at:]
}
