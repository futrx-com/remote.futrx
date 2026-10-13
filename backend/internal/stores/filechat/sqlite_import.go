package filechat

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"io"
	"os"
	"time"

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

// importBatchEvents bounds how many events are written per transaction while
// an archive is imported, so one huge chat cannot hold a write lock for the
// whole scan.
const importBatchEvents = 500

var archiveNewline = []byte{'\n'}

// archiveState tracks how much of a chat's JSONL rollback archive has been
// imported into the database.
type archiveState struct {
	sourceBytes int64
	sourceMtime int64
	prefixHash  uint64
	lastSeq     int64
	found       bool
}

// Prepare imports any archive history the database has not seen yet. It runs
// under the per-chat lock before every read, so it must return quickly when
// the archive has not changed.
func (l *sqliteLog) Prepare(ctx context.Context, id servicechat.ID) error {
	path := l.store.eventsPath(id)
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// The archive is created together with the chat, so a missing file
			// only means there is nothing to import yet.
			return nil
		}
		return err
	}
	size, mtime := info.Size(), info.ModTime().UnixNano()

	state, err := readArchiveState(ctx, l.db.db, id)
	if err != nil {
		return err
	}
	if !state.found {
		return l.importArchive(ctx, id, path, archiveState{}, size, mtime, true)
	}
	if size == state.sourceBytes && mtime == state.sourceMtime {
		return nil
	}
	if size < state.sourceBytes {
		// The archive shrank, which means it was rewritten (a rewind while the
		// database was rolled back). It is authoritative again.
		return l.importArchive(ctx, id, path, archiveState{}, size, mtime, true)
	}

	prefixMatches, err := archivePrefixMatches(ctx, path, state)
	if err != nil {
		return err
	}
	if !prefixMatches {
		return l.importArchive(ctx, id, path, archiveState{}, size, mtime, true)
	}
	if size == state.sourceBytes {
		// Only the modification time changed: the content still matches, so
		// just refresh the fingerprint.
		return l.recordArchive(ctx, id, state, size, mtime)
	}
	return l.importArchive(ctx, id, path, state, size, mtime, false)
}

func readArchiveState(
	ctx context.Context,
	db *sql.DB,
	id servicechat.ID,
) (archiveState, error) {
	var state archiveState
	var prefixHash int64
	err := db.QueryRowContext(ctx, `
		SELECT source_bytes, source_mtime_ns, source_prefix_hash, last_seq
		FROM chat_event_state
		WHERE chat_id = ?`, id,
	).Scan(&state.sourceBytes, &state.sourceMtime, &prefixHash, &state.lastSeq)
	if errors.Is(err, sql.ErrNoRows) {
		return archiveState{}, nil
	}
	if err != nil {
		return archiveState{}, err
	}
	state.prefixHash = uint64(prefixHash)
	state.found = true
	return state, nil
}

// archiveImported reports whether every archived event has been folded into
// the database, which is what makes the stored sequence numbers describe the
// whole stream rather than only what has been imported so far. The measured
// archive size comes back so a caller can read the tail sequence directly
// while the import is still running.
func (p *sqliteTranscriptProjection) archiveImported(
	ctx context.Context,
	id servicechat.ID,
) (bool, int64, error) {
	if p.db == nil {
		return false, 0, errChatStoreUnavailable
	}
	info, err := os.Stat(p.store.eventsPath(id))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return true, 0, nil
		}
		return false, 0, err
	}
	state, err := readArchiveState(ctx, p.db.db, id)
	if err != nil {
		return false, info.Size(), err
	}
	if !state.found {
		return info.Size() == 0, info.Size(), nil
	}
	imported := state.sourceBytes == info.Size() &&
		state.sourceMtime == info.ModTime().UnixNano()
	return imported, info.Size(), nil
}

// importArchive streams events from the archive into the database. When
// deleteFirst is set the chat's rows are rebuilt from the archive, which is
// what a first import or a rewritten archive requires.
func (l *sqliteLog) importArchive(
	ctx context.Context,
	id servicechat.ID,
	path string,
	state archiveState,
	size int64,
	mtime int64,
	deleteFirst bool,
) error {
	if deleteFirst {
		if _, err := l.db.db.ExecContext(ctx,
			`DELETE FROM chat_events WHERE chat_id = ?`, id); err != nil {
			return err
		}
	}

	offset := state.sourceBytes
	fallbackSeq := state.lastSeq
	hash := state.prefixHash
	if !state.found {
		offset, fallbackSeq = 0, 0
		hash = indexPrefixHashOffset64
	}

	batch := make([]servicechat.Event, 0, importBatchEvents)
	lastSeq := state.lastSeq
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		tx, err := l.db.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback() }()
		for _, event := range batch {
			if event.Seq > lastSeq {
				lastSeq = event.Seq
			}
			if err := upsertEventRow(ctx, tx, id, event); err != nil {
				return err
			}
		}
		if err := writeEventState(ctx, tx, id, lastSeq); err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		batch = batch[:0]
		return nil
	}

	consumedHash, err := scanArchiveEvents(ctx, path, offset, fallbackSeq, hash,
		func(event servicechat.Event) error {
			batch = append(batch, event)
			if len(batch) >= importBatchEvents {
				return flush()
			}
			return nil
		},
	)
	if err != nil {
		return err
	}
	if err := flush(); err != nil {
		return err
	}
	return l.recordArchiveWithHash(ctx, id, lastSeq, consumedHash, size, mtime)
}

func (l *sqliteLog) recordArchive(
	ctx context.Context,
	id servicechat.ID,
	state archiveState,
	size int64,
	mtime int64,
) error {
	return l.recordArchiveWithHash(ctx, id, state.lastSeq, state.prefixHash, size, mtime)
}

func (l *sqliteLog) recordArchiveWithHash(
	ctx context.Context,
	id servicechat.ID,
	lastSeq int64,
	hash uint64,
	size int64,
	mtime int64,
) error {
	_, err := l.db.db.ExecContext(ctx, `
		INSERT INTO chat_event_state
			(chat_id, last_seq, source_bytes, source_mtime_ns,
			 source_prefix_hash, imported_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(chat_id) DO UPDATE SET
			last_seq = excluded.last_seq,
			source_bytes = excluded.source_bytes,
			source_mtime_ns = excluded.source_mtime_ns,
			source_prefix_hash = excluded.source_prefix_hash,
			imported_at = excluded.imported_at`,
		id, lastSeq, size, mtime, int64(hash), time.Now().UnixMilli(),
	)
	return err
}

// scanArchiveEvents reads events from offset, mirroring the canonical JSONL
// scanner: empty lines are skipped, unparsable lines are ignored, and records
// without a sequence fall back to their ordinal. It returns the rolling prefix
// hash covering every byte it consumed.
func scanArchiveEvents(
	ctx context.Context,
	path string,
	offset int64,
	fallbackSeq int64,
	initialHash uint64,
	visit func(servicechat.Event) error,
) (uint64, error) {
	file, err := os.Open(path)
	if err != nil {
		return initialHash, err
	}
	defer file.Close()
	if offset > 0 {
		if _, err := file.Seek(offset, io.SeekStart); err != nil {
			return initialHash, err
		}
	}

	hash := initialHash
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), maxEventRecordBytes)
	seq := fallbackSeq
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return hash, err
		}
		line := scanner.Bytes()
		hash = updateIndexPrefixHash(hash, line)
		hash = updateIndexPrefixHash(hash, archiveNewline)
		if len(line) == 0 {
			continue
		}
		seq++
		event, err := decodeStoredEvent(line, seq)
		if err != nil {
			continue
		}
		if err := visit(event); err != nil {
			return hash, err
		}
	}
	if err := scanner.Err(); err != nil {
		return hash, err
	}
	return hash, nil
}
