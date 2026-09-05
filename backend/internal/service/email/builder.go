package email

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Mail composes one outbound email from A to Z: who it goes to, what it says,
// and how it looks. Every method returns the receiver so a caller reads as one
// statement:
//
//	err := mailer.Mail().
//		ToUser(ownerEmail).
//		Subject("Your run finished").
//		Heading("Run finished").
//		Text("The agent finished the run you started.").
//		KeyValues([2]string{"Project", name}, [2]string{"Duration", d.String()}).
//		Button("Open the run", runURL).
//		Note("You are receiving this because you started the run.").
//		Send(ctx)
//
// Validation failures are held on the builder rather than returned from each
// step, so a caller writes one error check at the end instead of eleven. The
// first failure wins; later steps still record their blocks but Build/Send
// return the original error.
//
// A Mail carries no From: the sender identity is the server's stored Gmail
// credential and is applied below this layer.
type Mail struct {
	mailer   *Mailer
	subject  string
	addrs    []string
	users    []string
	blocks   []block
	required bool
	err      error
}

// fail records the first composition error. Subsequent failures are dropped:
// the first one is the one that explains the mistake.
func (b *Mail) fail(err error) *Mail {
	if b.err == nil {
		b.err = err
	}
	return b
}

// requiredText trims s and rejects it if that leaves nothing - a caller
// passing "   " made the same mistake as passing "". It also rejects a raw
// carriage return or newline: every text field here can end up in rendered
// HTML or a header, and a value that already contains a line break came from
// a caller bug, not a legitimate composition.
func requiredText(field, s string) (string, error) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return "", fmt.Errorf("%w: %s is empty", ErrIncompleteMail, field)
	}
	if strings.ContainsAny(s, "\r\n") {
		return "", fmt.Errorf("%w: %s contains a line break", ErrIncompleteMail, field)
	}
	return trimmed, nil
}

// To addresses the mail to literal email addresses. Addresses are validated
// and normalised at Build time, and duplicates collapse.
func (b *Mail) To(addr ...string) *Mail {
	b.addrs = append(b.addrs, addr...)
	return b
}

// ToUser addresses the mail to application users, resolving each key through
// the Mailer's Directory when the mail is built. A Mailer with no Directory
// rejects the mail rather than silently dropping the recipient.
func (b *Mail) ToUser(userKey ...string) *Mail {
	b.users = append(b.users, userKey...)
	return b
}

// Subject sets the subject line. Calling it twice replaces the subject. It is
// validated at Build/Send, not here, so it can be set before or after the
// content blocks.
func (b *Mail) Subject(subject string) *Mail {
	b.subject = subject
	return b
}

// Required marks the mail as one that must actually reach Gmail: a 2FA code,
// a password reset, an invitation - anything where silently dropping the
// message on an unconfigured server would be a security or correctness
// failure, not a missed notification. Send/SendAsync on a Required mail
// return ErrNotConfigured instead of the usual silent no-op.
//
// Most mail should not call this: a schedule or run notification that
// vanishes because an administrator never set up email is a missed
// convenience, and forcing every such caller to handle ErrNotConfigured would
// only encourage them to ignore it.
func (b *Mail) Required() *Mail {
	b.required = true
	return b
}

// Heading adds the title line, centred under the logo.
func (b *Mail) Heading(heading string) *Mail {
	value, err := requiredText("heading", heading)
	if err != nil {
		return b.fail(err)
	}
	b.blocks = append(b.blocks, headingBlock{value: value})
	return b
}

// Text adds a paragraph of body copy.
func (b *Mail) Text(text string) *Mail {
	value, err := requiredText("text block", text)
	if err != nil {
		return b.fail(err)
	}
	b.blocks = append(b.blocks, textBlock{value: value})
	return b
}

// Button adds a call to action. The target must be an absolute http or https
// URL; anything else fails the mail rather than shipping a link recipients are
// being asked to trust.
func (b *Mail) Button(label, target string) *Mail {
	value, err := requiredText("button label", label)
	if err != nil {
		return b.fail(err)
	}
	safe, err := safeURL(target)
	if err != nil {
		return b.fail(err)
	}
	b.blocks = append(b.blocks, buttonBlock{label: value, target: safe})
	return b
}

// List adds a bulleted list. Every item must carry actual content.
func (b *Mail) List(items ...string) *Mail {
	if len(items) == 0 {
		return b.fail(fmt.Errorf("%w: list block has no items", ErrIncompleteMail))
	}
	trimmed := make([]string, len(items))
	for i, item := range items {
		value, err := requiredText("list item", item)
		if err != nil {
			return b.fail(err)
		}
		trimmed[i] = value
	}
	b.blocks = append(b.blocks, listBlock{items: trimmed})
	return b
}

// KeyValues adds a label/value summary table. Every label and value must
// carry actual content.
func (b *Mail) KeyValues(pairs ...[2]string) *Mail {
	if len(pairs) == 0 {
		return b.fail(fmt.Errorf("%w: key/value block has no pairs", ErrIncompleteMail))
	}
	trimmed := make([][2]string, len(pairs))
	for i, pair := range pairs {
		label, err := requiredText("key/value label", pair[0])
		if err != nil {
			return b.fail(err)
		}
		value, err := requiredText("key/value value", pair[1])
		if err != nil {
			return b.fail(err)
		}
		trimmed[i] = [2]string{label, value}
	}
	b.blocks = append(b.blocks, kvBlock{pairs: trimmed})
	return b
}

// Code adds a value meant to be read and copied, such as a one-time code.
func (b *Mail) Code(value string) *Mail {
	trimmed, err := requiredText("code block", value)
	if err != nil {
		return b.fail(err)
	}
	b.blocks = append(b.blocks, codeBlock{value: trimmed})
	return b
}

// Divider adds a horizontal rule between sections.
func (b *Mail) Divider() *Mail {
	b.blocks = append(b.blocks, dividerBlock{})
	return b
}

// Note adds small muted print, such as an expiry warning.
func (b *Mail) Note(note string) *Mail {
	value, err := requiredText("note", note)
	if err != nil {
		return b.fail(err)
	}
	b.blocks = append(b.blocks, noteBlock{value: value})
	return b
}

// Build renders the mail into one Message per recipient. Recipients get
// separate messages rather than a shared To list so that no recipient learns
// who else was mailed, and because the SMTP layer addresses one envelope at a
// time anyway.
func (b *Mail) Build(ctx context.Context) ([]Message, error) {
	if b.err != nil {
		return nil, b.err
	}
	subject, err := requiredText("subject", b.subject)
	if err != nil {
		return nil, err
	}
	if len(b.blocks) == 0 {
		return nil, fmt.Errorf("%w: no content", ErrIncompleteMail)
	}
	recipients, err := b.recipients(ctx)
	if err != nil {
		return nil, err
	}
	htmlBody, textBody := renderBlocks(b.blocks)
	messages := make([]Message, 0, len(recipients))
	for _, to := range recipients {
		messages = append(messages, Message{
			To:       to,
			Subject:  subject,
			Body:     textBody,
			HTMLBody: htmlBody,
		})
	}
	return messages, nil
}

// recipients resolves user keys, validates literal addresses, and returns the
// deduplicated result in the order it was given.
func (b *Mail) recipients(ctx context.Context) ([]string, error) {
	if len(b.addrs)+len(b.users) == 0 {
		return nil, fmt.Errorf("%w: no recipient", ErrIncompleteMail)
	}
	seen := make(map[string]struct{}, len(b.addrs)+len(b.users))
	out := make([]string, 0, len(b.addrs)+len(b.users))
	add := func(addr string) error {
		normalized, err := normalizeAddress(addr)
		if err != nil {
			return fmt.Errorf("%w: %q", ErrInvalidRecipient, addr)
		}
		if _, dup := seen[normalized]; dup {
			return nil
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
		return nil
	}
	for _, addr := range b.addrs {
		if err := add(addr); err != nil {
			return nil, err
		}
	}
	for _, key := range b.users {
		if b.mailer == nil || b.mailer.dir == nil {
			return nil, fmt.Errorf("%w: no directory to resolve user %q", ErrUnknownRecipient, key)
		}
		addr, err := b.mailer.dir.Address(ctx, key)
		if err != nil {
			return nil, err
		}
		if err := add(addr); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// Send delivers the mail synchronously and reports the outcome. Every
// recipient is attempted even if an earlier one fails, and the failures are
// joined, so one bad address does not silently cancel the rest.
//
// When the server has no Gmail credential configured, Send is a logged no-op
// returning nil, unless the mail was marked Required, in which case it
// returns ErrNotConfigured: a feature that merely notifies a user must not
// fail because an administrator has not set up email, but a feature that
// depends on the message arriving must know that it did not.
func (b *Mail) Send(ctx context.Context) error {
	messages, err := b.Build(ctx)
	if err != nil {
		return err
	}
	return b.mailer.deliver(ctx, messages, b.required)
}

// SendAsync validates and resolves the mail synchronously - so a composition
// mistake (a bad address, a missing subject, an unknown user) is returned to
// the caller immediately rather than only ever reaching a log line - then
// delivers it on a background goroutine with its own timeout and returns.
// The goroutine closes over the already-rendered messages, never the
// builder, so a caller reusing or mutating the Mail afterwards cannot race
// with the send.
//
// Delivery failures (as opposed to composition failures) happen after this
// call has returned and are only logged; use Send when the caller must learn
// the outcome. Background delivery is best-effort and in-memory: a send in
// flight when the process exits is lost, which is why Required mail is
// expected to go through Send.
func (b *Mail) SendAsync(ctx context.Context) error {
	messages, err := b.Build(ctx)
	if err != nil {
		return err
	}
	b.mailer.dispatchAsync(ctx, messages, b.required)
	return nil
}

// joinSendErrors collapses per-recipient failures into one error.
func joinSendErrors(errs []error) error {
	if len(errs) == 0 {
		return nil
	}
	return errors.Join(errs...)
}
