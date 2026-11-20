package sqlitechat

import (
	"sort"
	"strconv"

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

type transcriptItem struct {
	turnOrdinal  int64
	turnID       string
	turnStart    int64
	startSeq     int64
	endSeq       int64
	payload      []servicechat.Event
	payloadBytes int
}

// transcriptPageBuffer owns the budget and selected items for one reverse read.
// It assembles already bounded event previews in display order.
type transcriptPageBuffer struct {
	turnLimit int
	byteLimit int
	usedBytes int
	hasMore   bool
	items     []transcriptItem
	turns     map[int64]struct{}
}

func newTranscriptPageBuffer(turnLimit, byteLimit int) *transcriptPageBuffer {
	return &transcriptPageBuffer{
		turnLimit: turnLimit,
		byteLimit: byteLimit,
		items:     make([]transcriptItem, 0, 64),
		turns:     make(map[int64]struct{}, turnLimit),
	}
}

func (buffer *transcriptPageBuffer) appendItem(item transcriptItem) bool {
	_, knownTurn := buffer.turns[item.turnOrdinal]
	if (!knownTurn && len(buffer.turns) == buffer.turnLimit) ||
		(len(buffer.items) > 0 && buffer.usedBytes+item.payloadBytes > buffer.byteLimit) {
		buffer.hasMore = true
		return false
	}

	buffer.items = append(buffer.items, item)
	buffer.turns[item.turnOrdinal] = struct{}{}
	buffer.usedBytes += item.payloadBytes
	return true
}

func (buffer *transcriptPageBuffer) page(lastSeq int64) servicechat.TranscriptPage {
	for left, right := 0, len(buffer.items)-1; left < right; left, right = left+1, right-1 {
		buffer.items[left], buffer.items[right] = buffer.items[right], buffer.items[left]
	}
	turns := make([]servicechat.TranscriptTurn, 0, len(buffer.turns))
	var lastTurnOrdinal int64 = -1
	for _, item := range buffer.items {
		last := len(turns) - 1
		if last < 0 || lastTurnOrdinal != item.turnOrdinal {
			turns = append(turns, servicechat.TranscriptTurn{
				ID:       transcriptTurnID(item),
				StartSeq: item.startSeq,
				EndSeq:   item.endSeq,
				Events:   append([]servicechat.Event(nil), item.payload...),
			})
			lastTurnOrdinal = item.turnOrdinal
			continue
		}
		if item.endSeq > turns[last].EndSeq {
			turns[last].EndSeq = item.endSeq
		}
		turns[last].Events = append(turns[last].Events, item.payload...)
	}
	for index := range turns {
		sort.SliceStable(turns[index].Events, func(left, right int) bool {
			return turns[index].Events[left].Seq < turns[index].Events[right].Seq
		})
	}

	var nextBefore int64
	if buffer.hasMore && len(buffer.items) > 0 {
		nextBefore = buffer.items[0].startSeq
	}
	return servicechat.TranscriptPage{
		Turns:      turns,
		NextBefore: nextBefore,
		LastSeq:    lastSeq,
		HasMore:    buffer.hasMore,
	}
}

func transcriptTurnID(item transcriptItem) string {
	if item.turnID != "" {
		return item.turnID
	}
	return "legacy-" + strconv.FormatInt(item.turnStart, 10)
}
