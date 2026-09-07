package filechat

import (
	"context"
	"errors"
	"os"

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

func (index *chatEventIndex) lastEventSeq(
	ctx context.Context,
	id servicechat.ID,
	eventsPath string,
) (int64, error) {
	state, err := index.syncChat(ctx, id, eventsPath)
	return state.lastSeq, err
}

func (index *chatEventIndex) refreshAfterAppend(
	ctx context.Context,
	id servicechat.ID,
	eventsPath string,
) error {
	_, err := index.syncChatWithGrowth(ctx, id, eventsPath, true)
	return err
}

func (index *chatEventIndex) refreshAfterFallback(
	ctx context.Context,
	id servicechat.ID,
	eventsPath string,
) error {
	_, err := index.syncChat(ctx, id, eventsPath)
	return err
}

// syncChat validates the durable snapshot against the authoritative JSONL
// file. Appends extend the committed rows; rewrites and truncations rebuild
// them. No new state becomes visible until the whole observed range commits.
func (index *chatEventIndex) syncChat(
	ctx context.Context,
	id servicechat.ID,
	eventsPath string,
) (chatIndexState, error) {
	return index.syncChatWithGrowth(ctx, id, eventsPath, false)
}

func (index *chatEventIndex) syncChatWithGrowth(
	ctx context.Context,
	id servicechat.ID,
	eventsPath string,
	trustedAppend bool,
) (chatIndexState, error) {
	if err := index.availabilityError(); err != nil {
		return chatIndexState{}, err
	}
	if err := ctx.Err(); err != nil {
		return chatIndexState{}, err
	}
	info, err := os.Stat(eventsPath)
	var fileSize int64
	var fileMtimeNS int64
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return chatIndexState{}, err
		}
	} else {
		fileSize = info.Size()
		fileMtimeNS = info.ModTime().UnixNano()
	}

	state, found, err := index.readState(ctx, id)
	if err != nil {
		return chatIndexState{}, err
	}
	rebuild := !found || state.indexedBytes > fileSize ||
		(state.indexedBytes == fileSize && state.fileMtimeNS != fileMtimeNS) ||
		(state.indexedBytes < fileSize && !state.tailComplete)
	if !rebuild && !trustedAppend && state.indexedBytes < fileSize {
		matches, matchErr := chatIndexPrefixMatches(ctx, eventsPath, state)
		if matchErr != nil {
			return chatIndexState{}, matchErr
		}
		rebuild = !matches
	}
	if !rebuild && state.indexedBytes == fileSize {
		return state, nil
	}

	tx, err := index.db.BeginTx(ctx, nil)
	if err != nil {
		return chatIndexState{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if rebuild {
		if err := deleteChatIndexRows(ctx, tx, id); err != nil {
			return chatIndexState{}, err
		}
		state = newChatIndexState()
	}

	turn, err := readLastIndexedTurn(ctx, tx, id)
	if err != nil {
		return chatIndexState{}, err
	}
	if fileSize > state.indexedBytes {
		writer := newChatIndexWriter(ctx, tx, id, state, turn)
		state, err = writer.indexTail(eventsPath, fileSize)
		if err != nil {
			return chatIndexState{}, err
		}
	}
	state.indexedBytes = fileSize
	state.fileMtimeNS = fileMtimeNS
	if err := writeChatIndexState(ctx, tx, id, state); err != nil {
		return chatIndexState{}, err
	}
	if err := tx.Commit(); err != nil {
		return chatIndexState{}, err
	}
	if err := index.restrictFiles(); err != nil {
		return chatIndexState{}, err
	}
	return state, nil
}

func (index *chatEventIndex) rebuildChat(
	ctx context.Context,
	id servicechat.ID,
	eventsPath string,
) error {
	if err := index.deleteChat(ctx, id); err != nil {
		return err
	}
	_, err := index.syncChat(ctx, id, eventsPath)
	return err
}

func (index *chatEventIndex) deleteChat(ctx context.Context, id servicechat.ID) error {
	if err := index.availabilityError(); err != nil {
		return err
	}
	tx, err := index.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := deleteChatIndexRows(ctx, tx, id); err != nil {
		return err
	}
	return tx.Commit()
}
