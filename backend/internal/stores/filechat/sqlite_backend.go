package filechat

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

// chatStoreFilename is the shared database that owns every chat's event
// stream when the SQLite backend is selected.
const chatStoreFilename = "chats.sqlite"

// chatStoreDB is the shared event database. It is authoritative storage, so
// unlike the transcript index it is migrated forward and never dropped.
type chatStoreDB struct {
	db   *sql.DB
	path string
}

func openChatStoreDB(root string) (*chatStoreDB, error) {
	path := filepath.Join(root, chatStoreFilename)
	store, err := openChatStoreDBFile(path)
	if err == nil {
		return store, nil
	}
	if !isCorruptIndexError(err) {
		return nil, err
	}
	// The database is authoritative, so it is moved aside rather than deleted:
	// a fresh database reimports every chat from its JSONL archive, while the
	// damaged file stays on disk for inspection.
	log.Printf("chat store database is corrupt; quarantining it and reimporting from JSONL archives: %v", err)
	if quarantineErr := quarantineChatStoreFiles(path); quarantineErr != nil {
		return nil, errors.Join(err, quarantineErr)
	}
	return openChatStoreDBFile(path)
}

func openChatStoreDBFile(path string) (*chatStoreDB, error) {
	if err := createPrivateSQLiteFile(path); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", sqliteDSN(path))
	if err != nil {
		return nil, err
	}
	// WAL keeps transcript and event reads responsive while another chat is
	// being imported. Every pooled connection receives the hardened DSN.
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)

	store := &chatStoreDB{db: db, path: path}
	if err := store.initializeSchema(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("initialize chat store: %w", err)
	}
	if err := chmodPrivateSQLiteFiles(path); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("secure chat store: %w", err)
	}
	return store, nil
}

func (store *chatStoreDB) close() error {
	if store == nil || store.db == nil {
		return nil
	}
	return store.db.Close()
}

func quarantineChatStoreFiles(path string) error {
	marker := fmt.Sprintf(".corrupt-%d", time.Now().UnixNano())
	for _, suffix := range sqliteFileSuffixes {
		source := path + suffix
		info, err := os.Lstat(source)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("refuse to quarantine non-regular sqlite file: %s", source)
		}
		if err := os.Rename(source, path+marker+suffix); err != nil {
			return err
		}
	}
	return nil
}
