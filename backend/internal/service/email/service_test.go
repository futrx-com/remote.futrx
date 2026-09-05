package email

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeStore struct {
	creds     *Credentials
	saveCalls int
}

func (f *fakeStore) Credentials(context.Context) (*Credentials, error) { return f.creds, nil }
func (f *fakeStore) Save(_ context.Context, creds Credentials) error {
	f.saveCalls++
	f.creds = &creds
	return nil
}
func (f *fakeStore) Delete(context.Context) error {
	f.creds = nil
	return nil
}

type fakeSender struct {
	verifyErr    error
	sendErr      error
	sentTo       []Credentials
	sentMessages []Message
}

func (f *fakeSender) Verify(context.Context, Credentials) error { return f.verifyErr }
func (f *fakeSender) Send(_ context.Context, creds Credentials, msg Message) error {
	f.sentTo = append(f.sentTo, creds)
	f.sentMessages = append(f.sentMessages, msg)
	return f.sendErr
}

func TestServiceConfigure(t *testing.T) {
	t.Run("failing verify does not save and reports the cause", func(t *testing.T) {
		store := &fakeStore{}
		cause := errors.New("wrong password")
		sender := &fakeSender{verifyErr: cause}
		svc := New(store, sender)

		_, err := svc.Configure(context.Background(), Credentials{Address: "user@example.com", AppPassword: "abcd efgh ijkl mnop"})
		if !errors.Is(err, ErrVerificationFailed) {
			t.Fatalf("err = %v, want ErrVerificationFailed", err)
		}
		if err == nil || !strings.Contains(err.Error(), cause.Error()) {
			t.Errorf("err %v does not carry the cause text %q", err, cause.Error())
		}
		if store.saveCalls != 0 {
			t.Errorf("save calls = %d, want 0", store.saveCalls)
		}
	})

	t.Run("successful configure saves normalized credentials", func(t *testing.T) {
		store := &fakeStore{}
		sender := &fakeSender{}
		svc := New(store, sender)

		settings, err := svc.Configure(context.Background(), Credentials{Address: "User@Example.com", AppPassword: "abcd efgh ijkl mnop"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if settings.Address != "user@example.com" {
			t.Errorf("settings.Address = %q, want lowercased", settings.Address)
		}
		if store.creds == nil || store.creds.AppPassword != "abcdefghijklmnop" {
			t.Errorf("stored password = %+v, want whitespace stripped", store.creds)
		}
	})
}

func TestServiceSendTest(t *testing.T) {
	t.Run("empty store returns ErrNotConfigured", func(t *testing.T) {
		svc := New(&fakeStore{}, &fakeSender{})
		if err := svc.SendTest(context.Background(), "to@example.com"); !errors.Is(err, ErrNotConfigured) {
			t.Fatalf("err = %v, want ErrNotConfigured", err)
		}
	})

	t.Run("passes recipient through and uses the stored credentials", func(t *testing.T) {
		store := &fakeStore{creds: &Credentials{Address: "sender@example.com", AppPassword: "abcdefghijklmnop"}}
		sender := &fakeSender{}
		svc := New(store, sender)

		if err := svc.SendTest(context.Background(), "recipient@example.com"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(sender.sentMessages) != 1 || sender.sentMessages[0].To != "recipient@example.com" {
			t.Errorf("sent messages = %+v", sender.sentMessages)
		}
		if len(sender.sentTo) != 1 || sender.sentTo[0].Address != "sender@example.com" {
			t.Errorf("sender received wrong credentials: %+v", sender.sentTo)
		}
	})
}

func TestServiceSettings(t *testing.T) {
	svc := New(&fakeStore{}, &fakeSender{})
	settings, err := svc.Settings(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if settings.Configured {
		t.Errorf("settings = %+v, want Configured: false", settings)
	}
}

func TestServiceNilStore(t *testing.T) {
	svc := New(nil, nil)

	if _, err := svc.Settings(context.Background()); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("Settings err = %v, want ErrNotConfigured", err)
	}
	if _, err := svc.Configure(context.Background(), Credentials{}); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("Configure err = %v, want ErrNotConfigured", err)
	}
	if err := svc.SendTest(context.Background(), "to@example.com"); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("SendTest err = %v, want ErrNotConfigured", err)
	}
	if err := svc.Disable(context.Background()); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("Disable err = %v, want ErrNotConfigured", err)
	}
}
