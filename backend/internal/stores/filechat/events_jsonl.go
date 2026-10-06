package filechat

import (
	"bufio"
	"context"
	"encoding/json"
	"log"
	"os"
	"time"

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

// jsonlLog keeps each chat in its own append-only events.jsonl file and reads
// through the derived offset index whenever it is available.
type jsonlLog struct {
	store   *Store
	index   *chatEventIndex
	archive *jsonlArchive
}

func newJSONLLog(store *Store, index *chatEventIndex) *jsonlLog {
	return &jsonlLog{store: store, index: index, archive: newJSONLArchive(store)}
}

func (l *jsonlLog) Create(_ context.Context, id servicechat.ID) error {
	return l.archive.create(id)
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

	seq, indexErr := l.index.lastEventSeq(ctx, id, l.store.eventsPath(id))
	var err error
	if indexErr != nil {
		seq, err = l.store.lastEventSeqLocked(id)
	}
	if err != nil {
		return servicechat.Event{}, err
	}
	ev.Seq = seq + 1

	if _, err := l.archive.writeRecord(id, ev); err != nil {
		return servicechat.Event{}, err
	}
	// JSONL is authoritative for this backend. If the derived update fails,
	// the next indexed read or append retries from the last cached offset.
	if indexErr == nil {
		_ = l.index.refreshAfterAppend(context.Background(), id, l.store.eventsPath(id))
	} else {
		// The fallback scan assigned the sequence from canonical JSONL, but it
		// did not validate the cached prefix. Revalidate before extending it.
		_ = l.index.refreshAfterFallback(context.Background(), id, l.store.eventsPath(id))
	}
	return ev, nil
}

// Replace rewrites events.jsonl atomically so readers never observe a partial
// rewind.
func (l *jsonlLog) Replace(
	ctx context.Context,
	id servicechat.ID,
	events []servicechat.Event,
) error {
	return l.archive.replaceStream(ctx, id, func(
		_ context.Context,
		yield func(servicechat.Event) error,
	) error {
		for _, ev := range events {
			if err := yield(ev); err != nil {
				return err
			}
		}
		return nil
	})
}

// TruncateBefore streams the file once, writing the events kept by the
// timestamp filter into a temporary file and renaming it over the log, so the
// rewind never holds the conversation in memory.
func (l *jsonlLog) TruncateBefore(
	ctx context.Context,
	id servicechat.ID,
	beforeT int64,
) (int64, error) {
	var lastT int64
	err := l.archive.replaceStream(ctx, id, func(
		ctx context.Context,
		yield func(servicechat.Event) error,
	) error {
		var yieldErr error
		scanErr := l.store.scanEventsFile(ctx, id, func(ev servicechat.Event) bool {
			if ev.T >= beforeT {
				return true
			}
			if ev.T > lastT {
				lastT = ev.T
			}
			if err := yield(ev); err != nil {
				yieldErr = err
				return false
			}
			return true
		})
		if scanErr != nil {
			return scanErr
		}
		return yieldErr
	})
	return lastT, err
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
	page, err := l.index.readEventPage(ctx, id, l.store.eventsPath(id), beforeSeq, limit)
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
	events, err := l.index.readEventsAfter(ctx, id, l.store.eventsPath(id), afterSeq)
	if err == nil {
		return events, nil
	}
	l.store.discardInvalidChatIndex(ctx, id, err)
	return l.store.readEventsAfterFile(ctx, id, afterSeq)
}

func (l *jsonlLog) LastSeq(ctx context.Context, id servicechat.ID) (int64, error) {
	seq, err := l.index.lastEventSeq(ctx, id, l.store.eventsPath(id))
	if err == nil {
		return seq, nil
	}
	return l.store.lastEventSeqLocked(id)
}

// Prepare is a no-op: the JSONL file is the canonical copy.
func (l *jsonlLog) Prepare(context.Context, servicechat.ID) error {
	return nil
}

// CopyEvents streams the source file straight into an appended destination
// file, so a fork of a large conversation never materializes it in memory and
// never opens the destination once per event.
func (l *jsonlLog) CopyEvents(
	ctx context.Context,
	from servicechat.ID,
	to servicechat.ID,
) (int, servicechat.Event, error) {
	if from == to {
		return 0, servicechat.Event{}, nil
	}
	seq, err := l.LastSeq(ctx, to)
	if err != nil {
		return 0, servicechat.Event{}, err
	}
	file, err := os.OpenFile(
		l.store.eventsPath(to), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644,
	)
	if err != nil {
		return 0, servicechat.Event{}, err
	}
	writer := bufio.NewWriterSize(file, 64*1024)

	var last servicechat.Event
	copied := 0
	var writeErr error
	scanErr := l.store.scanEventsFile(ctx, from, func(ev servicechat.Event) bool {
		seq++
		ev.Seq = seq
		line, err := json.Marshal(eventRecordFromDomain(ev))
		if err != nil {
			writeErr = err
			return false
		}
		if _, err := writer.Write(append(line, '\n')); err != nil {
			writeErr = err
			return false
		}
		last, copied = ev, copied+1
		return true
	})
	if writeErr == nil {
		writeErr = writer.Flush()
	}
	if closeErr := file.Close(); writeErr == nil {
		writeErr = closeErr
	}
	if writeErr != nil {
		return copied, last, writeErr
	}
	if scanErr != nil {
		return copied, last, scanErr
	}
	// One derived refresh covers the whole copy. A failure only postpones the
	// work to the next indexed read, exactly as a failed append refresh does.
	if _, err := l.index.syncChat(ctx, to, l.store.eventsPath(to)); err != nil {
		log.Printf("chat %s: transcript index refresh after copy failed: %v", to, err)
	}
	return copied, last, nil
}

func (l *jsonlLog) Close() error {
	return nil
}
