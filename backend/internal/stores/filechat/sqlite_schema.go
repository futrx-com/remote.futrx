package filechat

import (
	"fmt"
)

// chatStoreSchemaVersion is the schema revision written to chats.sqlite by
// this binary. Unlike the disposable transcript index, this database is
// authoritative: schema changes are applied forward-only and never rebuilt
// from scratch.
const chatStoreSchemaVersion = 1

// chatStoreMigrations maps schema version to the statements that upgrade a
// database from the previous version. Statements must be idempotent so a
// partially applied migration can be retried.
var chatStoreMigrations = map[int][]string{
	1: {
		`CREATE TABLE IF NOT EXISTS chat_events (
			id INTEGER PRIMARY KEY,
			chat_id TEXT NOT NULL,
			seq INTEGER NOT NULL,
			t INTEGER NOT NULL,
			type TEXT NOT NULL,
			turn_id TEXT NOT NULL DEFAULT '',
			message_id TEXT NOT NULL DEFAULT '',
			item_id TEXT NOT NULL DEFAULT '',
			name TEXT NOT NULL DEFAULT '',
			provider TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT '',
			is_error INTEGER NOT NULL DEFAULT 0,
			search_text TEXT NOT NULL DEFAULT '',
			payload BLOB NOT NULL,
			UNIQUE (chat_id, seq)
		)`,
		`CREATE INDEX IF NOT EXISTS chat_events_by_time
			ON chat_events (chat_id, t)`,
		`CREATE INDEX IF NOT EXISTS chat_events_by_turn
			ON chat_events (chat_id, turn_id, seq)`,
		`CREATE INDEX IF NOT EXISTS chat_events_by_type
			ON chat_events (chat_id, type, seq)`,
		`CREATE TABLE IF NOT EXISTS chat_event_state (
			chat_id TEXT PRIMARY KEY,
			last_seq INTEGER NOT NULL DEFAULT 0,
			source_bytes INTEGER NOT NULL DEFAULT 0,
			source_mtime_ns INTEGER NOT NULL DEFAULT 0,
			source_prefix_hash INTEGER NOT NULL DEFAULT 0,
			imported_at INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS chat_events_fts USING fts5(
			search_text,
			content='chat_events',
			content_rowid='id',
			tokenize='unicode61 remove_diacritics 2'
		)`,
		`CREATE TRIGGER IF NOT EXISTS chat_events_fts_insert AFTER INSERT ON chat_events BEGIN
			INSERT INTO chat_events_fts(rowid, search_text) VALUES (new.id, new.search_text);
		END`,
		`CREATE TRIGGER IF NOT EXISTS chat_events_fts_delete AFTER DELETE ON chat_events BEGIN
			INSERT INTO chat_events_fts(chat_events_fts, rowid, search_text)
			VALUES ('delete', old.id, old.search_text);
		END`,
		`CREATE TRIGGER IF NOT EXISTS chat_events_fts_update AFTER UPDATE ON chat_events BEGIN
			INSERT INTO chat_events_fts(chat_events_fts, rowid, search_text)
			VALUES ('delete', old.id, old.search_text);
			INSERT INTO chat_events_fts(rowid, search_text) VALUES (new.id, new.search_text);
		END`,
	},
}

func (db *chatStoreDB) initializeSchema() error {
	var version int
	if err := db.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version > chatStoreSchemaVersion {
		return fmt.Errorf(
			"chat store schema version %d is newer than this build supports (%d)",
			version, chatStoreSchemaVersion,
		)
	}
	if version == chatStoreSchemaVersion {
		return nil
	}

	tx, err := db.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for next := version + 1; next <= chatStoreSchemaVersion; next++ {
		statements, ok := chatStoreMigrations[next]
		if !ok {
			return fmt.Errorf("chat store schema migration %d is missing", next)
		}
		for _, statement := range statements {
			if _, err := tx.Exec(statement); err != nil {
				return fmt.Errorf("chat store schema v%d: %w", next, err)
			}
		}
		if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", next)); err != nil {
			return err
		}
	}
	return tx.Commit()
}
