package filechat

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

// jsonlLog keeps each chat in its own append-only events.jsonl file and reads
// through the derived offset index whenever it is available.
type jsonlLog struct {
	store *Store
}

func newJSONLLog(store *Store) *jsonlLog {
	return &jsonlLog{store: store}
}

func (l *jsonlLog) Create(_ context.Context, id servicechat.ID) error {
	f, err := os.OpenFile(l.store.eventsPath(id), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err == nil {
		err = f.Close()
	}
	return err
}

// Remove is a no-op: Store deletes the whole chat directory, which already
// owns the event file.
func (l *jsonlLog) Remove(context.Context, servicechat.ID) error {
	return nil
}

func (l *jsonlLog) Append(
	ctx context.Context,
	id servicechat.ID,
	ev servicechat.Event,
) (servicechat.Event, error) {
	if ev.T == 0 {
		ev.T = time.Now().UnixMilli()
	}
	ev.NormalizeSession()

	seq, indexErr := l.store.index.lastEventSeq(ctx, id, l.store.eventsPath(id))
	var err error
	if indexErr != nil {
		seq, err = l.store.lastEventSeqLocked(id)
	}
	if err != nil {
		return servicechat.Event{}, err
	}
	ev.Seq = seq + 1

	if _, err := l.writeRecord(id, ev); err != nil {
		return servicechat.Event{}, err
	}
	// JSONL is authoritative for this backend. If the derived update fails,
	// the next indexed read or append retries from the last cached offset.
	if indexErr == nil {
		_ = l.store.index.refreshAfterAppend(context.Background(), id, l.store.eventsPath(id))
	} else {
		// The fallback scan assigned the sequence from canonical JSONL, but it
		// did not validate the cached prefix. Revalidate before extending it.
		_ = l.store.index.refreshAfterFallback(context.Background(), id, l.store.eventsPath(id))
	}
	return ev, nil
}

// writeRecord appends an already-sequenced event to the archive and returns
// the exact bytes written. The sequence number comes from the caller, so the
// file always agrees with the database.
func (l *jsonlLog) writeRecord(id servicechat.ID, ev servicechat.Event) ([]byte, error) {
	line, err := json.Marshal(eventRecordFromDomain(ev))
	if err != nil {
		return nil, err
	}
	line = append(line, '\n')

	f, err := os.OpenFile(
		l.store.eventsPath(id),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY,
		0o644,
	)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if _, err := f.Write(line); err != nil {
		return nil, err
	}
	return line, nil
}

// Replace rewrites events.jsonl atomically so readers never observe a partial
// rewind.
func (l *jsonlLog) Replace(
	ctx context.Context,
	id servicechat.ID,
	events []servicechat.Event,
) error {
	dir := l.store.chatDir(id)
	tmp := filepath.Join(dir, "events.jsonl.tmp")
	final := l.store.eventsPath(id)
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	for _, ev := range events {
		if err := enc.Encode(eventRecordFromDomain(ev)); err != nil {
			_ = f.Close()
			_ = os.Remove(tmp)
			return err
		}
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, final); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func (l *jsonlLog) ReadAll(ctx context.Context, id servicechat.ID) ([]servicechat.Event, error) {
	return l.store.readEventsFile(id)
}

func (l *jsonlLog) Scan(
	ctx context.Context,
	id servicechat.ID,
	visit func(servicechat.Event) bool,
) error {
	return l.store.scanEventsFile(ctx, id, visit)
}

func (l *jsonlLog) ReadPage(
	ctx context.Context,
	id servicechat.ID,
	beforeSeq int64,
	limit int,
) (servicechat.EventPage, error) {
	page, err := l.store.index.readEventPage(ctx, id, l.store.eventsPath(id), beforeSeq, limit)
	if err == nil {
		return page, nil
	}
	l.store.discardInvalidChatIndex(ctx, id, err)
	return l.store.readEventsPageFile(ctx, id, servicechat.EventPageQuery{
		Limit:     limit,
		BeforeSeq: beforeSeq,
	}, limit)
}

func (l *jsonlLog) ReadAfter(
	ctx context.Context,
	id servicechat.ID,
	afterSeq int64,
) ([]servicechat.Event, error) {
	events, err := l.store.index.readEventsAfter(ctx, id, l.store.eventsPath(id), afterSeq)
	if err == nil {
		return events, nil
	}
	l.store.discardInvalidChatIndex(ctx, id, err)
	return l.store.readEventsAfterFile(ctx, id, afterSeq)
}

func (l *jsonlLog) LastSeq(ctx context.Context, id servicechat.ID) (int64, error) {
	seq, err := l.store.index.lastEventSeq(ctx, id, l.store.eventsPath(id))
	if err == nil {
		return seq, nil
	}
	return l.store.lastEventSeqLocked(id)
}

// Prepare is a no-op: the JSONL file is the canonical copy.
func (l *jsonlLog) Prepare(context.Context, servicechat.ID) error {
	return nil
}

func (l *jsonlLog) Close() error {
	return nil
}
