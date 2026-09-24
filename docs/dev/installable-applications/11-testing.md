# 11 — Testing

## The catalog validates itself at build time

`NewRegistry()` loads and validates every application at server startup, and the tests
call it directly. So the fastest check that a new or edited application is well formed
is:

```bash
cd backend && go test ./internal/integration/containers/applications/
```

This catches: a mismatched `id`, a missing `name`, an invalid capability layout or
`scopes`, a `service` application with no port or no install script, a `ui` or
`backend` application declaring a port, a `ui` block naming a file that does not
exist, an empty `ui/` directory, a `backend/` root that is not a `package main`
program, or a host backend tree that carries `go.mod`, `go.sum`, `go.work`, or
`go.work.sum` instead of using Remote's generated module.

Backend source is also compiled by the repository's own build. The catalog is
a Go module: `backend/` is the executable package and child host directories
such as `backend/api/` and `backend/lifecycle/` are normal importable packages:

```bash
go build ./... && go vet ./...
```

A backend or one of its imported host siblings that does not compile fails
there, not on someone's server. The runtime build reproduces that layout in a
generated module, compiles `.`, and omits `backend/container/` entirely.

A malformed application fails the build — it never reaches a browser as a 404.

## Backend tests

```bash
cd backend
go test ./internal/integration/containers/applications/   # catalog + installer
go test ./internal/service/applications/                  # scoping + policy
go test ./internal/integration/applications/                # compiling and running backends
go test -race ./internal/lifecycle/                         # typed publishers + dynamic event bus/bridge
go test ./pkg/applications/...                               # the backend SDK
go build ./... && go vet ./...
```

| File | Covers |
|---|---|
| `registry_test.go` | catalog loading, capability inference, `ui/` discovery, the declared `ui` manifest, asset path traversal, reserved directories |
| `registry_backend_test.go` | `backend/` discovery, module-control-file rejection, host sibling inclusion, container-source exclusion, and every layout the registry refuses |
| `registry_events_test.go` | publisher/subscription names, versions, canonical namespaces, and backend requirements |
| `installer_test.go` | which `lxc` commands each scope issues — and, crucially, which it must **not** |
| `service/applications/ui_extensions_test.go` | which extensions a caller may load, and their install scope |
| `service/applications/backend_test.go` | who may call a backend, when, and what lifecycle does to its process |
| `service/applications/defaults_test.go` | one-time default installation, validation, adoption, retries, failure isolation, and respecting stop/uninstall |
| `stores/fileapplications/store_test.go` | atomic instance and `defaults.json` persistence, permissions, concurrency, and uninstall independence |
| `applications/host_test.go` | compiling, launching, one process per instance, restart, timeout, panic isolation, data retention, large seekable responses, and late-stream cleanup |
| `applications/events_test.go` | publication authorization, host-stamped identity, payload limits, runtime binding, and delivery |
| `applications/builder_test.go` | fingerprinting and the generated module files |
| `applications/catalog_test.go` | an API importing a sibling lifecycle package, with container source excluded, compiled and called end to end |
| `pkg/applications/mux_test.go` | route matching, method fallbacks, request helpers |
| `pkg/applications/rpc/events_test.go` | core-owned emitter binding and subscriber delivery across the backend RPC boundary |
| `pkg/applications/rpc/stream_test.go` | bounded absolute reads, seek behavior, disconnect cleanup, and blocked-read cancellation across the response-stream RPC boundary |
| `lifecycle/event_bus_test.go`, `application_event_bridge_test.go` | defensive payload copies and canonical version-1 core event envelopes |
| `handlers/applications_backend_handler_test.go` | which headers cross the boundary, plus streamed `GET`, `HEAD`, ranges, conditionals, and cancellation |
| `applications/file-management/backend/workspace/*_test.go` | rooted file access, symlink containment, bounded listing/search/archive behavior, media policy, and spool cleanup |
| `applications/file-management/backend/api/api_test.go` | trusted chat-context requirement, route JSON, status mapping, dispositions, media policy, and ZIP responses |

`applications` tests compile real backends with the Go toolchain, so they take
tens of seconds on a cold cache. `-short` skips exactly those:

```bash
go test -short ./internal/integration/applications/
```

They also skip themselves on a host with no Go toolchain rather than failing.

`installer_test.go` runs against a fake `command.Runner` that records every
invocation, so it asserts on absence as well as presence: a project-scope
install must issue no `launch`, `init`, `copy`, or `create`, and a project
uninstall must issue no `delete`. Those are easy to break in a refactor and
invisible in a diff.

## Frontend tests

```bash
cd frontend
npm test          # all tests
npm run build     # tsc -b + vite; type errors fail here
```

| File | Covers |
|---|---|
| `state/stores/extensions/extensionStore.test.ts` | ordering, unknown slots, `when` predicates, disposal, application removal, workspace panes, and all the scoping rules |
| `config/extensions.test.ts` | slot names are unique, and every slot declares an icon appearance |
| `app/extensions/extensionApi.test.ts` | frontend commands delegate to their core-owned surfaces |
| `app/extensions/extensionBackend.test.ts` | which running backend a call resolves to, and the URL it builds |
| `app/extensions/extensionPopup.test.ts` | popup cleanup and topmost-only Escape behavior with other surfaces |
| `applications/file-management/ui/scripts/*.test.mjs` | browser state transitions and file click/category/formatting policy |

Run one file directly while iterating:

```bash
node --experimental-strip-types --test \
  src/state/stores/extensions/extensionStore.test.ts
```

From the repository root, application-owned JavaScript can be tested without
the SPA build as well:

```bash
node --test applications/file-management/ui/scripts/*.test.mjs
```

Note the repo convention: modules that node tests import use **explicit `.ts`
extensions** in their relative imports (`import … from "../../../config/extensions.ts"`), because
node's ESM resolver does not add them. `allowImportingTsExtensions` is on in
`tsconfig.json`.

## The in-app self-test

`ui-playground` ships an API self-test that runs inside a real extension and
checks the contract end to end — view resolution, asset URLs, traversal being
refused, unknown slots degrading, disposers being idempotent.

Install it, open Settings → Applications, click **Run API self-test**. Ten
checks, pass/fail each. This is the cheapest regression check after changing
`extensionApi.ts`, because it exercises the *served* assets and the *real* API
object rather than a test double.

`backend-playground` ships the same thing for the other half: fourteen checks
against a real backend process, over the real route. Run it after changing
`pkg/applications`, `applications`, or the backend handler.

See [10 — Fixtures](10-fixtures.md).

## Manual verification

Some behaviour only exists in a browser: sizing, hover states, whether a
contribution actually lands in the right row.

```bash
cd backend  && go run ./cmd/remote
cd frontend && npm run dev
```

Then work through the fixture matrix in [10 — Fixtures](10-fixtures.md).

Remember: **editing anything under `ui/` or `backend/` requires a backend
rebuild**, because both are embedded in the binary. `npm run dev` will not pick
them up. A `backend/` edit is then recompiled by the server on the next install
or start, because the build fingerprint changed.

### What is worth checking by hand

| Behaviour | How |
|---|---|
| A contribution renders in the right place | Install a fixture; look |
| Icons match their neighbours | Compare against the app's own icons in the same row |
| Install gating | Confirm nothing appears before install |
| Scope gating | Two projects, one project-scoped install; switch chats |
| Lifecycle | Stop / start / uninstall, without reloading |
| Failure isolation | Make an extension throw; confirm the surface still renders |
| Theming | Toggle light/dark; confirm your CSS follows |
| A backend is a process | Watch `backend-playground`'s pid across stop and start |
| A backend survives a panic | Click **panic (survivable)**, then check the pid |
| Project event isolation | Publish from a project instance; verify only matching subscriber instances in that same project receive it, never a global instance |
| Global/catalog event reach | Publish globally or mutate the uploaded catalog; verify every matching running subscriber scope is eligible |
| Subscription lifecycle | Stop a subscriber, publish, start it, and confirm there is no replay of the missed event |
| Subscriber failure isolation | Make one `OnEvent` fail or time out; verify publication succeeds and later recipients are still attempted |

For a manifest event change, also test a publication with the wrong publisher,
event, and version; primitive, `null`, malformed, and over-64-KiB payloads; and
an empty payload normalized to `{}`. An application subscription selects an
event name, so exercise both the version the handler understands and one it
must ignore safely. See
[18 — Backend event lifecycle](18-application-events.md).

## Testing an install script

Install scripts only run against a real container, so they need a host with
working LXD. Nothing in CI executes them.

The loop: install at project scope → read the error and script output on the
installed row if it fails → fix → hit **Retry**, which installs again and
therefore also proves idempotency. (**Start** does not re-run the script.) See
[04 — Install scripts](04-install-scripts.md).

## What is not covered by tests

Be aware of the gaps rather than assuming coverage:

- **Install scripts are never executed** by any test.
- **A backend's own behaviour is only as tested as the backend.** The platform
  tests the contract and the host; what an application's `backend/` actually does is
  covered by whatever tests that application ships.
- **The HTTP handlers have no request-level tests** for the applications
  routes; only `uiAssetContentType` is unit-tested. The endpoints are exercised
  by hand.
- **Rendering is not unit-tested.** `ExtensionSlot.tsx` has no test; the
  registry it reads from does. Rendering is verified in a browser.
- **CI does not run `go test`.** Run it locally before pushing — see
  [CONTRIBUTING](../../../CONTRIBUTING.md).

## Before opening a pull request

```bash
cd backend  && gofmt -l ./internal ./cmd && go vet ./... && go test ./...
cd frontend && npm run build && npm test
```

Then, if you touched the extension surface or the backend contract, install
[`hello-remote`](../../../applications/hello-remote/README.md) at both scopes and confirm its
panel still greets you, reaches the supervised service, inspects the container,
and keeps its counter across a server restart. In a project install, also
confirm that the `hello-remote-inspector` skill is present.
