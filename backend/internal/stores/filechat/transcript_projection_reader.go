package filechat

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"

	configconstants "github.com/futrx-com/remote.futrx.com/internal/config/constants"
	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

// ReadTranscriptPage serves only complete materialized snapshots. Missing or
// stale legacy projections are rebuilt in the background and exposed as byte
// progress, so opening a large chat never performs a gigabyte request scan.
func (s *Store) ReadTranscriptPage(
	ctx context.Context,
	id servicechat.ID,
	query servicechat.TranscriptPageQuery,
) (servicechat.TranscriptPage, error) {
	if !servicechat.ValidID(id) {
		return servicechat.TranscriptPage{}, servicechat.ErrInvalidID
	}
	if err := s.index.availabilityError(); err != nil {
		return servicechat.TranscriptPage{}, fmt.Errorf(
			"%w: %v", servicechat.ErrTranscriptProjectionUnavailable, err,
		)
	}

	info, err := os.Stat(s.eventsPath(id))
	var totalBytes, mtimeNS int64
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return servicechat.TranscriptPage{}, err
		}
	} else {
		totalBytes = info.Size()
		mtimeNS = info.ModTime().UnixNano()
	}
	state, found, err := s.index.readState(ctx, id)
	if err != nil {
		return servicechat.TranscriptPage{}, err
	}
	ready := found && state.indexedBytes == totalBytes && state.fileMtimeNS == mtimeNS
	if !ready {
		s.startTranscriptIndex(id)
		indexedBytes := state.indexedBytes
		if indexedBytes < 0 || indexedBytes > totalBytes {
			indexedBytes = 0
		}
		lastSeq := state.lastSeq
		tailSeqKnown := totalBytes == 0
		if tailSeq, tailErr := lastStoredEventSeq(s.eventsPath(id), totalBytes); tailErr == nil && tailSeq > 0 {
			tailSeqKnown = true
			if tailSeq > lastSeq {
				lastSeq = tailSeq
			}
		}
		return servicechat.TranscriptPage{
			Turns:   []servicechat.TranscriptTurn{},
			LastSeq: lastSeq,
			Indexing: &servicechat.TranscriptIndexProgress{
				IndexedBytes: indexedBytes,
				TotalBytes:   totalBytes,
				TailSeqKnown: tailSeqKnown,
			},
		}, nil
	}
	return s.index.readProjectedTranscriptPage(ctx, id, state, query)
}

func lastStoredEventSeq(eventsPath string, fileSize int64) (int64, error) {
	if fileSize <= 0 {
		return 0, nil
	}
	window := int64(maxEventRecordBytes + 2)
	if window > fileSize {
		window = fileSize
	}
	file, err := os.Open(eventsPath)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	raw := make([]byte, int(window))
	if _, err := file.ReadAt(raw, fileSize-window); err != nil && !errors.Is(err, io.EOF) {
		return 0, err
	}
	lines := bytes.Split(raw, []byte{'\n'})
	for index := len(lines) - 1; index >= 0; index-- {
		line := bytes.TrimSuffix(lines[index], []byte{'\r'})
		if len(line) == 0 {
			continue
		}
		event, err := decodeStoredEvent(line, 0)
		if err == nil && event.Seq > 0 {
			return event.Seq, nil
		}
	}
	return 0, errors.New("last stored event has no sequence")
}

func (s *Store) startTranscriptIndex(id servicechat.ID) {
	s.indexingMu.Lock()
	if _, exists := s.indexing[id]; exists {
		s.indexingMu.Unlock()
		return
	}
	s.indexing[id] = struct{}{}
	s.indexWG.Add(1)
	s.indexingMu.Unlock()

	go func() {
		defer s.indexWG.Done()
		defer func() {
			s.indexingMu.Lock()
			delete(s.indexing, id)
			s.indexingMu.Unlock()
		}()
		lock := s.lock(id)
		lock.Lock()
		defer lock.Unlock()
		if _, err := os.Stat(s.chatDir(id)); err != nil {
			return
		}
		if _, err := s.index.syncChat(s.indexContext, id, s.eventsPath(id)); err != nil &&
			!errors.Is(err, context.Canceled) {
			log.Printf("background transcript projection for chat %s failed: %v", id, err)
		}
	}()
}

func (index *chatEventIndex) readProjectedTranscriptPage(
	ctx context.Context,
	id servicechat.ID,
	state chatIndexState,
	query servicechat.TranscriptPageQuery,
) (servicechat.TranscriptPage, error) {
	turnLimit := query.Limit
	if turnLimit <= 0 {
		turnLimit = configconstants.DefaultChatTranscriptTurnLimit
	}
	if turnLimit > configconstants.MaxChatTranscriptTurnLimit {
		turnLimit = configconstants.MaxChatTranscriptTurnLimit
	}
	byteLimit := query.ByteLimit
	if byteLimit <= 0 {
		byteLimit = configconstants.DefaultChatTranscriptByteLimit
	}
	if byteLimit > configconstants.MaxChatTranscriptByteLimit {
		byteLimit = configconstants.MaxChatTranscriptByteLimit
	}

	rows, err := index.db.QueryContext(ctx, `
		SELECT i.turn_ordinal, t.source_turn_id, t.start_seq,
		       i.start_seq, i.end_seq, i.payload_json, i.payload_bytes
		FROM chat_transcript_items AS i
		JOIN chat_transcript_turns AS t
		  ON t.chat_id = i.chat_id AND t.turn_ordinal = i.turn_ordinal
		WHERE i.chat_id = ? AND (? <= 0 OR i.start_seq < ?)
		ORDER BY i.start_seq DESC, i.item_key DESC`,
		id, query.BeforeSeq, query.BeforeSeq,
	)
	if err != nil {
		return servicechat.TranscriptPage{}, err
	}
	defer rows.Close()

	pageBuffer := newTranscriptPageBuffer(turnLimit, byteLimit)
	for rows.Next() {
		var item projectedTranscriptItem
		var payloadJSON []byte
		if err := rows.Scan(
			&item.turnOrdinal,
			&item.turnID,
			&item.turnStart,
			&item.startSeq,
			&item.endSeq,
			&payloadJSON,
			&item.payloadBytes,
		); err != nil {
			return servicechat.TranscriptPage{}, err
		}
		appended, err := pageBuffer.appendItem(item, payloadJSON)
		if err != nil {
			return servicechat.TranscriptPage{}, err
		}
		if !appended {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return servicechat.TranscriptPage{}, err
	}

	return pageBuffer.page(state.lastSeq), nil
}
