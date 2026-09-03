// Package fileusage persists the usage ledger as append-only monthly JSONL
// files plus one editable price table, all under DATA_DIR/usage.
//
// Layout:
//
//	DATA_DIR/usage/                 (0700)
//	DATA_DIR/usage/usage-2026-08.jsonl  (0600, append-only, one Record per line)
//	DATA_DIR/usage/prices.json          (0600, administrator-editable)
//
// Records carry token counts, model ids and cost, so the files are treated as
// operator-only data and never get the 0644 mode chat logs use.
package fileusage

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"

	serviceusage "github.com/futrx-com/remote.futrx.com/internal/service/usage"
)

var _ serviceusage.Repository = (*Store)(nil)

const (
	dirMode  = 0o700
	fileMode = 0o600
	// scanBufferMax bounds one JSONL line; records are small, but a corrupt
	// file must not be able to exhaust memory.
	scanBufferMax = 1 << 20
)

var monthFilePattern = regexp.MustCompile(`^usage-(\d{4}-\d{2})\.jsonl$`)

// Store is the file-backed usage ledger. One mutex serializes every write and
// every rebuild; reads take it too because appends are not atomic per line
// across the rewrite path.
type Store struct {
	dir        string
	pricesPath string

	mu     sync.RWMutex
	prices *serviceusage.PriceTable
}

func New(dataDir string) (*Store, error) {
	dir := filepath.Join(dataDir, "usage")
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return nil, fmt.Errorf("create usage directory: %w", err)
	}
	return &Store{dir: dir, pricesPath: filepath.Join(dir, "prices.json")}, nil
}

func (s *Store) Append(ctx context.Context, record serviceusage.Record) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	line, err := json.Marshal(record)
	if err != nil {
		return err
	}
	line = append(line, '\n')

	s.mu.Lock()
	defer s.mu.Unlock()
	file, err := os.OpenFile(s.monthPath(serviceusage.MonthKey(record.At)), os.O_APPEND|os.O_CREATE|os.O_WRONLY, fileMode)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Write(line)
	return err
}

func (s *Store) Scan(
	ctx context.Context,
	from, to int64,
	visit func(serviceusage.Record) bool,
) error {
	if visit == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	months, err := s.monthKeys()
	if err != nil {
		return err
	}
	for _, month := range months {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !monthInWindow(month, from, to) {
			continue
		}
		stop, err := s.scanMonth(ctx, month, from, to, visit)
		if err != nil {
			return err
		}
		if stop {
			return nil
		}
	}
	return nil
}

func (s *Store) scanMonth(
	ctx context.Context,
	month string,
	from, to int64,
	visit func(serviceusage.Record) bool,
) (bool, error) {
	file, err := os.Open(s.monthPath(month))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 8192), scanBufferMax)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var record serviceusage.Record
		if err := json.Unmarshal(line, &record); err != nil {
			// A truncated tail from an interrupted append must not hide the
			// rest of the ledger.
			continue
		}
		if from > 0 && record.At < from {
			continue
		}
		if to > 0 && record.At > to {
			continue
		}
		if !visit(record) {
			return true, nil
		}
	}
	return false, scanner.Err()
}

// ReplaceAll rewrites the ledger. Each month is written to a temp file and
// renamed into place, and month files that the new record set does not cover
// are removed so a rebuild cannot leave stale data behind.
func (s *Store) ReplaceAll(
	ctx context.Context,
	records []serviceusage.Record,
) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	byMonth := map[string][]serviceusage.Record{}
	for _, record := range records {
		month := serviceusage.MonthKey(record.At)
		byMonth[month] = append(byMonth[month], record)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	existing, err := s.monthKeys()
	if err != nil {
		return nil, err
	}
	written := make([]string, 0, len(byMonth))
	for month := range byMonth {
		written = append(written, month)
	}
	sort.Strings(written)
	for _, month := range written {
		if err := s.writeMonth(month, byMonth[month]); err != nil {
			return nil, err
		}
	}
	for _, month := range existing {
		if _, kept := byMonth[month]; kept {
			continue
		}
		if err := os.Remove(s.monthPath(month)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	return written, nil
}

func (s *Store) writeMonth(month string, records []serviceusage.Record) error {
	final := s.monthPath(month)
	tmp := final + ".tmp"
	file, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, fileMode)
	if err != nil {
		return err
	}
	writer := bufio.NewWriter(file)
	encoder := json.NewEncoder(writer)
	for _, record := range records {
		if err := encoder.Encode(record); err != nil {
			_ = file.Close()
			_ = os.Remove(tmp)
			return err
		}
	}
	if err := writer.Flush(); err != nil {
		_ = file.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, final); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func (s *Store) Prices(ctx context.Context) (serviceusage.PriceTable, error) {
	if err := ctx.Err(); err != nil {
		return serviceusage.PriceTable{}, err
	}
	s.mu.RLock()
	cached := s.prices
	s.mu.RUnlock()
	if cached != nil {
		return *cached, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.prices != nil {
		return *s.prices, nil
	}
	table, err := s.readPrices()
	if err != nil {
		return serviceusage.PriceTable{}, err
	}
	s.prices = &table
	return table, nil
}

// readPrices loads prices.json, seeding it with the shipped defaults the
// first time the ledger is used. A malformed file is reported rather than
// silently replaced, so an operator's bad edit is visible.
func (s *Store) readPrices() (serviceusage.PriceTable, error) {
	data, err := os.ReadFile(s.pricesPath)
	if errors.Is(err, os.ErrNotExist) {
		table, normalizeErr := serviceusage.DefaultPriceTable().Normalize()
		if normalizeErr != nil {
			return serviceusage.PriceTable{}, normalizeErr
		}
		if writeErr := s.writePrices(table); writeErr != nil {
			return serviceusage.PriceTable{}, writeErr
		}
		return table, nil
	}
	if err != nil {
		return serviceusage.PriceTable{}, err
	}
	var table serviceusage.PriceTable
	if err := json.Unmarshal(data, &table); err != nil {
		return serviceusage.PriceTable{}, fmt.Errorf("parse %s: %w", s.pricesPath, err)
	}
	return table.Normalize()
}

func (s *Store) SetPrices(
	ctx context.Context,
	table serviceusage.PriceTable,
) (serviceusage.PriceTable, error) {
	if err := ctx.Err(); err != nil {
		return serviceusage.PriceTable{}, err
	}
	normalized, err := table.Normalize()
	if err != nil {
		return serviceusage.PriceTable{}, err
	}
	normalized.UpdatedAt = table.UpdatedAt

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.writePrices(normalized); err != nil {
		return serviceusage.PriceTable{}, err
	}
	s.prices = &normalized
	return normalized, nil
}

func (s *Store) writePrices(table serviceusage.PriceTable) error {
	data, err := json.MarshalIndent(table, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp := s.pricesPath + ".tmp"
	if err := os.WriteFile(tmp, data, fileMode); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.pricesPath); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// monthKeys lists the YYYY-MM keys present on disk, oldest first.
func (s *Store) monthKeys() ([]string, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	months := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		match := monthFilePattern.FindStringSubmatch(entry.Name())
		if match == nil {
			continue
		}
		months = append(months, match[1])
	}
	sort.Strings(months)
	return months, nil
}

func (s *Store) monthPath(month string) string {
	return filepath.Join(s.dir, "usage-"+month+".jsonl")
}

// monthInWindow keeps the scan from opening files that cannot contain a
// matching record. Comparison is lexical because YYYY-MM sorts chronologically.
func monthInWindow(month string, from, to int64) bool {
	if from > 0 && month < serviceusage.MonthKey(from) {
		return false
	}
	if to > 0 && month > serviceusage.MonthKey(to) {
		return false
	}
	return true
}
