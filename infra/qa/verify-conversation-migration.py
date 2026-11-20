#!/usr/bin/env python3
"""Read-only QA validation. Output counts and hashes, never chat contents."""
import argparse
import hashlib
import itertools
import json
from pathlib import Path
import sqlite3
import stat
import struct


def require(condition, message):
    if not condition:
        raise RuntimeError(message)


def framed_hash(digest, seq, raw):
    digest.update(struct.pack(">QQ", seq, len(raw)))
    digest.update(raw)


def source_records(path):
    previous = 0
    if not path.exists():
        return
    with path.open("rb") as source:
        while True:
            line = source.readline(16 * 1024 * 1024 + 1)
            if not line:
                break
            require(len(line) <= 16 * 1024 * 1024, "oversized legacy record")
            raw = line.strip()
            if not raw:
                continue
            event = json.loads(raw)
            seq = event.get("seq") or previous + 1
            require(isinstance(seq, int) and seq > previous, "invalid legacy event order")
            previous = seq
            yield seq, raw


def fingerprint(meta, path):
    digest = hashlib.sha256(meta)
    if not path.exists():
        digest.update(b"\x00missing-events")
    else:
        info = path.stat()
        # Go os.FileMode keeps Unix permissions but uses its own special-bit
        # positions. The source importer records that representation.
        mode = stat.S_IMODE(info.st_mode) & 0o777
        for unix_bit, go_bit in ((stat.S_ISUID, 1 << 23), (stat.S_ISGID, 1 << 22), (stat.S_ISVTX, 1 << 20)):
            if info.st_mode & unix_bit:
                mode |= go_bit
        seconds, nanos = divmod(info.st_ctime_ns, 1_000_000_000)
        digest.update(f"\x00{info.st_size}:{info.st_mtime_ns}:{mode}:{info.st_dev}:{info.st_ino}:{seconds}:{nanos}".encode())
    return digest.hexdigest()


def validate(root):
    database = root / "conversations.sqlite"
    require(database.is_file(), "conversation database is missing")
    connection = sqlite3.connect(database.resolve().as_uri() + "?mode=ro", uri=True)
    connection.execute("PRAGMA query_only=ON")
    connection.execute("BEGIN")
    try:
        require(connection.execute("PRAGMA integrity_check").fetchall() == [("ok",)], "SQLite integrity failed")
        require(not connection.execute("PRAGMA foreign_key_check").fetchall(), "SQLite foreign keys failed")
        schema_version = connection.execute("PRAGMA user_version").fetchone()[0]
        require(schema_version in (1, 2), "unsupported conversation schema")
        status, manifest = connection.execute("SELECT status,source_manifest FROM conversation_migration WHERE id=1").fetchone()
        require(status == "complete", "migration is not complete")
        source_manifest = hashlib.sha256()
        source_total = hashlib.sha256()
        stored_total = hashlib.sha256()
        chats = events = baselines = 0
        rows = connection.execute("SELECT chat_id,branch_id,fingerprint,event_count,last_seq,status FROM conversation_imports ORDER BY chat_id").fetchall()
        source_ids = sorted(path.name for path in (root / "chats").iterdir()) if (root / "chats").exists() else []
        require(source_ids == [row[0] for row in rows], "source and imported chat sets differ")
        for chat_id, branch, recorded_fingerprint, count, last_seq, import_status in rows:
            directory = root / "chats" / chat_id
            raw_meta = (directory / "meta.json").read_bytes()
            meta = json.loads(raw_meta)
            require(meta.get("id") == chat_id, "source metadata identity mismatch")
            current_fingerprint = fingerprint(raw_meta, directory / "events.jsonl")
            require(current_fingerprint == recorded_fingerprint, "legacy backup changed since migration")
            source_manifest.update(f"{chat_id}:{current_fingerprint}\n".encode())
            require(import_status == "complete", "unfinished chat import")
            stored_meta = connection.execute("SELECT meta_json FROM chats WHERE id=?", (chat_id,)).fetchone()
            require(stored_meta is not None, "imported chat is missing")
            if isinstance(stored_meta[0], str):
                stored_meta = (stored_meta[0].encode(),)
            require(stored_meta[0] == raw_meta, "metadata changed since import; rerun verification before live mutations")
            stored = connection.execute("SELECT seq,raw_json FROM events WHERE branch_id=? AND raw_json IS NOT NULL ORDER BY seq", (branch,))
            local_count = local_last = 0
            for source_record, stored_record in itertools.zip_longest(source_records(directory / "events.jsonl"), stored):
                require(source_record is not None and stored_record is not None, "source/import event counts differ")
                seq, raw = stored_record
                if isinstance(raw, str):
                    raw = raw.encode()
                require(source_record == (seq, raw), "source/import event payload or sequence differs")
                framed_hash(source_total, *source_record)
                framed_hash(stored_total, seq, raw)
                local_count += 1
                local_last = seq
            require(local_count == count and local_last == last_seq, "import checkpoint count/boundary differs")
            provider = (meta.get("provider") or "codex").strip().lower()
            sessions = meta.get("sessions") or {}
            native = (sessions.get(provider) if provider in sessions else meta.get(provider + "SessionId")) or ""
            native = native.strip()
            if schema_version == 1:
                baseline = connection.execute("SELECT provider,account_id,native_session_id,history_through_seq,state FROM harness_sessions WHERE branch_id=?", (branch,)).fetchall()
            else:
                active, raw_sessions = connection.execute("SELECT active_branch_id,sessions_json FROM chats WHERE id=?", (chat_id,)).fetchone()
                require(active == branch, "active branch changed since import")
                baseline = []
                for key, session in json.loads(raw_sessions).items():
                    stored_provider, account = json.loads(key)
                    require(not session.get("runId"), "native session has active input delivery")
                    baseline.append((stored_provider, account, session.get("nativeSessionId", ""), session["historyThroughSeq"], session["state"]))
            if native:
                require(baseline == [(provider, meta.get("accountId", ""), native, last_seq, "legacy_assumed")], "selected native session baseline differs")
                baselines += 1
            else:
                require(not baseline, "unexpected native baseline for sessionless chat")
            chats += 1
            events += local_count
        require(source_manifest.hexdigest() == manifest or (not rows and manifest == "fresh"), "source manifest mismatch")
        for suffix in ("", "-wal", "-shm"):
            require(not (root / ("transcript-index.sqlite" + suffix)).exists(), "obsolete transcript index remains")
        return {"migration": status, "schema_version": schema_version, "chats": chats, "events": events, "legacy_baselines": baselines,
                "source_sha256": source_total.hexdigest(), "imported_sha256": stored_total.hexdigest(),
                "legacy_backups_unchanged": True, "integrity": "ok", "foreign_keys": "ok"}
    finally:
        connection.close()


# These original columns must remain byte-for-byte unchanged during v1 -> v2.
# New scalar indexes and the derived search cache are intentionally excluded.
CANONICAL_QUERIES = {
    "chats": "SELECT id,project_id,active_branch_id,title,last_message_at,meta_json FROM chats ORDER BY id",
    "branches": "SELECT id,chat_id,parent_branch_id,fork_seq,last_seq,last_turn_ordinal FROM branches ORDER BY id",
    "events": "SELECT branch_id,seq,t,event_type,turn_id,payload_json,raw_json FROM events ORDER BY branch_id,seq",
    "conversation_migration": "SELECT id,status,source_manifest,started_at,completed_at FROM conversation_migration ORDER BY id",
    "conversation_imports": "SELECT chat_id,branch_id,fingerprint,byte_offset,event_count,last_seq,hash_state,status FROM conversation_imports ORDER BY chat_id",
}


def hash_value(digest, value):
    if value is None:
        encoded = b"n"
    elif isinstance(value, bytes):
        encoded = b"b" + value
    elif isinstance(value, str):
        encoded = b"s" + value.encode()
    elif isinstance(value, int):
        encoded = b"i" + struct.pack(">q", value)
    else:
        raise RuntimeError("unexpected canonical field type")
    digest.update(struct.pack(">Q", len(encoded)))
    digest.update(encoded)


def open_readonly(path):
    require(path.is_file(), "verification database is missing")
    connection = sqlite3.connect(path.resolve().as_uri() + "?mode=ro", uri=True)
    connection.execute("PRAGMA query_only=ON")
    connection.execute("BEGIN")
    require(connection.execute("PRAGMA integrity_check").fetchall() == [("ok",)], "SQLite integrity failed")
    require(not connection.execute("PRAGMA foreign_key_check").fetchall(), "SQLite foreign keys failed")
    return connection


def validate_schema_upgrade(root, backup):
    require(backup.resolve() != (root / "conversations.sqlite").resolve(), "backup must be a different database")
    source = open_readonly(backup)
    target = None
    try:
        target = open_readonly(root / "conversations.sqlite")
        require(source.execute("PRAGMA user_version").fetchone()[0] == 1, "schema backup is not v1")
        require(target.execute("PRAGMA user_version").fetchone()[0] == 2, "current database is not v2")
        digest = hashlib.sha256()
        counts = {}
        for table, query in CANONICAL_QUERIES.items():
            digest.update(table.encode())
            count = 0
            for original, converted in itertools.zip_longest(source.execute(query), target.execute(query)):
                require(original is not None and converted is not None, f"{table} count changed during upgrade")
                require(original == converted, f"{table} canonical data changed; verify before live mutations")
                for value in original:
                    hash_value(digest, value)
                count += 1
            counts[table] = count

        expected = {row[0]: {} for row in source.execute("SELECT id FROM chats")}
        for chat_id, provider, account, native, through, state, open_run in source.execute("""
            SELECT c.id,s.provider,s.account_id,s.native_session_id,s.history_through_seq,s.state,
             EXISTS(SELECT 1 FROM runs r WHERE r.harness_session_id=s.id AND r.status IN ('pending','accepted'))
            FROM chats c JOIN harness_sessions s ON s.branch_id=c.active_branch_id
            WHERE s.state<>'retired'
            ORDER BY c.id,CASE WHEN s.state IN ('ready','legacy_assumed','pending') THEN 1 ELSE 0 END,s.updated_at,s.id
        """):
            if open_run or state not in ("ready", "legacy_assumed") or not native:
                state = "uncertain"
            session = {"historyThroughSeq": through, "state": state}
            if native:
                session["nativeSessionId"] = native
            expected[chat_id][(provider, account)] = session
        sessions = 0
        for chat_id, raw in target.execute("SELECT id,sessions_json FROM chats"):
            actual = {tuple(json.loads(key)): session for key, session in json.loads(raw).items()}
            require(actual == expected[chat_id], "current native session mapping changed during upgrade")
            sessions += len(actual)
        obsolete = {"turns", "harness_sessions", "runs", "handoffs", "content_blobs", "transcript_content_refs",
                    "transcript_items", "transcript_text_runs", "transcript_text_search", "transcript_text_pending"}
        tables = {row[0] for row in target.execute("SELECT name FROM sqlite_master WHERE type='table'")}
        require(not tables.intersection(obsolete), "obsolete conversation tables remain")
        require({"chats", "branches", "events", "transcript_search", "conversation_migration", "conversation_imports"} <= tables,
                "required v2 conversation tables are missing")
        return {"verification": "schema-upgrade", "from_schema": 1, "schema_version": 2,
                "counts": counts, "current_sessions": sessions, "canonical_sha256": digest.hexdigest(),
                "canonical_data_unchanged": True, "integrity": "ok", "foreign_keys": "ok"}
    finally:
        source.close()
        if target is not None:
            target.close()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--data-dir", type=Path, default=Path("/opt/remote.futrx/data"))
    parser.add_argument("--schema-backup", type=Path, help="verify v1 -> v2 against the reported pre-upgrade SQLite snapshot")
    args = parser.parse_args()
    result = validate_schema_upgrade(args.data_dir, args.schema_backup) if args.schema_backup else validate(args.data_dir)
    print(json.dumps(result, sort_keys=True))


if __name__ == "__main__":
    main()
