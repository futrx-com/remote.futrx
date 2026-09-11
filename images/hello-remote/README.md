# Hello Remote

The catalog's worked example. It is the smallest image that still exercises
both halves of the feature:

- **`plugin/`** — Go source the server compiles and runs as a child process,
  reachable at `/api/applications/<instance>/backend/<path>`.
- **`ui/`** — assets the SPA loads for users who installed the image, which
  call that plugin through `remote.backend.call(...)`.

It is `type: "backend"`, so it installs **nothing** into a container: no LXD
container, no port, no proxy device. That is what makes it the first thing to
install on a new server — if this app works, the catalog, the extension host
and the plugin host all work.

## Installing it

| Scope | Where |
|---|---|
| Global | **Settings → Applications** |
| Project | **Project → Applications** |

The install dialog shows one field, `Greeting`, because `image.json` declares
it in `env[]`. Whatever is typed there reaches the plugin as
`Instance.Env["HELLO_GREETING"]` — the same path a database image's password
takes.

Install it at both scopes to see multiple instances of one image: each is a
separate process with its own `DataDir` and its own counter.

**A server that runs a `backend` image needs a Go toolchain**, because plugin
source is compiled on the host. Without one, the install reports that on the
instance instead of failing the server. See
[14 — Troubleshooting](../../docs/dev/installable-images/14-troubleshooting.md).

## What it does

Two contributions, both calling the plugin:

| Where | What |
|---|---|
| **Say hello** on this image's application cards | Opens a popup with the plugin's reply |
| A panel below the applications list | Shows the greeting, and a button that counts one more |

The greeting comes back as `"<greeting>, <your email>."`. The email is proof
of something worth seeing: the browser never sent it. The server stamps the
signed-in caller onto every forwarded request and withholds the cookies that
authenticated it, so a plugin can tell who is asking and cannot act as them.

The counter is proof of the other half. The plugin process is killed on stop,
on uninstall, and on server restart, and is started again lazily by the next
call — so a count that survives is a count that reached `DataDir`. Restart the
server, open the panel, and the number is still there.

## Reading it

| File | Shows |
|---|---|
| `image.json` | The manifest: type, scopes, `env[]`, the `backend` block. The `ui` block is omitted, so the layout convention finds the entry, styles and views. |
| `plugin/main.go` | `Describe` / `Init` / `Handle`, a `Mux`, and the one piece of storage a plugin owns. |
| `plugin/main_test.go` | A plugin is ordinary Go in the catalog module, so `go test ./...` from the repository root covers it with no server involved. |
| `ui/scripts/main.js` | The entry module: one card button, one panel, and a render function that cleans up after itself. |
| `ui/views/panel.html`, `ui/style/hello.css` | The two conventions — views loaded by name, CSS written against the platform's theme tokens. |

Full documentation is in [`docs/dev/installable-images/`](../../docs/dev/installable-images/); the tutorial that builds an
image from nothing is
[07 — Tutorial](../../docs/dev/installable-images/07-tutorial-build-a-plugin.md).

## Editing it

Both `ui/` and `plugin/` are embedded in the binary, so a change to either
needs a backend rebuild — `npm run dev` will not pick it up. A `plugin/` edit
is recompiled by the server on the next start, because the build fingerprint
changed.
