package filechat

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

// copyBatchEvents bounds how many events a bulk copy reads and writes per
// transaction, so copying a large conversation stays bounded by memory.
const copyBatchEvents = 500

// maxSearchTextBytes bounds the text copied into the FTS index so a single
// oversized tool result cannot dominate the search index.
const maxSearchTextBytes = 64 * 1024

// sqliteLog keeps every chat in the shared chats.sqlite database. Appended
// events are mirrored back to events.jsonl so the JSONL backend can take over
// if the database ever has to be rolled back.
type sqliteLog struct {
	store  *Store
	db     *chatStoreDB
	mirror *jsonlArchive
}

func openSQLiteLog(store *Store) (*sqliteLog, error) {
	db, err := openChatStoreDB(store.root)
	if err != nil {
		return nil, err
	}
	return &sqliteLog{store: store, db: db, mirror: newJSONLArchive(store)}, nil
}

func (l *sqliteLog) Close() error {
	return l.db.close()
}

// Create clears any inherited rows for id and starts an empty mirrored archive.
// The archive is emptied first: if that fails the stored rows are left alone,
// and if it succeeds a later failure still converges because the empty archive
// makes the next import delete the leftovers.
func (l *sqliteLog) Create(ctx context.Context, id servicechat.ID) error {
	if err := l.mirror.create(id); err != nil {
		return err
	}
	if _, err := l.db.db.ExecContext(ctx,
		`DELETE FROM chat_events WHERE chat_id = ?`, id); err != nil {
		return err
	}
	_, err := l.db.db.ExecContext(ctx,
		`DELETE FROM chat_event_state WHERE chat_id = ?`, id)
	return err
}

// Remove drops the chat's rows. The archive lives in the chat directory, which
// Store deletes.
func (l *sqliteLog) Remove(ctx context.Context, id servicechat.ID) error {
	if _, err := l.db.db.ExecContext(ctx,
		`DELETE FROM chat_events WHERE chat_id = ?`, id); err != nil {
		return err
	}
	_, err := l.db.db.ExecContext(ctx,
		`DELETE FROM chat_event_state WHERE chat_id = ?`, id)
	return err
}

func (l *sqliteLog) Append(
	ctx context.Context,
	id servicechat.ID,
	ev servicechat.Event,
) (servicechat.Event, error) {
	stored, err := l.insert(ctx, id, ev)
	if err != nil {
		return servicechat.Event{}, err
	}
	l.mirrorAppend(id, stored)
	// The transcript projection is a read model over these rows. Refreshing it
	// here keeps a reopened chat from reporting index progress for one event.
	// A failure only postpones the work to the next read's background sync.
	if _, err := l.store.syncSQLiteTranscript(context.Background(), id); err != nil {
		log.Printf("chat %s: transcript projection update failed: %v", id, err)
	}
	return stored, nil
}

// insert stores one event with the next sequence number inside a single write
// transaction, so the sequence can never race another writer.
func (l *sqliteLog) insert(
	ctx context.Context,
	id servicechat.ID,
	ev servicechat.Event,
) (servicechat.Event, error) {
	tx, err := l.db.db.BeginTx(ctx, nil)
	if err != nil {
		return servicechat.Event{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var lastSeq int64
	err = tx.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(seq), 0) FROM chat_events WHERE chat_id = ?`, id,
	).Scan(&lastSeq)
	if err != nil {
		return servicechat.Event{}, err
	}
	ev.Seq = lastSeq + 1
	if err := upsertEventRow(ctx, tx, id, ev); err != nil {
		return servicechat.Event{}, err
	}
	if err := writeEventState(ctx, tx, id, ev.Seq); err != nil {
		return servicechat.Event{}, err
	}
	if err := tx.Commit(); err != nil {
		return servicechat.Event{}, err
	}
	return ev, nil
}

// CopyEvents streams the source rows into the destination in bounded
// transactions and mirrors each copied record, so a fork of a large
// conversation never materializes it in memory and never reprojects the
// destination once per event.
func (l *sqliteLog) CopyEvents(
	ctx context.Context,
	from servicechat.ID,
	to servicechat.ID,
) (int, servicechat.Event, error) {
	if from == to {
		return 0, servicechat.Event{}, nil
	}
	var last servicechat.Event
	var copied int
	var cursor int64
	for {
		events, next, err := l.readCopyBatch(ctx, from, cursor)
		if err != nil {
			return copied, last, err
		}
		if len(events) == 0 {
			break
		}
		if err := l.writeCopyBatch(ctx, to, events); err != nil {
			return copied, last, err
		}
		copied += len(events)
		last = events[len(events)-1]
		cursor = next
	}
	if _, err := l.store.syncSQLiteTranscript(ctx, to); err != nil &&
		!errors.Is(err, context.Canceled) {
		log.Printf("chat %s: transcript projection update after copy failed: %v", to, err)
	}
	return copied, last, nil
}

func (l *sqliteLog) readCopyBatch(
	ctx context.Context,
	from servicechat.ID,
	afterSeq int64,
) ([]servicechat.Event, int64, error) {
	rows, err := l.db.db.QueryContext(ctx, `
		SELECT seq, payload FROM chat_events
		WHERE chat_id = ? AND seq > ?
		ORDER BY seq
		LIMIT ?`, from, afterSeq, copyBatchEvents,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var events []servicechat.Event
	var lastSeq int64
	for rows.Next() {
		var seq int64
		var payload []byte
		if err := rows.Scan(&seq, &payload); err != nil {
			return nil, 0, err
		}
		event, err := decodeStoredEvent(payload, seq)
		if err != nil {
			return nil, 0, fmt.Errorf(
				"%w: decode stored event %d: %v", errInvalidChatEventIndex, seq, err,
			)
		}
		events = append(events, event)
		lastSeq = seq
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return events, lastSeq, nil
}

func (l *sqliteLog) writeCopyBatch(
	ctx context.Context,
	to servicechat.ID,
	events []servicechat.Event,
) error {
	tx, err := l.db.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var seq int64
	if err := tx.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(seq), 0) FROM chat_events WHERE chat_id = ?`, to,
	).Scan(&seq); err != nil {
		return err
	}
	for index := range events {
		seq++
		events[index].Seq = seq
		if err := upsertEventRow(ctx, tx, to, events[index]); err != nil {
			return err
		}
	}
	if err := writeEventState(ctx, tx, to, seq); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	// The archive is only a rollback copy, so mirroring runs after the
	// committed write and reports failure instead of failing the copy.
	for _, event := range events {
		l.mirrorAppend(to, event)
	}
	return nil
}

// Replace rewrites the chat's rows with events, keeping their sequence numbers,
// then replaces the mirrored archive with the same stream.
func (l *sqliteLog) Replace(
	ctx context.Context,
	id servicechat.ID,
	events []servicechat.Event,
) error {
	tx, err := l.db.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM chat_events WHERE chat_id = ?`, id); err != nil {
		return err
	}
	var lastSeq int64
	for _, ev := range events {
		if ev.Seq > lastSeq {
			lastSeq = ev.Seq
		}
		if err := upsertEventRow(ctx, tx, id, ev); err != nil {
			return err
		}
	}
	if err := writeEventState(ctx, tx, id, lastSeq); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	// The archive is a best-effort rollback copy: a failure here leaves it
	// stale rather than failing a rewind that already succeeded. A stale
	// archive keeps its recorded fingerprint, so the next import does not
	// rebuild the database from content the rewind already replaced.
	if err := l.mirror.replaceStream(ctx, id, func(
		_ context.Context,
		yield func(servicechat.Event) error,
	) error {
		for _, event := range events {
			if err := yield(event); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		logMirrorFailure(l.store.root, id, err)
	} else if err := l.recordArchiveReplace(ctx, id, lastSeq); err != nil &&
		!errors.Is(err, errMissingArchive) {
		logMirrorFailure(l.store.root, id, err)
	}
	return nil
}

// TruncateBefore keeps the events below beforeT. Timestamps normally advance
// with the sequence, so the survivors are a prefix and the tail can be dropped
// with one DELETE instead of re-inserting every kept row. When timestamps are
// not ordered by sequence the survivors are not a prefix, and the whole stream
// goes through Replace so the rewind drops exactly what the filter drops.
func (l *sqliteLog) TruncateBefore(
	ctx context.Context,
	id servicechat.ID,
	beforeT int64,
) (int64, error) {
	keepSeq, lastT, prefix, err := l.rewindKeepPrefix(ctx, id, beforeT)
	if err != nil {
		return 0, err
	}
	if !prefix {
		return l.truncateWholeStream(ctx, id, beforeT)
	}
	if err := l.dropRewindTail(ctx, id, keepSeq); err != nil {
		return 0, err
	}
	l.replaceArchiveFromDatabase(ctx, id, keepSeq)
	return lastT, nil
}

// rewindKeepPrefix scans only sequence numbers and timestamps to find the last
// event the rewind keeps. It reports whether the kept events form a prefix of
// the sequence, which is what makes dropping the tail safe.
func (l *sqliteLog) rewindKeepPrefix(
	ctx context.Context,
	id servicechat.ID,
	beforeT int64,
) (keepSeq int64, lastT int64, prefix bool, err error) {
	rows, err := l.db.db.QueryContext(ctx, `
		SELECT seq, t FROM chat_events
		WHERE chat_id = ?
		ORDER BY seq`, id)
	if err != nil {
		return 0, 0, false, err
	}
	defer rows.Close()

	seenDropped := false
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return 0, 0, false, err
		}
		var seq, t int64
		if err := rows.Scan(&seq, &t); err != nil {
			return 0, 0, false, err
		}
		if t >= beforeT {
			seenDropped = true
			continue
		}
		if seenDropped {
			// A kept event follows a dropped one, so the survivors are not a
			// prefix of the sequence and a tail delete would drop too much.
			return 0, 0, false, nil
		}
		keepSeq = seq
		if t > lastT {
			lastT = t
		}
	}
	if err := rows.Err(); err != nil {
		return 0, 0, false, err
	}
	return keepSeq, lastT, true, nil
}

// dropRewindTail deletes every event after keepSeq and records the new last
// sequence, so state, the byte counters, and the FTS rows follow the rewind.
func (l *sqliteLog) dropRewindTail(
	ctx context.Context,
	id servicechat.ID,
	keepSeq int64,
) error {
	tx, err := l.db.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM chat_events WHERE chat_id = ? AND seq > ?`,
		id, keepSeq); err != nil {
		return err
	}
	if err := writeEventState(ctx, tx, id, keepSeq); err != nil {
		return err
	}
	return tx.Commit()
}

// truncateWholeStream is the fallback for timestamps that are not ordered by
// sequence: read, filter, and rewrite, exactly as the rewind did before it
// was batched.
func (l *sqliteLog) truncateWholeStream(
	ctx context.Context,
	id servicechat.ID,
	beforeT int64,
) (int64, error) {
	events, err := l.ReadAll(ctx, id)
	if err != nil {
		return 0, err
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
	if err := l.Replace(ctx, id, kept); err != nil {
		return 0, err
	}
	return lastT, nil
}

// replaceArchiveFromDatabase rewrites the JSONL mirror from the rows that
// survived the rewind, reading them in bounded batches. The database has
// already committed, so a mirror failure is logged instead of reported, the
// same way an append treats its mirror.
func (l *sqliteLog) replaceArchiveFromDatabase(
	ctx context.Context,
	id servicechat.ID,
	lastSeq int64,
) {
	if err := l.mirror.replaceStream(ctx, id, func(
		ctx context.Context,
		yield func(servicechat.Event) error,
	) error {
		var yieldErr error
		scanErr := l.Scan(ctx, id, func(ev servicechat.Event) bool {
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
	}); err != nil {
		logMirrorFailure(l.store.root, id, err)
		return
	}
	if err := l.recordArchiveReplace(ctx, id, lastSeq); err != nil &&
		!errors.Is(err, errMissingArchive) {
		logMirrorFailure(l.store.root, id, err)
	}
}

func (l *sqliteLog) ReadAll(ctx context.Context, id servicechat.ID) ([]servicechat.Event, error) {
	rows, err := l.db.db.QueryContext(ctx, `
		SELECT seq, payload FROM chat_events
		WHERE chat_id = ?
		ORDER BY seq`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := make([]servicechat.Event, 0, 64)
	for rows.Next() {
		event, err := scanEventRow(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (l *sqliteLog) Scan(
	ctx context.Context,
	id servicechat.ID,
	visit func(servicechat.Event) bool,
) error {
	rows, err := l.db.db.QueryContext(ctx, `
		SELECT seq, payload FROM chat_events
		WHERE chat_id = ?
		ORDER BY seq`, id)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return err
		}
		event, err := scanEventRow(rows)
		if err != nil {
			return err
		}
		if !visit(event) {
			break
		}
	}
	return rows.Err()
}

// ReadPage returns the newest `limit` events older than beforeSeq in ascending
// order, matching the JSONL backend's cursor contract.
func (l *sqliteLog) ReadPage(
	ctx context.Context,
	id servicechat.ID,
	beforeSeq int64,
	limit int,
) (servicechat.EventPage, error) {
	var lastSeq int64
	if err := l.db.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(seq), 0) FROM chat_events WHERE chat_id = ?`, id,
	).Scan(&lastSeq); err != nil {
		return servicechat.EventPage{}, err
	}

	var candidates int
	if beforeSeq > 0 {
		if err := l.db.db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM chat_events
			WHERE chat_id = ? AND seq < ?`, id, beforeSeq,
		).Scan(&candidates); err != nil {
			return servicechat.EventPage{}, err
		}
	} else {
		if err := l.db.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM chat_events WHERE chat_id = ?`, id,
		).Scan(&candidates); err != nil {
			return servicechat.EventPage{}, err
		}
	}

	rows, err := l.db.db.QueryContext(ctx, `
		SELECT seq, payload FROM chat_events
		WHERE chat_id = ? AND (? <= 0 OR seq < ?)
		ORDER BY seq DESC
		LIMIT ?`, id, beforeSeq, beforeSeq, limit,
	)
	if err != nil {
		return servicechat.EventPage{}, err
	}
	defer rows.Close()

	page := make([]servicechat.Event, 0, limit)
	for rows.Next() {
		event, err := scanEventRow(rows)
		if err != nil {
			return servicechat.EventPage{}, err
		}
		page = append(page, event)
	}
	if err := rows.Err(); err != nil {
		return servicechat.EventPage{}, err
	}
	for left, right := 0, len(page)-1; left < right; left, right = left+1, right-1 {
		page[left], page[right] = page[right], page[left]
	}

	hasMore := candidates > len(page)
	var nextBefore int64
	if hasMore && len(page) > 0 {
		nextBefore = page[0].Seq
	}
	return servicechat.EventPage{
		Events:     page,
		NextBefore: nextBefore,
		LastSeq:    lastSeq,
		HasMore:    hasMore,
	}, nil
}

func (l *sqliteLog) ReadAfter(
	ctx context.Context,
	id servicechat.ID,
	afterSeq int64,
) ([]servicechat.Event, error) {
	rows, err := l.db.db.QueryContext(ctx, `
		SELECT seq, payload FROM chat_events
		WHERE chat_id = ? AND seq > ?
		ORDER BY seq`, id, afterSeq)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := make([]servicechat.Event, 0, 32)
	for rows.Next() {
		event, err := scanEventRow(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (l *sqliteLog) LastSeq(ctx context.Context, id servicechat.ID) (int64, error) {
	var seq int64
	err := l.db.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(seq), 0) FROM chat_events WHERE chat_id = ?`, id,
	).Scan(&seq)
	return seq, err
}

type eventRowScanner interface {
	Scan(dest ...any) error
}

func scanEventRow(row eventRowScanner) (servicechat.Event, error) {
	var seq int64
	var payload []byte
	if err := row.Scan(&seq, &payload); err != nil {
		return servicechat.Event{}, err
	}
	return decodeStoredEvent(payload, seq)
}

func upsertEventRow(
	ctx context.Context,
	execer interface {
		ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	},
	id servicechat.ID,
	ev servicechat.Event,
) error {
	payload, err := json.Marshal(eventRecordFromDomain(ev))
	if err != nil {
		return err
	}
	_, err = execer.ExecContext(ctx, `
		INSERT INTO chat_events
			(chat_id, seq, t, type, turn_id, message_id, item_id, name,
			 provider, status, is_error, search_text, payload)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(chat_id, seq) DO UPDATE SET
			t = excluded.t,
			type = excluded.type,
			turn_id = excluded.turn_id,
			message_id = excluded.message_id,
			item_id = excluded.item_id,
			name = excluded.name,
			provider = excluded.provider,
			status = excluded.status,
			is_error = excluded.is_error,
			search_text = excluded.search_text,
			payload = excluded.payload`,
		id,
		ev.Seq,
		ev.T,
		ev.Type,
		ev.TurnID,
		ev.MessageID,
		ev.ID,
		ev.Name,
		string(ev.Provider),
		ev.Status,
		boolToInt(ev.IsError),
		eventSearchText(ev),
		payload,
	)
	return err
}

// writeEventState records the highest sequence stored for a chat.
func writeEventState(
	ctx context.Context,
	execer interface {
		ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	},
	id servicechat.ID,
	lastSeq int64,
) error {
	_, err := execer.ExecContext(ctx, `
		INSERT INTO chat_event_state (chat_id, last_seq, imported_at)
		VALUES (?, ?, ?)
		ON CONFLICT(chat_id) DO UPDATE SET
			last_seq = excluded.last_seq,
			imported_at = excluded.imported_at`,
		id, lastSeq, time.Now().UnixMilli(),
	)
	return err
}

// mirrorAppend writes the event back to events.jsonl. The archive is only a
// rollback copy, so a failure is logged instead of failing the write that
// already committed to the database.
func (l *sqliteLog) mirrorAppend(id servicechat.ID, ev servicechat.Event) {
	line, err := l.mirror.writeRecord(id, ev)
	if err != nil {
		// The archive did not grow, so its recorded fingerprint still matches
		// it: the next successful append extends both together.
		logMirrorFailure(l.store.root, id, err)
		return
	}
	if err := l.recordArchiveAppend(context.Background(), id, ev.Seq, line); err != nil &&
		!errors.Is(err, errMissingArchive) {
		logMirrorFailure(l.store.root, id, err)
	}
}

// recordArchiveAppend folds the bytes just mirrored into the archive
// fingerprint so the next import can trust the recorded prefix.
func (l *sqliteLog) recordArchiveAppend(
	ctx context.Context,
	id servicechat.ID,
	lastSeq int64,
	line []byte,
) error {
	state, err := readArchiveState(ctx, l.db.db, id)
	if err != nil {
		return err
	}
	info, err := os.Stat(l.store.eventsPath(id))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return errMissingArchive
		}
		return err
	}
	size, mtime := info.Size(), info.ModTime().UnixNano()

	hash, base := indexPrefixHashOffset64, int64(0)
	if state.found {
		hash, base = state.prefixHash, state.sourceBytes
	}
	switch {
	case size == base+int64(len(line)):
		hash = updateIndexPrefixHash(hash, line)
	case size == base:
		// Nothing landed in the archive; the fingerprint is untouched.
	default:
		// The file no longer grew from the recorded offset (a partial write
		// or an outside change). Re-fingerprint it instead of guessing.
		if hash, err = hashArchiveFile(ctx, l.store.eventsPath(id)); err != nil {
			return err
		}
	}
	return l.recordArchiveWithHash(ctx, id, lastSeq, hash, size, mtime)
}

// recordArchiveReplace re-fingerprints the whole archive after it was rewritten
// for a rewind, because none of the previous prefix applies to the new bytes.
func (l *sqliteLog) recordArchiveReplace(
	ctx context.Context,
	id servicechat.ID,
	lastSeq int64,
) error {
	path := l.store.eventsPath(id)
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return errMissingArchive
		}
		return err
	}
	hash, err := hashArchiveFile(ctx, path)
	if err != nil {
		return err
	}
	return l.recordArchiveWithHash(ctx, id, lastSeq, hash, info.Size(), info.ModTime().UnixNano())
}

func logMirrorFailure(root string, id servicechat.ID, err error) {
	log.Printf("chat %s: JSONL rollback archive write failed: %v", id, err)
}

// eventSearchText collects the human-readable text of an event for the FTS
// index. Tool payloads are deliberately excluded: they are large, noisy, and
// already searchable through their transcript entries.
func eventSearchText(ev servicechat.Event) string {
	var text string
	switch ev.Type {
	case "user", "assistant_text", "thinking", "error", "complete":
		text = ev.Text
		if ev.Message != "" {
			if text != "" {
				text += "\n"
			}
			text += ev.Message
		}
	default:
		return ""
	}
	return utf8Prefix(text, maxSearchTextBytes)
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

var errMissingArchive = errors.New("chat archive is missing")
