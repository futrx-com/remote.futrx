# 15 — Application backends

An application can ship a `backend/` directory of Go source with its executable
at the root. The server builds it with its importable child packages, runs the
resulting executable as a separate process, and forwards HTTP calls to it — so an
application can add a **server-side feature**, and its `ui/` can call that
feature, without any change to the Remote codebase.

```
applications/my-backend/
  application.json
  backend/
    main.go           ← required executable and composition root
    api/              ← importable request-handling package
      api.go
    lifecycle/        ← optional imported package that owns event publishers
      events.go
  ui/                ← runs in the browser, calls the backend
    scripts/main.js
```

`ui/` is what an application can add to the interface. `backend/main.go` is the
entry point for what it adds to the server. Child directories under
`backend/` keep cohesive host concerns out of the composition root;
`backend/lifecycle/` conventionally owns event publication and subscription.
They all compile into the same backend process.

## The shortest complete backend

Place this single-file version at `backend/main.go`. Larger backends should
move request handling into `backend/api/` and leave only dependency composition
in `main.go`.

```go
package main

import (
	"net/http"

	"github.com/futrx-com/remote.futrx.com/pkg/applications"
	"github.com/futrx-com/remote.futrx.com/pkg/applications/rpc"
)

type backend struct {
	// OPTIONAL — Router is a routing convenience, not part of Backend.
	router   *applications.Router
	instance applications.Instance
}

// REQUIRED — serve a value implementing applications.Backend.
func main() {
	b := &backend{router: applications.NewRouter()}
	b.router.GET("hello", "Say hello", func(request applications.Request) applications.Response {
		return applications.JSON(http.StatusOK, map[string]string{
			"hello": request.Caller.Email,
		})
	})
	rpc.Serve(b)
}

// REQUIRED — APIVersion must be the SDK constant.
func (b *backend) Describe() (applications.Descriptor, error) {
	return applications.Descriptor{
		APIVersion: applications.APIVersion,
		Routes:     b.router.Routes(),
	}, nil
}

// REQUIRED — called once before the first request.
func (b *backend) Init(instance applications.Instance) error {
	b.instance = instance
	return nil
}

// REQUIRED — may be called concurrently.
func (b *backend) Handle(request applications.Request) (applications.Response, error) {
	return b.router.Serve(request), nil
}
```

and in `application.json`:

```json
{
  "id": "my-backend",
  "name": "My Backend",
  "scopes": ["global", "project"]
}
```

From the browser:

```js
const greeting = await remote.backend.call("hello");
```

That is the entire round trip.

## Required surface

The host executable is the Go program at the `backend/` root. Remote creates a
module there, so the composition root can import child host packages such as
`api/` and `lifecycle/`; it then builds `.`. These are the parts a backend must
have; `Router`, `applications.JSON`, route registration, persistence, and the
route table are conveniences rather than contract requirements.

| Requirement | Enforced by | Failure if omitted |
|---|---|---|
| `package main` and at least one non-test Go file in the `backend/` root | catalog validator | application is refused at load |
| no `go.mod`, `go.sum`, `go.work`, or `go.work.sum` anywhere in the host backend tree | catalog validator; Remote generates and owns the module boundary | application is refused at load |
| `func main()` calling `rpc.Serve` | compiler and backend handshake | build failure, or a process that cannot connect |
| `Describe() (applications.Descriptor, error)` | `applications.Backend` interface | build failure |
| `Init(applications.Instance) error` | `applications.Backend` interface | build failure |
| `Handle(applications.Request) (applications.Response, error)` | `applications.Backend` interface | build failure |
| `Descriptor.APIVersion: applications.APIVersion` | runtime handshake | host refuses the backend |

Do not repeat package metadata in `Describe`. Remote fills `Descriptor.Name`
and `Descriptor.Version` from `application.json` after the handshake, making
the manifest the single source for the UI, upgrade policy, and backend
descriptor. The backend owns only its API contract version and routes.

`backend/container/` is not part of that host module. The registry removes the
whole subtree before fingerprinting or compiling host source, even when the
backend has child packages. Container-only changes therefore cannot
accidentally enter the host binary through a broad `./...` build.

`backend/container/` is a separate, optional execution context. A root main
package there is built as one binary named after the application ID.
Alternatively, each immediate `backend/container/cmd/<binary>/` package is
built as a separately named binary. In both layouts the source is copied into
and built inside LXD, and the executable package requires only `package main`
and `func main()`. If a `cmd/*` package exists, Remote builds the discovered
`cmd/*` packages rather than the root as an executable. Remote owns the module
fallback, packaging, toolchain, installation, and build marker. See the
[container-side Go program layout](04-install-scripts.md#container-side-go-programs).

## The contract

The Go types are in
[`pkg/applications/contract.go`](../../../backend/pkg/applications/contract.go). A
backend implements three methods.

| Method | When | Notes |
|---|---|---|
| `Describe()` | once, on connect | Route table and API contract. Remote supplies the name and version from `application.json`. |
| `Init(Instance)` | once, before the first request | The install this process serves. Returning an error fails the app's install or start. |
| `Handle(Request)` | per request | May be called concurrently. |

Event emission is supplied through `Runtime.Events`; event subscription is an
optional capability layered on these three required methods. Neither changes
the `Backend` interface; see [Application events](#application-events).

### `Instance`

What `Init` receives, fixed for the process's lifetime:

| Field | Notes |
|---|---|
| `ID`, `ApplicationID` | the installed copy, and the application it came from |
| `ApplicationName`, `ApplicationVersion` | manifest metadata; do not copy it into backend constants |
| `Publishers`, `Subscriptions` | the validated event declarations from `application.json` |
| `Service` | the systemd unit declared by the manifest, if any |
| `Scope`, `ProjectID` | `"global"`, or `"project"` with the project |
| `ContainerName`, `InternalPort`, `ExternalPort` | the container half, when the application has one |
| `Env` | the install's resolved inputs, **including generated secrets** |
| `DataDir` | a directory on the host this instance owns and may write to |

`Env` carries real passwords: a database application's backend needs the one its own
`install.sh` generated. That is deliberate, and it is why what a backend does
with them is a review question — see [13 — Security model](13-security-model.md).

`DataDir` survives stop and start, and is deleted on uninstall. It is the only
storage the platform gives a backend.

### Application events

`application.json` is the only publisher registry. Remote validates every
publisher, event, and version while loading the package and builds a scoped
authorization registry for each installed instance. Application code does not
implement a publisher or an initialization hook.

When an application declares publishers, construct its backend with
`rpc.ServeWithRuntime`. Core binds `Runtime.Events` before `Backend.Init`:

```go
// backend/main.go
func main() {
	rpc.ServeWithRuntime(func(runtime applications.Runtime) applications.Backend {
		jobs := appLifecycle.NewJobs(runtime.Events)
		return appAPI.New(jobs)
	})
}
```

Put event identity and payload construction in an importable
`backend/lifecycle/` package. The API may trigger this business behavior, but it
does not implement the runtime capability:

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
		Publisher: "jobs", // local manifest name
		Event:     "completed",
		Version:   1,
		Payload:   payload,
	})
}
```

The lifecycle publisher owns both the interface consumed by the API and its
concrete emitter. `backend/main.go` is the only layer that constructs and wires
them. Multiple manifest publishers should use separate interface/concrete-type
pairs, even though core supplies all of them with the same scoped emitter.

The import path is stable because Remote's generated module is named
`futrx.local/catalog/applications/<application-id>/backend`, matching the
catalog module used in this checkout. `backend/lifecycle/` is an ownership
boundary inside that module, not a second process.

Remote accepts a publication only when publisher, event, and version exactly
match the manifest. `Payload` must be a non-null JSON object no larger than 64
KiB; an omitted payload becomes `{}`. The backend supplies no source identity.
Remote stamps application ID, installed-copy ID, scope, project, and the
canonical publisher before dispatch, so one backend cannot impersonate
another.

A nil error from `Emit` means the event was validated and submitted to the
best-effort in-memory dispatcher. Subscriber work happens asynchronously;
completion or failure is not reported to the producer, and a full bounded queue
may drop the event under overload.

#### Subscribing

Subscriptions remain an optional backend capability. Declare them in the
manifest and implement `applications.EventSubscriber` on the served backend or
on an embedded lifecycle owner:

Handle the event envelope delivered by Remote:

```go
// backend/lifecycle/events.go
func (e *Events) OnEvent(event applications.Event) error {
	if event.Source.Publisher != "remote.applications" || event.Version != 1 {
		return nil
	}
	switch event.Name {
	case "installed", "started":
		// Decode the documented JSON object in event.Payload.
	}
	return nil
}
```

`Event.Source` is trusted host metadata. `Event.Payload` is still data chosen
by the publisher and must be decoded and validated like any other external
input. `OnEvent` may overlap with `Handle`, so protect shared state. Returning
an error or panicking fails only that delivery and leaves the process running.
Event delivery uses the smaller of the manifest backend `timeoutMs` and 30
seconds. A handler that exceeds that deadline is different: Remote terminates
the unresponsive process so uncancellable event RPCs cannot accumulate, then
restores it lazily for a later request or event. Subscriber failure never rolls
back the publication or a lifecycle transition.

The complete namespace, routing, lifecycle, queue, and delivery contract is in
[18 — Backend event lifecycle](18-application-events.md).

### `Request`

| Field | Notes |
|---|---|
| `Method`, `Path`, `Query`, `Headers`, `Body` | the browser's call. `Path` is relative to the instance's `/backend/` prefix and has no leading slash. |
| `Caller` | `{ Email, IsAdmin }`, resolved by the server |
| `Context.Chat` | on a chat-scoped route only: `{ ID, ProjectID, WorkspaceRoot }`, resolved after core authorizes the chat |

**`Caller` is stamped by the server, not read from the request.** A browser
cannot forge it, which is what makes it usable for authorization. The transport
withholds the caller's `Cookie` and `Authorization` headers, so a backend is told
who is asking without being handed the means to act as them.

`Context` follows the same rule: ordinary backend calls always clear it, and a
chat-scoped call overwrites it with core's verified values. A global install
may serve any chat the caller can access. A project install is accepted only
for a chat in that same project. `WorkspaceRoot` is a convenient trusted input,
not a process sandbox; backend code retains its normal server privileges.

Bodies are capped at 1 MiB.

### `Response`

Small responses use `{ Status, Headers, Body }`. A zero `Status` is sent as
`200`. `Set-Cookie`, supplied `Content-Length`, and hop-by-hop headers are
dropped, and every response is served `nosniff`.

Build one with the helpers rather than by hand:

```go
applications.JSON(http.StatusOK, value)
applications.Text(http.StatusOK, "plain")
applications.Errorf(http.StatusForbidden, "%s may not do that", request.Caller.Email)
```

`Errorf` produces `{"error": "…"}`, which is the shape `remote.backend.call`
turns back into a thrown `Error` with your message intact.

For a file or another large seekable result, hand Remote an already-open
`io.ReadSeekCloser` instead of filling `Body`:

```go
file, err := os.Open(path)
if err != nil {
    return applications.Errorf(http.StatusNotFound, "%v", err), nil
}
info, err := file.Stat()
if err != nil {
    file.Close()
    return applications.Errorf(http.StatusInternalServerError, "%v", err), nil
}
return applications.Stream(file, info.Size(), info.ModTime(), map[string][]string{
    "Content-Type":        {"application/octet-stream"},
    "Content-Disposition": {`attachment; filename="export.zip"`},
}), nil
```

`applications.Stream(content, size, modTime, headers)` takes ownership of
`content`. `size` must be exact and non-negative. After the constructor returns,
do not also set `Body` or close the content yourself. Core closes it after the
transfer, when the caller disconnects, when transport setup fails, or when a
stream arrives after its call timed out. Cancellation may close content while a
read is blocked; `*os.File` supports that lifecycle, and custom readers must as
well.

Core serves streamed content through `http.ServeContent`, so `GET`, `HEAD`,
byte ranges (including multipart ranges), `If-Modified-Since`, and `If-Range`
work without application code. The stream constructor starts at `200`; do not
replace that status, because core owns the eventual `200`, `206`, `304`, or
`416` status and calculated content length. A non-zero `modTime` enables
`Last-Modified` conditionals. Reads cross
the process boundary in random-access chunks capped at 256 KiB rather than
buffering the whole response in either process.

Request uploads have not changed: `Request.Body` remains buffered and capped at
1 MiB. A future upload-streaming contract will be an explicit request API; it
will not silently change `Request.Body` or this response-stream lifecycle.

### `Router`

Optional, but it keeps a backend's advertised routes and its real routes the
same thing, because `Describe` renders `router.Routes()`.

```go
router.GET("health", "Process identity", handler)
router.POST("kv/*", "Write a key", handler)      // prefix route
router.Handle("*", "echo", "Any method", handler)
```

Patterns are exact or a `/*` prefix. An exact route beats a prefix; a longer
prefix beats a shorter one. Unmatched paths get `404`, and a matched path with
the wrong method gets `405`. Inside a prefix handler, `request.Tail("kv/")`
gives you the rest.

For `HEAD`, the router first honors an explicit `HEAD` or `"*"` method route.
If neither matches, it dispatches to the matching `GET` route while preserving
`request.Method == "HEAD"`. A streamed `router.GET(...)` endpoint therefore
gets normal HTTP `HEAD` behavior without a duplicate registration. A backend
that switches on `Request` itself instead of using `Router` must handle `HEAD`
explicitly.

## Reaching a backend from the browser

`remote.backend` is described in
[06 — Extension API](06-extension-api.md#remotebackend). The short version:

```js
remote.backend.available            // false when nothing is running
remote.backend.instances            // [{ instanceId, scope, projectId }]
remote.backend.call(path, options)  // → parsed JSON, throws on failure
remote.backend.fetch(path, options) // → the raw Response
remote.backend.describe(target)     // → the backend's route table
remote.backend.url(path, target)    // → the URL a call would use
```

An application installed globally *and* in two projects runs **three processes**, so
a call has to resolve to one. Pass the surface's project and it does the
obvious thing:

```js
remote.ui.register(remote.slots.applicationsPanel, async (host, context) => {
  const health = await remote.backend.call("health", {
    projectId: context.projectId,   // that project's backend, else the global one
  });
});
```

From a workspace pane, include `chatId` as well when the backend needs the
authorized workspace:

```js
await remote.backend.call("files", {
  chatId: context.chatId,
  projectId: context.projectId,
});
```

## Reaching a backend over HTTP

See [12 — HTTP API](12-http-api.md#backend-backend-routes). Both scopes:

```
GET    /api/applications/{instanceID}/backend                describe
ANY    /api/applications/{instanceID}/backend/{path...}      call
GET    /api/projects/{projectID}/applications/{instanceID}/backend
ANY    /api/projects/{projectID}/applications/{instanceID}/backend/{path...}
GET    /api/chats/{chatID}/applications/{instanceID}/backend
ANY    /api/chats/{chatID}/applications/{instanceID}/backend/{path...}
```

Calling a backend is the one action on a **global** instance that is not
admin-only: the backend is the server side of an extension that renders for
every signed-in user, so managing the app stays admin-only while calling it
does not. An application can narrow that itself:

```json
"backend": { "access": "admin" }
```

## What the server does with your source

Nothing is compiled until an application with a `backend/` executable is installed.
Then, on install — and on start, and on the first call after a restart:

```
1. select        backend/ root plus child host packages; exclude backend/container/
2. fingerprint   sha256(selected host source + SDK source + generated go.mod + Go version)
3. cache hit?    <dataDir>/backends/bin/<application>-<fingerprint>   → skip to 6
4. materialize   <dataDir>/backends/build/<application>-<fingerprint>/
                   src/main.go      executable composition root
                   src/api/         imported request package, when present
                   src/lifecycle/   imported event package, when present
                   src/go.mod       generated application-specific module
                   sdk/             pkg/applications, as a generated module
5. compile       go build -trimpath ., offline first, network only as a fallback
6. launch        one process per instance, over hashicorp/go-plugin
7. Describe      version and subscriber-capability check
8. BindEvents    core binds Runtime.Events when the manifest declares publishers
9. Init          the instance
```

Steps 4–5 happen once per source change; every later start is a `stat` and a
handshake. The build directory is removed on success and **kept on failure**,
so you can look at exactly what did not compile.

### Why source and not binaries

The catalog is embedded in the server binary with `//go:embed`, and a server
runs on whatever architecture it runs on. Shipping source keeps one catalog
portable across all of them, and keeps a backend reviewable as a diff rather
than as a blob. The cost is a Go toolchain on the server, which the installer
already provides.

### Dependencies

A backend may import the **standard library**, **this SDK**, and its own sibling
host packages. Remote generates `backend/` as a module named
`futrx.local/catalog/applications/<application-id>/backend`; an API imports its
lifecycle owner, for example, as
`futrx.local/catalog/applications/job-runner/backend/lifecycle`. The generated
`go.mod` pins every external module to the version the server itself was built
with, which is what lets a backend compile with no network at all.

`go.mod`, `go.sum`, `go.work`, and `go.work.sum` are rejected anywhere in the
host backend tree: Remote owns the module and workspace boundary. The separate
`backend/container/` build is excluded from that check. If host code needs a
third-party module, the honest answer today is to vendor the code into an
importable child package under `backend/` or add the dependency to the
server.

### Where things live

```
<dataDir>/backends/
  bin/<application>-<fingerprint>     compiled backend, shared by every instance
  build/<application>-<fingerprint>/  generated module, kept only after a failure
  build-cache/                  GOCACHE for backend builds
  home/                         fallback HOME, and therefore module cache,
                                when the service runs without one
  data/<instanceID>/            one instance's DataDir
```

`home/` appears only when the server process has no `HOME` — a systemd unit
without one, typically. It matters because it is then also the module cache, so
the first backend build on that server downloads its dependencies instead of
finding them in the cache the server's own build left behind. Give the service a
`HOME` and the offline path works from the first install.

### No toolchain, no backend

A server with no Go toolchain installs and runs everything else normally; an
application with a `backend/` executable reports the missing toolchain on its installed row. Set
`REMOTE_APPLICATION_GO` to point at a specific `go` binary if it is somewhere
unusual.

## Lifecycle

| Action | The backend process | Its `DataDir` |
|---|---|---|
| Install | compiled and started | created |
| Stop | killed | kept |
| Start | started again | kept |
| Uninstall | killed | **deleted** |
| Server restart | started again on the next call | kept |
| Crash | replaced on the next call | kept |
| Uploaded-package replacement | every old process is killed; the next call compiles the current package | kept |

Stop is the useful one: it is how a user turns a backend off without losing
what it stored.

Only running installed copies receive subscribed events. Install and start
make a declared subscription eligible after the copy reaches running state;
stop and uninstall remove that eligibility immediately. A lazily restored
backend repeats its normal handshake before the next event is delivered.

Because a backend restarts lazily, in-memory state is not durable and is not
meant to be. Anything that must survive belongs in `DataDir`.

## Failure isolation

| What happens | What the caller sees | What happens to the process |
|---|---|---|
| A route panics | the call fails, with the panic message | it keeps running |
| A route never returns | the call fails on the application's `timeoutMs` | it keeps running |
| The process dies | the call fails | it is replaced on the next call |
| The source does not compile | install or start fails, with the compiler's output | there is no process |
| The contract version mismatches | start fails, saying both versions | the process is killed |

`timeoutMs` defaults to 15000 and bounds producing a buffered response or
opening a streamed response. Any nonnegative manifest value is accepted for
compatibility, while the effective runtime timeout is capped at 300000. Once a
stream is open, transfer time follows the HTTP request rather than this setup
deadline. A timed-out call is abandoned rather than interrupted — net/rpc has
no per-call cancellation — so the backend may finish its work unobserved; if it
finishes by returning a stream, Remote immediately closes that late stream.

## Combining capabilities

`backend/main.go`, its imported child packages such as `backend/api/` and
`backend/lifecycle/`, and `ui/` install nothing in a container and work on a
host with no container runtime. The host packages become one executable and
one process. Add
`backend/container/` for Go commands built in LXD, and `infra/install.sh` only
for additional custom provisioning; the host backend can coordinate that
software and its UI can expose it to the user.

## Why net/rpc rather than gRPC

hashicorp/go-plugin offers both. Backends here are Go programs compiled from a
catalog embedded in this server, so there is no second language for a neutral
protocol to serve, and net/rpc keeps a backend's dependencies to this SDK and
the standard library — no protobuf, no code generation, no checked-in
`.pb.go`.

Buffered calls use the primary net/rpc connection. Each streamed response uses
go-plugin's multiplex broker for a separate bounded random-access reader. This
keeps existing `Backend`, `Request.Body`, `Response.Body`, and browser URLs
compatible while allowing the HTTP server to seek for ranges without holding
the complete result in memory.

The whole transport is
[`pkg/applications/rpc`](../../../backend/pkg/applications/rpc/rpc.go).
Moving to gRPC would be a change to that package and a recompile of the
catalog; `applications` — the types a backend author writes against — would not
move.

## Try it

`backend-playground` is the worked example: every mechanism above, exercised
once, with a UI that calls each route and a self-test that asserts the
contract. See [10 — Fixtures](10-fixtures.md).

## Related

- [02 — application.json reference](02-application-json.md#backend) — the `backend` block.
- [03 — Application capabilities](03-application-capabilities.md) — how the backend composition root combines with other capabilities.
- [06 — Extension API](06-extension-api.md#remotebackend) — `remote.backend` in full.
- [12 — HTTP API](12-http-api.md#backend-backend-routes) — the routes and their authorization.
- [13 — Security model](13-security-model.md#application-backends) — what a backend can do, and what stops it.
- [18 — Backend event lifecycle](18-application-events.md) — lifecycle-package ownership, manifest declarations, namespaces, scope routing, and delivery guarantees.
