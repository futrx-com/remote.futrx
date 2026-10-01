package filechat

import (
	"context"

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

// transcriptProjection owns the disposable read model built over an event
// log. JSONL addresses source events by byte range in a separate index, while
// SQLite addresses them by sequence in the event database itself.
type transcriptProjection interface {
	availabilityError() error
	sync(ctx context.Context, id servicechat.ID) (chatIndexState, error)
	delete(ctx context.Context, id servicechat.ID) error
	readPage(
		ctx context.Context,
		id servicechat.ID,
		query servicechat.TranscriptPageQuery,
	) (servicechat.TranscriptPage, error)
	readContent(
		ctx context.Context,
		id servicechat.ID,
		contentID string,
		afterBytes int64,
		limitBytes int,
	) (servicechat.TranscriptContentPage, error)
	readWindow(
		ctx context.Context,
		id servicechat.ID,
		beforeSeq int64,
		turnLimit int,
	) (servicechat.TranscriptEventWindow, error)
	close() error
}

type jsonlTranscriptProjection struct {
	store *Store
	index *chatEventIndex
}

func newJSONLTranscriptProjection(
	store *Store,
	index *chatEventIndex,
) *jsonlTranscriptProjection {
	return &jsonlTranscriptProjection{store: store, index: index}
}

func (p *jsonlTranscriptProjection) availabilityError() error {
	return p.index.availabilityError()
}

func (p *jsonlTranscriptProjection) sync(
	ctx context.Context,
	id servicechat.ID,
) (chatIndexState, error) {
	if err := p.availabilityError(); err != nil {
		return chatIndexState{}, err
	}
	return p.index.syncChat(ctx, id, p.store.eventsPath(id))
}

func (p *jsonlTranscriptProjection) delete(ctx context.Context, id servicechat.ID) error {
	return p.index.deleteChat(ctx, id)
}

func (p *jsonlTranscriptProjection) readWindow(
	ctx context.Context,
	id servicechat.ID,
	beforeSeq int64,
	turnLimit int,
) (servicechat.TranscriptEventWindow, error) {
	return p.index.readTranscriptWindow(
		ctx, id, p.store.eventsPath(id), beforeSeq, turnLimit,
	)
}

func (p *jsonlTranscriptProjection) close() error {
	return p.index.close()
}

var (
	_ transcriptProjection = (*jsonlTranscriptProjection)(nil)
	_ transcriptProjection = (*sqliteTranscriptProjection)(nil)
)
