package filechat

import (
	"fmt"
	"strings"
)

const (
	chatEventIndexSchemaVersion = 2

	chatEventIndexStateTable      = "chat_event_index_state"
	chatEventOffsetsTable         = "chat_event_offsets"
	chatEventOffsetsBySeqIndex    = "chat_event_offsets_by_seq"
	chatEventOffsetsByOffsetIndex = "chat_event_offsets_by_offset"
	chatTranscriptTurnsTable      = "chat_transcript_turns"
	chatTranscriptTurnsBySeqIndex = "chat_transcript_turns_by_start_seq"
	chatTranscriptItemsTable      = "chat_transcript_items"
	chatTranscriptItemsBySeqIndex = "chat_transcript_items_by_start_seq"
	chatTranscriptContentTable    = "chat_transcript_content_refs"
)

type chatEventIndexSchemaObject struct {
	kind      string
	name      string
	statement string
}

var chatEventIndexSchema = [...]chatEventIndexSchemaObject{
	{
		kind: "table",
		name: chatEventIndexStateTable,
		statement: `CREATE TABLE ` + chatEventIndexStateTable + ` (
			chat_id TEXT PRIMARY KEY,
			indexed_bytes INTEGER NOT NULL,
			event_ordinal INTEGER NOT NULL,
			last_seq INTEGER NOT NULL,
			file_mtime_ns INTEGER NOT NULL,
			prefix_hash INTEGER NOT NULL,
			tail_complete INTEGER NOT NULL
		)`,
	},
	{
		kind: "table",
		name: chatEventOffsetsTable,
		statement: `CREATE TABLE ` + chatEventOffsetsTable + ` (
			chat_id TEXT NOT NULL,
			event_ordinal INTEGER NOT NULL,
			event_seq INTEGER NOT NULL,
			byte_offset INTEGER NOT NULL,
			byte_length INTEGER NOT NULL,
			PRIMARY KEY (chat_id, event_ordinal)
		) WITHOUT ROWID`,
	},
	{
		kind: "index",
		name: chatEventOffsetsBySeqIndex,
		statement: `CREATE INDEX ` + chatEventOffsetsBySeqIndex + `
			ON ` + chatEventOffsetsTable + ` (chat_id, event_seq)`,
	},
	{
		kind: "index",
		name: chatEventOffsetsByOffsetIndex,
		statement: `CREATE INDEX ` + chatEventOffsetsByOffsetIndex + `
			ON ` + chatEventOffsetsTable + ` (chat_id, byte_offset)`,
	},
	{
		kind: "table",
		name: chatTranscriptTurnsTable,
		statement: `CREATE TABLE ` + chatTranscriptTurnsTable + ` (
			chat_id TEXT NOT NULL,
			turn_ordinal INTEGER NOT NULL,
			source_turn_id TEXT NOT NULL,
			has_user INTEGER NOT NULL,
			start_seq INTEGER NOT NULL,
			end_seq INTEGER NOT NULL,
			start_offset INTEGER NOT NULL,
			end_offset INTEGER NOT NULL,
			PRIMARY KEY (chat_id, turn_ordinal)
		) WITHOUT ROWID`,
	},
	{
		kind: "index",
		name: chatTranscriptTurnsBySeqIndex,
		statement: `CREATE INDEX ` + chatTranscriptTurnsBySeqIndex + `
			ON ` + chatTranscriptTurnsTable + ` (chat_id, start_seq)`,
	},
	{
		kind: "table",
		name: chatTranscriptItemsTable,
		statement: `CREATE TABLE ` + chatTranscriptItemsTable + ` (
			chat_id TEXT NOT NULL,
			turn_ordinal INTEGER NOT NULL,
			item_key TEXT NOT NULL,
			start_seq INTEGER NOT NULL,
			end_seq INTEGER NOT NULL,
			payload_json BLOB NOT NULL,
			payload_bytes INTEGER NOT NULL,
			PRIMARY KEY (chat_id, turn_ordinal, item_key)
		) WITHOUT ROWID`,
	},
	{
		kind: "index",
		name: chatTranscriptItemsBySeqIndex,
		statement: `CREATE INDEX ` + chatTranscriptItemsBySeqIndex + `
			ON ` + chatTranscriptItemsTable + ` (chat_id, start_seq)`,
	},
	{
		kind: "table",
		name: chatTranscriptContentTable,
		statement: `CREATE TABLE ` + chatTranscriptContentTable + ` (
			chat_id TEXT NOT NULL,
			content_id TEXT NOT NULL,
			turn_ordinal INTEGER NOT NULL,
			item_key TEXT NOT NULL,
			field_kind TEXT NOT NULL,
			field_key TEXT NOT NULL,
			source_offset INTEGER NOT NULL,
			source_length INTEGER NOT NULL,
			content_bytes INTEGER NOT NULL,
			PRIMARY KEY (chat_id, content_id)
		) WITHOUT ROWID`,
	},
}

var chatEventIndexTableDropOrder = [...]string{
	chatTranscriptContentTable,
	chatTranscriptItemsTable,
	chatEventOffsetsTable,
	chatTranscriptTurnsTable,
	chatEventIndexStateTable,
}

func (index *chatEventIndex) initializeSchema() error {
	var version int
	if err := index.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version == chatEventIndexSchemaVersion {
		current, err := index.hasCurrentSchema()
		if err != nil {
			return err
		}
		if current {
			return nil
		}
	}

	// The index is derived state. Replacing an unknown or obsolete schema is
	// safer and simpler than migrating rows that can be rebuilt from JSONL.
	tx, err := index.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, table := range chatEventIndexTableDropOrder {
		if _, err := tx.Exec("DROP TABLE IF EXISTS " + table); err != nil {
			return err
		}
	}
	for _, object := range chatEventIndexSchema {
		if _, err := tx.Exec(object.statement); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", chatEventIndexSchemaVersion)); err != nil {
		return err
	}
	return tx.Commit()
}

func (index *chatEventIndex) hasCurrentSchema() (bool, error) {
	predicates := make([]string, 0, len(chatEventIndexSchema))
	arguments := make([]any, 0, len(chatEventIndexSchema)*2)
	for _, object := range chatEventIndexSchema {
		predicates = append(predicates, "(type = ? AND name = ?)")
		arguments = append(arguments, object.kind, object.name)
	}

	var count int
	err := index.db.QueryRow(
		"SELECT COUNT(*) FROM sqlite_schema WHERE "+strings.Join(predicates, " OR "),
		arguments...,
	).Scan(&count)
	if err != nil {
		return false, err
	}
	return count == len(chatEventIndexSchema), nil
}
