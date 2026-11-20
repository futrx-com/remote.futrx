package sqlitechat

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"

	constants "github.com/futrx-com/remote.futrx.com/internal/config/constants"
	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

func (s *Store) ReadTranscriptPage(ctx context.Context, id servicechat.ID, q servicechat.TranscriptPageQuery) (servicechat.TranscriptPage, error) {
	branch, last, err := activeBranch(ctx, s.db, id)
	if err != nil {
		return servicechat.TranscriptPage{}, err
	}
	limit := q.Limit
	if limit <= 0 {
		limit = constants.DefaultChatTranscriptTurnLimit
	}
	if limit > constants.MaxChatTranscriptTurnLimit {
		limit = constants.MaxChatTranscriptTurnLimit
	}
	budget := q.ByteLimit
	if budget <= 0 {
		budget = constants.DefaultChatTranscriptByteLimit
	}
	if budget > constants.MaxChatTranscriptByteLimit {
		budget = constants.MaxChatTranscriptByteLimit
	}
	before := q.BeforeSeq
	if before <= 0 {
		before = math.MaxInt64
	}
	segments, err := branchSegments(ctx, s.db, branch, last)
	if err != nil {
		return servicechat.TranscriptPage{}, err
	}
	buffer := newTranscriptPageBuffer(limit, budget)
	for _, seg := range segments {
		cursor := before
		stop := false
		for !stop {
			headers, err := s.readTranscriptHeaders(ctx, seg, cursor)
			if err != nil {
				return servicechat.TranscriptPage{}, err
			}
			for _, h := range headers {
				item := h.item
				if _, known := buffer.turns[item.turnOrdinal]; !known && len(buffer.turns) == buffer.turnLimit {
					buffer.hasMore = true
					stop = true
					break
				}
				item, err = s.readTranscriptItem(ctx, seg, h)
				if err != nil {
					return servicechat.TranscriptPage{}, err
				}
				added := buffer.appendItem(item)
				if !added {
					stop = true
					break
				}
			}
			if stop || len(headers) < 64 {
				break
			}
			cursor = headers[len(headers)-1].item.startSeq
		}
		if stop {
			break
		}
	}
	return buffer.page(last), nil
}

type transcriptItemHeader struct {
	item           transcriptItem
	key, eventType string
	mode           transcriptItemMode
}

// Read and close a bounded header cursor before fetching any item payloads.
// This keeps concurrent page readers from exhausting the connection pool.
func (s *Store) readTranscriptHeaders(ctx context.Context, seg branchSegment, before int64) ([]transcriptItemHeader, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT turn_ordinal,turn_key,turn_start_seq,seq,item_key,item_mode,event_type
                FROM events WHERE branch_id=? AND visible=1 AND item_start_seq<? AND seq<=? AND seq=item_start_seq
                ORDER BY item_start_seq DESC,seq DESC LIMIT 64`, seg.id, before, seg.through)
	if err != nil {
		return nil, err
	}
	headers := make([]transcriptItemHeader, 0, 64)
	for rows.Next() {
		var h transcriptItemHeader
		if err = rows.Scan(&h.item.turnOrdinal, &h.item.turnID, &h.item.turnStart, &h.item.startSeq, &h.key, &h.mode, &h.eventType); err != nil {
			rows.Close()
			return nil, err
		}
		headers = append(headers, h)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	return headers, nil
}

func (s *Store) readTranscriptItem(ctx context.Context, seg branchSegment, h transcriptItemHeader) (transcriptItem, error) {
	item, key, mode, eventType := h.item, h.key, h.mode, h.eventType
	var seqs []int64
	if mode == transcriptItemSingle {
		seqs = []int64{item.startSeq}
	} else {
		types := []string{eventType}
		if mode == transcriptItemAppend {
			if eventType == "tool_use_start" || eventType == "tool_use_end" {
				types = []string{"tool_use_start", "tool_use_end"}
			} else {
				types = []string{"interaction_request", "interaction_resolved"}
			}
		}
		for _, typ := range types {
			var seq int64
			err := s.db.QueryRowContext(ctx, `SELECT seq FROM events WHERE branch_id=? AND turn_ordinal=? AND item_key=? AND event_type=? AND seq<=? ORDER BY seq DESC LIMIT 1`, seg.id, item.turnOrdinal, key, typ, seg.through).Scan(&seq)
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			if err != nil {
				return item, err
			}
			seqs = append(seqs, seq)
		}
	}
	for _, seq := range seqs {
		ev, err := s.readDisplayEvent(ctx, seg.id, seq)
		if err != nil {
			return item, err
		}
		if seq > item.endSeq {
			item.endSeq = seq
		}
		if mode == transcriptItemReplace {
			ev.Seq = item.startSeq
		}
		item.payload = append(item.payload, ev)
	}
	raw, err := json.Marshal(item.payload)
	if err != nil {
		return item, err
	}
	item.payloadBytes = len(raw)
	return item, nil
}
