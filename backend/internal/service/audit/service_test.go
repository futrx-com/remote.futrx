package audit

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"
)

type fakeStore struct {
	mu        sync.Mutex
	entries   []Entry
	appendErr error
	pruned    []time.Time
	pruneN    int
}

func (s *fakeStore) Append(entry Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.appendErr != nil {
		return s.appendErr
	}
	s.entries = append(s.entries, entry)
	return nil
}

func (s *fakeStore) Query(query Query) (Page, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Page{Entries: append([]Entry(nil), s.entries...), NextCursor: query.Cursor}, nil
}

func (s *fakeStore) Export(context.Context, time.Time, time.Time, io.Writer) error {
	return nil
}

func (s *fakeStore) Prune(oldest time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruned = append(s.pruned, oldest)
	return s.pruneN, nil
}

func (s *fakeStore) recorded() []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Entry(nil), s.entries...)
}

func fixedClock(when time.Time) Option {
	return WithClock(func() time.Time { return when })
}

func TestRecordFillsMissingFieldsFromContext(t *testing.T) {
	store := &fakeStore{}
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	service := New(store, fixedClock(now))

	ctx := WithCaller(context.Background(), Caller{
		Actor:     Actor{Email: "Admin@Example.com", Sub: "local-admin", IsAdmin: true},
		IP:        "203.0.113.9",
		UserAgent: "curl/8",
	})
	service.Record(ctx, Success(ActionProjectCreate, Target{Type: TargetProject, ID: "p1"}, Meta{"slug": "demo"}))

	entries := store.recorded()
	if len(entries) != 1 {
		t.Fatalf("recorded %d entries, want 1", len(entries))
	}
	got := entries[0]
	if got.Actor.Email != "admin@example.com" {
		t.Fatalf("actor email = %q, want the normalized address", got.Actor.Email)
	}
	if got.Actor.Sub != "local-admin" || !got.Actor.IsAdmin {
		t.Fatalf("actor = %+v, want the context principal", got.Actor)
	}
	if got.IP != "203.0.113.9" || got.UserAgent != "curl/8" {
		t.Fatalf("request fields = %q/%q", got.IP, got.UserAgent)
	}
	if !got.At.Equal(now) {
		t.Fatalf("at = %s, want %s", got.At, now)
	}
	if !got.OK || got.Error != "" {
		t.Fatalf("entry = %+v, want a success", got)
	}
}

func TestRecordKeepsExplicitActorOverContext(t *testing.T) {
	store := &fakeStore{}
	service := New(store)

	ctx := WithCaller(context.Background(), Caller{
		Actor: Actor{Email: "someone@example.com"},
		IP:    "198.51.100.7",
	})
	entry := Result(ActionAuthLoginFailure, Target{Type: TargetSession}, nil, errors.New("bad password"))
	entry.Actor = Actor{Email: "attempted@example.com"}
	service.Record(ctx, entry)

	got := store.recorded()[0]
	if got.Actor.Email != "attempted@example.com" {
		t.Fatalf("actor = %q, want the explicitly supplied identity", got.Actor.Email)
	}
	// The network half still comes from the request context.
	if got.IP != "198.51.100.7" {
		t.Fatalf("ip = %q, want the context value", got.IP)
	}
	if got.OK || got.Error != "bad password" {
		t.Fatalf("entry = %+v, want the failure recorded", got)
	}
}

func TestRecordIgnoresUnusableEntriesAndStoreFailures(t *testing.T) {
	store := &fakeStore{}
	service := New(store)

	service.Record(context.Background(), Entry{Action: "   "})
	if len(store.recorded()) != 0 {
		t.Fatal("an entry without an action was recorded")
	}

	store.appendErr = errors.New("disk full")
	// A store failure must not panic or surface; Record has no error return
	// precisely so instrumented code cannot be broken by the audit log.
	service.Record(context.Background(), Success(ActionProjectDelete, Target{}, nil))

	var nilService *Service
	nilService.Record(context.Background(), Success(ActionProjectDelete, Target{}, nil))
}

func TestEnsureCallerDoesNotOverwriteAnExistingCaller(t *testing.T) {
	outer := WithCaller(context.Background(), Caller{Actor: Actor{Email: "first@example.com"}})
	inner := EnsureCaller(outer, Caller{Actor: Actor{Email: "second@example.com"}})

	caller, ok := CallerFrom(inner)
	if !ok {
		t.Fatal("no caller on the context")
	}
	if caller.Actor.Email != "first@example.com" {
		t.Fatalf("actor = %q, want the outermost resolution to win", caller.Actor.Email)
	}

	fresh := EnsureCaller(context.Background(), Caller{Actor: Actor{Email: "Third@Example.com"}})
	caller, _ = CallerFrom(fresh)
	if caller.Actor.Email != "third@example.com" {
		t.Fatalf("actor = %q, want the normalized new caller", caller.Actor.Email)
	}
}

func TestQueryNormalizesAndClampsLimits(t *testing.T) {
	store := &fakeStore{}

	cases := []struct {
		name string
		in   int
		want int
	}{
		{name: "default", in: 0, want: defaultQueryLimit},
		{name: "negative falls back to the default", in: -5, want: defaultQueryLimit},
		{name: "explicit", in: 10, want: 10},
		{name: "clamped", in: 100000, want: maxQueryLimit},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var seen Query
			probe := &queryProbe{store: store, seen: &seen}
			probeService := New(probe)
			if _, err := probeService.Query(context.Background(), Query{Limit: tc.in}); err != nil {
				t.Fatalf("Query: %v", err)
			}
			if seen.Limit != tc.want {
				t.Fatalf("limit = %d, want %d", seen.Limit, tc.want)
			}
		})
	}

	var seen Query
	probeService := New(&queryProbe{store: store, seen: &seen})
	if _, err := probeService.Query(context.Background(), Query{Actor: "  Admin@Example.com "}); err != nil {
		t.Fatalf("Query: %v", err)
	}
	if seen.Actor != "admin@example.com" {
		t.Fatalf("actor filter = %q, want it normalized like stored identities", seen.Actor)
	}
}

func TestQueryWithoutStoreReportsUnavailable(t *testing.T) {
	service := New(nil)
	if _, err := service.Query(context.Background(), Query{}); !errors.Is(err, ErrStoreUnavailable) {
		t.Fatalf("err = %v, want ErrStoreUnavailable", err)
	}
	if err := service.Export(context.Background(), time.Time{}, time.Time{}, io.Discard); !errors.Is(err, ErrStoreUnavailable) {
		t.Fatalf("export err = %v, want ErrStoreUnavailable", err)
	}
	// Recording against a missing store is a no-op, not a panic.
	service.Record(context.Background(), Success(ActionAuthLogout, Target{}, nil))
}

func TestPruneUsesTheRetentionWindow(t *testing.T) {
	now := time.Date(2026, 8, 17, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		name      string
		months    int
		wantPrune bool
		wantMonth time.Month
		wantYear  int
	}{
		{name: "default keeps twelve months", months: DefaultRetentionMonths, wantPrune: true, wantMonth: time.September, wantYear: 2025},
		{name: "one month keeps only the current file", months: 1, wantPrune: true, wantMonth: time.August, wantYear: 2026},
		{name: "zero disables pruning", months: 0, wantPrune: false},
		{name: "negative disables pruning", months: -3, wantPrune: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeStore{pruneN: 2}
			service := New(store, WithRetentionMonths(tc.months), fixedClock(now))
			removed, err := service.Prune()
			if err != nil {
				t.Fatalf("Prune: %v", err)
			}
			if !tc.wantPrune {
				if removed != 0 || len(store.pruned) != 0 {
					t.Fatalf("removed = %d, calls = %d, want pruning disabled", removed, len(store.pruned))
				}
				return
			}
			if removed != 2 {
				t.Fatalf("removed = %d, want the store's count", removed)
			}
			if len(store.pruned) != 1 {
				t.Fatalf("prune calls = %d, want 1", len(store.pruned))
			}
			oldest := store.pruned[0]
			if oldest.Month() != tc.wantMonth || oldest.Year() != tc.wantYear {
				t.Fatalf("oldest kept month = %s %d, want %s %d", oldest.Month(), oldest.Year(), tc.wantMonth, tc.wantYear)
			}
		})
	}
}

func TestRecorderOrNopReplacesNil(t *testing.T) {
	if _, ok := RecorderOrNop(nil).(Nop); !ok {
		t.Fatal("a nil recorder was not replaced by Nop")
	}
	service := New(&fakeStore{})
	if RecorderOrNop(service) != Recorder(service) {
		t.Fatal("a usable recorder was replaced")
	}
}

// queryProbe captures the normalized query the service hands to the store.
type queryProbe struct {
	store *fakeStore
	seen  *Query
}

func (p *queryProbe) Append(entry Entry) error { return p.store.Append(entry) }

func (p *queryProbe) Query(query Query) (Page, error) {
	*p.seen = query
	return Page{}, nil
}

func (p *queryProbe) Export(ctx context.Context, from, to time.Time, dst io.Writer) error {
	return p.store.Export(ctx, from, to, dst)
}

func (p *queryProbe) Prune(oldest time.Time) (int, error) { return p.store.Prune(oldest) }
