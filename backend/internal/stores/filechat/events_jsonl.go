package filechat

import (
	"bufio"
	"context"
	"encoding/json"
	"log"
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
	return l.replaceStream(ctx, id, func(
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
	err := l.replaceStream(ctx, id, func(
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

// replaceStream rewrites events.jsonl from a streaming source: every event the
// source yields is encoded into a temporary file that is renamed over the log
// once the source finishes. A failed source leaves the original untouched.
func (l *jsonlLog) replaceStream(
	ctx context.Context,
	id servicechat.ID,
	src eventSource,
) error {
	dir := l.store.chatDir(id)
	tmp := filepath.Join(dir, "events.jsonl.tmp")
	final := l.store.eventsPath(id)
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	if err := src(ctx, func(ev servicechat.Event) error {
		return enc.Encode(eventRecordFromDomain(ev))
	}); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
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
	if _, err := l.store.index.syncChat(ctx, to, l.store.eventsPath(to)); err != nil {
		log.Printf("chat %s: transcript index refresh after copy failed: %v", to, err)
	}
	return copied, last, nil
}

func (l *jsonlLog) Close() error {
	return nil
}
