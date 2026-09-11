# 15 — Backend plugins

An image can ship a `plugin/` directory of Go source. The server compiles it,
runs it as a separate process, and forwards HTTP calls to it — so an image can
add a **server-side feature**, and its `ui/` can call that feature, without any
change to the Remote codebase.

```
images/my-plugin/
  image.json
  plugin/            ← Go source, compiled and run on the host
    main.go
  ui/                ← runs in the browser, calls the plugin
    scripts/main.js
```

`ui/` is what an image can add to the interface. `plugin/` is what it can add
to the server. Together they are the whole shape of a feature that could
otherwise only be added by editing this repository.

## The shortest complete plugin

```go
package main

import (
	"net/http"

	"github.com/futrx-com/remote.futrx.com/pkg/appplugin"
	"github.com/futrx-com/remote.futrx.com/pkg/appplugin/pluginrpc"
)

type backend struct {
	mux      *appplugin.Mux
	instance appplugin.Instance
}

func main() {
	b := &backend{mux: appplugin.NewMux()}
	b.mux.GET("hello", "Say hello", func(request appplugin.Request) appplugin.Response {
		return appplugin.JSON(http.StatusOK, map[string]string{
			"hello": request.Caller.Email,
		})
	})
	pluginrpc.Serve(b)
}

func (b *backend) Describe() (appplugin.Descriptor, error) {
	return appplugin.Descriptor{
		Name:       "My Plugin",
		Version:    "1",
		APIVersion: appplugin.APIVersion,
		Routes:     b.mux.Routes(),
	}, nil
}

func (b *backend) Init(instance appplugin.Instance) error {
	b.instance = instance
	return nil
}

func (b *backend) Handle(request appplugin.Request) (appplugin.Response, error) {
	return b.mux.Serve(request), nil
}
```

and in `image.json`:

```json
{
  "id": "my-plugin",
  "name": "My Plugin",
  "type": "backend",
  "scopes": ["global", "project"]
}
```

From the browser:

```js
const greeting = await remote.backend.call("hello");
```

That is the entire round trip.

## The contract

The Go types are in
[`pkg/appplugin/contract.go`](../../../backend/pkg/appplugin/contract.go). A
plugin implements three methods.

| Method | When | Notes |
|---|---|---|
| `Describe()` | once, on connect | Identity and route table. Must report `appplugin.APIVersion` or the host refuses the plugin. |
| `Init(Instance)` | once, before the first request | The install this process serves. Returning an error fails the app's install or start. |
| `Handle(Request)` | per request | May be called concurrently. |

### `Instance`

What `Init` receives, fixed for the process's lifetime:

| Field | Notes |
|---|---|
| `ID`, `ImageID` | the installed copy, and the image it came from |
| `Scope`, `ProjectID` | `"global"`, or `"project"` with the project |
| `ContainerName`, `InternalPort`, `ExternalPort` | the container half, when the image has one |
| `Env` | the install's resolved inputs, **including generated secrets** |
| `DataDir` | a directory on the host this instance owns and may write to |

`Env` carries real passwords: a database image's plugin needs the one its own
`install.sh` generated. That is deliberate, and it is why what a plugin does
with them is a review question — see [13 — Security model](13-security-model.md).

`DataDir` survives stop and start, and is deleted on uninstall. It is the only
storage the platform gives a plugin.

### `Request`

| Field | Notes |
|---|---|
| `Method`, `Path`, `Query`, `Headers`, `Body` | the browser's call. `Path` is relative to the instance's `/backend/` prefix and has no leading slash. |
| `Caller` | `{ Email, IsAdmin }`, resolved by the server |

**`Caller` is stamped by the server, not read from the request.** A browser
cannot forge it, which is what makes it usable for authorization. The transport
withholds the caller's `Cookie` and `Authorization` headers, so a plugin is told
who is asking without being handed the means to act as them.

Bodies are capped at 1 MiB.

### `Response`

`{ Status, Headers, Body }`. A zero `Status` is sent as `200`. `Set-Cookie` and
hop-by-hop headers are dropped, and every response is served `nosniff`.

Build one with the helpers rather than by hand:

```go
appplugin.JSON(http.StatusOK, value)
appplugin.Text(http.StatusOK, "plain")
appplugin.Errorf(http.StatusForbidden, "%s may not do that", request.Caller.Email)
```

`Errorf` produces `{"error": "…"}`, which is the shape `remote.backend.call`
turns back into a thrown `Error` with your message intact.

### `Mux`

Optional, but it keeps a plugin's advertised routes and its real routes the
same thing, because `Describe` renders `mux.Routes()`.

```go
mux.GET("health", "Process identity", handler)
mux.POST("kv/*", "Write a key", handler)      // prefix route
mux.Handle("*", "echo", "Any method", handler)
```

Patterns are exact or a `/*` prefix. An exact route beats a prefix; a longer
prefix beats a shorter one. Unmatched paths get `404`, and a matched path with
the wrong method gets `405`. Inside a prefix handler, `request.Tail("kv/")`
gives you the rest.

## Reaching a plugin from the browser

`remote.backend` is described in
[06 — Extension API](06-extension-api.md#remotebackend). The short version:

```js
remote.backend.available            // false when nothing is running
remote.backend.instances            // [{ instanceId, scope, projectId }]
remote.backend.call(path, options)  // → parsed JSON, throws on failure
remote.backend.fetch(path, options) // → the raw Response
remote.backend.describe(target)     // → the plugin's route table
remote.backend.url(path, target)    // → the URL a call would use
```

An image installed globally *and* in two projects runs **three processes**, so
a call has to resolve to one. Pass the surface's project and it does the
obvious thing:

```js
remote.ui.register(remote.slots.applicationsPanel, async (host, context) => {
  const health = await remote.backend.call("health", {
    projectId: context.projectId,   // that project's plugin, else the global one
  });
});
```

## Reaching a plugin over HTTP

See [12 — HTTP API](12-http-api.md#backend-plugin-routes). Both scopes:

```
GET    /api/applications/{instanceID}/backend                describe
ANY    /api/applications/{instanceID}/backend/{path...}      call
GET    /api/projects/{projectID}/applications/{instanceID}/backend
ANY    /api/projects/{projectID}/applications/{instanceID}/backend/{path...}
```

Calling a plugin is the one action on a **global** instance that is not
admin-only: the plugin is the server side of an extension that renders for
every signed-in user, so managing the app stays admin-only while calling it
does not. An image can narrow that itself:

```json
"backend": { "access": "admin" }
```

## What the server does with your source

Nothing is compiled until an image with a `plugin/` directory is installed.
Then, on install — and on start, and on the first call after a restart:

```
1. fingerprint   sha256(plugin source + SDK source + generated go.mod + Go version)
2. cache hit?    <dataDir>/plugins/bin/<image>-<fingerprint>   → skip to 5
3. materialize   <dataDir>/plugins/build/<image>-<fingerprint>/
                   src/   your plugin/ + a generated go.mod
                   sdk/   pkg/appplugin, as a module named after this repo
4. compile       go build -trimpath, offline first, network only as a fallback
5. launch        one process per instance, over hashicorp/go-plugin
6. Describe      version check
7. Init          the instance
```

Steps 3–4 happen once per source change; every later start is a `stat` and a
handshake. The build directory is removed on success and **kept on failure**,
so you can look at exactly what did not compile.

### Why source and not binaries

The catalog is embedded in the server binary with `//go:embed`, and a server
runs on whatever architecture it runs on. Shipping source keeps one catalog
portable across all of them, and keeps a plugin reviewable as a diff rather
than as a blob. The cost is a Go toolchain on the server, which the installer
already provides.

### Dependencies

A plugin may import the **standard library** and **this SDK**. The generated
`go.mod` pins every module to the version the server itself was built with,
which is what lets a plugin compile with no network at all.

A `go.mod` inside `plugin/` is rejected at catalog load: the server writes that
file. If you need a third-party module, the honest answer today is to vendor
the code you need into `plugin/` or add the dependency to the server.

### Where things live

```
<dataDir>/plugins/
  bin/<image>-<fingerprint>     compiled plugin, shared by every instance
  build/<image>-<fingerprint>/  generated module, kept only after a failure
  build-cache/                  GOCACHE for plugin builds
  home/                         fallback HOME, and therefore module cache,
                                when the service runs without one
  data/<instanceID>/            one instance's DataDir
```

`home/` appears only when the server process has no `HOME` — a systemd unit
without one, typically. It matters because it is then also the module cache, so
the first plugin build on that server downloads its dependencies instead of
finding them in the cache the server's own build left behind. Give the service a
`HOME` and the offline path works from the first install.

### No toolchain, no plugin

A server with no Go toolchain installs and runs everything else normally; an
image with a `plugin/` reports the missing toolchain on its installed row. Set
`REMOTE_PLUGIN_GO` to point at a specific `go` binary if it is somewhere
unusual.

## Lifecycle

| Action | The plugin process | Its `DataDir` |
|---|---|---|
| Install | compiled and started | created |
| Stop | killed | kept |
| Start | started again | kept |
| Uninstall | killed | **deleted** |
| Server restart | started again on the next call | kept |
| Crash | replaced on the next call | kept |

Stop is the useful one: it is how a user turns a backend off without losing
what it stored.

Because a plugin restarts lazily, in-memory state is not durable and is not
meant to be. Anything that must survive belongs in `DataDir`.

## Failure isolation

| What happens | What the caller sees | What happens to the process |
|---|---|---|
| A route panics | the call fails, with the panic message | it keeps running |
| A route never returns | the call fails on the image's `timeoutMs` | it keeps running |
| The process dies | the call fails | it is replaced on the next call |
| The source does not compile | install or start fails, with the compiler's output | there is no process |
| The contract version mismatches | start fails, saying both versions | the process is killed |

`timeoutMs` defaults to 15000 and is per call. A timed-out call is abandoned
rather than interrupted — net/rpc has no cancellation — so the plugin finishes
its work unobserved and answers the next request normally.

## Choosing a type

```
Does installing it run software in a container?
├── yes → "service"    (and it may still ship plugin/ and ui/)
└── no
    ├── does it need server-side code?  → "backend"
    └── is it only browser code?        → "ui"
```

`backend` and `ui` both install nothing in a container and work on a host with
no container runtime at all. A `service` image may ship a `plugin/` too — the
container half provisions the software, the plugin half is what its UI talks
to.

## Why net/rpc rather than gRPC

hashicorp/go-plugin offers both. Plugins here are Go programs compiled from a
catalog embedded in this server, so there is no second language for a neutral
protocol to serve, and net/rpc keeps a plugin's dependencies to this SDK and
the standard library — no protobuf, no code generation, no checked-in
`.pb.go`.

The whole transport is
[`pkg/appplugin/pluginrpc`](../../../backend/pkg/appplugin/pluginrpc/pluginrpc.go).
Moving to gRPC would be a change to that package and a recompile of the
catalog; `appplugin` — the types a plugin author writes against — would not
move.

## Try it

`backend-playground` is the worked example: every mechanism above, exercised
once, with a UI that calls each route and a self-test that asserts the
contract. See [10 — Fixtures](10-fixtures.md).

## Related

- [02 — image.json reference](02-image-json.md#backend) — the `backend` block.
- [03 — Image types](03-image-types.md) — where `backend` sits beside the others.
- [06 — Extension API](06-extension-api.md#remotebackend) — `remote.backend` in full.
- [12 — HTTP API](12-http-api.md#backend-plugin-routes) — the routes and their authorization.
- [13 — Security model](13-security-model.md#backend-plugins) — what a plugin can do, and what stops it.
