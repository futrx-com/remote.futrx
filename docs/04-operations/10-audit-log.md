# Audit log

Remote records who did what on the box: sign-ins, project and membership
changes, secret reads and writes, container lifecycle transitions, agent runs,
scheduled-task edits, settings changes, self-updates, and workspace file,
terminal, and IDE access. The trail is append-only JSONL on the host, readable
by administrators in **Settings → Audit log** and over an admin-only API.

The audit log is a forensic record, not a metrics system. It answers "who
changed this and when", not "how many requests per second".

## Where it lives

```
DATA_DIR/audit/
├── audit-2026-06.jsonl
├── audit-2026-07.jsonl
└── audit-2026-08.jsonl   ← current month, appended to
```

- One file per calendar month (UTC), mode `0600`, directory mode `0700`.
- Writes are a single `O_APPEND` write of one JSON object plus a newline, under
  a process-level mutex. There is exactly one backend process touching
  `DATA_DIR`, which is the assumption the whole store rests on.
- Files are never rewritten. Retention deletes whole months; nothing edits a
  line in place.

Monthly files are what make both ends cheap: retention has a unit it can
delete outright, and a query for "last week" opens one file instead of the
whole history.

## Line format

Each line is one entry. The on-disk format **is** the API format — the export
endpoint streams these bytes to the client unchanged.

```json
{
  "at": "2026-08-17T09:14:02.481Z",
  "actor": { "email": "admin@example.com", "sub": "local-admin", "isAdmin": true },
  "action": "project.secret.set",
  "target": { "type": "project", "id": "a1b2c3d4", "name": "Website" },
  "ip": "203.0.113.9",
  "userAgent": "Mozilla/5.0 …",
  "meta": { "key": "STRIPE_API_KEY" },
  "ok": true
}
```

| Field | Meaning |
| --- | --- |
| `at` | RFC3339 UTC timestamp |
| `actor` | Authenticated principal; all fields empty for server-initiated actions (scheduler, janitors, reconciliation) |
| `action` | Dot-separated action name (see below) |
| `target` | What was acted on. `name` is captured at write time so the entry stays readable after a rename or delete |
| `ip` | Left-most `X-Forwarded-For` entry, falling back to the socket address |
| `userAgent` | Truncated at 512 bytes |
| `meta` | Action-specific detail. **Never contains secret values** — a secret read records that it happened and how many keys were returned, not their contents |
| `ok` | `false` when the action failed |
| `error` | Failure message, truncated at 512 bytes; absent on success |

Failed attempts are recorded too. A rejected sign-in, a forbidden secret write,
and a container start that could not converge all leave a line with
`"ok": false`.

## Action names

Names are hierarchical so the read API can filter by prefix: `project.` selects
every project action, `project.secret.` only the secret ones.

| Prefix | Actions |
| --- | --- |
| `auth.` | `login.success`, `login.failure`, `logout`, `admin.claim` |
| `user.` | `invite`, `remove`, `role-change` |
| `project.` | `create`, `rename`, `delete` |
| `project.member.` | `add`, `remove` |
| `project.secret.` | `read`, `set`, `delete` |
| `project.container.` | `start`, `stop`, `restart`, `recycle`, `repair-network`, `limits` |
| `project.browser.` | `start`, `stop` |
| `chat.` | `create`, `delete` |
| `agent.run.` | `start`, `cancel` — `meta` carries `provider`, `chatId`, `projectId`, and the scheduled task/run ids for unattended turns |
| `schedule.` | `create`, `update`, `arm`, `delete`, `run-now` |
| `settings.` | `google-oauth.configure`, `agent.connect`, `agent.disconnect` |
| `self-update.` | `trigger` |
| `workspace.file.` | `upload`, `download`, `archive-download` |
| `workspace.` | `git.checkout`, `ide.open`, `terminal.open`, `terminal.close` |

`project.container.start` is only recorded when the container was not already
running. Every agent run calls start for convergence, so recording the no-op
would bury the real transitions.

There is no `settings.agent.disconnect` flow in the product yet; cancelling an
in-flight provider login is what currently produces that action.

## Who the actor is

The authentication middleware resolves the session once per `/api/*` and `/ws*`
request and stashes the principal, client IP, and user agent on the request
context. Services record against that context, so an action taken three layers
down still names the person who asked for it, without threading an
`*http.Request` through the service layer.

Sign-in routes are the exception: a login request has no session to resolve, so
those handlers supply the attempted identity themselves and the middleware
contributes only the IP and user agent.

Long-lived WebSockets (chat runs, container terminals) copy the caller onto a
background-rooted context, because a run outlives the request that started it.

## Reading it

### In the UI

**Settings → Audit log** (administrators only) shows time, actor, action,
target, IP, and status, with filters for actor, action prefix, and a date
range, plus a load-more button and an export link. Non-administrators see a
notice instead of the table.

### API

Both routes are admin-only and re-check the admin role — the general API gate
only proves the caller is registered.

| Method | Route | Purpose |
| --- | --- | --- |
| GET | `/api/admin/audit` | Newest-first page of entries |
| GET | `/api/admin/audit/export` | Stream the raw stored JSONL for a range |

`GET /api/admin/audit` parameters:

| Parameter | Meaning |
| --- | --- |
| `actor` | Exact actor email (case-insensitive) |
| `action` | Action **prefix** |
| `target` | Exact `target.id` |
| `from`, `to` | RFC3339 or unix milliseconds; `to` is exclusive |
| `limit` | Page size, default 50, maximum 500 |
| `cursor` | `nextCursor` from a previous page |

The response is `{"entries": [...], "nextCursor": "2026-08:214"}`. `nextCursor`
is absent once the filtered range is exhausted. A cursor is a position inside
one filtered range — changing a filter invalidates it, so start a new query
rather than reusing one.

```bash
# Every secret read in August, newest first
curl -s --cookie cookies.txt \
  'https://remote.example.com/api/admin/audit?action=project.secret.read&from=2026-08-01T00:00:00Z&limit=100'

# Everything one user did, as a downloadable archive
curl -s --cookie cookies.txt \
  'https://remote.example.com/api/admin/audit/export?from=2026-08-01T00:00:00Z' \
  | grep '"email":"someone@example.com"'
```

The export endpoint filters by time range only; narrow further with `jq` or
`grep` on the JSONL.

### On the host

The files are plain JSONL, so the usual tools work:

```bash
jq -c 'select(.action | startswith("project.secret"))' \
  /opt/remote.futrx/data/audit/audit-2026-08.jsonl
```

## Retention

`AUDIT_RETENTION_MONTHS` (default `12`) sets how many monthly files to keep,
including the current one. A janitor goroutine runs at startup and every 24
hours, deleting whole monthly files older than the window. Set it to `0` to
disable pruning and keep every file forever.

Create a systemd override rather than editing the installed unit template:

```bash
sudo systemctl edit remote.futrx
```

```ini
[Service]
Environment=AUDIT_RETENTION_MONTHS=24
```

```bash
sudo systemctl daemon-reload
sudo systemctl restart remote.futrx
```

Retention deletes files; it never truncates one. Archive with the export
endpoint (or copy the files) before shortening the window.

## Failure behavior

Auditing never breaks the action it records. `Record` has no error return: a
failed write is logged to journald as `audit: drop <action> entry: …` and
dropped. A full disk therefore costs audit coverage, not availability — watch
for that log line if the trail matters to you.

## What is not covered

- Anything happening **inside** a container after the boundary is crossed. The
  log records that a terminal was opened or an agent run started, not the
  commands typed or the files the agent touched. The chat event log is the
  record for agent activity.
- Read-only browsing: listing projects, opening a chat, or viewing the file
  tree is not recorded. Secret reads are, because they disclose credentials.
- No tamper-evidence. The files are ordinary root-owned files on a box whose
  backend already runs as root; an attacker with host root can edit them. Ship
  them off-box if you need an immutable trail.
- No alerting or retention beyond the monthly window. See
  [Known limitations](../known-limitations.md).

## Code map

- Service and context helper: [`backend/internal/service/audit/`](../../backend/internal/service/audit/)
- File store: [`backend/internal/stores/fileaudit/store.go`](../../backend/internal/stores/fileaudit/store.go)
- Admin API: [`backend/internal/transport/http/handlers/audit_handler.go`](../../backend/internal/transport/http/handlers/audit_handler.go)
- Caller propagation: [`backend/internal/transport/http/audit.go`](../../backend/internal/transport/http/audit.go), [`backend/internal/transport/http/middleware/auth.go`](../../backend/internal/transport/http/middleware/auth.go)
- UI: [`frontend/src/ui/settings/AuditLogSettings.tsx`](../../frontend/src/ui/settings/AuditLogSettings.tsx)
