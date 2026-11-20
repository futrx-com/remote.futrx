# Conversation storage and migration

Remote's authoritative conversation database is
`DATA_DIR/conversations.sqlite`, on the host. One schema holds all projects;
project/chat/branch identifiers and compound indexes scope reads. Harnesses
continue using their own native session files in persistent provider homes.
The database is not a replacement for native memory, configuration, workspace
files, or the harness's compaction state.

## Ownership and schema

`internal/stores/sqlitechat` implements the chat repository and run persistence
ports. `internal/service/harness` owns common runtime contracts;
`internal/integration/harnesses/<provider>` owns native protocol translation.
The prompt service coordinates history delivery, without putting SQL inside
harness adapters.

The normal conversation model has three tables:

| Table | Purpose |
| --- | --- |
| `chats` | Metadata, active branch, and a small `sessions_json` map keyed by harness/account for that branch |
| `branches` | Parent and retained-history cutoff, append counters, and search progress |
| `events` | Ordered authoritative event JSON, with indexed scalar turn and item identifiers |

`transcript_search` is the single derived full-text index. The two legacy import
bookkeeping tables, `conversation_migration` and `conversation_imports`, retain
cutover status and resumable checkpoints. SQLite's internal FTS tables are
implementation details of that one index.

Turns, tool-call grouping, and display items are located through indexed fields
on events. A transcript page assembles only the requested items; it does not
replay the whole conversation. Large tool inputs and outputs are read from
their original event fields on demand. There are no stored transcript snapshots,
separate content blobs, or run/handoff audit tables. Imported `raw_json` remains
an exact copy of the legacy source record alongside the normalized event; the
simplification does not remove that preservation copy.

An event append commits its original payload, grouping fields, branch counters,
and any metadata change in one transaction. Search catches up lazily from saved
events when requested, using bounded text chunks to match text split across
streaming fragments. Its per-branch checkpoint avoids reindexing unchanged
history on later searches. Search respects each ancestor's retained cutoff,
including a document that crosses that cutoff. Search processing is outside the
message-save transaction, so a search-index failure does not reject a message.
The first search after upgrading can take longer while its index is populated.
If the search table is missing at startup, only that cache is recreated and its
checkpoints reset so subsequent searches rebuild from saved events.

The connection uses WAL, `synchronous=FULL`, foreign keys, and a busy timeout.
An exclusive `.conversations.lock` lease allows one backend or offline importer
to own the store. Do not delete this lock file while a process holds it.
`PRAGMA user_version` identifies the supported schema; an unknown version or
missing authoritative table fails closed instead of recreating history.

## Upgrade migration

The production installer/updater and QA deployers run migration after stopping
the old backend, using the update's existing run freeze. The 30-second HTTP
health deadline begins only after this separate migration phase finishes.
The migration command is also available for stopped-service recovery:

```bash
sudo systemctl stop remote.futrx.service
sudo remote migrate-chats
sudo systemctl start remote.futrx.service
```

The importer streams `chats/<id>/meta.json` and `events.jsonl` into a new
database. It never promotes the old disposable `transcript-index.sqlite`.
Existing metadata, raw event JSON, unknown fields, native identifiers and
timestamps are preserved. Missing legacy sequence values receive the next
sequence number; explicit increasing sequences and gaps are retained. A valid
empty chat can have no events file. Missing history for a chat with recorded
activity, malformed JSON, duplicate/decreasing sequences and mismatched IDs
are errors, never silently skipped.

Each batch commits events, source byte offset, count, sequence boundary and
incremental checksum together. The default bounds are 128 events or 4 MiB
(one legacy event may be up to 16 MiB). A retry seeks directly to the last
committed byte offset. Metadata checksums and event file size, modification
time, inode/device and change time detect changed sources. The source must
remain frozen across retries.

Before cutover, the importer verifies ordered raw-event checksums, normalized
event payloads, counts, branch boundaries, exact metadata, SQLite integrity and
foreign keys. A durable completion record makes subsequent upgrades skip the
import. Incomplete migration prevents backend startup; there is no automatic
fallback to JSONL and no runtime dual writing.

After successful validation the old `transcript-index.sqlite`, `-wal` and
`-shm` files are removed. The legacy chat directory stays untouched as an
inactive migration backup. It is not updated with new messages. Retire it
deliberately after validation and a verified SQLite backup; do not mistake it
for current history.

## Upgrading an existing SQLite database

Schema v2 upgrades the current SQLite database; it never reimports the inactive
legacy JSONL files. This preserves messages, forks, rewinds, and metadata changes
created after the original cutover.

With the backend stopped and the exclusive store lease held, `migrate-chats`
first creates a SQLite `VACUUM INTO` snapshot named
`conversations.before-v2-<timestamp>-<random>.sqlite`. It verifies integrity and
foreign keys and syncs the backup before changing the database. The conversion
backfills the indexed event fields, moves current native-session state into
`chats.sessions_json`, removes the obsolete tables, and sets `user_version=2`.
Exact canonical event bytes, metadata, branch relationships, and import
checkpoints are hashed before and after conversion and must match. The schema
change is one transaction: interruption rolls it back, and rerunning retries
against the retained v1 database. Later upgrades skip the completed conversion.

Only the active branch's current harness/account mappings carry forward. An
unfinished or uncertain delivery remains unsafe to resume. Historical attempt
ledgers and composed prompt copies are deliberately removed; the pre-upgrade
snapshot retains them if needed for investigation.

## Existing native sessions

The migration keeps the native session of the chat's last selected provider
and marks its bookmark at the imported history tail as `legacy_assumed`.
This is an explicitly accepted baseline, not evidence that this harness saw
all earlier mixed-provider messages. Historical gaps are not reconstructed.
Other old native references remain in preserved metadata but do not establish
synchronized sessions. Their next selection follows the fresh-session handoff
path. No native harness directories are edited or deleted by migration.

Each current mapping stores its native session ID, last safe event sequence,
state, and a pending run token/delivery method while a run is active. Preparing
a run persists the pending marker before contacting the harness. Input must
have provider acceptance evidence, and the run must complete successfully,
before the safe bookmark advances. Failure, interruption, or a process restart
with pending input causes the next attempt to start a fresh native session.
Callbacks identify both the branch and pending run; a late callback cannot
advance a replacement session. Fork and rewind begin with an empty session map.
No full prompt or history transfer is duplicated into a separate audit ledger.

New handoffs are composed as historical JSON context plus the current prompt,
not inserted as native historical turns. Delivery evidence is provider-specific:
Codex acknowledges `turn/start`; Claude exposes an assistant UUID/session;
Kimi requires assistant output and its session resume hint; Antigravity exposes
a completed `user_input` step and native conversation identity. These signals
are not a universal guarantee of native filesystem fsync. Failed, interrupted
or uncertain runs leave the mapping unsafe and start fresh on the next attempt.
History is never silently truncated or summarized by Remote. Kimi's current
argument transport rejects inputs above 120 KiB before launch; other harness
or model limits can also reject large history transfers.

## Backup, recovery, and rollback

Take a supported SQLite snapshot, for example with the SQLite CLI's online
backup command (the destination directory must already exist):

```bash
sqlite3 /opt/remote.futrx/data/conversations.sqlite \
  ".backup '/srv/remote-backups/conversations.sqlite'"
sqlite3 /srv/remote-backups/conversations.sqlite 'PRAGMA integrity_check;'
```

Do not copy only the live `.sqlite` file while WAL writes are active. Keep
backups of the other Remote state, provider homes and workspaces as required;
the conversation database alone cannot restore those resources. Restore a
snapshot only with the backend stopped, and preserve the displaced database
and its sidecars until the restored instance is verified.

`remote storage-status` reports `legacy`, `importing`, or `complete` without
starting the server. `remote storage-version` reports
`conversations-sqlite-v2` without touching storage. `remote storage-schema`
reads the actual database schema version (zero when no database exists). Deploy
scripts compare the binary capability with that version before rollback. Before cutover, legacy sources are
still authoritative. After cutover, a JSONL-only binary cannot read new
messages, so automatic rollback to it is blocked. A failed health check then
leaves the service stopped; recover with a compatible SQLite release. After the
v2 conversion, automatic rollback to a v1 binary is blocked too. Restoring the
pre-v2 snapshot is only appropriate before accepting new v2 messages; afterward
use a v2-compatible binary and current data. An unreadable or uncertain storage
status also blocks unsafe rollback.

If migration stops, correct the reported condition and rerun it with the
backend stopped. Checkpoint resume requires the original source files to
remain unchanged, including their filesystem identity and timestamps.
Replacing files with copies restored from backup changes that identity even
when their contents match. If legacy sources were changed or replaced after
an aborted import, the source check will reject resume: archive the unfinished
database and sidecars before starting a new import, only after verifying that
cutover never completed. Never discard a completed database to retry a migration.

## Verification

```bash
cd backend
go test ./internal/stores/sqlitechat ./internal/stores/chatmigration
go test -race ./internal/stores/chatmigration
cd ..
bash infra/tests/chat-migration-test.sh
bash infra/tests/deploy-app-script-test.sh
bash infra/tests/qa-scripts-test.sh
# Immediately after an initial JSONL import, before live chat mutations:
python3 infra/qa/verify-conversation-migration.py --data-dir /opt/remote.futrx/data
# Immediately after v1 -> v2, compare against the reported SQLite snapshot:
python3 infra/qa/verify-conversation-migration.py --data-dir /opt/remote.futrx/data \
  --schema-backup /opt/remote.futrx/data/conversations.before-v2-<timestamp>-<random>.sqlite
```

Migration fixtures cover interruption/retry, source mutation, corrupt tails,
unknown payload preservation, missing sequences/files, ID mismatches,
validation failures, native-session baselines, post-migration deletion and
repeated upgrades. Schema-upgrade checks also cover canonical byte preservation,
active session maps, failed conversion rollback, and v1/v2 rollback rejection.
QA should additionally exercise the real updater against
an existing installation and confirm the deployed SHA and HTTP health.
