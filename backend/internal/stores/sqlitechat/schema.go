package sqlitechat

// Events are the authoritative transcript. The scalar grouping fields are
// indexed pointers into that history, not a second copy of its content.
const schema = `
CREATE TABLE chats (
 id TEXT PRIMARY KEY, project_id TEXT NOT NULL, active_branch_id TEXT NOT NULL,
 title TEXT NOT NULL, last_message_at INTEGER NOT NULL, meta_json BLOB NOT NULL,
 sessions_json BLOB NOT NULL DEFAULT '{}'
);
CREATE INDEX chats_by_project ON chats(project_id,last_message_at DESC,id);
CREATE TABLE branches (
 id TEXT PRIMARY KEY, chat_id TEXT NOT NULL,
 parent_branch_id TEXT REFERENCES branches(id), fork_seq INTEGER NOT NULL DEFAULT 0,
 last_seq INTEGER NOT NULL DEFAULT 0, last_turn_ordinal INTEGER NOT NULL DEFAULT 0,
 last_turn_id TEXT NOT NULL DEFAULT '', last_turn_start_seq INTEGER NOT NULL DEFAULT 0,
 last_turn_has_user INTEGER NOT NULL DEFAULT 0,
 search_seq INTEGER NOT NULL DEFAULT 0, search_pending_json BLOB NOT NULL DEFAULT '{}'
);
CREATE INDEX branches_by_chat ON branches(chat_id);
CREATE TABLE events (
 branch_id TEXT NOT NULL REFERENCES branches(id) ON DELETE CASCADE,
 seq INTEGER NOT NULL, t INTEGER NOT NULL, event_type TEXT NOT NULL,
 turn_id TEXT NOT NULL, payload_json BLOB NOT NULL, raw_json BLOB,
 turn_ordinal INTEGER NOT NULL, turn_start_seq INTEGER NOT NULL, turn_key TEXT NOT NULL,
 item_key TEXT NOT NULL, item_mode INTEGER NOT NULL, item_start_seq INTEGER NOT NULL,
 visible INTEGER NOT NULL,
 PRIMARY KEY(branch_id,seq)
) WITHOUT ROWID;
` + eventIndexSchema

const eventIndexSchema = `
CREATE INDEX events_by_time ON events(branch_id,t,seq);
CREATE INDEX events_by_turn ON events(branch_id,turn_ordinal,seq);
CREATE INDEX events_by_item ON events(branch_id,turn_ordinal,item_key,seq);
CREATE INDEX events_by_item_type ON events(branch_id,turn_ordinal,item_key,event_type,seq DESC);
CREATE INDEX events_visible_items ON events(branch_id,item_start_seq DESC,seq DESC) WHERE visible=1 AND seq=item_start_seq;
CREATE INDEX events_turn_starts ON events(branch_id,seq DESC) WHERE seq=turn_start_seq;
`
