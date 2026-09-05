package email

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// configuredMailer builds a Mailer over a store that already holds a
// credential, so sends reach the fake sender.
func configuredMailer(dir Directory) (*Mailer, *fakeSender) {
	store := &fakeStore{creds: &Credentials{Address: "server@example.com", AppPassword: "abcdefghijklmnop"}}
	sender := &fakeSender{}
	return NewMailer(New(store, sender), dir), sender
}

func TestMailBuildRendersBothParts(t *testing.T) {
	mailer, _ := configuredMailer(nil)

	messages, err := mailer.Mail().
		To("Someone@Example.com").
		Subject("Your run finished").
		Heading("Run finished").
		Text("The agent finished the run you started.").
		KeyValues([2]string{"Project", "remote"}, [2]string{"Duration", "4m12s"}).
		Button("Open the run", "https://example.com/runs/1").
		Divider().
		Note("You are receiving this because you started the run.").
		Build(context.Background())
	if err != nil {
		t.Fatalf("Build returned %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("messages = %d, want 1", len(messages))
	}
	msg := messages[0]

	if msg.To != "someone@example.com" {
		t.Errorf("To = %q, want the normalized address", msg.To)
	}
	if msg.Subject != "Your run finished" {
		t.Errorf("Subject = %q", msg.Subject)
	}
	for _, want := range []string{"Run finished", "The agent finished", "remote", "4m12s", "Open the run", "https://example.com/runs/1", "receiving this because"} {
		if !strings.Contains(msg.HTMLBody, want) {
			t.Errorf("HTMLBody missing %q", want)
		}
		if !strings.Contains(msg.Body, want) {
			t.Errorf("Body (text/plain) missing %q", want)
		}
	}
	if !strings.Contains(msg.HTMLBody, "<!DOCTYPE html>") {
		t.Error("HTMLBody is not wrapped in the branded shell")
	}
	if strings.Contains(msg.Body, "<") {
		t.Errorf("Body should be plain text, got %q", msg.Body)
	}
}

func TestMailFansOutPerRecipient(t *testing.T) {
	mailer, _ := configuredMailer(nil)

	messages, err := mailer.Mail().
		To("a@example.com", "b@example.com").
		To("C@example.com", "a@example.com").
		Subject("Notice").
		Heading("Notice").
		Build(context.Background())
	if err != nil {
		t.Fatalf("Build returned %v", err)
	}
	want := []string{"a@example.com", "b@example.com", "c@example.com"}
	if len(messages) != len(want) {
		t.Fatalf("messages = %d, want %d (duplicates collapsed)", len(messages), len(want))
	}
	for i, msg := range messages {
		if msg.To != want[i] {
			t.Errorf("messages[%d].To = %q, want %q", i, msg.To, want[i])
		}
		if msg.HTMLBody != messages[0].HTMLBody {
			t.Errorf("messages[%d] body differs from the first; every recipient gets the same content", i)
		}
	}
}

func TestMailCompositionErrors(t *testing.T) {
	mailer, sender := configuredMailer(nil)

	cases := []struct {
		name string
		mail func() *Mail
		want error
	}{
		{"no recipient", func() *Mail {
			return mailer.Mail().Subject("s").Heading("h")
		}, ErrIncompleteMail},
		{"no subject", func() *Mail {
			return mailer.Mail().To("a@example.com").Heading("h")
		}, ErrIncompleteMail},
		{"no content", func() *Mail {
			return mailer.Mail().To("a@example.com").Subject("s")
		}, ErrIncompleteMail},
		{"bad button URL", func() *Mail {
			return mailer.Mail().To("a@example.com").Subject("s").Heading("h").Button("Click", "javascript:alert(1)")
		}, ErrIncompleteMail},
		{"empty text", func() *Mail {
			return mailer.Mail().To("a@example.com").Subject("s").Heading("h").Text("")
		}, ErrIncompleteMail},
		{"bad address", func() *Mail {
			return mailer.Mail().To("not an address").Subject("s").Heading("h")
		}, ErrInvalidRecipient},
		{"user without a directory", func() *Mail {
			return mailer.Mail().ToUser("someone@example.com").Subject("s").Heading("h")
		}, ErrUnknownRecipient},
		{"whitespace-only heading", func() *Mail {
			return mailer.Mail().To("a@example.com").Subject("s").Heading("   ")
		}, ErrIncompleteMail},
		{"whitespace-only subject", func() *Mail {
			return mailer.Mail().To("a@example.com").Subject("   ").Heading("h")
		}, ErrIncompleteMail},
		{"subject with a line break", func() *Mail {
			return mailer.Mail().To("a@example.com").Subject("s\r\nBcc: attacker@evil.com").Heading("h")
		}, ErrIncompleteMail},
		{"blank list item", func() *Mail {
			return mailer.Mail().To("a@example.com").Subject("s").List("fine", "  ")
		}, ErrIncompleteMail},
		{"blank key/value", func() *Mail {
			return mailer.Mail().To("a@example.com").Subject("s").KeyValues([2]string{"Label", "  "})
		}, ErrIncompleteMail},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := len(sender.sentMessages)
			err := tc.mail().Send(context.Background())
			if !errors.Is(err, tc.want) {
				t.Fatalf("Send err = %v, want %v", err, tc.want)
			}
			if len(sender.sentMessages) != before {
				t.Error("a mail that failed composition was still sent")
			}
		})
	}
}

func TestMailReportsTheFirstErrorNotTheLast(t *testing.T) {
	mailer, _ := configuredMailer(nil)

	err := mailer.Mail().
		To("a@example.com").
		Subject("s").
		Button("Click", "javascript:alert(1)").
		Text("").
		Send(context.Background())
	if err == nil || !strings.Contains(err.Error(), "button URL") {
		t.Fatalf("err = %v, want the first failure (the button URL)", err)
	}
}
