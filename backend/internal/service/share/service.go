package share

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"sort"
	"strings"
	"time"

	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
)

// Service is the policy layer over share links: it mints tokens, bounds their
// lifetime, and answers the edge's "may this anonymous request through?"
// question.
type Service struct {
	repo     Repository
	projects Projects
	now      func() time.Time
}

// Option customizes a Service at construction.
type Option func(*Service)

// WithClock replaces the wall clock, so expiry behavior is testable.
func WithClock(now func() time.Time) Option {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}

func New(repo Repository, projects Projects, options ...Option) *Service {
	service := &Service{repo: repo, projects: projects, now: time.Now}
	for _, option := range options {
		option(service)
	}
	return service
}

// Create mints one share link for a project's preview port. The plaintext
// token is returned exactly once; only its digest reaches the store.
func (s *Service) Create(
	ctx context.Context,
	projectID serviceproject.ID,
	input CreateInput,
	createdBy string,
) (Created, error) {
	if s == nil || s.repo == nil {
		return Created{}, ErrUnavailable
	}
	if !serviceproject.ValidID(projectID) {
		return Created{}, serviceproject.ErrInvalidID
	}
	if err := ShareablePort(input.Port); err != nil {
		return Created{}, err
	}
	ttl, err := resolveTTL(input.TTLHours)
	if err != nil {
		return Created{}, err
	}
	project, err := s.projects.Get(ctx, projectID)
	if err != nil {
		return Created{}, err
	}

	token, err := newToken()
	if err != nil {
		return Created{}, err
	}
	id, err := newID()
	if err != nil {
		return Created{}, err
	}

	now := s.now()
	record := Share{
		ID:        id,
		TokenHash: hashToken(token),
		Port:      input.Port,
		Label:     sanitizeLabel(input.Label),
		CreatedBy: strings.ToLower(strings.TrimSpace(createdBy)),
		CreatedAt: now.UnixMilli(),
		ExpiresAt: now.Add(ttl).UnixMilli(),
	}

	if _, err := s.repo.Update(ctx, projectID, func(stored []Share) ([]Share, error) {
		live := activeOnly(stored, now.UnixMilli())
		if len(live) >= MaxPerProject {
			return nil, ErrTooManyShares
		}
		return append(live, record), nil
	}); err != nil {
		return Created{}, err
	}

	return Created{Share: record, Token: token, Slug: project.Slug}, nil
}

// List returns the project's still-usable links, newest first. Expired and
// revoked records are storage detail and never surface.
func (s *Service) List(ctx context.Context, projectID serviceproject.ID) ([]Share, error) {
	if s == nil || s.repo == nil {
		return nil, ErrUnavailable
	}
	if !serviceproject.ValidID(projectID) {
		return nil, serviceproject.ErrInvalidID
	}
	if _, err := s.projects.Get(ctx, projectID); err != nil {
		return nil, err
	}
	stored, err := s.repo.List(ctx, projectID)
	if err != nil {
		return nil, err
	}
	live := activeOnly(stored, s.now().UnixMilli())
	sort.SliceStable(live, func(i, j int) bool {
		return live[i].CreatedAt > live[j].CreatedAt
	})
	return live, nil
}

// Revoke closes one link immediately. Revoking an already-revoked or expired
// link reports ErrNotFound, so the caller sees the same outcome as a bad id.
func (s *Service) Revoke(ctx context.Context, projectID serviceproject.ID, id ID) error {
	if s == nil || s.repo == nil {
		return ErrUnavailable
	}
	if !serviceproject.ValidID(projectID) {
		return serviceproject.ErrInvalidID
	}
	if id == "" {
		return ErrNotFound
	}
	if _, err := s.projects.Get(ctx, projectID); err != nil {
		return err
	}
	now := s.now().UnixMilli()
	_, err := s.repo.Update(ctx, projectID, func(stored []Share) ([]Share, error) {
		next := make([]Share, len(stored))
		copy(next, stored)
		for index := range next {
			if next[index].ID != id || !next[index].Active(now) {
				continue
			}
			next[index].RevokedAt = now
			return next, nil
		}
		return nil, ErrNotFound
	})
	return err
}

// Validate answers the first hop of edge authorization: does this plaintext
// token grant access to exactly this preview host and port?
func (s *Service) Validate(
	ctx context.Context,
	slug string,
	port int,
	token string,
) (Share, bool) {
	if s == nil || s.repo == nil || token == "" {
		return Share{}, false
	}
	if err := ShareablePort(port); err != nil {
		return Share{}, false
	}
	shares, ok := s.sharesForSlug(ctx, slug)
	if !ok {
		return Share{}, false
	}
	digest := hashToken(token)
	now := s.now().UnixMilli()
	for _, candidate := range shares {
		if candidate.Port != port || !candidate.Active(now) {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(candidate.TokenHash), []byte(digest)) == 1 {
			return candidate, true
		}
	}
	return Share{}, false
}

// Allows answers the repeat-visit hop: the visitor already exchanged a token
// for a signed cookie, so only the link's continued existence is in question.
// Re-reading the store here is what makes Revoke take effect immediately.
func (s *Service) Allows(ctx context.Context, slug string, port int, id ID) bool {
	if s == nil || s.repo == nil || id == "" {
		return false
	}
	if err := ShareablePort(port); err != nil {
		return false
	}
	shares, ok := s.sharesForSlug(ctx, slug)
	if !ok {
		return false
	}
	now := s.now().UnixMilli()
	for _, candidate := range shares {
		if candidate.ID == id && candidate.Port == port && candidate.Active(now) {
			return true
		}
	}
	return false
}

func (s *Service) sharesForSlug(ctx context.Context, slug string) ([]Share, bool) {
	slug = strings.ToLower(strings.TrimSpace(slug))
	if slug == "" || s.projects == nil {
		return nil, false
	}
	project, err := s.projects.GetBySlug(ctx, slug)
	if err != nil {
		return nil, false
	}
	shares, err := s.repo.List(ctx, project.ID)
	if err != nil {
		return nil, false
	}
	return shares, true
}

func resolveTTL(hours int) (time.Duration, error) {
	if hours == 0 {
		return DefaultTTL, nil
	}
	ttl := time.Duration(hours) * time.Hour
	if ttl < MinTTL || ttl > MaxTTL {
		return 0, ErrInvalidTTL
	}
	return ttl, nil
}

func activeOnly(shares []Share, nowMilli int64) []Share {
	live := make([]Share, 0, len(shares))
	for _, candidate := range shares {
		if candidate.Active(nowMilli) {
			live = append(live, candidate)
		}
	}
	return live
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func newToken() (string, error) {
	buf := make([]byte, TokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func newID() (ID, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return ID(hex.EncodeToString(buf)), nil
}

// sanitizeLabel keeps operator notes to a single printable line so they stay
// safe to render and cheap to store.
func sanitizeLabel(label string) string {
	cleaned := strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' {
			return ' '
		}
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, label)
	cleaned = strings.TrimSpace(cleaned)
	runes := []rune(cleaned)
	if len(runes) > MaxLabelLength {
		cleaned = strings.TrimSpace(string(runes[:MaxLabelLength]))
	}
	return cleaned
}
