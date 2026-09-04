# 11 — Testing

## The catalog validates itself at build time

`NewRegistry()` loads and validates every image at server startup, and the tests
call it directly. So the fastest check that a new or edited image is well formed
is:

```bash
cd backend && go test ./internal/integration/containers/applications/
```

This catches: a mismatched `id`, a missing `name`, an invalid `type` or
`scopes`, a `service` image with no port or no install script, a `ui` image
declaring a port, a `ui` block naming a file that does not exist, and an empty
`ui/` directory.

A malformed image fails the build — it never reaches a browser as a 404.

## Backend tests

```bash
cd backend
go test ./internal/integration/containers/applications/   # catalog + installer
go test ./internal/service/applications/                  # scoping + policy
go build ./... && go vet ./...
```

| File | Covers |
|---|---|
| `registry_test.go` | catalog loading, image kinds, `ui/` discovery, the declared `ui` manifest, asset path traversal, reserved directories |
| `installer_test.go` | which `lxc` commands each scope issues — and, crucially, which it must **not** |
| `service/applications/ui_extensions_test.go` | which extensions a caller may load, and their install scope |

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

See [10 — Fixtures](10-fixtures.md).

## Manual verification

Some behaviour only exists in a browser: sizing, hover states, whether a
contribution actually lands in the right row.

```bash
cd backend  && go run ./cmd/remote
cd frontend && npm run dev
```

Then work through the fixture matrix in [10 — Fixtures](10-fixtures.md).

Remember: **editing anything under `ui/` requires a backend rebuild**, because
the assets are embedded in the binary. `npm run dev` will not pick them up.

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
- **The HTTP handlers have no request-level tests** for the applications
  routes; only `uiAssetContentType` is unit-tested. The endpoints are exercised
  by hand.
- **Rendering is not unit-tested.** `ExtensionSlot.tsx` has no test; the
  registry it reads from does. Rendering is verified in a browser.
- **CI does not run `go test`.** Run it locally before pushing — see
  [CONTRIBUTING](../../../../../../../CONTRIBUTING.md).

## Before opening a pull request

```bash
cd backend  && gofmt -l ./internal ./cmd && go vet ./... && go test ./...
cd frontend && npm run build && npm test
```

Then, if you touched the extension surface, install `ui-playground` and run its
self-test.
