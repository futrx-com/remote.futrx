# API and realtime transport

The Go backend serves the embedded SPA, JSON HTTP endpoints, tus uploads, and
the application's WebSocket routes from one server.

## Request path

```mermaid
flowchart LR
    Client["Browser client"] --> Caddy["Caddy HTTPS"]
    Caddy --> Middleware["Auth and onboarding middleware"]
    Middleware --> Handler["HTTP or WebSocket handler"]
    Handler --> Access["Role and project access checks"]
    Access --> Service["Application service"]
    Service --> Store["File store"]
    Service --> Integration["LXD, Git, tmux, host filesystem"]
```

All `/api/*` and `/ws*` requests require a signed session for a registered user.
They are also blocked until local-admin setup and at least one module declared
as an access-gate provider is ready, except the module-driven agent-auth
catalog, streams, and compatibility auth routes needed to finish onboarding.

## Authentication routes

| Method | Route | Purpose |
| --- | --- | --- |
| GET | `/auth/me` | Current auth and onboarding status |
| POST | `/auth/local/claim` | Create the local administrator or complete legacy setup |
| POST | `/auth/local/login` | Local administrator password login |
| GET | `/auth/google/login` | Start Google OAuth, optionally preserving a safe return URL |
| GET | `/auth/google/callback` | Validate OAuth state, authorize invited email, and issue session |
| GET | `/auth/logout` | Clear platform cookies and return to the app |
| GET | `/auth/verify` | Caddy forward-auth check; preview hosts also check project membership |
| GET, PUT | `/api/admin/auth/google` | Read or replace Google OAuth configuration; admin only |
| GET, PUT, DELETE | `/api/admin/email` | Read, replace, or clear the SMTP sender configuration; admin only |
| POST | `/api/admin/email/test` | Send a test email from the configured sender; admin only |

## Users and settings

| Method | Route | Purpose |
| --- | --- | --- |
| GET, POST | `/api/admin/users` | List or add registered users; admin only |
| DELETE | `/api/admin/users/{email}` | Remove a user; admin only |
| PUT | `/api/admin/users/{email}/role` | Promote or demote a user; admin only |
| GET, PATCH | `/api/me/settings` | Read or update current user's appearance and chat defaults |
| GET | `/api/server/info` | Host, CPU, memory, storage, network, and process snapshot |

## Agent authentication, capabilities, and skills

| Method | Route | Purpose |
| --- | --- | --- |
| GET | `/api/agent-auth` | Ordered module descriptors plus normalized current auth snapshots; available before the provider gate opens |
| GET | `/api/{provider}/auth-status` | Legacy provider-specific status payload for an available managed binding; external bindings return `null` |
| POST | `/api/{provider}/login/start` | Start or resume a managed authorization-code flow; currently Claude; admin only |
| POST | `/api/{provider}/login/code` | Submit a managed authorization code; currently Claude; admin only |
| POST | `/api/{provider}/login/cancel` | Cancel a managed authorization-code flow; currently Claude; admin only |
| POST | `/api/{provider}/login/device` | Start a managed device flow; currently Codex and Kimi; admin only |
| POST | `/api/{provider}/login/api-key` | Validate and save a managed provider API key; currently MiniMax; optional `{label, accountId}` creates or reconnects a named account; admin only; response never includes the key |
| DELETE | `/api/{provider}/login/api-key` | Remove a managed provider API key; currently MiniMax; admin only |
| GET | `/api/agent-capabilities[?projectId=<id>&refresh=1]` | Discover normalized provider/model controls on the host or in an accessible project; `refresh=1` bypasses the current backend cache entry |
| GET | `/api/skills?provider=...&projectId=...` | List accessible provider and project skills |

The module descriptor chooses one auth mode: managed authorization code,
managed device flow, managed API key, external, or none. Route registration follows the built
auth binding, so a provider exposes only operations valid for its declared
flow. MiniMax uses a write-only managed API-key binding whose status exposes
only configured/unconfigured. Antigravity uses an external binding with no
observable status, status stream, or managed host-login route; users
authenticate it with `agy` inside each project.

`GET /api/agent-auth` is the frontend's authoritative auth registry. Each row
contains `provider`, `label`, optional `default`, `executionScopes`, an
`authentication` object (`mode`, optional `instructions`, and
`satisfiesAccessGate`, and optional API-key creation metadata), and a normalized `status` object. Status contains
`authenticated`, an optional warning, and one login shape shared by managed
code/device flows (`active`, URL, optional code/timestamps/completion/error).
Managed API-key status contains only the configured boolean plus the same
redacted account metadata when named keys are enabled.
No-auth modules report authenticated immediately. External modules publish
their instructions but no managed status stream or mutation controls.

The auth registry is separate from model capability caching. Each
`GET /api/agent-auth` reads current binding snapshots, and managed changes
arrive through WebSockets; these values do not wait for the 24-hour/2-hour
model-catalog TTL.

The access gate uses descriptor policy rather than a fixed provider list. A
no-auth module marked `satisfiesAccessGate` opens it immediately; managed
code/device/API-key modules require an authenticated binding. External authentication
cannot satisfy the gate because Remote cannot observe it reliably. The current
built-ins mark Claude, Codex, and Kimi as gate providers; MiniMax is managed
but not a gate provider, and Antigravity is external.

The capability response has a `providers` array in registry order. Each
provider includes its `source` (`live` or `fallback`), optional warning and
structured unavailability reason, models, per-model reasoning efforts and
service tiers, modes, model/control defaults, the module default flag,
execution scopes, authentication metadata, and
declared session/skill/browser/scheduling/execution-policy features.
Omitting `projectId` selects the host/loose-chat scope. Supplying it requires
admin status or project membership and selects that project's current
container. Only the literal `refresh=1` forces discovery; other values use the
normal cache path. Refresh bypasses a completed entry but joins a same-scope
discovery already in flight. The route does not start a stopped project.

The cache is backend-process memory, shared across web clients by execution
scope. Fully live, warning-free results use a 24-hour TTL; any fallback or
warning shortens the complete scope to 2 hours. A backend restart clears it.
See [Capability discovery](../02-workspaces/04-chat-and-agents.md#capability-discovery)
for provider probes and all refresh triggers.

Each provider probe receives the same `AGENT_CAPABILITY_TIMEOUT` deadline. The
default is 30 seconds for the provider's complete discovery operation; setting
the Go-duration environment value to `0` disables the deadline. Providers are
still probed concurrently, so the setting is not multiplied into a sequential
whole-catalog timeout.

## Project routes

| Method | Route | Purpose |
| --- | --- | --- |
| GET, POST | `/api/projects` | List visible projects or create a project |
| POST | `/api/projects/reorder` | Update project ordering |
| GET, PATCH, DELETE | `/api/projects/{id}` | Read, rename, or admin-delete a project |
| POST | `/api/projects/{id}/start` | Start or relaunch a project |
| POST | `/api/projects/{id}/stop` | Stop a project |
| POST | `/api/projects/{id}/restart` | Force restart or relaunch a project |
| GET | `/api/projects/{id}/container` | Detailed container inspection |
| PUT | `/api/projects/{id}/limits` | Set CPU, memory, and disk overrides; admin only |
| POST | `/api/projects/{id}/repair-network` | Reconfigure container networking and reinspect |
| GET | `/api/projects/{id}/apps` | List externally reachable container listeners |
| GET | `/api/projects/{id}/agent-browser` | Get Agent Browser core/view status and record activity |
| POST | `/api/projects/{id}/agent-browser/start` | Ensure Agent Browser is starting or ready |
| DELETE | `/api/projects/{id}/agent-browser` | Stop the complete Agent Browser |
| DELETE | `/api/projects/{id}/agent-browser?scope=view` | Stop only the noVNC view |
| GET | `/api/projects/{id}/secrets` | List project secrets |
| PUT, DELETE | `/api/projects/{id}/secrets/{key}` | Set or delete one secret |
| GET, POST | `/api/projects/{id}/access` | List members or add a registered email |
| DELETE | `/api/projects/{id}/access/{email}` | Remove a member |
| GET | `/internal/tls-ask?domain=...` | Caddy allow-check for on-demand project certificates |

Every `{id}` project route first requires admin status or project membership. Resource-limit changes and project deletion add an admin-only check.

## Chat routes

| Method | Route | Purpose |
| --- | --- | --- |
| GET, POST | `/api/chats` | List visible chats or create a chat |
| GET, PATCH, DELETE | `/api/chats/{id}` | Read, update, or delete chat metadata/history |
| GET | `/api/chats/{id}/events?limit=&before=` | Page persisted events backward by sequence |
| GET | `/api/chats/{id}/transcript?limit=&before=` | Page complete transcript turns backward; adjacent text deltas are compacted |
| POST | `/api/chats/{id}/rewind` | Remove a selected prompt and later events |
| POST | `/api/chats/{id}/fork` | Copy metadata/history and defer provider-session fork |
| POST | `/api/chats/{id}/read` | Mark current history read |
| POST | `/api/chats/{id}/unread` | Force unread state |
| GET | `/api/chats/{id}/ide-open?path=...` | Validate path and redirect to the correct IDE URL |
| GET | `/api/chats/{id}/media-open?path=...` | Serve supported workspace media inline |
| GET | `/api/chats/{id}/files?path=...` | List a workspace directory |
| GET | `/api/chats/{id}/files/search?q=...` | Search workspace filenames |
| GET | `/api/chats/{id}/files/download?path=...` | Download one file |
| GET | `/api/chats/{id}/files/download-folder?path=...` | Stream a folder ZIP |
| GET | `/api/chats/{id}/history/repos` | Discover workspace Git repositories |
| GET | `/api/chats/{id}/history/commits?repo=&limit=` | List commits |
| GET | `/api/chats/{id}/history/diff?repo=&sha=` | Read one commit patch |
| POST | `/api/chats/{id}/history/checkout` | Optional checkpoint and detached checkout |

All chat routes resolve the caller and enforce the chat's project membership. Loose chats have no project membership check.

## Application and agent routes

Browser extensions call their installed backend through
`/api/applications/{instance}/backend/{path}` or the project-scoped equivalent.
Remote stamps `Request.Caller` from the session and clears `Request.Agent`.

Agents call `/agent-api/applications/<application-id>/<path>` with a temporary
bearer grant supplied through `REMOTE_APPLICATION_API` and
`REMOTE_APPLICATION_GRANT`. Eligible applications opt into `backend.agentTools`.
Remote resolves the installation from the grant and stamps current caller and
turn context; the application owns route permissions and interprets its own
context. Interactive turns can use eligible global/same-project tools;
application-started turns can use only their own installation's tools. Bodies
cap at 64 KiB, grants expire after four hours, and the provider run revokes them
when it ends.

The other direction uses the application backend SDK, with
`backend.agentTurns` and `rpc.ServeWithRuntime`:

| SDK control | Purpose |
| --- | --- |
| `Runtime.AgentTurns.Start` | Submit a prompt to an existing chat under a captured owner's current authority |
| `Runtime.AgentTurns.Read` | Read that installation's accepted request status, result, and paginated transcript |
| `Runtime.AgentTurns.Forget` | Remove an inactive receipt after durable application acknowledgment, retaining chat history |

These controls use the chat's existing provider/model/settings and share normal
prompt execution. Request IDs are installation-scoped, prompts cap at 32 KiB,
and optional opaque JSON context caps at 64 KiB. Core rechecks current
registration and project/chat access. Workflow state stays in the application;
core persists only generic execution receipts. `backend.background`
independently opts running backend processes into restart/crash recovery.

Scheduled Tasks is one consumer. It owns its `tasks` resource, clock, claims,
retries, and completion rules. Agent tools call
`/agent-api/applications/scheduled-tasks/tasks` and application-defined subpaths;
browser clients use the normal backend transport. Core does not interpret
task/run IDs or completion commands. See the
[application API reference](../dev/installable-applications/12-http-api.md)
and [agent runtime guide](../dev/installable-applications/25-application-agent-runtime.md).

## Upload and auxiliary routes

| Method | Route | Purpose |
| --- | --- | --- |
| POST, HEAD, PATCH, GET, DELETE | `/api/uploads[/<upload-id>]` | tus resumable upload lifecycle |
| GET | `/__remote_inspector` | Same-origin preview inspection wrapper |
| GET, POST | `/api/sessions` | List or create host tmux sessions |
| DELETE | `/api/sessions/{name}` | Delete tmux session |
| POST | `/api/sessions/{name}/send` | Send text into tmux session |
| POST | `/api/sessions/{name}/upload` | Multipart upload into tmux working directory |

The upload access check happens when the random upload URL is created. Later chunk requests rely on possession of that URL.

## WebSocket routes

| Route | Direction | Messages |
| --- | --- | --- |
| `/ws/workspace` | Server to client | Snapshot, chat upsert/delete, project upsert/delete |
| `/ws/chat/{id}?since=<seq>` | Both | Client `prompt` or `cancel`; server chat events and `sync` |
| `/ws/agent-auth/{provider}` | Server to client | Normalized auth snapshots for an available managed auth binding |
| `/ws/{provider}/auth-status` | Server to client | Legacy provider-specific auth status payloads |
| `/ws?session={name}` | Both | Auxiliary tmux PTY binary data and control messages |

Chat membership is checked before the WebSocket upgrade. The Terminal application's socket is not a core route: it is served on the project's application host, where the [web gateway](../dev/installable-applications/19-project-application-web-routes.md) checks project membership before the upgrade. Removing a member prevents future checked connections, but the backend does not currently close or reauthorize that member's already-open sockets.

## Realtime channels

```mermaid
flowchart TD
    Browser["Browser"] --> WorkspaceWS["Workspace WebSocket"]
    Browser --> ChatWS["Active chat WebSocket"]
    Browser --> TerminalWS["Terminal application WebSocket on its application host"]
    Browser --> AuthWS["/ws/agent-auth/provider while onboarding or in Settings"]

    WorkspaceWS --> WorkspaceHub["Workspace hub"]
    ChatWS --> RunHub["Per-chat run hub"]
    TerminalWS --> PTY["Application web gateway to the in-container terminal service"]
    AuthWS --> AuthService["Provider auth subscription"]

    WorkspaceHub --> Repositories["Repository notifications"]
    RunHub --> EventStore["Persisted JSONL events"]
    RunHub --> Prompt["Prompt start and cancel"]
```

## Chat reconnect and replay

```mermaid
sequenceDiagram
    participant UI
    participant HTTP as Events API
    participant WS as Chat WebSocket
    participant Store as Event store

    UI->>HTTP: Load latest event page
    HTTP->>Store: Read bounded page
    Store-->>UI: events, lastSeq, nextBefore, hasMore
    UI->>WS: Connect with since=lastSeq
    WS->>Store: Read events after sequence
    Store-->>WS: Missed events
    WS-->>UI: Replay, then sync state, then live events
    WS--xUI: Connection drops
    UI->>WS: Exponential reconnect with latest applied sequence
```

The workspace and chat streams send ping frames every 25 seconds. The frontend chat socket reconnects from 400 ms up to 5 seconds and requests only unseen sequences.

## Chat event shapes

| Event | Main fields | Persisted |
| --- | --- | ---: |
| `user` | `text` | Yes |
| `assistant_text` | `text`, optional `messageId` | Yes |
| `thinking` | `text` | Yes |
| `tool_use_start` | `id`, `name`, `input` | Yes |
| `tool_use_end` | `id`, `output`, `isError` | Yes |
| `system` | `subtype`, `data` | Yes |
| `session` | provider and provider session ID | Yes |
| `complete` | usage payload | Yes |
| `error` | message | Usually yes; lock/contention errors may be transient |
| `sync` | `running` | No |

## Common status behavior

- `400`: invalid IDs, paths, values, or JSON.
- `401`: missing or invalid session.
- `403`: valid user without role or project access.
- `404`: missing chat, project, user, file, or repository target.
- `409`: running-chat conflict, dirty Git state, protected last-admin/member
  guardrail, duplicate project display name, or duplicate user.
- `412`: onboarding gate is incomplete.
- `413`: upload or request body is too large.

## Code map

- Route composition: [`backend/internal/transport/transport.go`](../../backend/internal/transport/transport.go)
- HTTP server: [`backend/internal/transport/http/server.go`](../../backend/internal/transport/http/server.go)
- Frontend route constants: [`frontend/src/config/routes.ts`](../../frontend/src/config/routes.ts)
- Chat socket: [`backend/internal/transport/ws/chat_socket.go`](../../backend/internal/transport/ws/chat_socket.go)
- Workspace socket: [`backend/internal/transport/ws/workspace_socket.go`](../../backend/internal/transport/ws/workspace_socket.go)
