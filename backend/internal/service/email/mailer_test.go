package email

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeDirectory struct {
	addresses map[string]string
	failWith  error // returned for any key not in addresses, instead of ErrUnknownRecipient
	lookups   []string
}

func (f *fakeDirectory) Address(_ context.Context, userKey string) (string, error) {
	f.lookups = append(f.lookups, userKey)
	addr, ok := f.addresses[userKey]
	if !ok {
		if f.failWith != nil {
			return "", f.failWith
		}
		return "", ErrUnknownRecipient
	}
	return addr, nil
}

// TestMailerToUserSurfacesDirectoryInfrastructureErrors covers the
// distinction email_directory.go draws between "this user does not exist"
// (ErrUnknownRecipient) and "the directory could not be consulted" - a
// Directory implementation that returns some other error must have that
// error reach the caller unchanged, not be reported as an unknown recipient.
func TestMailerToUserSurfacesDirectoryInfrastructureErrors(t *testing.T) {
	infraErr := errors.New("user store unavailable")
	dir := &fakeDirectory{failWith: infraErr}
	mailer, _ := configuredMailer(dir)

	err := mailer.Mail().
		ToUser("someone@example.com").
		Subject("Hello").
		Heading("Hello").
		Send(context.Background())
	if !errors.Is(err, infraErr) {
		t.Fatalf("err = %v, want the underlying infrastructure error, not ErrUnknownRecipient", err)
	}
	if errors.Is(err, ErrUnknownRecipient) {
		t.Error("an infrastructure failure must not be reported as ErrUnknownRecipient")
	}
}

func TestMailerSendUsesStoredCredentials(t *testing.T) {
	mailer, sender := configuredMailer(nil)

	err := mailer.Mail().
		To("someone@example.com").
		Subject("Hello").
		Heading("Hello").
		Text("Body copy.").
		Send(context.Background())
	if err != nil {
		t.Fatalf("Send returned %v", err)
	}
	if len(sender.sentMessages) != 1 {
		t.Fatalf("sent %d messages, want 1", len(sender.sentMessages))
	}
	if got := sender.sentTo[0].Address; got != "server@example.com" {
		t.Errorf("sent as %q, want the stored sender identity", got)
	}
	if got := sender.sentMessages[0].To; got != "someone@example.com" {
		t.Errorf("To = %q", got)
	}
}

func TestMailerUnconfiguredIsASilentNoop(t *testing.T) {
	// A store holding nothing: an administrator has never set up email.
	sender := &fakeSender{}
	mailer := NewMailer(New(&fakeStore{}, sender), nil)

	if mailer.Enabled(context.Background()) {
		t.Error("Enabled = true with no stored credential")
	}
	err := mailer.Mail().
		To("someone@example.com").
		Subject("Hello").
		Heading("Hello").
		Send(context.Background())
	if err != nil {
		t.Fatalf("Send returned %v, want nil - a feature must not fail because email is unconfigured", err)
	}
	if len(sender.sentMessages) != 0 {
		t.Errorf("sent %d messages, want 0", len(sender.sentMessages))
	}
}

func TestMailerReportsSendFailures(t *testing.T) {
	store := &fakeStore{creds: &Credentials{Address: "server@example.com", AppPassword: "abcdefghijklmnop"}}
	sender := &fakeSender{sendErr: errors.New("mailbox full")}
	mailer := NewMailer(New(store, sender), nil)

	err := mailer.Mail().
		To("a@example.com", "b@example.com").
		Subject("Hello").
		Heading("Hello").
		Send(context.Background())
	if !errors.Is(err, ErrSendFailed) {
		t.Fatalf("err = %v, want ErrSendFailed", err)
	}
	if len(sender.sentMessages) != 2 {
		t.Errorf("attempted %d recipients, want 2 - one failure must not cancel the rest", len(sender.sentMessages))
	}
	if got := strings.Count(err.Error(), "mailbox full"); got != 2 {
		t.Errorf("joined error mentions the cause %d times, want 2", got)
	}
}

func TestMailerSendAsync(t *testing.T) {
	mailer, sender := configuredMailer(nil)

	err := mailer.Mail().
		To("someone@example.com").
		Subject("Hello").
		Heading("Hello").
		SendAsync(context.Background())
	if err != nil {
		t.Fatalf("SendAsync returned %v", err)
	}
	mailer.Wait()

	if len(sender.sentMessages) != 1 {
		t.Fatalf("sent %d messages after Wait, want 1", len(sender.sentMessages))
	}
}

func TestMailerSendAsyncReturnsCompositionErrorsSynchronously(t *testing.T) {
	mailer, sender := configuredMailer(nil)

	err := mailer.Mail().
		To("not an address").
		Subject("Hello").
		Heading("Hello").
		SendAsync(context.Background())
	if !errors.Is(err, ErrInvalidRecipient) {
		t.Fatalf("err = %v, want ErrInvalidRecipient returned immediately, not just logged", err)
	}
	mailer.Wait()
	if len(sender.sentMessages) != 0 {
		t.Errorf("a mail that failed to compose was still sent")
	}
}

func TestMailRequiredFailsInsteadOfSkippingWhenUnconfigured(t *testing.T) {
	sender := &fakeSender{}
	mailer := NewMailer(New(&fakeStore{}, sender), nil)

	err := mailer.Mail().
		To("someone@example.com").
		Subject("Your verification code").
		Code("482913").
		Required().
		Send(context.Background())
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("err = %v, want ErrNotConfigured - a Required mail must not silently vanish", err)
	}
	if len(sender.sentMessages) != 0 {
		t.Error("sent a message despite reporting ErrNotConfigured")
	}
}

func TestMailNotRequiredStillSkipsSilentlyWhenUnconfigured(t *testing.T) {
	sender := &fakeSender{}
	mailer := NewMailer(New(&fakeStore{}, sender), nil)

	err := mailer.Mail().
		To("someone@example.com").
		Subject("A notice").
		Heading("A notice").
		Send(context.Background())
	if err != nil {
		t.Fatalf("Send returned %v, want nil for a non-Required mail", err)
	}
}

func TestMaskAddress(t *testing.T) {
	cases := map[string]string{
		"jane.doe@example.com": "j***@example.com",
		"a@example.com":        "a***@example.com",
		"not-an-address":       "***",
		"":                     "***",
	}
	for in, want := range cases {
		if got := maskAddress(in); got != want {
			t.Errorf("maskAddress(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMailerSendAsyncSurvivesACancelledCallerContext(t *testing.T) {
	mailer, sender := configuredMailer(nil)
	ctx, cancel := context.WithCancel(context.Background())

	mailer.Mail().
		To("someone@example.com").
		Subject("Hello").
		Heading("Hello").
		SendAsync(ctx)
	cancel() // the HTTP request that raised this email is answered and gone
	mailer.Wait()

	if len(sender.sentMessages) != 1 {
		t.Fatalf("sent %d messages, want 1 - the request context must not abort a background send", len(sender.sentMessages))
	}
}

func TestMailerToUser(t *testing.T) {
	dir := &fakeDirectory{addresses: map[string]string{"owner@example.com": "owner@example.com"}}
	mailer, sender := configuredMailer(dir)

	t.Run("resolves a known user", func(t *testing.T) {
		err := mailer.Mail().
			ToUser("owner@example.com").
			Subject("Hello").
			Heading("Hello").
			Send(context.Background())
		if err != nil {
			t.Fatalf("Send returned %v", err)
		}
		if len(dir.lookups) != 1 {
			t.Errorf("directory lookups = %d, want 1", len(dir.lookups))
		}
		if len(sender.sentMessages) != 1 {
			t.Fatalf("sent %d messages, want 1", len(sender.sentMessages))
		}
	})

	t.Run("rejects an unknown user", func(t *testing.T) {
		err := mailer.Mail().
			ToUser("stranger@example.com").
			Subject("Hello").
			Heading("Hello").
			Send(context.Background())
		if !errors.Is(err, ErrUnknownRecipient) {
			t.Fatalf("err = %v, want ErrUnknownRecipient", err)
		}
	})
}

func TestNilMailerDoesNotPanic(t *testing.T) {
	var mailer *Mailer

	if mailer.Enabled(context.Background()) {
		t.Error("nil Mailer reports Enabled")
	}
	err := mailer.Mail().
		To("someone@example.com").
		Subject("Hello").
		Heading("Hello").
		Send(context.Background())
	if err != nil {
		t.Errorf("Send on a nil Mailer returned %v, want nil", err)
	}
	mailer.Mail().To("someone@example.com").Subject("Hello").Heading("Hello").SendAsync(context.Background())
	mailer.Wait()
}
