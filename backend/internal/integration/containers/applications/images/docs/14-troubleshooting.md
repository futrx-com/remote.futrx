# 14 — Troubleshooting

## The server will not start after I added an image

`NewRegistry()` validates the whole catalog at startup and fails loudly. The
log line names the image and the reason:

```
load application catalog: load image "my-plugin": ui: entry: scripts/main.js not found
```

Common causes:

| Message | Cause |
|---|---|
| `image id "x" does not match directory "y"` | `id` in `image.json` differs from the directory name |
| `invalid type "daemon"` | `type` must be `service`, `ui`, or absent |
| `missing port.internal` | a `service` image needs a port |
| `type "ui" must not declare port, service, or healthcheck` | remove them, or change the type |
| `type "ui" requires a ui/ directory` | a UI image with no `ui/` does nothing |
| `read install script "install.sh"` | a `service` image needs one |
| `ui: entry: … not found` | the `ui` block names a file that does not exist |
| `ui: … exists but is empty` | `ui/` has no files at all |

Reproduce without running the server:

```bash
cd backend && go test ./internal/integration/containers/applications/
```

## My image does not appear in the catalog

- **Did you rebuild the backend?** The catalog is embedded; `npm run dev` does
  not pick up image changes.
- **Is the directory directly under `images/`?** Nested directories are not
  scanned.
- **Does the image support this scope?** The grid only lists images whose
  `scopes` include the scope you are looking at. A `"scopes": ["project"]`
  image never appears in the global Applications page.
- **Is it already installed?** Installed images show "Installed" instead of an
  Install button.

## I installed it but nothing appears in the UI

Work down the three gates:

1. **Does the image have a `ui/`?** Check the catalog response:

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
[extensions] my-plugin failed to load: …
[extensions] my-plugin: unknown slot "chat.header"
[extensions] my-plugin: scripts/main.js exports no default function
```

## The entry module loads but my button is missing

- **Wrong slot name.** Use `remote.slots.chatHeaderActions`, not the string. An
  unknown name logs `unknown slot` and is dropped.
- **A `when` predicate returning false.** Log inside it. A predicate that
  *throws* also hides the contribution — that is logged too.
- **The surface is not mounted.** `chatHeaderActions` needs an open chat;
  `projectRowActions` only appears on hover; `applicationCardActions` needs an
  installed instance of some image.
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

- The sync only runs on lifecycle actions and at load. Changing the store
  behind the app's back (editing JSON under `DATA_DIR`) will not trigger it —
  reload the page.
- Note that the **stylesheet stays** in the document by design; only the
  contributions are removed. Namespace your CSS so this is harmless.

## `PUT /port` returns an error for my image

`ui` images have no port. The error is
`applications: not supported for this image type`.

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

A `ui` image never touches LXD, which is why the fixtures install anywhere.

## The self-test reports a failure

Each check names what it proves — see [10 — Fixtures](10-fixtures.md). Two
produce console noise **by design**: a 404 (traversal refused) and an "unknown
slot" warning. Those are assertions passing, not failures.

## Where to look in the code

| Symptom | File |
|---|---|
| Catalog will not load | `registry.go` |
| Wrong container behaviour | `installer.go` |
| Wrong instances or scopes returned | `service/applications/service.go` |
| Wrong status code or authorization | `transport/http/handlers/applications_handler.go` |
| Extension not loading in the browser | `app/extensions/extensionHost.ts` |
| Contribution not rendering | `state/stores/extensions/extensionStore.ts`, `state/hooks/extensions/extensionContributionState.ts`, `ui/primitives/ExtensionSlot.tsx` |
| Button looks wrong | `app/extensions/extensionApi.ts`, `config/extensions.ts` |
