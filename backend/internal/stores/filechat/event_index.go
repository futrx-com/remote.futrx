package filechat

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"

	sqlite "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

const chatEventIndexFilename = "transcript-index.sqlite"

type chatEventIndex struct {
	db          *sql.DB
	path        string
	unavailable error
}

func newChatEventIndex(root string) (*chatEventIndex, error) {
	path := filepath.Join(root, chatEventIndexFilename)
	index, err := openChatEventIndex(path)
	if err == nil {
		return index, nil
	}
	if !isCorruptIndexError(err) {
		return nil, err
	}
	if removeErr := removeSQLiteFiles(path); removeErr != nil {
		return nil, errors.Join(err, removeErr)
	}
	return openChatEventIndex(path)
}

func openChatEventIndex(path string) (*chatEventIndex, error) {
	if err := createPrivateSQLiteFile(path); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", sqliteDSN(path))
	if err != nil {
		return nil, err
	}
	// WAL lets reads for already-indexed chats proceed while another chat is
	// backfilled. Every pooled connection receives the hardened DSN settings.
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)

	index := &chatEventIndex{db: db, path: path}
	if err := index.initializeSchema(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("initialize chat event index: %w", err)
	}
	if err := chmodPrivateSQLiteFiles(index.path); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("secure chat event index: %w", err)
	}
	return index, nil
}

func isCorruptIndexError(err error) bool {
	var sqliteErr *sqlite.Error
	if !errors.As(err, &sqliteErr) {
		return false
	}
	code := sqliteErr.Code() & 0xff
	return code == sqlite3.SQLITE_CORRUPT || code == sqlite3.SQLITE_NOTADB
}

func unavailableChatEventIndex(root string, err error) *chatEventIndex {
	return &chatEventIndex{
		path:        filepath.Join(root, chatEventIndexFilename),
		unavailable: err,
	}
}

func (index *chatEventIndex) availabilityError() error {
	if index == nil {
		return errors.New("chat event index is unavailable")
	}
	return index.unavailable
}

func (index *chatEventIndex) close() error {
	if index == nil || index.db == nil {
		return nil
	}
	return index.db.Close()
}
