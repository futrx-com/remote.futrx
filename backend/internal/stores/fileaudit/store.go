// Package fileaudit stores the audit trail as append-only JSONL, one file per
// calendar month under <dataDir>/audit/audit-YYYY-MM.jsonl.
//
// Monthly files give retention a unit it can delete whole (no rewriting of a
// live append-only file) and give reads a cheap way to touch only the months a
// query actually covers. Appends use O_APPEND with one write per line, which is
// why the on-disk format is exactly the API format: the export endpoint streams
// these bytes straight to the client.
package fileaudit

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	serviceaudit "github.com/futrx-com/remote.futrx.com/internal/service/audit"
)

var _ serviceaudit.Store = (*Store)(nil)

const (
	filePrefix   = "audit-"
	fileSuffix   = ".jsonl"
	monthLayout  = "2006-01"
	dirMode      = 0o700
	fileMode     = 0o600
	maxLineBytes = 1 << 20
	scanBufBytes = 64 << 10
)

// ErrInvalidCursor is returned for a cursor that did not come from this store.
var ErrInvalidCursor = errors.New("invalid audit cursor")

type Store struct {
	root string

	mu sync.Mutex
}

func New(dataDir string) (*Store, error) {
	root := filepath.Join(dataDir, "audit")
	if err := os.MkdirAll(root, dirMode); err != nil {
		return nil, fmt.Errorf("create audit dir: %w", err)
	}
	return &Store{root: root}, nil
}

func (s *Store) Append(entry serviceaudit.Entry) error {
	line, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal audit entry: %w", err)
	}
	line = append(line, byte('\n'))

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.root, dirMode); err != nil {
		return fmt.Errorf("create audit dir: %w", err)
	}
	file, err := os.OpenFile(s.pathFor(entry.At), os.O_APPEND|os.O_CREATE|os.O_WRONLY, fileMode)
	if err != nil {
		return fmt.Errorf("open audit log: %w", err)
	}
	defer file.Close()
	if _, err := file.Write(line); err != nil {
		return fmt.Errorf("write audit log: %w", err)
	}
	return nil
}

// Query walks the relevant monthly files newest-first, keeping only the last
// `limit` matches of each file so memory stays bounded regardless of file size.
func (s *Store) Query(query serviceaudit.Query) (serviceaudit.Page, error) {
	limit := query.Limit
	if limit <= 0 {
		return serviceaudit.Page{Entries: []serviceaudit.Entry{}}, nil
	}

	cursorMonth, cursorLine, err := parseCursor(query.Cursor)
	if err != nil {
		return serviceaudit.Page{}, err
	}

	months, err := s.months()
	if err != nil {
		return serviceaudit.Page{}, err
	}
	sort.Sort(sort.Reverse(sort.StringSlice(months)))

	entries := make([]serviceaudit.Entry, 0, limit)
	positions := make([]string, 0, limit)
	for _, month := range months {
		if len(entries) >= limit {
			break
		}
		if cursorMonth != "" && month > cursorMonth {
			continue
		}
		if !monthOverlaps(month, query.From, query.To) {
			continue
		}
		stopBefore := 0
		if month == cursorMonth {
			stopBefore = cursorLine
		}
		found, lines, err := s.tailMatches(month, query, limit-len(entries), stopBefore)
		if err != nil {
			return serviceaudit.Page{}, err
		}
		for i := len(found) - 1; i >= 0; i-- {
			entries = append(entries, found[i])
			positions = append(positions, formatCursor(month, lines[i]))
		}
	}

	page := serviceaudit.Page{Entries: entries}
	if len(entries) == limit && len(positions) == len(entries) {
		page.NextCursor = positions[len(positions)-1]
	}
	return page, nil
}

// Export writes the raw stored lines for the range oldest-first. Lines are
// copied verbatim rather than re-encoded, so an export is a byte-faithful
// extract of the log.
func (s *Store) Export(ctx context.Context, from, to time.Time, dst io.Writer) error {
	months, err := s.months()
	if err != nil {
		return err
	}
	sort.Strings(months)
	writer := bufio.NewWriter(dst)
	for _, month := range months {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !monthOverlaps(month, from, to) {
			continue
		}
		if err := s.exportMonth(ctx, month, from, to, writer); err != nil {
			return err
		}
	}
	return writer.Flush()
}

// Prune removes whole monthly files older than the month containing oldest.
func (s *Store) Prune(oldest time.Time) (int, error) {
	cutoff := oldest.UTC().Format(monthLayout)
	months, err := s.months()
	if err != nil {
		return 0, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	removed := 0
	for _, month := range months {
		if month >= cutoff {
			continue
		}
		if err := os.Remove(s.pathForMonth(month)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return removed, fmt.Errorf("remove audit log %s: %w", month, err)
		}
		removed++
	}
	return removed, nil
}

func (s *Store) pathFor(at time.Time) string {
	return s.pathForMonth(at.UTC().Format(monthLayout))
}

func (s *Store) pathForMonth(month string) string {
	return filepath.Join(s.root, filePrefix+month+fileSuffix)
}

// months lists the month keys present on disk, ascending.
func (s *Store) months() ([]string, error) {
	dirEntries, err := os.ReadDir(s.root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read audit dir: %w", err)
	}
	months := make([]string, 0, len(dirEntries))
	for _, dirEntry := range dirEntries {
		if dirEntry.IsDir() {
			continue
		}
		name := dirEntry.Name()
		if !strings.HasPrefix(name, filePrefix) || !strings.HasSuffix(name, fileSuffix) {
			continue
		}
		month := strings.TrimSuffix(strings.TrimPrefix(name, filePrefix), fileSuffix)
		if _, err := time.Parse(monthLayout, month); err != nil {
			continue
		}
		months = append(months, month)
	}
	sort.Strings(months)
	return months, nil
}

// tailMatches returns up to limit matching entries from one monthly file,
// oldest-first, with their 1-based line numbers. When stopBefore is non-zero
// only lines strictly before it are considered, which is how a cursor resumes
// mid-file.
func (s *Store) tailMatches(
	month string,
	query serviceaudit.Query,
	limit int,
	stopBefore int,
) ([]serviceaudit.Entry, []int, error) {
	file, err := os.Open(s.pathForMonth(month))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("open audit log %s: %w", month, err)
	}
	defer file.Close()

	ring := newEntryRing(limit)
	scanner := newLineScanner(file)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		if stopBefore > 0 && lineNumber >= stopBefore {
			break
		}
		raw := scanner.Bytes()
		if len(raw) == 0 {
			continue
		}
		var entry serviceaudit.Entry
		if err := json.Unmarshal(raw, &entry); err != nil {
			continue
		}
		if !matches(entry, query) {
			continue
		}
		ring.push(entry, lineNumber)
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("read audit log %s: %w", month, err)
	}
	entries, lines := ring.drain()
	return entries, lines, nil
}

func (s *Store) exportMonth(
	ctx context.Context,
	month string,
	from, to time.Time,
	dst *bufio.Writer,
) error {
	file, err := os.Open(s.pathForMonth(month))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("open audit log %s: %w", month, err)
	}
	defer file.Close()

	scanner := newLineScanner(file)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		raw := scanner.Bytes()
		if len(raw) == 0 {
			continue
		}
		var stamped struct {
			At time.Time `json:"at"`
		}
		if err := json.Unmarshal(raw, &stamped); err != nil {
			continue
		}
		if !withinRange(stamped.At, from, to) {
			continue
		}
		if _, err := dst.Write(raw); err != nil {
			return err
		}
		if err := dst.WriteByte(byte('\n')); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read audit log %s: %w", month, err)
	}
	return nil
}

func newLineScanner(file *os.File) *bufio.Scanner {
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, scanBufBytes), maxLineBytes)
	return scanner
}

func matches(entry serviceaudit.Entry, query serviceaudit.Query) bool {
	if query.Actor != "" && entry.Actor.Email != query.Actor {
		return false
	}
	if query.Action != "" && !strings.HasPrefix(entry.Action, query.Action) {
		return false
	}
	if query.Target != "" && entry.Target.ID != query.Target {
		return false
	}
	return withinRange(entry.At, query.From, query.To)
}

func withinRange(at, from, to time.Time) bool {
	if !from.IsZero() && at.Before(from) {
		return false
	}
	if !to.IsZero() && !at.Before(to) {
		return false
	}
	return true
}

// monthOverlaps reports whether a monthly file could hold entries inside the
// requested range. Entries land in the file for the month they were written
// in, so a non-overlapping file is skipped without opening it.
func monthOverlaps(month string, from, to time.Time) bool {
	start, err := time.ParseInLocation(monthLayout, month, time.UTC)
	if err != nil {
		return false
	}
	end := start.AddDate(0, 1, 0)
	if !from.IsZero() && !end.After(from) {
		return false
	}
	if !to.IsZero() && !start.Before(to) {
		return false
	}
	return true
}

// Cursors are "<month>:<line>": the position of the last entry a page
// returned. Monthly files are append-only, so a line number stays stable.
func formatCursor(month string, line int) string {
	return month + ":" + strconv.Itoa(line)
}

func parseCursor(cursor string) (string, int, error) {
	if cursor == "" {
		return "", 0, nil
	}
	month, rawLine, ok := strings.Cut(cursor, ":")
	if !ok {
		return "", 0, ErrInvalidCursor
	}
	if _, err := time.Parse(monthLayout, month); err != nil {
		return "", 0, ErrInvalidCursor
	}
	line, err := strconv.Atoi(rawLine)
	if err != nil || line < 1 {
		return "", 0, ErrInvalidCursor
	}
	return month, line, nil
}

// entryRing keeps the most recent size pushes so scanning a huge monthly file
// costs a fixed amount of memory.
type entryRing struct {
	entries []serviceaudit.Entry
	lines   []int
	next    int
	filled  bool
}

func newEntryRing(size int) *entryRing {
	if size < 1 {
		size = 1
	}
	return &entryRing{entries: make([]serviceaudit.Entry, size), lines: make([]int, size)}
}

func (r *entryRing) push(entry serviceaudit.Entry, line int) {
	r.entries[r.next] = entry
	r.lines[r.next] = line
	r.next = (r.next + 1) % len(r.entries)
	if r.next == 0 {
		r.filled = true
	}
}

// drain returns the buffered entries oldest-first.
func (r *entryRing) drain() ([]serviceaudit.Entry, []int) {
	count := r.next
	start := 0
	if r.filled {
		count = len(r.entries)
		start = r.next
	}
	entries := make([]serviceaudit.Entry, 0, count)
	lines := make([]int, 0, count)
	for i := 0; i < count; i++ {
		index := (start + i) % len(r.entries)
		entries = append(entries, r.entries[index])
		lines = append(lines, r.lines[index])
	}
	return entries, lines
}
