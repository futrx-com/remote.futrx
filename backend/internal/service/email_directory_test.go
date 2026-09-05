package service

import (
	"context"
	"errors"
	"testing"

	serviceemail "github.com/futrx-com/remote.futrx.com/internal/service/email"
	serviceuser "github.com/futrx-com/remote.futrx.com/internal/service/user"
)

type directoryTestRepo struct {
	users   map[string]serviceuser.User
	failErr error // returned by Get for any email, instead of the normal lookup
}

func (r *directoryTestRepo) List(context.Context) ([]serviceuser.User, error) { return nil, nil }

func (r *directoryTestRepo) Get(_ context.Context, email string) (*serviceuser.User, error) {
	if r.failErr != nil {
		return nil, r.failErr
	}
	u, ok := r.users[email]
	if !ok {
		return nil, serviceuser.ErrUserNotFound
	}
	return &u, nil
}

func (r *directoryTestRepo) Add(context.Context, serviceuser.User) error { return nil }
func (r *directoryTestRepo) Remove(context.Context, string) error        { return nil }
func (r *directoryTestRepo) SetRole(context.Context, string, serviceuser.Role) error {
	return nil
}
func (r *directoryTestRepo) Count(context.Context) (int, error) { return 0, nil }

func TestEmailDirectoryUnknownUser(t *testing.T) {
	repo := &directoryTestRepo{users: map[string]serviceuser.User{}}
	dir := emailDirectory{users: serviceuser.New(repo)}

	_, err := dir.Address(context.Background(), "nobody@example.com")
	if !errors.Is(err, serviceemail.ErrUnknownRecipient) {
		t.Fatalf("err = %v, want ErrUnknownRecipient for a user that does not exist", err)
	}
}

func TestEmailDirectoryResolvesARegisteredUser(t *testing.T) {
	repo := &directoryTestRepo{users: map[string]serviceuser.User{
		"owner@example.com": {Email: "owner@example.com"},
	}}
	dir := emailDirectory{users: serviceuser.New(repo)}

	addr, err := dir.Address(context.Background(), "owner@example.com")
	if err != nil {
		t.Fatalf("Address returned %v", err)
	}
	if addr != "owner@example.com" {
		t.Errorf("addr = %q", addr)
	}
}

// TestEmailDirectoryDoesNotMaskInfrastructureErrors is the fix for a review
// finding: a store/repo failure (context deadline, disk error, ...) must not
// be reported as ErrUnknownRecipient, which a caller could mistake for "this
// user does not exist" and, for Required mail such as 2FA, wrongly treat as
// a permanent, non-retryable failure.
func TestEmailDirectoryDoesNotMaskInfrastructureErrors(t *testing.T) {
	infraErr := errors.New("user store unavailable")
	repo := &directoryTestRepo{failErr: infraErr}
	dir := emailDirectory{users: serviceuser.New(repo)}

	_, err := dir.Address(context.Background(), "owner@example.com")
	if !errors.Is(err, infraErr) {
		t.Fatalf("err = %v, want the underlying infrastructure error", err)
	}
	if errors.Is(err, serviceemail.ErrUnknownRecipient) {
		t.Error("an infrastructure failure must not be reported as ErrUnknownRecipient")
	}
}

func TestEmailDirectoryNilUsers(t *testing.T) {
	dir := emailDirectory{}
	_, err := dir.Address(context.Background(), "owner@example.com")
	if !errors.Is(err, serviceemail.ErrUnknownRecipient) {
		t.Fatalf("err = %v, want ErrUnknownRecipient when there is no user directory", err)
	}
}
