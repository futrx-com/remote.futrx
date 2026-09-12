# Reads, writes, and recovery

The durable index accelerates history by mapping complete turns to event byte
ranges. It never replaces the JSONL event log and never stores event payloads.

## Open and read flow

```mermaid
sequenceDiagram
    participant Browser
    participant HTTP as Chat HTTP handler
    participant Service as chat.Service
    participant Store as filechat.Store
    participant Index as chatEventIndex
    participant SQLite
    participant JSONL as events.jsonl

    Browser->>HTTP: GET transcript, limit 10 or 20
    HTTP->>Service: TranscriptPage query
    Service->>Store: ReadTranscriptEventWindow
    Store->>Index: Synchronize and locate turns
    alt Fast path: durable state matches unchanged JSONL
        Index->>SQLite: Read state and turn ranges
    else Lazy path: index is absent, behind, or invalid
        Index->>JSONL: Scan the observed file or missing tail
        Index->>SQLite: Commit state, offsets, and turn ranges
    end
    Index->>JSONL: Read only the selected byte ranges
    JSONL-->>Store: Canonical events
    Store-->>Service: Requested turns plus one older turn
    Service-->>HTTP: Compact complete turns and set cursor
    HTTP-->>Browser: Transcript page
```

The initial browser request asks for 10 complete turns. **Load older messages**
asks for 20 complete turns per page. The storage window includes one
additional older turn so the application service can preserve cursor and
`HasMore` semantics.

For an unchanged, previously indexed chat, synchronization is a file-stat and
durable-state validation followed by range lookup and bounded JSONL reads. It
does not rebuild the derived rows on every reopen. For an unindexed chat, the
first request performs the lazy backfill before it can locate the requested
turns.

## Append and incremental synchronization

```mermaid
sequenceDiagram
    participant Producer
    participant Store as filechat.Store
    participant Index as chatEventIndex
    participant JSONL as events.jsonl
    participant SQLite

    Producer->>Store: AppendEvent
    Store->>Store: Acquire per-chat lock
    Store->>Index: Synchronize and read last sequence
    alt Index is available
        Index->>SQLite: Read validation state
        Index->>JSONL: Validate or scan any missing tail
        Index->>SQLite: Commit if state advanced
    else Index is unavailable
        Store->>JSONL: Scan canonical log for last sequence
    end
    Store->>JSONL: Append event with next sequence
    Store->>Index: Best-effort refresh after append
    Index->>JSONL: Scan only the newly appended tail
    Index->>SQLite: Commit new offsets, turns, and state
    Store-->>Producer: Persisted event
```

The JSONL append is authoritative. The derived refresh normally scans only the
new tail and commits all observed changes in one transaction. If that refresh
fails, the append remains valid; the next indexed read or append retries
synchronization from canonical storage.

## Validation and recovery

```mermaid
flowchart TD
    Request["Indexed read or append"] --> Stat["Stat events.jsonl and load index state"]
    Stat --> Found{"State exists?"}
    Found -->|No| Rebuild["Delete chat rows and scan full JSONL"]
    Found -->|Yes| Shape{"File shape still valid?"}
    Shape -->|Same size and mtime| Ready["Use committed rows"]
    Shape -->|Truncated, mtime changed at same size,<br/>or grew after incomplete tail| Rebuild
    Shape -->|Trusted local append| Extend["Scan unindexed tail"]
    Shape -->|Other growth| Prefix{"Stored prefix fingerprint matches?"}
    Prefix -->|Yes| Extend
    Prefix -->|No| Rebuild
    Rebuild --> Commit["Commit replacement rows transactionally"]
    Extend --> Commit
    Ready --> Range["Read indexed JSONL byte ranges"]
    Commit --> Range
    Range --> Valid{"Indexed read valid?"}
    Valid -->|Yes| Result["Return canonical events"]
    Valid -->|No| Discard["Discard invalid chat rows"]
    Discard --> Fallback["Scan canonical JSONL and return events"]
```

Validation uses file size, modification time, incomplete-tail state, and a
rolling prefix fingerprint before untrusted growth. Truncation, a same-size
mtime change, growth after an incomplete tail, or a mismatched prefix triggers
a transactional rebuild for that chat. Normal growth extends the existing
rows. The fast path trusts unchanged size and mtime, so an out-of-band
same-size rewrite that also preserves mtime is not detectable. An invalid
offset or range is discarded and the request falls back to a full canonical
scan.

An unknown schema version is replaced rather than migrated because all rows
are derived. If SQLite is corrupt when opened, Remote removes the index files
and recreates them. Operators may likewise delete
`DATA_DIR/transcript-index.sqlite` while Remote is stopped; it will be rebuilt
from JSONL. Losing or rebuilding the index never changes transcript content.
