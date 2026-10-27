# 25 — Application agent runtime

Application backends can request ordinary agent turns and expose their own tools
to those turns. Scheduling, build monitoring, inbox processing, and other
workflows use the same SDK; Remote does not interpret their jobs or results.

## Opt in

The three backend flags are independent and default to `false`:

```json
{
  "backend": {
    "background": true,
    "agentTurns": true,
    "agentTools": true,
    "access": "registered"
  }
}
```

`background` restores running installations at Remote startup and checks them
again every 15 seconds, including recovery after a child crash. Stopped
installations stay stopped. Other backends keep their existing lazy recovery.
Start and stop use the normal application lifecycle; background recovery does
not change the persisted status or start a stopped project container itself.
A cold build uses the existing five-minute backend startup deadline.

`agentTurns` binds `Runtime.AgentTurns` before `Backend.Init` when served with
`rpc.ServeWithRuntime`. Retain it in the application API or workflow worker.
Launch background work after initialization and retry temporary busy errors:
installation and lifecycle changes can temporarily fence execution.

## Agent controls

[`AgentTurns`](../../../backend/pkg/applications/agent_turns.go) is a Go SDK
capability that calls Remote over the backend's RPC connection. These methods
are not browser HTTP endpoints.

| Method | What the application controls |
|---|---|
| `Start(AgentTurnRequest) (AgentTurn, error)` | Submits a prompt to an existing chat for a captured owner, with a stable request ID and optional application context; returns an execution receipt without waiting for the turn to finish |
| `Read(AgentTurnQuery) (AgentTurn, error)` | Polls that installation's accepted request for status, output, error, and paginated transcript events |
| `Forget(requestID string) error` | Removes the installation's receipt after durable application acknowledgment; active work returns `ErrAgentBusy` |

`AgentTurnRequest` fields:

| Field | Meaning |
|---|---|
| `RequestID` | Required, nonblank operation ID scoped to this installation; at most 256 bytes |
| `ChatID` | Required ID of an existing, accessible chat; at most 256 bytes |
| `OwnerEmail` | Owner captured from the Remote-stamped `Request.Caller.Email` when the application accepts work |
| `Prompt` | Required, nonblank prompt; at most 32 KiB |
| `Context` | Optional `json.RawMessage`, valid JSON of at most 64 KiB; application-owned data forwarded to the turn's own application tools |

```go
turn, err := runtime.AgentTurns.Start(applications.AgentTurnRequest{
    RequestID: "build-123-check-1",
    ChatID: savedChatID,
    OwnerEmail: savedOwnerEmail,
    Prompt: "Check build_123 and report its result.",
    Context: json.RawMessage(`{"buildId":"build_123"}`),
})
```

Capture the owner from Remote-stamped `Request.Caller` when accepting work.
Remote checks current registration, current administrator status, project
membership, the chat's project, and the running installation on each start/read.
Project installations can address only their own project's chats. Global
installations can address chats the captured owner may access, matching normal
chat visibility. Browser-supplied roles and agent context do not establish
execution authority.

The normal prompt service still owns provider selection, session handling,
container startup, transcript, usage, notifications, and one turn per chat.
Application turns share a server-wide limit of two active turns and respect
maintenance. These limits are platform execution policy.

Usage records carry `applicationId` and `applicationRequestId`, and result push
notifications use a generic application category with neutral wording. Core
does not infer scheduling from application identity. Scheduled Tasks uses this
same attribution contract; scheduling markers remain only for legacy history.

The SDK does not expose cancellation, pause/resume, chat creation, or
provider/model overrides. `Read` addresses the installation's own request ID,
not an arbitrary existing turn. `Forget` removes execution metadata without
cancelling a turn or deleting its chat transcript. Workflow actions such as
pausing a schedule or completing a job are application-defined backend routes.

## Receipts and reading turns

`RequestID` identifies one logical operation within one installation. Persist it
in application state before starting work. Identical retries return the same
running or completed receipt. Reusing it with different owner, chat, prompt, or
context returns `ErrAgentRequestChanged`. Other sentinel errors distinguish
busy execution, denied access, unavailable runtime, and missing receipts; they
remain identifiable with `errors.Is` across the process boundary.

| SDK error | Meaning and response |
|---|---|
| `ErrAgentAccess` | The installation is stopped or lacks the opt-in, or the captured owner's current registration/chat/project access no longer permits execution or reading |
| `ErrAgentBusy` | A lifecycle transition, active chat, maintenance, or the shared two-turn admission limit temporarily blocks execution; also returned when forgetting active work. Retry later with the same request ID and input |
| `ErrAgentTurnNotFound` | `Read` found no receipt for that request in this installation |
| `ErrAgentRequestChanged` | The same request ID was reused with different accepted input; retain the original input or create a new operation ID |
| `ErrAgentUnavailable` | The runtime is unbound or unavailable; check the opt-in and `rpc.ServeWithRuntime` wiring |

Input validation, storage, transport, and provider errors may also be returned;
they are not all represented by these sentinel errors.

`AgentTurns.Read(AgentTurnQuery{RequestID: id, Limit: 100})` returns status, output,
error, and a page of the existing chat transcript filtered by that turn's
`TurnID`. Events are JSON objects in chronological order. `Limit` defaults to
200 and is capped at 200. Use `NextBefore` as `BeforeSeq` while `HasMore` is true
for older events. Request IDs and chat IDs are capped at 256 bytes, prompts at
32 KiB, and valid JSON context at 64 KiB.

`AgentTurnQuery` contains `RequestID`, optional `BeforeSeq`, and `Limit`.
An omitted `BeforeSeq` reads the latest page. The returned `AgentTurn` contains
`RequestID`, `ChatID`, `TurnID`, `Status`, `Output`, `Error`, `Events`,
`NextBefore`, and `HasMore`. Save the application's request ID and poll `Read`
from its worker; a `running` result is not a completion acknowledgment.

Statuses are `running`, `succeeded`, `failed`, and `interrupted`. Core persists
accepted input and results under `DATA_DIR/application-turns/`, using hashed
instance/request filenames, private files, fsync, and atomic rename. A child
restart reconnects to its existing execution; a Remote restart retains completed
results and reports unfinished receipts as interrupted. Starting an interrupted
request again retries it: delivery is at least once across host crashes, not
exactly once. External side effects should be safe to retry.

Persist your application's acknowledgment before `AgentTurns.Forget(id)`.
Forgetting an active turn returns `ErrAgentBusy`. Uninstall removes the instance's
receipts; stop/start and upgrades retain them. Receipts are execution metadata,
not application workflow storage. The application owns its state in `DataDir`.

## Application tools

`agentTools` lets supported providers receive a short-lived grant for eligible
running installations plus their discovered application skills, without manual
skill selection. Provider modules declare `Features.ApplicationTools` and
forward the issued runtime environment through their normal launch adapter:

- `REMOTE_APPLICATION_API` is the installation's public base URL with
  `/agent-api/applications` appended, issued in the provider's turn environment.
- `REMOTE_APPLICATION_GRANT` is a bearer capability for the current turn.

For project containers using the LXD bridge resolver, host convergence pins
the public API hostname to the bridge's IPv4 gateway and disables importing
the host's `/etc/hosts` into bridge DNS. This prevents a host loopback alias
from directing application tools back into the container. The API URL and
normal HTTPS certificate verification remain unchanged; application CLIs do
not need an IP-address override. Fresh installs and full infrastructure updates
apply the pin, while application-only deployments do not. See
[Container-to-host API DNS](../../04-operations/09-deployment-and-operations.md#container-to-host-api-dns)
for the managed settings, operational effects, and verification commands.

An application CLI calls `${REMOTE_APPLICATION_API}/<application-id>/<path>`
with that bearer value. Remote forwards method, path, query and body to the
installation's backend, while stamping the current caller and `Request.Agent`.
Bodies are limited to 64 KiB. Browser calls clear `Request.Agent`.

| Trusted `Request.Agent` field | Meaning |
|---|---|
| `ChatID` | The chat associated with the grant |
| `Background` | `true` for a turn started by this application installation |
| `RequestID` | The accepted application request ID for a background turn |
| `Context` | The accepted opaque JSON context for a background turn |

`Request.Agent` is `nil` on ordinary browser calls. The agent endpoint consumes
the bearer header and forwards no incoming HTTP headers to the backend.

Interactive grants cover eligible global installations and installations in the
chat's project; a project installation takes precedence over a global copy of
the same application. Background application turns receive access only to their
own installation. They carry `AgentContext.Background`, `RequestID`, and the
opaque JSON `Context` from the accepted execution request. Interactive context
contains the trusted `ChatID`. The application interprets this data and decides
which routes or actions are allowed. Core has no task schema, completion route,
completion marker, or scheduling permission scope.

Grants are revoked when the provider run ends, expire after four hours, and
recheck installation state and owner authority on use. They are runtime values,
not project secrets; application tools should never log them.

## Ownership and trust

`internal/service/applications` owns execution authority, admission, receipts,
scoped tools, and background recovery. `pkg/applications` is the public contract;
`pkg/applications/rpc` and `internal/integration/applications` bind it across child
processes. `internal/integration/agents` continues to implement provider-specific
launching and parsing. Application backends own workflow decisions and tool
permissions.

These capabilities are available to trusted, admitted backend code. Backends
already run with the server's host privileges and are not sandboxed. A backend
supplies its captured owner; this SDK does not make hostile backend code safe.
See [13 — Security model](13-security-model.md).

For the route and response codes, see [12 — HTTP API](12-http-api.md#agent-to-application-routes).
For an implementation that owns its workflow, see
[Scheduled Tasks](../../../applications/scheduled-tasks/README.md).
