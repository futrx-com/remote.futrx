package sqlitechat

// All data needed for normal operation lives here. Native harness storage is
// deliberately outside this schema. Derived tables are updated in the same
// transaction as their authoritative events, and can be rebuilt explicitly.
const schema = `
CREATE TABLE chats (
 id TEXT PRIMARY KEY, project_id TEXT NOT NULL, active_branch_id TEXT NOT NULL,
 title TEXT NOT NULL, last_message_at INTEGER NOT NULL, meta_json BLOB NOT NULL
);
CREATE INDEX chats_by_project ON chats(project_id,last_message_at DESC,id);
CREATE TABLE branches (
 id TEXT PRIMARY KEY, chat_id TEXT NOT NULL,
 parent_branch_id TEXT REFERENCES branches(id), fork_seq INTEGER NOT NULL DEFAULT 0,
 last_seq INTEGER NOT NULL DEFAULT 0, last_turn_ordinal INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX branches_by_chat ON branches(chat_id);
CREATE TABLE events (
 branch_id TEXT NOT NULL REFERENCES branches(id) ON DELETE CASCADE,
 seq INTEGER NOT NULL, t INTEGER NOT NULL, event_type TEXT NOT NULL,
 turn_id TEXT NOT NULL, payload_json BLOB NOT NULL, raw_json BLOB,
 PRIMARY KEY(branch_id,seq)
) WITHOUT ROWID;
CREATE INDEX events_by_time ON events(branch_id,t,seq);
CREATE TABLE turns (
 branch_id TEXT NOT NULL REFERENCES branches(id) ON DELETE CASCADE,
 ordinal INTEGER NOT NULL, source_turn_id TEXT NOT NULL, has_user INTEGER NOT NULL,
 start_seq INTEGER NOT NULL, end_seq INTEGER NOT NULL,
 PRIMARY KEY(branch_id,ordinal)
) WITHOUT ROWID;
CREATE INDEX turns_by_seq ON turns(branch_id,start_seq);
CREATE TABLE content_blobs (
 id TEXT PRIMARY KEY, media_type TEXT NOT NULL, byte_length INTEGER NOT NULL,
 content BLOB NOT NULL
);
CREATE TABLE transcript_items (
 id INTEGER PRIMARY KEY,
 branch_id TEXT NOT NULL REFERENCES branches(id) ON DELETE CASCADE,
 turn_ordinal INTEGER NOT NULL, item_key TEXT NOT NULL,
 start_seq INTEGER NOT NULL, end_seq INTEGER NOT NULL,
 payload_json BLOB NOT NULL, payload_bytes INTEGER NOT NULL,
 search_text TEXT NOT NULL,
 UNIQUE(branch_id,turn_ordinal,item_key,end_seq)
);
CREATE INDEX transcript_items_by_seq ON transcript_items(branch_id,start_seq DESC,end_seq DESC);
CREATE TABLE transcript_content_refs (
 branch_id TEXT NOT NULL REFERENCES branches(id) ON DELETE CASCADE,
 content_id TEXT NOT NULL, event_seq INTEGER NOT NULL,
 blob_id TEXT NOT NULL REFERENCES content_blobs(id),
 PRIMARY KEY(branch_id,content_id)
) WITHOUT ROWID;
CREATE VIRTUAL TABLE transcript_search USING fts5(text, tokenize='unicode61');
CREATE TABLE transcript_text_runs (
 id INTEGER PRIMARY KEY,
 branch_id TEXT NOT NULL REFERENCES branches(id) ON DELETE CASCADE,
 turn_id TEXT NOT NULL, group_key TEXT NOT NULL,
 start_seq INTEGER NOT NULL,end_seq INTEGER NOT NULL,
 parts_json BLOB NOT NULL
);
CREATE INDEX transcript_text_runs_by_seq ON transcript_text_runs(branch_id,end_seq,start_seq);
CREATE VIRTUAL TABLE transcript_text_search USING fts5(text, tokenize='unicode61');
CREATE TABLE transcript_text_pending (
 branch_id TEXT PRIMARY KEY REFERENCES branches(id) ON DELETE CASCADE,
 turn_id TEXT NOT NULL,group_key TEXT NOT NULL,parts_json BLOB NOT NULL,
 has_new INTEGER NOT NULL
);
`
