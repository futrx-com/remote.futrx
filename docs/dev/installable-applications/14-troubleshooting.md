# 14 — Troubleshooting

## The server will not start after I added an application

`NewRegistry()` validates the whole catalog at startup and fails loudly. The
log line names the application and the reason:

```
load application catalog: load application "my-backend": ui: entry: scripts/main.js not found
```

Common causes:

| Message | Cause |
|---|---|
| `application id "x" does not match directory "y"` | `id` in `application.json` differs from the directory name |
| `port and healthcheck require a container capability` | declare `service`, `backend/container/`, or `infra/install.sh`, or remove the container-only fields |
| `service.command must start with an absolute executable path` | put the executable and each argument in the manifest's `service.command` array |
| `port.defaultExternal and healthcheck require port.internal` | declare the internal listener port |
| `read install script "…"` | the explicitly configured install script does not exist |
| `application has no infra, backend, ui, or skills` | add at least one capability directory |
| `ui: entry: … not found` | the `ui` block names a file that does not exist |
| `ui: … exists but is empty` | `ui/` has no files at all |
| `backend: … contains no package main source` | the `backend/` root needs an executable Go program; child libraries such as `backend/api/` and `backend/lifecycle/` are not entry points |
| `backend: … declares package "helper", want main` | every non-test `.go` file directly in `backend/` must be `package main`; child host packages may use their own package name |
| `backend: backend/…/go.mod is not supported` | the server generates the host backend module; remove `go.mod`, `go.sum`, `go.work`, and `go.work.sum` from the host tree |
| `backend: invalid access "everyone"` | `access` is `registered` or `admin` |

Reproduce without running the server:

```bash
cd backend && go test ./internal/integration/containers/applications/
```

## My application does not appear in the catalog

- **Did you rebuild the backend?** The catalog is embedded; `npm run dev` does
  not pick up application changes.
- **Is the directory directly under `applications/`?** Nested directories are not
  scanned.
- **Does the application support this scope?** The grid only lists applications whose
  `scopes` include the scope you are looking at. A `"scopes": ["project"]`
  application never appears in the global Applications page.
- **Is it already installed?** Installed applications show "Installed" instead of an
  Install button.

## I installed it but nothing appears in the UI

Work down the three gates:

1. **Does the application have a `ui/`?** Check the catalog response:

   ```bash
   curl -s -b cookies.txt localhost:7682/api/applications/catalog \
     | python3 -c "import sys,json;print([(i['id'], bool(i.get('ui'))) for i in json.load(sys.stdin)])"
   ```

2. **Is it in the allowed list?** This is the gate that matters most:

   ```bash
   curl -s -b cookies.txt localhost:7682/api/applications/ui
   ```

   Empty means the backend does not think you have it installed and running.
   Check the instance's **status** — a `stopped` or `error` instance loads
   nothing. Hit **Start**.

3. **Is this surface in scope?** A project-installed extension only renders
   inside that project. Open a chat belonging to it. See
   [08 — Scoping and visibility](08-scoping-and-visibility.md).

Then check the browser console. The host logs every failure:

```
[extensions] my-backend failed to load: …
[extensions] my-backend: unknown slot "chat.header"
[extensions] my-backend: scripts/main.js exports no default function
```

## The entry module loads but my button is missing

- **Wrong slot name.** Use `remote.slots.chatHeaderActions`, not the string. An
  unknown name logs `unknown slot` and is dropped.
- **A `when` predicate returning false.** Log inside it. A predicate that
  *throws* also hides the contribution — that is logged too.
- **The surface is not mounted.** `chatHeaderActions` needs an open chat;
  `projectRowActions` only appears on hover; `applicationCardActions` needs an
  installed instance of some application.
- **Scope.** See above.

Fastest diagnosis: install `ui-playground` and see whether *its* flask appears
in the same slot. If it does, the problem is in your extension; if it does not,
it is the gate or the surface.

## I edited a file under `ui/` and nothing changed

Two reasons, usually both:

1. **The assets are embedded in the binary.** Rebuild:
   `cd backend && go build ./... && go run ./cmd/remote`.
2. **The browser cached them.** Assets are served with
   `Cache-Control: private, max-age=300`. Hard-reload, or keep DevTools open
   with "Disable cache".

## The docs said rebuild, I rebuilt, and the browser still runs my old code

It should not any more: extension assets are served with a content `ETag` and
`Cache-Control: private, no-cache`, so a reload revalidates and picks up a
rebuild immediately.

If you are on a build from before that change, the assets carried
`max-age=300` with no validator, and the browser held them — an ES module for
the page's lifetime. A hard reload, or five minutes, cleared it.

## My CSS has no effect

Tailwind classes **do not work** in extension code — the class you wrote does
not exist in the stylesheet the browser loaded. Write ordinary CSS in
`ui/style/*.css` and use the platform's CSS custom properties. See
[09 — Styling and icons](09-styling-and-icons.md).

Also check the stylesheet is actually declared: with no explicit `ui` block it
is discovered from `style/*.css`, but an explicit block that omits `styles`
loads none.

## My icon does not show

- **Built-in key:** unrecognised keys silently fall back to a server mark.
  Check the list in [09 — Styling and icons](09-styling-and-icons.md).
- **Own asset:** the value must start with `ui/` — `"ui/assets/logo.svg"`, not
  `"assets/logo.svg"` or `"/ui/assets/logo.svg"`.
- **In a slot:** if the SVG has hard-coded `width`/`height` it will not scale
  to the slot's size. Remove them. If it is invisible, it probably has no
  `stroke`/`fill` of `currentColor`.

## My contribution renders twice

`chatHeaderActions` mounts in two orientations — a horizontal header rail and a
vertical rail — and only one is visible at a time. Querying the DOM will find
both; measure `getBoundingClientRect().width > 0` to count visible ones.

## Contributions from an uninstalled extension are still there

They should be removed on the next sync. If they are not:

- The sync runs on this tab's own lifecycle actions, when the tab is brought
  back to the foreground, when an Applications surface loads, and at page load.
  An uninstall performed in another tab, by another administrator, or through
  the API reaches this tab only at one of those points, so a foreground tab
  that is sitting still keeps drawing the extension until then.
- Changing the store behind the app's back (editing JSON under `DATA_DIR`)
  triggers nothing at all — reload the page.
- Note that the **stylesheet stays** in the document by design; only the
  contributions are removed. Namespace your CSS so this is harmless.

## `PUT /port` returns an error for my application

Applications without infrastructure and `port.internal` have no port. The error is
`applications: capability not supported`.

## A project install created a container

It should not — a project-scope install uses the project's own container. If
you see `futrx-app-*` appear for a project install, that is a bug in
`installer.go:ensureContainer`; `installer_test.go` asserts against it.

## Install fails with an LXD error

The message and the tail of the script output appear on the instance row. Common
cases:

| Symptom | Cause |
|---|---|
| `Failed to find image` / `lxc init: exit status 1` | the base image is not present on this host |
| `Installing LXD snap, please be patient` | LXD is not ready yet |
| script output ends mid-`apt-get` | no network in the container, or the 8-minute timeout was hit |

An application without infrastructure never touches LXD, which is why the UI
and backend-only fixtures install anywhere. An application with `backend/` does
need a Go toolchain.

## My application's backend will not start

The installed row carries the reason, because a backend that fails to start
fails the install.

| Message | Cause |
|---|---|
| `no Go toolchain found` | the server has no `go`. Install one, or set `REMOTE_APPLICATION_GO` to its path. |
| `compile backend "x": …` | your source does not compile. The compiler's output is in the message; the generated build directory is kept at `<dataDir>/applications/build/<application>-<fingerprint>/` so you can look at exactly what it tried to build. |
| `module lookup disabled by GOPROXY=off` in the offline attempt only | normal — the host retries with the network. If the *second* attempt also failed, the message shows both. |
| `backend x reports contract version 2, this server speaks 1` | the backend was written against a different `applications.APIVersion`. Rebuild the catalog. |
| `start backend x: … handshake` | the backend exited before completing the handshake. It is almost always a `panic` in `main` before `rpc.Serve`, or a `Serve` call that was never reached. |
| `initialize backend x: …` | your `Init` returned an error. |

Compile the executable once locally before installing. Its child host packages
are ordinary imports in the catalog module at the repository root:

```bash
go build ./applications/<id>/backend
```

## My backend runs but calls fail

| Symptom | Cause |
|---|---|
| `409 … is not running` | the app is stopped. A stopped backend's backend is off, exactly as a stopped UI extension is unloaded. |
| `404 … application has no backend backend` | the application ships no `backend/`, or you are calling the wrong instance |
| `403 … restricted to administrators` | the application declares `"access": "admin"` |
| `403` from the backend itself | the backend's own `Request.Caller` check refused you |
| `backend call timed out` | the route took longer than the application's `timeoutMs`. The backend is still running; the call was abandoned. |
| `backend panicked: …` | a route panicked. The process survived — check `health` and you will see the same pid. |
| `remote.backend.available is false` | the application ships no `backend/`, or no install of it is running for this caller |
| `no running backend with id …` | you passed an `instanceId` that is not in `remote.backend.instances` |

## My backend does not see my edit

The catalog is embedded, so a `backend/` edit needs a backend rebuild — and then
the fingerprint changes, so the next install or start recompiles it. Stop and
start the app to force it without reinstalling.

If you are sure the source changed and the backend did not, check that the
binary under `<dataDir>/applications/bin/` has a new fingerprint suffix; the old one
is pruned on a successful build.

## My backend lost its data

`DataDir` survives stop and start and is deleted on **uninstall**. In-memory
state is not durable at all: the host restarts a backend lazily after a crash or
a server restart, so anything that must survive belongs in `DataDir`.

## The self-test reports a failure

Each check names what it proves — see [10 — Fixtures](10-fixtures.md). Some
produce console noise **by design**: in `ui-playground`, a 404 (traversal
refused) and an "unknown slot" warning; in `backend-playground`, a failed
`boom` call and a refused unknown route. Those are assertions passing, not
failures.

## Where to look in the code

| Symptom | File |
|---|---|
| Catalog will not load | `registry.go`, `registry_backend.go` |
| Wrong container behaviour | `installer.go` |
| Wrong instances or scopes returned | `service/applications/service.go` |
| Wrong status code or authorization | `transport/http/handlers/applications_handler.go` |
| Extension not loading in the browser | `app/extensions/extensionHost.ts` |
| Contribution not rendering | `state/stores/extensions/extensionStore.ts`, `state/hooks/extensions/extensionContributionState.ts`, `ui/primitives/ExtensionSlot.tsx` |
| Button looks wrong | `app/extensions/extensionApi.ts`, `config/extensions.ts` |
| Backend will not compile or start | `internal/integration/applications/builder.go`, `host.go` |
| Backend call refused or mis-authorized | `service/applications/backend.go` |
| Wrong headers or status from a backend | `transport/http/handlers/applications_backend_handler.go` |
| `remote.backend` reaches the wrong instance | `app/extensions/extensionBackend.ts` |
