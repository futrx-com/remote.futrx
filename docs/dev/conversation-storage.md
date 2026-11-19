# Conversation storage and migration

Remote's authoritative conversation database is
`DATA_DIR/conversations.sqlite`, on the host. One schema holds all projects;
project/chat/branch identifiers and compound indexes scope reads. Harnesses
continue using their own native session files in persistent provider homes.
The database is not a replacement for native memory, configuration, workspace
files, or the harness's compaction state.

## Ownership and schema

`internal/stores/sqlitechat` implements the chat repository and run persistence
ports. `internal/harness` owns common runtime contracts;
`internal/integration/harnesses/<provider>` owns native protocol translation.
The prompt service coordinates history delivery, without putting SQL inside
harness adapters.

| Tables | Purpose |
| --- | --- |
| `chats`, `branches` | Metadata, active branch, shared parent history and cutoffs |
| `events`, `turns` | Ordered authoritative events and prompt-turn boundaries |
| `harness_sessions` | Native session/provider/account identity and delivered-history boundary |
| `runs`, `handoffs` | Execution attempts, composed input, delivery state and outcomes |
| `content_blobs`, `transcript_content_refs` | Large content and references used by transcript views |
| `transcript_items`, `transcript_search` | Derived paginated transcript and full-text search |
| `transcript_text_runs`, `transcript_text_search`, `transcript_text_pending` | Bounded assembled-text search across streaming fragments |
| `conversation_migration`, `conversation_imports` | One-time cutover state and resumable import checkpoints |

Runtime event writes and transcript projection changes share a transaction.
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

## Existing native sessions

The migration keeps the native session of the chat's last selected provider
and marks its bookmark at the imported history tail as `legacy_assumed`.
This is an explicitly accepted baseline, not evidence that this harness saw
all earlier mixed-provider messages. Historical gaps are not reconstructed.
Other old native references remain in preserved metadata but do not establish
synchronized sessions. Their next selection follows the fresh-session handoff
path. No native harness directories are edited or deleted by migration.

New handoffs are composed as historical JSON context plus the current prompt,
not inserted as native historical turns. Delivery evidence is provider-specific:
Codex acknowledges `turn/start`; Claude exposes an assistant UUID/session;
Kimi requires assistant output and its session resume hint; Antigravity exposes
a completed `user_input` step and native conversation identity. These signals
are not a universal guarantee of native filesystem fsync. Failed, interrupted
or uncertain runs retire the mapping and start fresh on the next attempt.
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
`conversations-sqlite-v1` without touching storage. Deploy scripts use these
commands to check rollback compatibility. Before cutover, legacy sources are
still authoritative. After cutover, a JSONL-only binary cannot read new
messages, so automatic rollback to it is blocked. A failed health check then
leaves the service stopped; recover with a compatible SQLite release. An
unreadable or uncertain storage status also blocks unsafe rollback.

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
# On QA immediately after upgrade, before live chat mutations:
python3 infra/qa/verify-conversation-migration.py --data-dir /opt/remote.futrx/data
```

Migration fixtures cover interruption/retry, source mutation, corrupt tails,
unknown payload preservation, missing sequences/files, ID mismatches,
validation failures, native-session baselines, post-migration deletion and
repeated upgrades. QA should additionally exercise the real updater against
an existing installation and confirm the deployed SHA and HTTP health.
