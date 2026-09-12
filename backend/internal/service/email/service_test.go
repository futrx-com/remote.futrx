package email

import (
	"context"
	"errors"
	"strings"
	"testing"

	emailapplication "github.com/futrx-com/remote.futrx.com/internal/model/email/application"
	emaildomain "github.com/futrx-com/remote.futrx.com/internal/model/email/domain"
)

type fakeStore struct {
	cfg       *emailapplication.SMTPConfiguration
	saveCalls int
}

func (f *fakeStore) Configuration(context.Context) (*emailapplication.SMTPConfiguration, error) {
	return f.cfg, nil
}
func (f *fakeStore) Save(_ context.Context, cfg emailapplication.SMTPConfiguration) error {
	f.saveCalls++
	f.cfg = &cfg
	return nil
}
func (f *fakeStore) Delete(context.Context) error {
	f.cfg = nil
	return nil
}

type fakeSender struct {
	verifyErr    error
	sendErr      error
	verifiedWith []emailapplication.SMTPConfiguration
	sentTo       []emailapplication.SMTPConfiguration
	sentMessages []emaildomain.Message
}

func (f *fakeSender) Verify(_ context.Context, cfg emailapplication.SMTPConfiguration) error {
	f.verifiedWith = append(f.verifiedWith, cfg)
	return f.verifyErr
}
func (f *fakeSender) Send(_ context.Context, cfg emailapplication.SMTPConfiguration, msg emaildomain.Message) error {
	f.sentTo = append(f.sentTo, cfg)
	f.sentMessages = append(f.sentMessages, msg)
	return f.sendErr
}

func authenticatedRequest(overrides func(*ConfigureRequest)) ConfigureRequest {
	password := "s3cret-value"
	req := ConfigureRequest{
		Host:           "smtp.example.com",
		Port:           587,
		TLSMode:        emailapplication.TLSModeSTARTTLS,
		Authentication: emailapplication.AuthenticationPlain,
		Username:       "mailer@example.com",
		Password:       &password,
		FromAddress:    "mailer@example.com",
	}
	if overrides != nil {
		overrides(&req)
	}
	return req
}

func TestServiceConfigure(t *testing.T) {
	t.Run("failing verify does not save and reports the cause", func(t *testing.T) {
		store := &fakeStore{}
		cause := errors.New("wrong password")
		sender := &fakeSender{verifyErr: cause}
		svc := New(store, sender)

		_, err := svc.Configure(context.Background(), authenticatedRequest(nil))
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

	t.Run("a failed edit retains the previous working configuration", func(t *testing.T) {
		working := emailapplication.SMTPConfiguration{
			Host: "smtp.example.com", Port: 587,
			TLSMode: emailapplication.TLSModeSTARTTLS, Authentication: emailapplication.AuthenticationPlain,
			Username: "mailer@example.com", Password: "old-secret", FromAddress: "mailer@example.com",
		}
		store := &fakeStore{cfg: &working}
		sender := &fakeSender{verifyErr: errors.New("wrong password")}
		svc := New(store, sender)

		_, err := svc.Configure(context.Background(), authenticatedRequest(func(r *ConfigureRequest) {
			bad := "wrong-secret"
			r.Password = &bad
		}))
		if !errors.Is(err, ErrVerificationFailed) {
			t.Fatalf("err = %v, want ErrVerificationFailed", err)
		}
		if store.cfg == nil || store.cfg.Password != "old-secret" {
			t.Errorf("stored configuration changed after a failed edit: %+v", store.cfg)
		}
	})

	t.Run("successful configure saves the normalized candidate", func(t *testing.T) {
		store := &fakeStore{}
		sender := &fakeSender{}
		svc := New(store, sender)

		settings, err := svc.Configure(context.Background(), authenticatedRequest(func(r *ConfigureRequest) {
			r.FromAddress = "Mailer@Example.com"
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if settings.Configuration == nil || settings.Configuration.FromAddress != "mailer@example.com" {
			t.Errorf("settings = %+v, want lowercased fromAddress", settings.Configuration)
		}
		if store.cfg == nil || store.cfg.Password != "s3cret-value" {
			t.Errorf("stored configuration = %+v", store.cfg)
		}
		if settings.Configuration.PasswordConfigured != true {
			t.Errorf("PasswordConfigured = %v, want true", settings.Configuration.PasswordConfigured)
		}
	})

	t.Run("omitted password on edit reuses the stored secret", func(t *testing.T) {
		store := &fakeStore{cfg: &emailapplication.SMTPConfiguration{
			Host: "smtp.example.com", Port: 587,
			TLSMode: emailapplication.TLSModeSTARTTLS, Authentication: emailapplication.AuthenticationPlain,
			Username: "mailer@example.com", Password: "existing-secret", FromAddress: "mailer@example.com",
		}}
		sender := &fakeSender{}
		svc := New(store, sender)

		_, err := svc.Configure(context.Background(), authenticatedRequest(func(r *ConfigureRequest) {
			r.Password = nil
			r.Port = 465
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(sender.verifiedWith) != 1 || sender.verifiedWith[0].Password != "existing-secret" {
			t.Errorf("verify was not called with the retained secret: %+v", sender.verifiedWith)
		}
		if store.cfg.Password != "existing-secret" || store.cfg.Port != 465 {
			t.Errorf("stored configuration = %+v", store.cfg)
		}
	})

	t.Run("first configure with an omitted password is rejected without a network call", func(t *testing.T) {
		store := &fakeStore{}
		sender := &fakeSender{}
		svc := New(store, sender)

		_, err := svc.Configure(context.Background(), authenticatedRequest(func(r *ConfigureRequest) {
			r.Password = nil
		}))
		if !errors.Is(err, ErrInvalidConfiguration) {
			t.Fatalf("err = %v, want ErrInvalidConfiguration", err)
		}
		if store.saveCalls != 0 || len(sender.verifiedWith) != 0 {
			t.Error("a rejected configure attempt reached the network or the store")
		}
	})

	t.Run("an explicitly empty password is rejected for authenticated mode", func(t *testing.T) {
		store := &fakeStore{cfg: &emailapplication.SMTPConfiguration{
			Host: "smtp.example.com", Port: 587, TLSMode: emailapplication.TLSModeSTARTTLS,
			Authentication: emailapplication.AuthenticationPlain, Username: "mailer@example.com",
			Password: "existing-secret", FromAddress: "mailer@example.com",
		}}
		svc := New(store, &fakeSender{})

		empty := ""
		_, err := svc.Configure(context.Background(), authenticatedRequest(func(r *ConfigureRequest) {
			r.Password = &empty
		}))
		if !errors.Is(err, ErrInvalidConfiguration) {
			t.Fatalf("err = %v, want ErrInvalidConfiguration", err)
		}
	})

	t.Run("switching to no authentication clears credentials", func(t *testing.T) {
		store := &fakeStore{cfg: &emailapplication.SMTPConfiguration{
			Host: "relay.internal", Port: 587, TLSMode: emailapplication.TLSModeSTARTTLS,
			Authentication: emailapplication.AuthenticationPlain, Username: "mailer@example.com",
			Password: "existing-secret", FromAddress: "mailer@example.com",
		}}
		svc := New(store, &fakeSender{})

		settings, err := svc.Configure(context.Background(), ConfigureRequest{
			Host: "relay.internal", Port: 25,
			TLSMode: emailapplication.TLSModeNone, Authentication: emailapplication.AuthenticationNone,
			FromAddress: "noreply@example.com",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if settings.Configuration.PasswordConfigured {
			t.Error("PasswordConfigured = true after switching to no authentication")
		}
		if store.cfg.Username != "" || store.cfg.Password != "" {
			t.Errorf("stored configuration retained credentials: %+v", store.cfg)
		}
	})

	t.Run("plaintext with authentication is rejected before dialing", func(t *testing.T) {
		sender := &fakeSender{}
		svc := New(&fakeStore{}, sender)

		_, err := svc.Configure(context.Background(), authenticatedRequest(func(r *ConfigureRequest) {
			r.TLSMode = emailapplication.TLSModeNone
		}))
		if !errors.Is(err, ErrInvalidConfiguration) {
			t.Fatalf("err = %v, want ErrInvalidConfiguration", err)
		}
		if len(sender.verifiedWith) != 0 {
			t.Error("plaintext + authentication reached the network")
		}
	})

	t.Run("invalid host is rejected", func(t *testing.T) {
		svc := New(&fakeStore{}, &fakeSender{})
		_, err := svc.Configure(context.Background(), authenticatedRequest(func(r *ConfigureRequest) {
			r.Host = "https://smtp.example.com/path"
		}))
		if !errors.Is(err, ErrInvalidConfiguration) {
			t.Fatalf("err = %v, want ErrInvalidConfiguration", err)
		}
	})

	t.Run("out-of-range port is rejected", func(t *testing.T) {
		svc := New(&fakeStore{}, &fakeSender{})
		_, err := svc.Configure(context.Background(), authenticatedRequest(func(r *ConfigureRequest) {
			r.Port = 0
		}))
		if !errors.Is(err, ErrInvalidConfiguration) {
			t.Fatalf("err = %v, want ErrInvalidConfiguration", err)
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

	t.Run("passes recipient through and uses the stored configuration", func(t *testing.T) {
		store := &fakeStore{cfg: &emailapplication.SMTPConfiguration{
			Host: "smtp.example.com", Port: 587, TLSMode: emailapplication.TLSModeSTARTTLS,
			Authentication: emailapplication.AuthenticationPlain, Username: "mailer@example.com",
			Password: "s3cret-value", FromAddress: "sender@example.com",
		}}
		sender := &fakeSender{}
		svc := New(store, sender)

		if err := svc.SendTest(context.Background(), "recipient@example.com"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(sender.sentMessages) != 1 || sender.sentMessages[0].To != "recipient@example.com" {
			t.Errorf("sent messages = %+v", sender.sentMessages)
		}
		if len(sender.sentTo) != 1 || sender.sentTo[0].FromAddress != "sender@example.com" {
			t.Errorf("sender received wrong configuration: %+v", sender.sentTo)
		}
	})
}

func TestServiceSettings(t *testing.T) {
	svc := New(&fakeStore{}, &fakeSender{})
	settings, err := svc.Settings(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if settings.Configuration != nil {
		t.Errorf("settings = %+v, want a nil Configuration", settings)
	}
}

func TestServiceNilStore(t *testing.T) {
	svc := New(nil, nil)

	if _, err := svc.Settings(context.Background()); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("Settings err = %v, want ErrNotConfigured", err)
	}
	if _, err := svc.Configure(context.Background(), ConfigureRequest{}); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("Configure err = %v, want ErrNotConfigured", err)
	}
	if err := svc.SendTest(context.Background(), "to@example.com"); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("SendTest err = %v, want ErrNotConfigured", err)
	}
	if err := svc.Disable(context.Background()); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("Disable err = %v, want ErrNotConfigured", err)
	}
}
