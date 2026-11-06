# Terminal

Terminal opens a shell in your project container, beside the chat. It replaces
the terminal that used to be built into Remote.

Install Terminal from a project's Applications page, or install it globally
to add it to every existing project. Run the global install again to add it
to projects created later. Global installs use the project containers because
`globallyInstalledInsideContainers` is `true`.
Once Terminal is running in a project, its chats show a **Container terminal**
button in the header.

## What it is made of

| Part | Where | What it does |
|---|---|---|
| Service | `backend/container/` | Built into the project container as `/usr/local/bin/remote-terminal`. `remote-terminal serve` runs as `remote-terminal.service`, owns every shell, and answers only on the root-only Unix socket `/run/remote-terminal/terminal.sock`. |
| Bridge | `backend/container/internal/bridge/` | `remote-terminal bridge` relays requests from stdin to that socket, one JSON line each way, concurrently. |
| Host backend | `backend/` | Registers the `sessions`, `output`, `input` and `resize` routes and relays them through one `lxc exec <container> -- remote-terminal bridge` process per installation. |
| Extension | `ui/` | Registers a drawer with `remote.ui.registerDrawer` and renders xterm.js in it, talking to the backend through `remote.backend`. |

Nothing listens on a TCP port, and there is no web route or application
hostname. The drawer calls its project installation's backend at
`/api/projects/<project>/applications/<instance>/backend/<route>`, where Remote
checks the session and project membership on every request, as for any
application backend.

```text
backend/
  main.go          composition: one bridge per installation
  api/             routes, relayed to the container unchanged
  containerio/     bridge lifecycle; process.go owns JSON lines and pending calls
  config/          the container command path and call timeout
  container/
    cmd/remote-terminal/  `serve` and `bridge`
    internal/config/      container defaults and resource limits
    internal/sessions/    registry and reaping; session.go owns each shell and replay
    internal/server/      request handlers; socket.go owns Unix socket setup
    internal/bridge/      stdin/stdout to the socket
    internal/shell/       bash -l, its environment and starting directory
ui/
  scripts/main.js                 drawer registration and dependency composition
  scripts/controllers/           drawer lifecycle, header and connection bindings
  scripts/terminalView.js         xterm.js, deferred creation, sizing and visibility
  scripts/terminalFind.js         find controls and search-addon bindings
  scripts/config/                drawer labels, theme and search options
  scripts/connection.js           attach, read loop, input batching, reconnect
  scripts/shellApi.js             backend routes
  scripts/reconnect.js            retry policy
  style/                         xterm.css and the drawer's scoped styles
  vendor/                        xterm.js and its add-ons
```

## How the shell is reached

The backend API does not carry WebSockets, so the stream is split into ordinary
calls. Each is relayed to the service as is:

- `POST sessions` with `{ session, cwd, cols, rows }` opens the chat's shell,
  starting it if needed, and returns its `generation`.
- `GET output?session=&generation=&offset=` returns output after `offset`. The
  service waits up to 20 seconds for the shell to print, so an idle pane makes
  one request at a time. `offset=-1` asks for the replay. A response with
  `reset: true` starts again from the oldest retained output.
- `POST input?session=&generation=` types its body into the shell. The drawer
  sends keystrokes in order and batches what is typed while one is in flight.
- `POST resize` with `{ session, generation, cols, rows }` resizes the
  terminal.

The bridge runs requests concurrently, so a waiting output read never holds up
typing. A request for a generation the service no longer has answers 404, and
the drawer opens the chat's shell again; input from a viewer of an ended shell
can never reach its replacement. If the bridge exits, its waiting requests
fail, the drawer retries, and the backend starts another bridge.

## Behaviour

- **One shell per chat.** The chat ID names the session. The shell is
  `bash -l` as root with `TERM=xterm-256color`.
- **Working directory.** The extension maps the chat's host path
  `/var/lib/remote/projects/<slug>/workspace[/sub]` to `/workspace[/sub]`, and
  the drawer's header shows that container path. The service starts the shell
  there if the path resolves to a directory inside `/workspace`,
  including through symlinks. Otherwise, it starts in `/workspace`. The
  directory is ignored for a shell that is already running.
- **Reconnect.** The shell keeps running when a request fails or the page
  reloads. If the connection drops while the drawer is open, the drawer retries
  after 1, 2, 4, 8 and then every 10 seconds. A closed drawer does not retry; it
  reconnects when shown again. On every attach the last 256 KiB of output is
  replayed.
- **Ending.** A shell nobody has read for 10 minutes is ended. A mounted drawer
  keeps reading while it is closed, so its shell stays alive. When a shell
  exits, the drawer shows a notice and starts a new one on Enter instead of
  reconnecting. At most 64 shells run per project.
- **Lifetime.** Shells belong to the container service, so a dropped bridge, a
  restarted backend or a restarted Remote leaves them running. Stopping,
  restarting or upgrading the application stops the service and ends every
  shell in the project.
- **Opening.** The drawer attaches to the shell straight away but builds xterm
  only once Remote's pane has finished opening, so the opening animation does
  not stall; output that arrives first is shown when the terminal appears. The
  backend starts its bridge when it starts, and `backend.background` restores it
  after Remote restarts, so the first open does not wait for `lxc exec`.
- **Find.** Ctrl/Cmd+F searches the terminal's buffer.
- **Drawer.** The pane, its header, the close button, the resize handle and the
  remembered width belong to Remote; see
  [`remote.ui.registerDrawer`](../../docs/dev/installable-applications/06-extension-api.md#remoteuiregisterdraweroptions).

## Security

- The shell runs as root in the project container, as the built-in terminal did.
  Anyone who can use the project can use it.
- Remote's project backend routes admit only project members. The backend runs
  one fixed command, and browser input travels only inside the bridge's JSON
  lines. The bridge forwards only the four terminal routes, and the service
  validates every request and keeps the starting directory inside `/workspace`.
- The service listens only on a Unix socket in a root-only directory, so no
  port is reachable on the container network.

## Upgrading from version 3

Versions up to 3 ran the same `remote-terminal.service` listening on port 8843
and served the page from `terminal--<project-slug>.<host>`. Upgrading rewrites
the unit to `remote-terminal serve`, which listens on the Unix socket instead;
as with every upgrade, the restart ends running shells.

## Verify an installation

1. Install Terminal in a project and wait for **running**.
2. Open a chat in that project and select **Container terminal**. The header
   should read `Connected - /workspace`.
3. Run `MARKER=1`, reload the page, reopen the drawer, and run `echo $MARKER`.
   It should print `1`, confirming that you returned to the same shell.
4. In the container, `systemctl status remote-terminal` shows the service,
   `ss -ltnp` shows nothing on port 8843, and
   `/run/remote-terminal/terminal.sock` exists with mode `0600`.
5. Restart Remote, reopen the drawer, and run `echo $MARKER` again. It still
   prints `1`.

## Develop

```bash
go test ./applications/terminal/...
node --test applications/terminal/ui/scripts/*.test.mjs
FUTRX_BROWSER_TEST=1 go test -count=1 ./applications/terminal/backend/api/ -run TestTerminalBrowser -v
```

The Go tests start real shells in pseudo-terminals, so they need Linux. The
backend tests build the container command and run it locally as both service
and bridge, leaving out only `lxc exec`; they include a shell surviving its
bridge and a new backend process. The browser check drives the drawer in
Chromium over the same path; it needs Node, Playwright and its Chromium
installation. Running a shell in a real project container
needs a live Remote/LXD installation.

`ui/vendor/` holds unmodified ESM builds of `@xterm/xterm` 6.0.0,
`@xterm/addon-fit` 0.11.0 and `@xterm/addon-search` 0.16.0 under their MIT
license (`ui/vendor/LICENSE`), with xterm's stylesheet at `ui/style/xterm.css`.
To update them, copy `lib/*.mjs` and `css/xterm.css` from the npm packages and
bump `version` in `application.json`.

## Guides

- [Architecture](docs/architecture.md)
- [Limitations](docs/limitations.md)
- [Troubleshooting](docs/troubleshooting.md)
- [Usage](docs/usage.md)
