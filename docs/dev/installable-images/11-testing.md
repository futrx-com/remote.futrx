# 11 — Testing

## The catalog validates itself at build time

`NewRegistry()` loads and validates every image at server startup, and the tests
call it directly. So the fastest check that a new or edited image is well formed
is:

```bash
cd backend && go test ./internal/integration/containers/applications/
```

This catches: a mismatched `id`, a missing `name`, an invalid `type` or
`scopes`, a `service` image with no port or no install script, a `ui` or
`backend` image declaring a port, a `ui` block naming a file that does not
exist, an empty `ui/` directory, and a `plugin/` that is not a `package main`
program or that carries its own `go.mod`.

Plugin source is also compiled by the repository's own build, because a
`plugin/` directory is an ordinary package inside this module:

```bash
cd backend && go build ./... && go vet ./...
```

A plugin that does not compile fails there, not on someone's server.

A malformed image fails the build — it never reaches a browser as a 404.

## Backend tests

```bash
cd backend
go test ./internal/integration/containers/applications/   # catalog + installer
go test ./internal/service/applications/                  # scoping + policy
go test ./internal/integration/pluginhost/                # compiling and running plugins
go test ./pkg/appplugin/...                               # the plugin SDK
go build ./... && go vet ./...
```

| File | Covers |
|---|---|
| `registry_test.go` | catalog loading, image kinds, `ui/` discovery, the declared `ui` manifest, asset path traversal, reserved directories |
| `registry_plugin_test.go` | `plugin/` discovery and every layout the registry refuses |
| `installer_test.go` | which `lxc` commands each scope issues — and, crucially, which it must **not** |
| `service/applications/ui_extensions_test.go` | which extensions a caller may load, and their install scope |
| `service/applications/backend_test.go` | who may call a plugin, when, and what lifecycle does to its process |
| `pluginhost/host_test.go` | compiling, launching, one process per instance, restart, timeout, panic isolation, data retention |
| `pluginhost/builder_test.go` | fingerprinting and the generated module files |
| `pluginhost/catalog_test.go` | the shipped `backend-playground`, compiled and called end to end |
| `pkg/appplugin/mux_test.go` | route matching, method fallbacks, request helpers |
| `handlers/applications_backend_handler_test.go` | which headers cross the boundary in each direction |

`pluginhost` tests compile real plugins with the Go toolchain, so they take
tens of seconds on a cold cache. `-short` skips exactly those:

```bash
go test -short ./internal/integration/pluginhost/
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
| `state/stores/extensions/extensionStore.test.ts` | ordering, unknown slots, `when` predicates, disposal, `removeImage`, and all the scoping rules |
| `config/extensions.test.ts` | slot names are unique, and every slot declares an icon appearance |
| `app/extensions/extensionBackend.test.ts` | which running plugin a call resolves to, and the URL it builds |

Run one file directly while iterating:

```bash
node --experimental-strip-types --test \
  src/state/stores/extensions/extensionStore.test.ts
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
against a real plugin process, over the real route. Run it after changing
`pkg/appplugin`, `pluginhost`, or the backend handler.

See [10 — Fixtures](10-fixtures.md).

## Manual verification

Some behaviour only exists in a browser: sizing, hover states, whether a
contribution actually lands in the right row.

```bash
cd backend  && go run ./cmd/remote
cd frontend && npm run dev
```

Then work through the fixture matrix in [10 — Fixtures](10-fixtures.md).

Remember: **editing anything under `ui/` or `plugin/` requires a backend
rebuild**, because both are embedded in the binary. `npm run dev` will not pick
them up. A `plugin/` edit is then recompiled by the server on the next install
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
| A plugin is a process | Watch `backend-playground`'s pid across stop and start |
| A plugin survives a panic | Click **panic (survivable)**, then check the pid |

## Testing an install script

Install scripts only run against a real container, so they need a host with
working LXD. Nothing in CI executes them.

The loop: install at project scope → read the error and script output on the
installed row if it fails → fix → hit **Start**, which re-runs the script and
therefore also proves idempotency. See
[04 — Install scripts](04-install-scripts.md).

## What is not covered by tests

Be aware of the gaps rather than assuming coverage:

- **Install scripts are never executed** by any test.
- **A plugin's own behaviour is only as tested as the plugin.** The platform
  tests the contract and the host; what an image's `plugin/` actually does is
  covered by whatever tests that image ships.
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

Then, if you touched the extension surface or the plugin contract, install
[`hello-remote`](../../../backend/internal/integration/containers/applications/images/hello-remote/README.md) at both scopes and confirm its
panel still greets you and still counts across a server restart.
