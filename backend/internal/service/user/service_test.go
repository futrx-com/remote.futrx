package user

import (
	"context"
	"errors"
	"testing"
)

type removalRepository struct {
	Repository
	users   map[string]User
	removed []string
}

func (r *removalRepository) Get(_ context.Context, email string) (*User, error) {
	user, exists := r.users[email]
	if !exists {
		return nil, nil
	}
	return &user, nil
}

func (r *removalRepository) Remove(_ context.Context, email string) error {
	r.removed = append(r.removed, email)
	delete(r.users, email)
	return nil
}

type removalCleanup struct {
	emails []string
	err    error
}

func (c *removalCleanup) CleanupRemovedUser(_ context.Context, email string) error {
	c.emails = append(c.emails, email)
	return c.err
}

func TestRemoveCleansIdentityResourcesBeforeDeletingTheUser(t *testing.T) {
	repo := &removalRepository{users: map[string]User{
		"member@example.com": {Email: "member@example.com", Role: RoleMember},
	}}
	cleanup := &removalCleanup{}
	service := New(repo, WithRemovalCleanup(cleanup))

	if err := service.Remove(context.Background(), " Member@Example.COM "); err != nil {
		t.Fatal(err)
	}
	if len(cleanup.emails) != 1 || cleanup.emails[0] != "member@example.com" {
		t.Fatalf("cleanup emails = %v", cleanup.emails)
	}
	if len(repo.removed) != 1 || repo.removed[0] != "member@example.com" {
		t.Fatalf("removed users = %v", repo.removed)
	}
}

func TestRemoveKeepsTheUserActiveWhenRevocationFails(t *testing.T) {
	repo := &removalRepository{users: map[string]User{
		"member@example.com": {Email: "member@example.com", Role: RoleMember},
	}}
	wantErr := errors.New("revoke failed")
	service := New(repo, WithRemovalCleanup(&removalCleanup{err: wantErr}))

	if err := service.Remove(context.Background(), "member@example.com"); !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want cleanup failure", err)
	}
	if len(repo.removed) != 0 {
		t.Fatalf("user was deleted before cleanup succeeded: %v", repo.removed)
	}
}
