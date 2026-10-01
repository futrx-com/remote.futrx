package fileaudit

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	serviceaudit "github.com/futrx-com/remote.futrx.com/internal/service/audit"
)

func at(month time.Month, day, hour int) time.Time {
	return time.Date(2026, month, day, hour, 0, 0, 0, time.UTC)
}

func entry(when time.Time, actor, action, target string) serviceaudit.Entry {
	return serviceaudit.Entry{
		At:     when,
		Actor:  serviceaudit.Actor{Email: actor},
		Action: action,
		Target: serviceaudit.Target{Type: "project", ID: target},
		OK:     true,
	}
}

func newStore(t *testing.T) *Store {
	t.Helper()
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return store
}

func seed(t *testing.T, store *Store, entries ...serviceaudit.Entry) {
	t.Helper()
	for _, e := range entries {
		if err := store.Append(e); err != nil {
			t.Fatalf("Append %s: %v", e.Action, err)
		}
	}
}

func actions(page serviceaudit.Page) []string {
	out := make([]string, 0, len(page.Entries))
	for _, e := range page.Entries {
		out = append(out, e.Action)
	}
	return out
}

func TestAppendRotatesByMonthAndReadsNewestFirst(t *testing.T) {
	store := newStore(t)
	seed(t, store,
		entry(at(time.June, 1, 9), "a@example.com", "project.create", "p1"),
		entry(at(time.July, 2, 9), "b@example.com", "project.delete", "p1"),
		entry(at(time.August, 3, 9), "a@example.com", "auth.login.success", "p2"),
	)

	for _, month := range []string{"2026-06", "2026-07", "2026-08"} {
		if _, err := os.Stat(filepath.Join(store.root, "audit-"+month+".jsonl")); err != nil {
			t.Fatalf("expected monthly file for %s: %v", month, err)
		}
	}

	page, err := store.Query(serviceaudit.Query{Limit: 10})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	want := []string{"auth.login.success", "project.delete", "project.create"}
	if got := actions(page); !equalStrings(got, want) {
		t.Fatalf("actions = %v, want %v", got, want)
	}
	if page.NextCursor != "" {
		t.Fatalf("NextCursor = %q, want empty for an exhausted range", page.NextCursor)
	}
}

func TestQueryFilters(t *testing.T) {
	store := newStore(t)
	seed(t, store,
		entry(at(time.June, 1, 9), "a@example.com", "project.create", "p1"),
		entry(at(time.June, 2, 9), "b@example.com", "project.secret.read", "p1"),
		entry(at(time.July, 1, 9), "a@example.com", "project.secret.set", "p2"),
		entry(at(time.July, 2, 9), "a@example.com", "auth.login.failure", "p2"),
	)

	cases := []struct {
		name  string
		query serviceaudit.Query
		want  []string
	}{
		{
			name:  "actor",
			query: serviceaudit.Query{Actor: "b@example.com", Limit: 10},
			want:  []string{"project.secret.read"},
		},
		{
			name:  "action prefix selects a whole family",
			query: serviceaudit.Query{Action: "project.secret.", Limit: 10},
			want:  []string{"project.secret.set", "project.secret.read"},
		},
		{
			name:  "target id",
			query: serviceaudit.Query{Target: "p2", Limit: 10},
			want:  []string{"auth.login.failure", "project.secret.set"},
		},
		{
			name:  "time range excludes the upper bound",
			query: serviceaudit.Query{From: at(time.June, 2, 0), To: at(time.July, 2, 9), Limit: 10},
			want:  []string{"project.secret.set", "project.secret.read"},
		},
		{
			name:  "combined actor and prefix",
			query: serviceaudit.Query{Actor: "a@example.com", Action: "project.", Limit: 10},
			want:  []string{"project.secret.set", "project.create"},
		},
		{
			name:  "no match",
			query: serviceaudit.Query{Actor: "nobody@example.com", Limit: 10},
			want:  []string{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			page, err := store.Query(tc.query)
			if err != nil {
				t.Fatalf("Query: %v", err)
			}
			if got := actions(page); !equalStrings(got, tc.want) {
				t.Fatalf("actions = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestQueryPaginatesNewestFirstAcrossMonths(t *testing.T) {
	store := newStore(t)
	seed(t, store,
		entry(at(time.June, 1, 9), "a@example.com", "one", "p1"),
		entry(at(time.June, 2, 9), "a@example.com", "two", "p1"),
		entry(at(time.July, 1, 9), "a@example.com", "three", "p1"),
		entry(at(time.July, 2, 9), "a@example.com", "four", "p1"),
		entry(at(time.July, 3, 9), "a@example.com", "five", "p1"),
	)

	var seen []string
	cursor := ""
	for page := 0; page < 5; page++ {
		result, err := store.Query(serviceaudit.Query{Limit: 2, Cursor: cursor})
		if err != nil {
			t.Fatalf("Query page %d: %v", page, err)
		}
		seen = append(seen, actions(result)...)
		cursor = result.NextCursor
		if cursor == "" {
			break
		}
	}
	want := []string{"five", "four", "three", "two", "one"}
	if !equalStrings(seen, want) {
		t.Fatalf("paged actions = %v, want %v", seen, want)
	}
}

func TestQueryPaginationKeepsFilterAcrossPages(t *testing.T) {
	store := newStore(t)
	seed(t, store,
		entry(at(time.June, 1, 9), "a@example.com", "project.create", "p1"),
		entry(at(time.June, 1, 10), "b@example.com", "project.create", "p1"),
		entry(at(time.June, 1, 11), "a@example.com", "project.delete", "p1"),
		entry(at(time.June, 1, 12), "a@example.com", "project.rename", "p1"),
	)

	first, err := store.Query(serviceaudit.Query{Actor: "a@example.com", Limit: 2})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if got := actions(first); !equalStrings(got, []string{"project.rename", "project.delete"}) {
		t.Fatalf("first page = %v", got)
	}
	second, err := store.Query(serviceaudit.Query{Actor: "a@example.com", Limit: 2, Cursor: first.NextCursor})
	if err != nil {
		t.Fatalf("Query second: %v", err)
	}
	if got := actions(second); !equalStrings(got, []string{"project.create"}) {
		t.Fatalf("second page = %v", got)
	}
}

func TestQueryRejectsForeignCursor(t *testing.T) {
	store := newStore(t)
	seed(t, store, entry(at(time.June, 1, 9), "a@example.com", "project.create", "p1"))

	for _, cursor := range []string{"nonsense", "2026-06", "2026-13:1", "2026-06:0", "2026-06:x"} {
		if _, err := store.Query(serviceaudit.Query{Limit: 5, Cursor: cursor}); err == nil {
			t.Fatalf("cursor %q was accepted", cursor)
		}
	}
}

func TestQuerySkipsCorruptLines(t *testing.T) {
	store := newStore(t)
	seed(t, store, entry(at(time.June, 1, 9), "a@example.com", "project.create", "p1"))
	path := filepath.Join(store.root, "audit-2026-06.jsonl")
	if err := os.WriteFile(path, []byte("{not json}\n"+mustLine(t, store, path)), 0o600); err != nil {
		t.Fatalf("rewrite: %v", err)
	}

	page, err := store.Query(serviceaudit.Query{Limit: 5})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if got := actions(page); !equalStrings(got, []string{"project.create"}) {
		t.Fatalf("actions = %v, want the one readable line", got)
	}
}

func TestExportStreamsRawLinesOldestFirst(t *testing.T) {
	store := newStore(t)
	seed(t, store,
		entry(at(time.June, 1, 9), "a@example.com", "project.create", "p1"),
		entry(at(time.July, 1, 9), "a@example.com", "project.delete", "p1"),
		entry(at(time.August, 1, 9), "a@example.com", "auth.logout", "p1"),
	)

	var buf bytes.Buffer
	if err := store.Export(context.Background(), at(time.July, 1, 0), at(time.August, 2, 0), &buf); err != nil {
		t.Fatalf("Export: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("exported %d lines, want 2: %q", len(lines), buf.String())
	}
	if !strings.Contains(lines[0], "project.delete") || !strings.Contains(lines[1], "auth.logout") {
		t.Fatalf("export not oldest-first: %q", buf.String())
	}
}

func TestPruneDropsWholeMonthsOutsideRetention(t *testing.T) {
	store := newStore(t)
	seed(t, store,
		entry(at(time.May, 1, 9), "a@example.com", "old", "p1"),
		entry(at(time.June, 1, 9), "a@example.com", "kept", "p1"),
		entry(at(time.July, 1, 9), "a@example.com", "kept-too", "p1"),
	)

	removed, err := store.Prune(at(time.June, 1, 0))
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}
	if _, err := os.Stat(filepath.Join(store.root, "audit-2026-05.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("May file still present: %v", err)
	}

	page, err := store.Query(serviceaudit.Query{Limit: 10})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if got := actions(page); !equalStrings(got, []string{"kept-too", "kept"}) {
		t.Fatalf("actions after prune = %v", got)
	}

	// Pruning is idempotent: a second pass has nothing left to remove.
	if removed, err = store.Prune(at(time.June, 1, 0)); err != nil || removed != 0 {
		t.Fatalf("second Prune = %d, %v; want 0, nil", removed, err)
	}
}

func TestQueryOnEmptyStore(t *testing.T) {
	store := newStore(t)
	page, err := store.Query(serviceaudit.Query{Limit: 10})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(page.Entries) != 0 {
		t.Fatalf("entries = %d, want 0", len(page.Entries))
	}
}

// mustLine returns the single stored line of a monthly file, so a test can
// rewrite the file around it.
func mustLine(t *testing.T, _ *Store, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(raw)
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
