package filechat

import (
	"bufio"
	"context"
	crand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

var _ servicechat.Repository = (*Store)(nil)
var _ servicechat.TranscriptEventSource = (*Store)(nil)
var _ servicechat.TranscriptEventWindowSource = (*Store)(nil)

// Store manages chat dirs on disk. Single writer per chat via a per-id mutex
// map; concurrent access across different chats is fine.
type Store struct {
	root   string
	index  *chatEventIndex
	mu     sync.Mutex
	locks  map[servicechat.ID]*sync.Mutex
	metaMu sync.RWMutex
	metas  map[servicechat.ID]servicechat.Meta
}

func New(root string) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(root, "chats"), 0o755); err != nil {
		return nil, err
	}
	index, err := newChatEventIndex(root)
	if err != nil {
		log.Printf("chat event index unavailable; using canonical JSONL scans: %v", err)
		index = unavailableChatEventIndex(root, err)
	}
	store := &Store{
		root:  root,
		index: index,
		locks: map[servicechat.ID]*sync.Mutex{},
		metas: map[servicechat.ID]servicechat.Meta{},
	}
	if err := store.loadMetaIndex(); err != nil {
		_ = index.close()
		return nil, err
	}
	return store, nil
}

// Close releases the derived chat event index. Callers that create a bounded
// Store lifetime (notably commands and tests) should call it explicitly.
func (s *Store) Close() error {
	return s.index.close()
}

// WarmRecentChatIndexes best-effort synchronizes the most recently active
// chats in one worker. It is intended for background startup migration:
// startup itself stays fast, while likely-to-open chats avoid paying the
// one-time backfill on their first request.
func (s *Store) WarmRecentChatIndexes(ctx context.Context, limit int) error {
	if limit <= 0 {
		return nil
	}
	if err := s.index.availabilityError(); err != nil {
		return err
	}
	metas, err := s.List(ctx)
	if err != nil {
		return err
	}
	if len(metas) > limit {
		metas = metas[:limit]
	}
	var warmErrors []error
	for _, meta := range metas {
		if err := ctx.Err(); err != nil {
			return errors.Join(append(warmErrors, err)...)
		}
		lk := s.lock(meta.ID)
		lk.Lock()
		_, err := s.index.syncChat(ctx, meta.ID, s.eventsPath(meta.ID))
		lk.Unlock()
		if err != nil {
			warmErrors = append(warmErrors, fmt.Errorf("chat %s: %w", meta.ID, err))
		}
	}
	return errors.Join(warmErrors...)
}

func (s *Store) chatDir(id servicechat.ID) string {
	return filepath.Join(s.root, "chats", string(id))
}

func (s *Store) eventsPath(id servicechat.ID) string {
	return filepath.Join(s.chatDir(id), "events.jsonl")
}

func (s *Store) lock(id servicechat.ID) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m, ok := s.locks[id]; ok {
		return m
	}
	m := &sync.Mutex{}
	s.locks[id] = m
	return m
}

func newChatID() servicechat.ID {
	var b [6]byte
	_, _ = crand.Read(b[:])
	return servicechat.ID(hex.EncodeToString(b[:]))
}

func (s *Store) Create(ctx context.Context, meta servicechat.Meta) (servicechat.Meta, error) {
	if meta.ID == "" {
		meta.ID = newChatID()
	}
	if !servicechat.ValidID(meta.ID) {
		return servicechat.Meta{}, servicechat.ErrInvalidID
	}
	lk := s.lock(meta.ID)
	lk.Lock()
	defer lk.Unlock()

	now := time.Now().UnixMilli()
	if meta.CreatedAt == 0 {
		meta.CreatedAt = now
	}
	if meta.LastMessageAt == 0 {
		meta.LastMessageAt = now
	}
	if meta.LastReadAt == 0 {
		meta.LastReadAt = meta.LastMessageAt
	}
	if meta.Title == "" {
		meta.Title = "New chat"
	}
	meta.Provider = servicechat.NormalizeProvider(meta.Provider)
	meta.NormalizeSessions()
	meta.ReasoningEffort = servicechat.NormalizeReasoningEffort(meta.ReasoningEffort)
	meta.ServiceTier = servicechat.NormalizeServiceTier(meta.ServiceTier)
	meta.SelectedSkills = servicechat.NormalizeSelectedSkills(meta.SelectedSkills, meta.Provider)
	if meta.Mode == "" {
		meta.Mode = "default"
	}
	_ = s.index.deleteChat(ctx, meta.ID)
	dir := s.chatDir(meta.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return meta, err
	}
	if err := s.writeMeta(meta); err != nil {
		return meta, err
	}
	s.setCachedMeta(meta)
	f, err := os.OpenFile(s.eventsPath(meta.ID), os.O_CREATE|os.O_WRONLY, 0o644)
	if err == nil {
		err = f.Close()
	}
	if err != nil {
		return meta, err
	}
	return meta, nil
}

func (s *Store) List(ctx context.Context) ([]servicechat.Meta, error) {
	s.metaMu.RLock()
	out := make([]servicechat.Meta, 0, len(s.metas))
	for _, meta := range s.metas {
		out = append(out, meta)
	}
	s.metaMu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].LastMessageAt > out[j].LastMessageAt })
	return out, nil
}

func (s *Store) Get(ctx context.Context, id servicechat.ID) (servicechat.Meta, error) {
	if !servicechat.ValidID(id) {
		return servicechat.Meta{}, servicechat.ErrInvalidID
	}
	s.metaMu.RLock()
	meta, ok := s.metas[id]
	s.metaMu.RUnlock()
	if ok {
		return meta, nil
	}
	meta, err := s.readMeta(id)
	if err != nil {
		return servicechat.Meta{}, err
	}
	s.setCachedMeta(meta)
	return meta, nil
}

func (s *Store) Update(
	ctx context.Context,
	id servicechat.ID,
	fn func(*servicechat.Meta),
) (servicechat.Meta, error) {
	if !servicechat.ValidID(id) {
		return servicechat.Meta{}, servicechat.ErrInvalidID
	}
	lk := s.lock(id)
	lk.Lock()
	defer lk.Unlock()

	meta, err := s.Get(ctx, id)
	if err != nil {
		return servicechat.Meta{}, err
	}
	fn(&meta)
	meta.Provider = servicechat.NormalizeProvider(meta.Provider)
	meta.NormalizeSessions()
	meta.ReasoningEffort = servicechat.NormalizeReasoningEffort(meta.ReasoningEffort)
	meta.ServiceTier = servicechat.NormalizeServiceTier(meta.ServiceTier)
	meta.SelectedSkills = servicechat.NormalizeSelectedSkills(meta.SelectedSkills, meta.Provider)
	if err := s.writeMeta(meta); err != nil {
		return meta, err
	}
	s.setCachedMeta(meta)
	return meta, nil
}

func (s *Store) Delete(ctx context.Context, id servicechat.ID) error {
	if !servicechat.ValidID(id) {
		return servicechat.ErrInvalidID
	}
	lk := s.lock(id)
	lk.Lock()
	defer lk.Unlock()

	_ = s.index.deleteChat(ctx, id)
	if err := os.RemoveAll(s.chatDir(id)); err != nil {
		return err
	}
	s.metaMu.Lock()
	delete(s.metas, id)
	s.metaMu.Unlock()
	// Keep the mutex entry for the process lifetime. A waiter may already hold
	// its pointer, and delete/recreate operations must continue sharing it.
	return nil
}

// AppendEvent writes one event to events.jsonl and bumps lastMessageAt.
// Safe for concurrent calls on the same chat (serialized via per-id lock).
func (s *Store) AppendEvent(ctx context.Context, id servicechat.ID, ev servicechat.Event) (servicechat.Event, error) {
	if !servicechat.ValidID(id) {
		return servicechat.Event{}, servicechat.ErrInvalidID
	}
	if ev.T == 0 {
		ev.T = time.Now().UnixMilli()
	}
	ev.NormalizeSession()
	lk := s.lock(id)
	lk.Lock()
	defer lk.Unlock()

	seq, indexErr := s.index.lastEventSeq(ctx, id, s.eventsPath(id))
	var err error
	if indexErr != nil {
		seq, err = s.lastEventSeqLocked(id)
	}
	if err != nil {
		return servicechat.Event{}, err
	}
	ev.Seq = seq + 1

	line, err := json.Marshal(eventRecordFromDomain(ev))
	if err != nil {
		return servicechat.Event{}, err
	}
	line = append(line, '\n')

	f, err := os.OpenFile(
		s.eventsPath(id),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY,
		0o644,
	)
	if err != nil {
		return servicechat.Event{}, err
	}
	defer f.Close()
	if _, err := f.Write(line); err != nil {
		return servicechat.Event{}, err
	}
	// JSONL is authoritative. If this derived update fails, the next indexed
	// read or append retries from the last cached byte offset.
	if indexErr == nil {
		_ = s.index.refreshAfterAppend(context.Background(), id, s.eventsPath(id))
	} else {
		// The fallback scan assigned the sequence from canonical JSONL, but it
		// did not validate the cached prefix. Revalidate before extending it.
		_ = s.index.refreshAfterFallback(context.Background(), id, s.eventsPath(id))
	}
	if eventTouchesChatMeta(ev.Type) {
		meta, err := s.Get(ctx, id)
		if err == nil {
			meta.LastMessageAt = ev.T
			if err := s.writeMeta(meta); err == nil {
				s.setCachedMeta(meta)
			}
		}
	}
	return ev, nil
}

func (s *Store) ReadEvents(ctx context.Context, id servicechat.ID) ([]servicechat.Event, error) {
	if !servicechat.ValidID(id) {
		return nil, servicechat.ErrInvalidID
	}
	return s.readEventsFile(id)
}

// ScanEvents visits the raw append-only event stream in storage order. The
// caller owns any projection policy applied while visiting.
func (s *Store) ScanEvents(
	ctx context.Context,
	id servicechat.ID,
	visit func(servicechat.Event),
) error {
	if !servicechat.ValidID(id) {
		return servicechat.ErrInvalidID
	}
	return s.scanEventsFile(ctx, id, func(event servicechat.Event) bool {
		visit(event)
		return true
	})
}

func (s *Store) ReadEventsPage(
	ctx context.Context,
	id servicechat.ID,
	query servicechat.EventPageQuery,
) (servicechat.EventPage, error) {
	if !servicechat.ValidID(id) {
		return servicechat.EventPage{}, servicechat.ErrInvalidID
	}
	limit := query.Limit
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}
	lk := s.lock(id)
	lk.Lock()
	defer lk.Unlock()

	page, err := s.index.readEventPage(ctx, id, s.eventsPath(id), query.BeforeSeq, limit)
	if err == nil {
		return page, nil
	}
	s.discardInvalidChatIndex(ctx, id, err)
	return s.readEventsPageFile(ctx, id, query, limit)
}

func (s *Store) readEventsPageFile(
	ctx context.Context,
	id servicechat.ID,
	query servicechat.EventPageQuery,
	limit int,
) (servicechat.EventPage, error) {
	var lastSeq int64
	var candidates int
	events := make([]servicechat.Event, 0, limit)
	err := s.scanEventsFile(ctx, id, func(ev servicechat.Event) bool {
		if ev.Seq > lastSeq {
			lastSeq = ev.Seq
		}
		if query.BeforeSeq > 0 && ev.Seq >= query.BeforeSeq {
			return true
		}
		candidates++
		events = append(events, ev)
		if len(events) > limit {
			copy(events, events[1:])
			events = events[:limit]
		}
		return true
	})
	if err != nil {
		return servicechat.EventPage{}, err
	}

	hasMore := candidates > len(events)
	var nextBefore int64
	if hasMore && len(events) > 0 {
		nextBefore = events[0].Seq
	}
	return servicechat.EventPage{
		Events:     events,
		NextBefore: nextBefore,
		LastSeq:    lastSeq,
		HasMore:    hasMore,
	}, nil
}

func (s *Store) ReadEventsAfter(
	ctx context.Context,
	id servicechat.ID,
	afterSeq int64,
) ([]servicechat.Event, error) {
	if !servicechat.ValidID(id) {
		return nil, servicechat.ErrInvalidID
	}
	lk := s.lock(id)
	lk.Lock()
	defer lk.Unlock()

	events, err := s.index.readEventsAfter(ctx, id, s.eventsPath(id), afterSeq)
	if err == nil {
		return events, nil
	}
	s.discardInvalidChatIndex(ctx, id, err)
	return s.readEventsAfterFile(ctx, id, afterSeq)
}

func (s *Store) readEventsAfterFile(
	ctx context.Context,
	id servicechat.ID,
	afterSeq int64,
) ([]servicechat.Event, error) {
	out := make([]servicechat.Event, 0, 32)
	err := s.scanEventsFile(ctx, id, func(ev servicechat.Event) bool {
		if ev.Seq > afterSeq {
			out = append(out, ev)
		}
		return true
	})
	return out, err
}

// ReadTranscriptEventWindow uses the derived turn/offset index to read
// only the requested page plus one older turn. That extra turn lets the service
// preserve its existing HasMore and cursor projection behavior.
func (s *Store) ReadTranscriptEventWindow(
	ctx context.Context,
	id servicechat.ID,
	beforeSeq int64,
	turnLimit int,
) (servicechat.TranscriptEventWindow, error) {
	if !servicechat.ValidID(id) {
		return servicechat.TranscriptEventWindow{}, servicechat.ErrInvalidID
	}
	lk := s.lock(id)
	lk.Lock()
	defer lk.Unlock()

	window, err := s.index.readTranscriptWindow(ctx, id, s.eventsPath(id), beforeSeq, turnLimit)
	if err == nil {
		return window, nil
	}
	s.discardInvalidChatIndex(ctx, id, err)

	// The index is disposable. Preserve availability by falling back to the
	// canonical log if it cannot be synchronized or read.
	window = servicechat.TranscriptEventWindow{}
	err = s.scanEventsFile(ctx, id, func(event servicechat.Event) bool {
		window.Events = append(window.Events, event)
		if event.Seq > window.LastSeq {
			window.LastSeq = event.Seq
		}
		return true
	})
	return window, err
}

func (s *Store) discardInvalidChatIndex(
	ctx context.Context,
	id servicechat.ID,
	readErr error,
) {
	if ctx.Err() == nil && errors.Is(readErr, errInvalidChatEventIndex) {
		_ = s.index.deleteChat(ctx, id)
	}
}

// TruncateEventsBefore rewinds a chat by removing the selected event and every
// event after it. The returned slice is the complete remaining history.
func (s *Store) TruncateEventsBefore(ctx context.Context, id servicechat.ID, beforeT int64) ([]servicechat.Event, error) {
	if !servicechat.ValidID(id) {
		return nil, servicechat.ErrInvalidID
	}
	if beforeT <= 0 {
		return nil, servicechat.ErrInvalidRewindTimestamp
	}
	lk := s.lock(id)
	lk.Lock()
	defer lk.Unlock()

	events, err := s.readEventsFile(id)
	if err != nil {
		return nil, err
	}
	kept := make([]servicechat.Event, 0, len(events))
	var lastT int64
	for _, ev := range events {
		if ev.T >= beforeT {
			continue
		}
		kept = append(kept, ev)
		if ev.T > lastT {
			lastT = ev.T
		}
	}

	tmp := filepath.Join(s.chatDir(id), "events.jsonl.tmp")
	final := filepath.Join(s.chatDir(id), "events.jsonl")
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	enc := json.NewEncoder(f)
	for _, ev := range kept {
		if err := enc.Encode(eventRecordFromDomain(ev)); err != nil {
			_ = f.Close()
			_ = os.Remove(tmp)
			return nil, err
		}
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return nil, err
	}
	if err := os.Rename(tmp, final); err != nil {
		_ = os.Remove(tmp)
		return nil, err
	}
	// A rewind replaces the JSONL file. Rebuild the cached offsets now;
	// size-based recovery on the next read remains a backstop.
	_ = s.index.rebuildChat(context.Background(), id, final)

	if meta, err := s.Get(ctx, id); err == nil {
		if lastT == 0 {
			lastT = meta.CreatedAt
		}
		meta.ClearSessionIDs()
		meta.LastMessageAt = lastT
		if err := s.writeMeta(meta); err == nil {
			s.setCachedMeta(meta)
		}
	}

	return kept, nil
}

func (s *Store) writeMeta(meta servicechat.Meta) error {
	dir := s.chatDir(meta.ID)
	tmp := filepath.Join(dir, "meta.json.tmp")
	final := filepath.Join(dir, "meta.json")
	data, err := json.MarshalIndent(metaRecordFromDomain(meta), "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, final)
}

func (s *Store) loadMetaIndex() error {
	entries, err := os.ReadDir(filepath.Join(s.root, "chats"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		id := servicechat.ID(e.Name())
		if !e.IsDir() || !servicechat.ValidID(id) {
			continue
		}
		meta, err := s.readMeta(id)
		if err != nil {
			continue
		}
		s.metas[id] = meta
	}
	return nil
}

func (s *Store) readMeta(id servicechat.ID) (servicechat.Meta, error) {
	if !servicechat.ValidID(id) {
		return servicechat.Meta{}, servicechat.ErrInvalidID
	}
	data, err := os.ReadFile(filepath.Join(s.chatDir(id), "meta.json"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return servicechat.Meta{}, servicechat.ErrNotFound
		}
		return servicechat.Meta{}, err
	}
	var rec metaRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return servicechat.Meta{}, err
	}
	meta := rec.toDomain()
	if meta.ID == "" {
		meta.ID = id
	}
	return meta, nil
}

func (s *Store) setCachedMeta(meta servicechat.Meta) {
	s.metaMu.Lock()
	defer s.metaMu.Unlock()
	s.metas[meta.ID] = meta
}

func (s *Store) readEventsFile(id servicechat.ID) ([]servicechat.Event, error) {
	out := make([]servicechat.Event, 0, 64)
	err := s.scanEventsFile(context.Background(), id, func(ev servicechat.Event) bool {
		out = append(out, ev)
		return true
	})
	return out, err
}

func (s *Store) scanEventsFile(
	ctx context.Context,
	id servicechat.ID,
	visit func(servicechat.Event) bool,
) error {
	f, err := os.Open(s.eventsPath(id))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), maxEventRecordBytes)
	var seq int64
	for sc.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		seq++
		ev, err := decodeStoredEvent(line, seq)
		if err != nil {
			continue
		}
		if !visit(ev) {
			break
		}
	}
	return sc.Err()
}

func (s *Store) lastEventSeqLocked(id servicechat.ID) (int64, error) {
	var last int64
	err := s.scanEventsFile(context.Background(), id, func(ev servicechat.Event) bool {
		if ev.Seq > last {
			last = ev.Seq
		}
		return true
	})
	return last, err
}

func eventTouchesChatMeta(eventType string) bool {
	return eventType == "user" || eventType == "complete" || eventType == "error"
}
