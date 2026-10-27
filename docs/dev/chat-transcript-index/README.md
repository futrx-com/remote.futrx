# Durable chat transcript index developer guide

This guide explains how Remote serves bounded transcript pages from canonical
chat logs without rescanning an entire conversation on every request. The
SQLite database is a disposable, durable index over JSONL byte ranges; it is
not a second copy of the transcript.

## Mental model

| Concern | Value | Meaning |
| --- | --- | --- |
| Startup warm breadth | 10 chats | The most recently active chats are synchronized in full by one background worker |
| Initial transcript page | 10 turns | The browser requests 10 complete turns when a chat opens |
| Older transcript page | 20 turns | Each **Load older** request asks for 20 complete turns |

The first number counts **chats**. The other two count **turns**. Warming one
chat indexes its complete currently observed `events.jsonl`, not only the
turns that the browser initially displays.

`DATA_DIR/transcript-index.sqlite` persists validation state (including file
metadata), event byte offsets, and transcript turn ranges. Event payloads
remain only in each chat's `events.jsonl`, which is always authoritative. The
backend has no application-owned cache of transcript payload bytes. The
operating system and SQLite may cache file pages opportunistically, but that is
not the durable contract.

## Documents

| Document | Explains |
| --- | --- |
| [Ownership and startup warming](01-ownership-and-startup.md) | Layer boundaries, the command-owned goroutine, and when a dedicated service would be justified |
| [Reads, writes, and recovery](02-read-write-and-recovery.md) | Fast and lazy chat opens, bounded JSONL reads, incremental append sync, validation, and fallback |

## Expected opening behavior

Reopening a chat whose durable index still matches its JSONL does not rebuild
the index. Remote validates the stored state, looks up the requested turn
ranges, and reads only those bounded ranges from `events.jsonl`. This is the
steady-state fast path, but it is not a promise that payload bytes are already
resident in memory.

A chat outside the startup set is indexed lazily on its first indexed read.
The same is true if the worker has not reached a selected chat yet. That first
request can therefore pay the one-time synchronization cost; later unchanged
opens reuse the persisted index.

## Main source locations

- [`backend/cmd/remote/chat_index_warmup.go`](../../../backend/cmd/remote/chat_index_warmup.go)
- [`backend/internal/stores/stores.go`](../../../backend/internal/stores/stores.go)
- [`backend/internal/stores/filechat/`](../../../backend/internal/stores/filechat/)
- [`backend/internal/service/chat/transcript.go`](../../../backend/internal/service/chat/transcript.go)
- [`backend/internal/transport/http/handlers/chat_handler.go`](../../../backend/internal/transport/http/handlers/chat_handler.go)
- [`frontend/src/state/hooks/chat/useChat.ts`](../../../frontend/src/state/hooks/chat/useChat.ts)
- [`frontend/src/config/api.ts`](../../../frontend/src/config/api.ts)
