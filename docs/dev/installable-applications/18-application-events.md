# 18 — Backend event lifecycle

Application events let one running application backend announce a fact and let
other running backends react without either package importing the other. Remote
also exposes its own application lifecycle through the same delivery envelope.

This is an in-process notification facility, not a durable message broker:
delivery is at most once, memory-only, and never replayed after a server restart.
Use durable workflow state, an outbox, or an external broker when missing an
event is not acceptable.

## The two layers

Remote keeps its core domain events strongly typed. `ApplicationPublisher`
publishes typed catalog and installed-copy transitions inside
`backend/internal/lifecycle`. `ApplicationEventBridge` translates those facts
into the public dynamic `applications.Event` envelope.

Application backends emit and consume only the dynamic contract. Event code is
conventionally owned by an importable `backend/lifecycle/` package. The API and
lifecycle packages may run in the same backend process, but the API backend
does not implement the publisher runtime:

```text
Application service
    -> typed ApplicationPublisher
    -> ApplicationEventBridge
    -> bounded EventBus queue
    -> bounded application delivery queue
    -> matching running backends

Application backend process
    -> backend/lifecycle business event
    -> core-owned Runtime.Events emitter
    -> manifest validation and trusted source stamping
    -> bounded EventBus queue
    -> bounded application delivery queue
    -> matching running backends
```

Core code that genuinely needs the dynamic feed may subscribe directly to
`lifecycle.EventBus`. That subscription is explicit composition-root wiring;
there is no package singleton, reflection, or automatic discovery. The bus
invokes direct core callbacks asynchronously on one ordered worker. Callbacks
must still be fast and honor shutdown cancellation because one slow callback
delays later dynamic events. Panics are recovered and logged so one hook cannot
kill the worker. A direct core callback receives the raw feed; application scope
filtering is performed later by the backend delivery adapter, so a core
subscriber must apply any audience policy its own side effect requires.

## Declare publishers and subscriptions

An event-capable application needs a `backend/` executable and declarations
in `application.json`:

```json
{
  "id": "job-runner",
  "name": "Job Runner",
  "version": "1",
  "scopes": ["global", "project"],
  "publishers": [
    {
      "name": "jobs",
      "events": [
        {
          "name": "completed",
          "version": 1,
          "description": "A job completed successfully."
        }
      ]
    }
  ],
  "subscriptions": [
    {
      "publisher": "remote.applications",
      "events": ["installed", "uninstalled"]
    },
    {
      "publisher": "applications.importer.imports",
      "events": ["completed"]
    }
  ]
}
```

`publishers[].name` is local to the declaring application. The backend above
publishes to `jobs`; subscribers refer to it as
`applications.job-runner.jobs`. This qualification is performed by Remote and
prevents a package from claiming another package's namespace.

Canonical publishers have one of two forms:

| Owner | Canonical publisher |
|---|---|
| Remote core | `remote.applications` |
| Application | `applications.<application-id>.<local-publisher>` |

The `remote` and `applications` leading segments are reserved, so local
publisher names cannot begin with either. Publisher and event names use
lowercase alphanumeric segments separated by `.` or `-`.

An event declaration identifies one current payload schema by name and version.
Versions start at `1`; move the number when the payload contract changes.
Subscriptions select event names rather than versions, so a subscriber must
inspect `Event.Version` and ignore or reject versions it cannot decode.

The complete field-level validation is in
[02 — application.json reference](02-application-json.md#publishers).

## Own event behavior in `backend/lifecycle`

The `backend/` root is the required `package main` and composition root. Keep
publisher, subscriber, and event-state behavior in importable child packages
so the request package does not become the owner of unrelated concerns:

```text
backend/
  main.go         required executable; calls rpc.ServeWithRuntime
  api/
    api.go        request handling and backend contract
  lifecycle/
    jobs.go       typed business-event triggers
  container/      excluded from the host module and built in LXD
```

Remote generates one module rooted at `backend/`, excludes `container/`, and
builds `.`. Its module path is
`futrx.local/catalog/applications/<application-id>/backend`, matching the
catalog module used in this checkout. `go.mod`, `go.sum`, `go.work`, and
`go.work.sum` are forbidden throughout the host tree so an application cannot
replace that generated boundary. The executable asks the RPC runtime to supply
core-owned capabilities, then composes the API and lifecycle layers:

```go
func main() {
	rpc.ServeWithRuntime(func(runtime applications.Runtime) applications.Backend {
		jobs := appLifecycle.NewJobs(runtime.Events)
		return appAPI.New(jobs)
	})
}
```

`Runtime.Events` is implemented and initialized by Remote, not by the API or
lifecycle package. This remains one binary, one per-instance process, and one
RPC handshake. Each child directory is a source-ownership boundary, not an
independently launched process.

## Emit from the lifecycle owner

Retain the core-owned emitter behind a typed business-event method:

```go
// backend/lifecycle/jobs.go
package lifecycle

// JobEvents is the consumer-facing contract owned by this publisher.
type JobEvents interface {
	Completed(jobID string) error
}

type Jobs struct {
	events applications.EventEmitter
}

func NewJobs(events applications.EventEmitter) *Jobs {
	return &Jobs{events: events}
}

func (j *Jobs) Completed(jobID string) error {
	payload, err := json.Marshal(map[string]string{"jobId": jobID})
	if err != nil {
		return err
	}
	return j.events.Emit(applications.Publication{
		Publisher: "jobs",
		Event:     "completed",
		Version:   1,
		Payload:   payload,
	})
}
```

The publisher package owns both `JobEvents` and `Jobs`. The API accepts the
interface, while `backend/main.go` constructs the concrete implementation. For
multiple publishers, give each publisher its own interface and concrete type,
construct each from the same core-owned `Runtime.Events`, and pass them to the
API separately. Hello Remote demonstrates this with `GreetingEvents` and
`InspectionEvents`.

The manifest is the only publisher registry. During startup, Remote constructs
an authorization registry from its validated declarations and binds
`Runtime.Events` before calling `Backend.Init`. The application neither
implements nor initializes a publisher. Stop and uninstall terminate the one
process; a later start binds a fresh emitter before initialization.

Every publication must exactly match a declared local publisher, event name,
and version. The payload must be a valid, non-null JSON object no larger than
64 KiB. The SDK's RPC emitter rejects oversized payloads before transport,
and the host independently validates size and shape at its trust boundary. An
omitted payload is normalized to `{}`. Arrays, strings, numbers, booleans,
`null`, malformed JSON, and oversized objects are rejected before they reach
the bus.

The backend cannot supply `Event.Source`. Remote stamps the installed
application ID, instance ID, scope, project ID, and canonical publisher. Treat
those source fields as trusted routing metadata; treat the publisher-defined
payload as untrusted input.

`EventEmitter.Emit` returning nil means Remote validated and submitted the
event to its best-effort in-memory dispatcher. It does not wait for subscribers,
cannot report their result, and does not guarantee delivery when an overload
queue is full.

## Consume events in the lifecycle owner

Declare `subscriptions` and implement the optional
`applications.EventSubscriber` capability:

```go
// backend/lifecycle/events.go
func (e *Events) OnEvent(event applications.Event) error {
	if event.Source.Publisher != "applications.importer.imports" ||
		event.Name != "completed" || event.Version != 1 {
		return nil
	}

	var payload struct {
		ImportID string `json:"importId"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return err
	}
	// React idempotently where the side effect matters.
	return nil
}
```

The delivered envelope is:

```go
type Event struct {
	Source  EventSource
	Name    string
	Version int
	Payload json.RawMessage
}
```

`Event.Source` identifies the publisher and originating installed copy:

```json
{
  "applicationId": "importer",
  "instanceId": "instance-123",
  "scope": "project",
  "projectId": "project-456",
  "publisher": "applications.importer.imports"
}
```

An event handler has no browser caller and no `Request.Caller`; authorization
must be based on the declared publisher, trusted source metadata, scope, and
validated payload. The manifest backend `access` setting governs browser calls,
not machine-to-machine event delivery.

Remote does not know the meaning of application-defined payload fields and
does not redact them. Never publish a password, token, or other secret unless
every scope-eligible subscriber is intentionally allowed to receive it.
`OnEvent` may overlap with ordinary `Handle` calls, so a backend must
synchronize shared state. An error or panic affects only that delivery and is
logged by Remote. Delivery uses the smaller of the manifest backend
`timeoutMs` and 30 seconds. A timeout also terminates that unresponsive backend
process so its uncancellable RPC cannot accumulate; a later request or event
restores the process lazily. None of these outcomes fails publication or
prevents later recipients from being attempted.

## Scope routing

Origin scope controls which matching subscription instances are eligible:

| Event origin | Recipients |
|---|---|
| Project instance | Running project-scoped subscribers in that same project only; never global subscribers |
| Global instance | Every matching running subscriber, global or project-scoped |
| Remote catalog event (no instance scope) | Every matching running subscriber, global or project-scoped |

This rule follows the source of the fact, not the application IDs involved. A
project-origin event cannot escape its project even when a global instance has
a matching subscription. A global event is server-wide and may reach every
project instance that subscribed.

Only installed copies whose persisted state is `running` are considered.
Successful install and start make their declared subscriptions eligible;
successful stop and uninstall remove that eligibility. After a server restart,
an eligible backend is restored lazily and completes `Describe`, `Init`, and
any event-capability initialization before delivery.

## Remote's `remote.applications` publisher

Remote publishes seven version-1 application lifecycle events. Catalog events
identify a catalog entry; instance events identify an installed copy.

| Event | Kind | Emitted after |
|---|---|---|
| `added` | catalog | A new uploaded package is stored, validated, and in the live catalog |
| `updated` | catalog | An uploaded package replacement is stored, validated, and live |
| `deleted` | catalog | Cascade uninstall has completed and the uploaded package has left the catalog |
| `installed` | instance | Provisioning, backend startup, and the final running record succeed |
| `uninstalled` | instance | Teardown and deletion of the installed record succeed |
| `started` | instance | A stopped copy reaches and persists running state |
| `stopped` | instance | A running copy reaches and persists stopped state |

The envelope always has `Source.ApplicationID: "remote"`,
`Source.Publisher: "remote.applications"`, and `Version: 1`. Catalog events
have no source instance, scope, or project. Their payload is:

```json
{
  "version": 1,
  "applicationId": "hello-remote"
}
```

Instance events additionally stamp the subject copy's routing fields into the
source and payload. A project-scoped example is:

```json
{
  "version": 1,
  "applicationId": "hello-remote",
  "instanceId": "instance-123",
  "scope": "project",
  "projectId": "project-456"
}
```

For a global instance, `scope` is `global` and `projectId` is omitted. In these
core events, `Source.ApplicationID` identifies Remote as the emitter while the
payload's `applicationId` identifies the subject application.

Built-in catalog entries loaded at startup do not emit `added`. Rejected or
failed operations emit nothing. Install emits only `installed`; start emits
only `started`. Same-state start/stop requests and automatic upgrades do not
invent transitions. Retry cleanup is silent, and a successful retry emits one
`installed`. Removing an uploaded package emits committed `uninstalled` events
for its copies before `deleted`.

## Delivery and overload

The event bus is in-memory and has no persistence, replay, acknowledgement, or
retry. It has two deliberate bounded asynchronous boundaries:

- `EventBus.Publish` snapshots the current core subscribers, copies the event,
  and nonblockingly enqueues it on a 256-event, drop-new queue;
- one bus worker invokes each accepted snapshot in publication and registration
  order using its process-lifecycle context, not the producer's request context;
- a direct core callback panic is recovered and logged, while a slow callback
  delays the bus and should therefore move slow work behind its own bounded
  worker;
- the application delivery adapter is one bus subscriber and nonblockingly
  enqueues into its own 256-event, drop-new queue, isolating the bus worker from
  backend startup and handlers;
- one application worker begins delivery attempts in accepted-event and
  recipient order;
- backend startup and each recipient call are bounded by the smaller of that
  application's backend timeout and 30 seconds;
- a recipient error, panic, process failure, or timeout is logged and routing
  continues; and
- a slow recipient can delay later delivery for at most 30 seconds, but cannot
  fail the producer or permanently block either publishing path.

If either bounded queue is full, that boundary drops and logs the newly
submitted event while retaining events already queued. A drop at the bus means
no subscriber sees the event; a drop at the application adapter may occur after
a direct core callback has already seen it. Publishing itself never waits for
application code or a direct core hook, so a hook may safely request a lifecycle
write for the same instance that emitted the event.

The RPC transport cannot cancel an `OnEvent` call already running inside a
backend. On timeout Remote terminates that process, which releases the
host-side RPC and prevents one stuck handler per later event from accumulating.
The timed-out event is not retried; a later event starts a clean process and
performs the full capability handshake before delivery. Backends must still
keep `OnEvent` concurrency-safe because it may overlap `Handle` calls.

A process crash can lose both queued and currently delivered events. A
subscriber registered after an event is published receives no history. Write
handlers so duplicate-safe behavior is easy if a producer later retries at its
own domain level, but do not assume this facility itself retries.

The application service registers its router with `EventBus` during service
construction and unregisters it when the service closes or its lifecycle
context ends. Process shutdown joins that worker before stopping backend
children, so an accepted event cannot lazily relaunch a process after the host
starts shutting down. Backends do not mutate the bus registry directly: the
router resolves the current manifest and running instances for each event,
which makes stop, start, uninstall, package replacement, and server shutdown
obey the same lifecycle state.

## Not the browser event API

Manifest `publishers` and `subscriptions` are server-side, backend-to-backend
events. The browser's `remote.events.on(...)` extension API is a separate,
tab-local mechanism for UI events such as `upload.completed`. It does not
publish to this bus, cannot subscribe to `remote.applications`, and has
different lifecycle and failure semantics.

## Related

- [02 — application.json reference](02-application-json.md#publishers) — declaration fields and validation.
- [15 — Application backends](15-application-backends.md#optional-event-capabilities) — composing the lifecycle owner into the required API backend.
- [13 — Security model](13-security-model.md#backend-event-security) — trust boundaries and review checklist.
- [Lifecycle publishers and subscribers](../lifecycle-events.md) — the typed core publishers and bridge.
