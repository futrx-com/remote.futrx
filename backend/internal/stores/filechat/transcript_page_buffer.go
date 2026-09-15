package filechat

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

type projectedTranscriptItem struct {
	turnOrdinal  int64
	turnID       string
	turnStart    int64
	startSeq     int64
	endSeq       int64
	payload      []servicechat.Event
	payloadBytes int
}

// transcriptPageBuffer owns the budget and selected items for one reverse read.
// It decodes accepted payloads only, then assembles turns in display order.
type transcriptPageBuffer struct {
	turnLimit int
	byteLimit int
	usedBytes int
	hasMore   bool
	items     []projectedTranscriptItem
	turns     map[int64]struct{}
}

func newTranscriptPageBuffer(turnLimit, byteLimit int) *transcriptPageBuffer {
	return &transcriptPageBuffer{
		turnLimit: turnLimit,
		byteLimit: byteLimit,
		items:     make([]projectedTranscriptItem, 0, 64),
		turns:     make(map[int64]struct{}, turnLimit),
	}
}

func (buffer *transcriptPageBuffer) appendItem(item projectedTranscriptItem, payloadJSON []byte) (bool, error) {
	_, knownTurn := buffer.turns[item.turnOrdinal]
	if (!knownTurn && len(buffer.turns) == buffer.turnLimit) ||
		(len(buffer.items) > 0 && buffer.usedBytes+item.payloadBytes > buffer.byteLimit) {
		buffer.hasMore = true
		return false, nil
	}
	if err := json.Unmarshal(payloadJSON, &item.payload); err != nil {
		return false, fmt.Errorf("%w: decode projected item: %v", errInvalidChatEventIndex, err)
	}
	buffer.items = append(buffer.items, item)
	buffer.turns[item.turnOrdinal] = struct{}{}
	buffer.usedBytes += item.payloadBytes
	return true, nil
}

func (buffer *transcriptPageBuffer) page(lastSeq int64) servicechat.TranscriptPage {
	for left, right := 0, len(buffer.items)-1; left < right; left, right = left+1, right-1 {
		buffer.items[left], buffer.items[right] = buffer.items[right], buffer.items[left]
	}
	projectedTurns := make([]servicechat.TranscriptTurn, 0, len(buffer.turns))
	var lastTurnOrdinal int64 = -1
	for _, item := range buffer.items {
		last := len(projectedTurns) - 1
		if last < 0 || lastTurnOrdinal != item.turnOrdinal {
			projectedTurns = append(projectedTurns, servicechat.TranscriptTurn{
				ID:       projectedTurnID(item),
				StartSeq: item.startSeq,
				EndSeq:   item.endSeq,
				Events:   append([]servicechat.Event(nil), item.payload...),
			})
			lastTurnOrdinal = item.turnOrdinal
			continue
		}
		projectedTurns[last].EndSeq = item.endSeq
		projectedTurns[last].Events = append(projectedTurns[last].Events, item.payload...)
	}
	for index := range projectedTurns {
		sort.SliceStable(projectedTurns[index].Events, func(left, right int) bool {
			return projectedTurns[index].Events[left].Seq < projectedTurns[index].Events[right].Seq
		})
	}

	var nextBefore int64
	if buffer.hasMore && len(buffer.items) > 0 {
		nextBefore = buffer.items[0].startSeq
	}
	return servicechat.TranscriptPage{
		Turns:      projectedTurns,
		NextBefore: nextBefore,
		LastSeq:    lastSeq,
		HasMore:    buffer.hasMore,
	}
}

func projectedTurnID(item projectedTranscriptItem) string {
	if item.turnID != "" {
		return item.turnID
	}
	return "legacy-" + strconv.FormatInt(item.turnStart, 10)
}
