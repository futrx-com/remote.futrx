package audit

import (
	"context"
	"errors"
	"io"
	"log"
	"strings"
	"time"
)

// DefaultRetentionMonths is the number of monthly files kept when
// AUDIT_RETENTION_MONTHS is unset.
const DefaultRetentionMonths = 12

const (
	defaultQueryLimit = 50
	maxQueryLimit     = 500
	maxErrorLength    = 512
)

// ErrStoreUnavailable is returned by the read API when the service was built
// without a store.
var ErrStoreUnavailable = errors.New("audit log is unavailable")

// Query filters the audit log. Zero values mean "no filter". Results are
// always newest first.
type Query struct {
	// Actor matches one principal's email exactly (case-insensitive).
	Actor string
	// Action matches on prefix: "project." selects every project action.
	Action string
	// Target matches Target.ID exactly.
	Target string
	// From and To bound Entry.At; To is exclusive.
	From time.Time
	To   time.Time
	// Limit caps the page size (default 50, max 500).
	Limit int
	// Cursor resumes below the last entry of a previous page.
	Cursor string
}

// Page is one newest-first slice of the log. NextCursor is empty when the
// filtered range is exhausted.
type Page struct {
	Entries    []Entry `json:"entries"`
	NextCursor string  `json:"nextCursor,omitempty"`
}

// Store is the persistence port. The file-backed implementation lives in
// internal/stores/fileaudit.
type Store interface {
	// Append adds one entry to the current month's file.
	Append(entry Entry) error
	// Query returns a newest-first page, reading only the monthly files the
	// time range and cursor require.
	Query(query Query) (Page, error)
	// Export streams matching raw JSONL lines oldest-first.
	Export(ctx context.Context, from, to time.Time, dst io.Writer) error
	// Prune deletes whole monthly files older than the month containing
	// `oldest`, returning how many files it removed.
	Prune(oldest time.Time) (int, error)
}

type Option func(*Service)

// WithRetentionMonths sets how many monthly files the janitor keeps. Values
// below 1 disable pruning entirely.
func WithRetentionMonths(months int) Option {
	return func(s *Service) { s.retentionMonths = months }
}

// WithClock overrides time.Now, for tests.
func WithClock(now func() time.Time) Option {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}

// Service is the audit log's read/write facade. It is safe for concurrent use
// and never returns a write error to instrumented code.
type Service struct {
	store           Store
	retentionMonths int
	now             func() time.Time
}

var _ Recorder = (*Service)(nil)

func New(store Store, options ...Option) *Service {
	service := &Service{
		store:           store,
		retentionMonths: DefaultRetentionMonths,
		now:             time.Now,
	}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service
}

// Record writes one entry. Missing fields are filled from ctx: the caller
// stashed by the transport layer supplies actor, IP, and user agent, and an
// unset At becomes now. A write failure is logged and dropped — auditing must
// never turn a working request into an error.
func (s *Service) Record(ctx context.Context, entry Entry) {
	if s == nil || s.store == nil {
		return
	}
	entry.Action = strings.TrimSpace(entry.Action)
	if entry.Action == "" {
		return
	}
	if entry.At.IsZero() {
		entry.At = s.now()
	}
	entry.At = entry.At.UTC()
	if caller, ok := CallerFrom(ctx); ok {
		if entry.Actor.Empty() {
			entry.Actor = caller.Actor
		}
		if entry.IP == "" {
			entry.IP = caller.IP
		}
		if entry.UserAgent == "" {
			entry.UserAgent = caller.UserAgent
		}
	}
	entry.Actor.Email = NormalizeActorEmail(entry.Actor.Email)
	entry.Error = truncate(entry.Error, maxErrorLength)
	if err := s.store.Append(entry); err != nil {
		log.Printf("audit: drop %s entry: %v", entry.Action, err)
	}
}

// Query returns a newest-first page of the log.
func (s *Service) Query(_ context.Context, query Query) (Page, error) {
	if s == nil || s.store == nil {
		return Page{}, ErrStoreUnavailable
	}
	query.Actor = NormalizeActorEmail(query.Actor)
	query.Action = strings.TrimSpace(query.Action)
	query.Target = strings.TrimSpace(query.Target)
	query.Cursor = strings.TrimSpace(query.Cursor)
	if query.Limit <= 0 {
		query.Limit = defaultQueryLimit
	}
	if query.Limit > maxQueryLimit {
		query.Limit = maxQueryLimit
	}
	page, err := s.store.Query(query)
	if err != nil {
		return Page{}, err
	}
	if page.Entries == nil {
		page.Entries = []Entry{}
	}
	return page, nil
}

// Export streams the raw JSONL for a time range, oldest first, so an operator
// can archive or grep the log outside the UI.
func (s *Service) Export(ctx context.Context, from, to time.Time, dst io.Writer) error {
	if s == nil || s.store == nil {
		return ErrStoreUnavailable
	}
	return s.store.Export(ctx, from, to, dst)
}

// RetentionMonths reports the configured retention window.
func (s *Service) RetentionMonths() int {
	if s == nil {
		return 0
	}
	return s.retentionMonths
}

// Prune deletes monthly files that fell out of the retention window.
func (s *Service) Prune() (int, error) {
	if s == nil || s.store == nil {
		return 0, ErrStoreUnavailable
	}
	if s.retentionMonths < 1 {
		return 0, nil
	}
	oldest := s.now().UTC().AddDate(0, -(s.retentionMonths - 1), 0)
	return s.store.Prune(oldest)
}

// StartJanitor prunes once now and then on every tick until ctx is done. The
// deployment calls it with 24h; tests pass something shorter.
func (s *Service) StartJanitor(ctx context.Context, interval time.Duration) {
	if s == nil || s.store == nil || s.retentionMonths < 1 {
		return
	}
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			if removed, err := s.Prune(); err != nil {
				log.Printf("audit: prune: %v", err)
			} else if removed > 0 {
				log.Printf("audit: pruned %d monthly file(s) past retention", removed)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}
