package user

import (
	"context"
	"regexp"
	"time"
)

// emailPattern is intentionally permissive: anything with a non-empty local
// part, an "@", and a domain with at least one dot. The real source of truth
// is the OAuth provider — this is just a sanity check before we store an
// admin-entered email.
var emailPattern = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

type Service struct {
	repo    Repository
	cleanup RemovalCleanup
}

// Option configures an optional user-service collaborator.
type Option func(*Service)

// WithRemovalCleanup revokes identity-keyed resources before user deletion.
func WithRemovalCleanup(cleanup RemovalCleanup) Option {
	return func(service *Service) {
		service.cleanup = cleanup
	}
}

func New(repo Repository, options ...Option) *Service {
	service := &Service{repo: repo}
	for _, option := range options {
		option(service)
	}
	return service
}

func (s *Service) List(ctx context.Context) ([]User, error) {
	if s == nil || s.repo == nil {
		return nil, nil
	}
	return s.repo.List(ctx)
}

func (s *Service) Get(ctx context.Context, email string) (*User, error) {
	if s == nil || s.repo == nil {
		return nil, ErrUserNotFound
	}
	email = NormalizeEmail(email)
	if !emailPattern.MatchString(email) {
		return nil, ErrInvalidEmail
	}
	return s.repo.Get(ctx, email)
}

func (s *Service) IsRegistered(ctx context.Context, email string) (bool, error) {
	if s == nil || s.repo == nil {
		return false, nil
	}
	email = NormalizeEmail(email)
	if email == "" {
		return false, nil
	}
	u, err := s.repo.Get(ctx, email)
	if err != nil {
		return false, err
	}
	return u != nil, nil
}

func (s *Service) IsAdmin(ctx context.Context, email string) (bool, error) {
	if s == nil || s.repo == nil {
		return false, nil
	}
	email = NormalizeEmail(email)
	if email == "" {
		return false, nil
	}
	u, err := s.repo.Get(ctx, email)
	if err != nil {
		return false, err
	}
	return u != nil && u.Role == RoleAdmin, nil
}

// Add validates email + role, lowercases, fails if the user already exists.
// addedBy is the admin who initiated the add (empty for bootstrap).
func (s *Service) Add(ctx context.Context, email string, role Role, addedBy string) (User, error) {
	if s == nil || s.repo == nil {
		return User{}, ErrUserNotFound
	}
	email = NormalizeEmail(email)
	if !emailPattern.MatchString(email) {
		return User{}, ErrInvalidEmail
	}
	if !ValidRole(role) {
		return User{}, ErrInvalidRole
	}
	existing, err := s.repo.Get(ctx, email)
	if err != nil {
		return User{}, err
	}
	if existing != nil {
		return User{}, ErrUserExists
	}
	u := User{
		Email:   email,
		Role:    role,
		AddedAt: time.Now().UnixMilli(),
		AddedBy: NormalizeEmail(addedBy),
	}
	if err := s.repo.Add(ctx, u); err != nil {
		return User{}, err
	}
	return u, nil
}

// Remove refuses to delete the last admin so the box can't lock its owners out.
func (s *Service) Remove(ctx context.Context, email string) error {
	if s == nil || s.repo == nil {
		return ErrUserNotFound
	}
	email = NormalizeEmail(email)
	if !emailPattern.MatchString(email) {
		return ErrInvalidEmail
	}
	existing, err := s.repo.Get(ctx, email)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrUserNotFound
	}
	if existing.Role == RoleAdmin {
		admins, err := s.countAdmins(ctx)
		if err != nil {
			return err
		}
		if admins <= 1 {
			return ErrCannotRemoveLastAdmin
		}
	}
	if s.cleanup != nil {
		if err := s.cleanup.CleanupRemovedUser(ctx, email); err != nil {
			return err
		}
	}
	return s.repo.Remove(ctx, email)
}

// SetRole refuses to demote the last admin.
func (s *Service) SetRole(ctx context.Context, email string, role Role) (User, error) {
	if s == nil || s.repo == nil {
		return User{}, ErrUserNotFound
	}
	email = NormalizeEmail(email)
	if !emailPattern.MatchString(email) {
		return User{}, ErrInvalidEmail
	}
	if !ValidRole(role) {
		return User{}, ErrInvalidRole
	}
	existing, err := s.repo.Get(ctx, email)
	if err != nil {
		return User{}, err
	}
	if existing == nil {
		return User{}, ErrUserNotFound
	}
	if existing.Role == RoleAdmin && role != RoleAdmin {
		admins, err := s.countAdmins(ctx)
		if err != nil {
			return User{}, err
		}
		if admins <= 1 {
			return User{}, ErrCannotDemoteLastAdmin
		}
	}
	if err := s.repo.SetRole(ctx, email, role); err != nil {
		return User{}, err
	}
	updated := *existing
	updated.Role = role
	return updated, nil
}

// Count returns the total number of registered users.
func (s *Service) Count(ctx context.Context) (int, error) {
	if s == nil || s.repo == nil {
		return 0, nil
	}
	return s.repo.Count(ctx)
}

func (s *Service) countAdmins(ctx context.Context) (int, error) {
	all, err := s.repo.List(ctx)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, u := range all {
		if u.Role == RoleAdmin {
			n++
		}
	}
	return n, nil
}
