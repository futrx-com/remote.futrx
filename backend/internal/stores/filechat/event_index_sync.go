package filechat

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"

	configconstants "github.com/futrx-com/remote.futrx.com/internal/config/constants"
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
// them. Large legacy files publish resumable checkpoints; transcript readers
// only serve the projection after the observed file is fully indexed.
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

	if rebuild {
		tx, err := index.db.BeginTx(ctx, nil)
		if err != nil {
			return chatIndexState{}, err
		}
		if err := deleteChatIndexRows(ctx, tx, id); err != nil {
			_ = tx.Rollback()
			return chatIndexState{}, err
		}
		state = newChatIndexState()
		state.fileMtimeNS = fileMtimeNS
		if err := writeChatIndexState(ctx, tx, id, state); err != nil {
			_ = tx.Rollback()
			return chatIndexState{}, err
		}
		if err := tx.Commit(); err != nil {
			return chatIndexState{}, err
		}
	}

	for state.indexedBytes < fileSize {
		if err := ctx.Err(); err != nil {
			return state, err
		}
		checkpoint, err := chatIndexCheckpoint(eventsPath, state.indexedBytes, fileSize)
		if err != nil {
			return state, err
		}
		tx, err := index.db.BeginTx(ctx, nil)
		if err != nil {
			return state, err
		}
		turn, err := readLastIndexedTurn(ctx, tx, id)
		if err != nil {
			_ = tx.Rollback()
			return state, err
		}
		writer := newChatIndexWriter(ctx, tx, id, state, turn)
		state, err = writer.indexTail(eventsPath, checkpoint)
		if err != nil {
			_ = tx.Rollback()
			return state, err
		}
		state.fileMtimeNS = fileMtimeNS
		if err := writeChatIndexState(ctx, tx, id, state); err != nil {
			_ = tx.Rollback()
			return state, err
		}
		if err := tx.Commit(); err != nil {
			return state, err
		}
	}
	if err := index.restrictFiles(); err != nil {
		return state, err
	}
	return state, nil
}

func chatIndexCheckpoint(eventsPath string, indexedBytes, fileSize int64) (int64, error) {
	target := indexedBytes + configconstants.ChatIndexCheckpointBytes
	if target >= fileSize {
		return fileSize, nil
	}
	file, err := os.Open(eventsPath)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	if _, err := file.Seek(target, io.SeekStart); err != nil {
		return 0, err
	}
	raw, err := bufio.NewReader(file).ReadBytes('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return 0, err
	}
	return target + int64(len(raw)), nil
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
