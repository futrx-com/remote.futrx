package share

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
)

const (
	projectOne = serviceproject.ID("aaaa1111")
	projectTwo = serviceproject.ID("bbbb2222")
	slugOne    = "alpha"
	slugTwo    = "beta"
)

func TestCreateValidatesInput(t *testing.T) {
	tests := []struct {
		name    string
		input   CreateInput
		project serviceproject.ID
		wantErr error
	}{
		{name: "defaults to 24h", input: CreateInput{Port: 3000}, project: projectOne},
		{name: "explicit ttl", input: CreateInput{Port: 3000, TTLHours: 168}, project: projectOne},
		{name: "max ttl", input: CreateInput{Port: 3000, TTLHours: 720}, project: projectOne},
		{
			name:    "port below range",
			input:   CreateInput{Port: 1023},
			project: projectOne,
			wantErr: ErrInvalidPort,
		},
		{
			name:    "port above range",
			input:   CreateInput{Port: 70000},
			project: projectOne,
			wantErr: ErrInvalidPort,
		},
		{
			name:    "agent browser port",
			input:   CreateInput{Port: AgentBrowserPort},
			project: projectOne,
			wantErr: ErrPortNotShareable,
		},
		{
			name:    "ttl below minimum",
			input:   CreateInput{Port: 3000, TTLHours: -1},
			project: projectOne,
			wantErr: ErrInvalidTTL,
		},
		{
			name:    "ttl above maximum",
			input:   CreateInput{Port: 3000, TTLHours: 721},
			project: projectOne,
			wantErr: ErrInvalidTTL,
		},
		{
			name:    "unknown project",
			input:   CreateInput{Port: 3000},
			project: serviceproject.ID("cccc3333"),
			wantErr: serviceproject.ErrNotFound,
		},
		{
			name:    "malformed project id",
			input:   CreateInput{Port: 3000},
			project: serviceproject.ID("nope"),
			wantErr: serviceproject.ErrInvalidID,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service, _ := newTestService(t)
			created, err := service.Create(
				context.Background(), test.project, test.input, "Owner@Example.com",
			)
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("Create error = %v, want %v", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Create: %v", err)
			}
			if len(created.Token) < 43 {
				t.Fatalf("token length = %d, want at least 43 (32 random bytes)", len(created.Token))
			}
			if created.Slug != slugOne {
				t.Fatalf("slug = %q, want %q", created.Slug, slugOne)
			}
			if created.Share.CreatedBy != "owner@example.com" {
				t.Fatalf("createdBy = %q, want it normalized", created.Share.CreatedBy)
			}
			wantTTL := DefaultTTL
			if test.input.TTLHours != 0 {
				wantTTL = time.Duration(test.input.TTLHours) * time.Hour
			}
			gotTTL := time.UnixMilli(created.Share.ExpiresAt).Sub(time.UnixMilli(created.Share.CreatedAt))
			if gotTTL != wantTTL {
				t.Fatalf("ttl = %s, want %s", gotTTL, wantTTL)
			}
		})
	}
}

func TestCreateStoresOnlyTheTokenDigest(t *testing.T) {
	service, repo := newTestService(t)
	created, err := service.Create(
		context.Background(), projectOne, CreateInput{Port: 3000}, "owner@example.com",
	)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	stored := repo.snapshot(projectOne)
	if len(stored) != 1 {
		t.Fatalf("stored %d shares, want 1", len(stored))
	}
	digest := sha256.Sum256([]byte(created.Token))
	if stored[0].TokenHash != hex.EncodeToString(digest[:]) {
		t.Fatalf("tokenHash = %q, want the SHA-256 of the token", stored[0].TokenHash)
	}
	if strings.Contains(repo.encoded(projectOne), created.Token) {
		t.Fatal("the plaintext token reached the store")
	}
}

func TestCreateBoundsLabelAndLinkCount(t *testing.T) {
	service, _ := newTestService(t)
	created, err := service.Create(context.Background(), projectOne, CreateInput{
		Port:  3000,
		Label: "  client\ndemo" + strings.Repeat("x", MaxLabelLength),
	}, "owner@example.com")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len([]rune(created.Share.Label)) > MaxLabelLength {
		t.Fatalf("label length = %d, want at most %d", len(created.Share.Label), MaxLabelLength)
	}
	if strings.ContainsAny(created.Share.Label, "\n\r") {
		t.Fatalf("label = %q, want newlines flattened", created.Share.Label)
	}

	for i := 1; i < MaxPerProject; i++ {
		if _, err := service.Create(
			context.Background(), projectOne, CreateInput{Port: 3000}, "owner@example.com",
		); err != nil {
			t.Fatalf("Create number %d: %v", i+1, err)
		}
	}
	if _, err := service.Create(
		context.Background(), projectOne, CreateInput{Port: 3000}, "owner@example.com",
	); !errors.Is(err, ErrTooManyShares) {
		t.Fatalf("Create beyond the cap = %v, want %v", err, ErrTooManyShares)
	}
}

func TestValidate(t *testing.T) {
	base := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	now := base
	service, _ := newTestService(t, WithClock(func() time.Time { return now }))

	created, err := service.Create(
		context.Background(), projectOne, CreateInput{Port: 3000}, "owner@example.com",
	)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	otherProject, err := service.Create(
		context.Background(), projectTwo, CreateInput{Port: 3000}, "owner@example.com",
	)
	if err != nil {
		t.Fatalf("Create for second project: %v", err)
	}

	tests := []struct {
		name  string
		slug  string
		port  int
		token string
		at    time.Time
		want  bool
	}{
		{
			name: "matching slug port and token",
			slug: slugOne, port: 3000, token: created.Token, at: base, want: true,
		},
		{name: "wrong port", slug: slugOne, port: 3001, token: created.Token, at: base},
		{name: "wrong slug", slug: slugTwo, port: 3000, token: created.Token, at: base},
		{name: "unknown slug", slug: "ghost", port: 3000, token: created.Token, at: base},
		{name: "empty slug", slug: "", port: 3000, token: created.Token, at: base},
		{name: "empty token", slug: slugOne, port: 3000, token: "", at: base},
		{name: "wrong token", slug: slugOne, port: 3000, token: "not-the-token", at: base},
		{
			name: "another project's token",
			slug: slugOne, port: 3000, token: otherProject.Token, at: base,
		},
		{
			name: "agent browser port",
			slug: slugOne, port: AgentBrowserPort, token: created.Token, at: base,
		},
		{
			name: "one second before expiry",
			slug: slugOne, port: 3000, token: created.Token,
			at: base.Add(DefaultTTL - time.Second), want: true,
		},
		{
			name: "after expiry",
			slug: slugOne, port: 3000, token: created.Token,
			at: base.Add(DefaultTTL + time.Second),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			now = test.at
			_, got := service.Validate(context.Background(), test.slug, test.port, test.token)
			if got != test.want {
				t.Fatalf("Validate(%q, %d) = %v, want %v", test.slug, test.port, got, test.want)
			}
		})
	}
}

func TestRevokeStopsValidationAndListing(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	service, repo := newTestService(t, WithClock(func() time.Time { return now }))
	ctx := context.Background()

	created, err := service.Create(ctx, projectOne, CreateInput{Port: 3000}, "owner@example.com")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, ok := service.Validate(ctx, slugOne, 3000, created.Token); !ok {
		t.Fatal("Validate before revoke = false, want true")
	}
	if !service.Allows(ctx, slugOne, 3000, created.Share.ID) {
		t.Fatal("Allows before revoke = false, want true")
	}

	if err := service.Revoke(ctx, projectOne, created.Share.ID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if _, ok := service.Validate(ctx, slugOne, 3000, created.Token); ok {
		t.Fatal("Validate after revoke = true, want false")
	}
	if service.Allows(ctx, slugOne, 3000, created.Share.ID) {
		t.Fatal("Allows after revoke = true, want false")
	}
	list, err := service.List(ctx, projectOne)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("List after revoke returned %d shares, want 0", len(list))
	}
	if stored := repo.snapshot(projectOne); len(stored) != 1 || stored[0].RevokedAt != now.UnixMilli() {
		t.Fatalf("stored record after revoke = %#v, want revokedAt stamped", stored)
	}
	if err := service.Revoke(ctx, projectOne, created.Share.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second Revoke = %v, want %v", err, ErrNotFound)
	}
	if err := service.Revoke(ctx, projectOne, ID("missing")); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Revoke of an unknown id = %v, want %v", err, ErrNotFound)
	}
}

func TestListReturnsLiveSharesNewestFirst(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	service, _ := newTestService(t, WithClock(func() time.Time { return now }))
	ctx := context.Background()

	short, err := service.Create(ctx, projectOne, CreateInput{Port: 3000, TTLHours: 1}, "owner@example.com")
	if err != nil {
		t.Fatalf("Create short: %v", err)
	}
	now = now.Add(time.Minute)
	long, err := service.Create(ctx, projectOne, CreateInput{Port: 5173, TTLHours: 168}, "owner@example.com")
	if err != nil {
		t.Fatalf("Create long: %v", err)
	}

	list, err := service.List(ctx, projectOne)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 || list[0].ID != long.Share.ID || list[1].ID != short.Share.ID {
		t.Fatalf("List = %#v, want the newest first", list)
	}

	now = now.Add(2 * time.Hour)
	list, err = service.List(ctx, projectOne)
	if err != nil {
		t.Fatalf("List after expiry: %v", err)
	}
	if len(list) != 1 || list[0].ID != long.Share.ID {
		t.Fatalf("List after expiry = %#v, want only the 7-day link", list)
	}
}

func newTestService(t *testing.T, options ...Option) (*Service, *memoryRepo) {
	t.Helper()
	repo := &memoryRepo{shares: map[serviceproject.ID][]Share{}}
	projects := &fakeProjects{metas: []serviceproject.Meta{
		{ID: projectOne, Slug: slugOne},
		{ID: projectTwo, Slug: slugTwo},
	}}
	return New(repo, projects, options...), repo
}

type fakeProjects struct {
	metas []serviceproject.Meta
}

func (p *fakeProjects) Get(_ context.Context, id serviceproject.ID) (serviceproject.Meta, error) {
	if !serviceproject.ValidID(id) {
		return serviceproject.Meta{}, serviceproject.ErrInvalidID
	}
	for _, meta := range p.metas {
		if meta.ID == id {
			return meta, nil
		}
	}
	return serviceproject.Meta{}, serviceproject.ErrNotFound
}

func (p *fakeProjects) GetBySlug(_ context.Context, slug string) (serviceproject.Meta, error) {
	for _, meta := range p.metas {
		if meta.Slug == slug {
			return meta, nil
		}
	}
	return serviceproject.Meta{}, serviceproject.ErrNotFound
}

type memoryRepo struct {
	mu     sync.Mutex
	shares map[serviceproject.ID][]Share
}

func (r *memoryRepo) List(_ context.Context, projectID serviceproject.ID) ([]Share, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Share(nil), r.shares[projectID]...), nil
}

func (r *memoryRepo) Update(
	_ context.Context,
	projectID serviceproject.ID,
	fn func([]Share) ([]Share, error),
) ([]Share, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	next, err := fn(append([]Share(nil), r.shares[projectID]...))
	if err != nil {
		return nil, err
	}
	r.shares[projectID] = next
	return next, nil
}

func (r *memoryRepo) snapshot(projectID serviceproject.ID) []Share {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Share(nil), r.shares[projectID]...)
}

// encoded renders everything the repository holds, so a test can assert no
// plaintext token is anywhere inside it.
func (r *memoryRepo) encoded(projectID serviceproject.ID) string {
	var builder strings.Builder
	for _, record := range r.snapshot(projectID) {
		builder.WriteString(string(record.ID))
		builder.WriteString(record.TokenHash)
		builder.WriteString(record.Label)
		builder.WriteString(record.CreatedBy)
	}
	return builder.String()
}
