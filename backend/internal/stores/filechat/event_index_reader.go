package filechat

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

var errInvalidChatEventIndex = errors.New("invalid chat event index")

type indexedEventLocation struct {
	offset int64
	length int64
	seq    int64
}

func (index *chatEventIndex) readEventPage(
	ctx context.Context,
	id servicechat.ID,
	eventsPath string,
	beforeSeq int64,
	limit int,
) (servicechat.EventPage, error) {
	state, err := index.syncChat(ctx, id, eventsPath)
	if err != nil {
		return servicechat.EventPage{}, err
	}
	locations, hasMore, err := index.eventPageLocations(ctx, id, beforeSeq, limit)
	if err != nil {
		return servicechat.EventPage{}, err
	}
	events, err := readIndexedEvents(ctx, eventsPath, locations)
	if err != nil {
		return servicechat.EventPage{}, err
	}
	var nextBefore int64
	if hasMore && len(events) > 0 {
		nextBefore = events[0].Seq
	}
	return servicechat.EventPage{
		Events:     events,
		NextBefore: nextBefore,
		LastSeq:    state.lastSeq,
		HasMore:    hasMore,
	}, nil
}

func (index *chatEventIndex) readEventsAfter(
	ctx context.Context,
	id servicechat.ID,
	eventsPath string,
	afterSeq int64,
) ([]servicechat.Event, error) {
	if _, err := index.syncChat(ctx, id, eventsPath); err != nil {
		return nil, err
	}
	locations, err := index.eventLocationsAfter(ctx, id, afterSeq)
	if err != nil {
		return nil, err
	}
	return readIndexedEvents(ctx, eventsPath, locations)
}

func (index *chatEventIndex) readTranscriptWindow(
	ctx context.Context,
	id servicechat.ID,
	eventsPath string,
	beforeSeq int64,
	turnLimit int,
) (servicechat.TranscriptEventWindow, error) {
	state, err := index.syncChat(ctx, id, eventsPath)
	if err != nil {
		return servicechat.TranscriptEventWindow{}, err
	}
	locations, err := index.transcriptLocations(ctx, id, beforeSeq, turnLimit)
	if err != nil {
		return servicechat.TranscriptEventWindow{}, err
	}
	events, err := readIndexedEvents(ctx, eventsPath, locations)
	if err != nil {
		return servicechat.TranscriptEventWindow{}, err
	}
	return servicechat.TranscriptEventWindow{
		Events:  events,
		LastSeq: state.lastSeq,
	}, nil
}

func (index *chatEventIndex) transcriptLocations(
	ctx context.Context,
	id servicechat.ID,
	beforeSeq int64,
	turnLimit int,
) ([]indexedEventLocation, error) {
	if turnLimit <= 0 {
		turnLimit = 1
	}
	rows, err := index.db.QueryContext(ctx, `
		SELECT start_offset, end_offset
		FROM chat_transcript_turns
		WHERE chat_id = ? AND (? <= 0 OR start_seq < ?)
		ORDER BY turn_ordinal DESC
		LIMIT ?`, id, beforeSeq, beforeSeq, turnLimit+1,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var firstOffset int64 = -1
	var lastOffset int64
	for rows.Next() {
		var startOffset, endOffset int64
		if err := rows.Scan(&startOffset, &endOffset); err != nil {
			return nil, err
		}
		if startOffset < 0 || endOffset < startOffset {
			return nil, fmt.Errorf("%w: transcript turn range", errInvalidChatEventIndex)
		}
		if firstOffset < 0 || startOffset < firstOffset {
			firstOffset = startOffset
		}
		if endOffset > lastOffset {
			lastOffset = endOffset
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if firstOffset < 0 {
		return nil, nil
	}
	locations, err := index.eventLocationsInRange(ctx, id, firstOffset, lastOffset)
	if err != nil {
		return nil, err
	}
	if len(locations) == 0 || locations[0].offset != firstOffset {
		return nil, fmt.Errorf("%w: transcript range has no matching first event", errInvalidChatEventIndex)
	}
	last := locations[len(locations)-1]
	if last.length <= 0 || last.offset > lastOffset || lastOffset-last.offset != last.length {
		return nil, fmt.Errorf("%w: transcript range has no matching last event", errInvalidChatEventIndex)
	}
	return locations, nil
}

func (index *chatEventIndex) eventPageLocations(
	ctx context.Context,
	id servicechat.ID,
	beforeSeq int64,
	limit int,
) ([]indexedEventLocation, bool, error) {
	rows, err := index.db.QueryContext(ctx, `
		SELECT byte_offset, byte_length, event_seq
		FROM chat_event_offsets
		WHERE chat_id = ? AND (? <= 0 OR event_seq < ?)
		ORDER BY event_ordinal DESC
		LIMIT ?`, id, beforeSeq, beforeSeq, limit+1,
	)
	if err != nil {
		return nil, false, err
	}
	locations, err := scanEventLocations(rows)
	if err != nil {
		return nil, false, err
	}
	hasMore := len(locations) > limit
	if hasMore {
		locations = locations[:limit]
	}
	for left, right := 0, len(locations)-1; left < right; left, right = left+1, right-1 {
		locations[left], locations[right] = locations[right], locations[left]
	}
	return locations, hasMore, nil
}

func (index *chatEventIndex) eventLocationsAfter(
	ctx context.Context,
	id servicechat.ID,
	afterSeq int64,
) ([]indexedEventLocation, error) {
	rows, err := index.db.QueryContext(ctx, `
			SELECT byte_offset, byte_length, event_seq
			FROM chat_event_offsets INDEXED BY chat_event_offsets_by_seq
			WHERE chat_id = ? AND event_seq > ?
			ORDER BY event_ordinal`, id, afterSeq,
	)
	if err != nil {
		return nil, err
	}
	return scanEventLocations(rows)
}

func (index *chatEventIndex) eventLocationsInRange(
	ctx context.Context,
	id servicechat.ID,
	startOffset int64,
	endOffset int64,
) ([]indexedEventLocation, error) {
	rows, err := index.db.QueryContext(ctx, `
			SELECT byte_offset, byte_length, event_seq
			FROM chat_event_offsets INDEXED BY chat_event_offsets_by_offset
			WHERE chat_id = ? AND byte_offset >= ? AND byte_offset < ?
			ORDER BY byte_offset`, id, startOffset, endOffset,
	)
	if err != nil {
		return nil, err
	}
	return scanEventLocations(rows)
}

func scanEventLocations(rows *sql.Rows) ([]indexedEventLocation, error) {
	defer rows.Close()
	locations := make([]indexedEventLocation, 0, 64)
	for rows.Next() {
		var location indexedEventLocation
		if err := rows.Scan(&location.offset, &location.length, &location.seq); err != nil {
			return nil, err
		}
		locations = append(locations, location)
	}
	return locations, rows.Err()
}

func readIndexedEvents(
	ctx context.Context,
	eventsPath string,
	locations []indexedEventLocation,
) ([]servicechat.Event, error) {
	if len(locations) == 0 {
		return nil, nil
	}
	file, err := os.Open(eventsPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	if locations[0].offset < 0 {
		return nil, fmt.Errorf("%w: event offset is negative", errInvalidChatEventIndex)
	}
	if _, err := file.Seek(locations[0].offset, io.SeekStart); err != nil {
		return nil, err
	}
	reader := bufio.NewReader(file)
	position := locations[0].offset

	events := make([]servicechat.Event, 0, len(locations))
	for _, location := range locations {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if location.length <= 0 || location.length > maxEventRecordBytes+2 {
			return nil, fmt.Errorf("%w: event length %d", errInvalidChatEventIndex, location.length)
		}
		if location.offset < position {
			return nil, fmt.Errorf("%w: event offsets are out of order", errInvalidChatEventIndex)
		}
		if gap := location.offset - position; gap > 0 {
			if _, err := io.CopyN(io.Discard, reader, gap); err != nil {
				return nil, indexedReadError(err)
			}
			position += gap
		}
		line := make([]byte, int(location.length))
		if _, err := io.ReadFull(reader, line); err != nil {
			return nil, indexedReadError(err)
		}
		position += location.length
		line = bytes.TrimSuffix(line, []byte{'\n'})
		line = bytes.TrimSuffix(line, []byte{'\r'})
		event, err := decodeStoredEvent(line, location.seq)
		if err != nil {
			return nil, fmt.Errorf(
				"%w: decode event at byte %d: %v",
				errInvalidChatEventIndex,
				location.offset,
				err,
			)
		}
		if event.Seq != location.seq {
			return nil, fmt.Errorf(
				"%w: event sequence mismatch at byte %d: row has %d, record has %d",
				errInvalidChatEventIndex,
				location.offset,
				location.seq,
				event.Seq,
			)
		}
		events = append(events, event)
	}
	return events, nil
}

func indexedReadError(err error) error {
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return fmt.Errorf("%w: event range exceeds canonical log: %v", errInvalidChatEventIndex, err)
	}
	return err
}
