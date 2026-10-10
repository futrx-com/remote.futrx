# Terminal

A shell in the project container, docked beside the chat. It replaces the
terminal that used to be built into Remote.

Install it in one project from that project's Applications page, or install it
globally to put a copy in every project, including projects created later
(`globallyInstalledInsideContainers` is `true`, so a global install never uses
a container of its own). While a project's copy is running, that project's
chats show a **Container terminal** button in the chat header.

## What it is made of

| Part | Where | What it does |
|---|---|---|
| Service | `backend/container/cmd/remote-terminal/` | Runs inside the project container as `remote-terminal.service`, listening on `0.0.0.0:8843`. Owns the shells and serves the terminal page. |
| Page | `backend/container/cmd/remote-terminal/web/` | xterm.js, embedded in the service binary. Opens the WebSocket, reconnects, and provides find. |
| Extension | `ui/scripts/` | Registers a drawer with `remote.ui.registerDrawer` and frames the page in it. |

The manifest declares `"web": { "port": 8843, "subdomain": "terminal" }`, so
the page lives at `https://terminal--<project-slug>.<host>/`. Remote's gateway
checks the session and project membership on every request, including the
WebSocket upgrade, before proxying to the service. The page and its socket
share that origin, which is why the drawer frames the page instead of
connecting from Remote's own origin.

There is no host backend, no install script, and no install input. The service
is built in the container from source by Remote and uses only the Go standard
library, including its own small WebSocket and PTY code.

## Behaviour

- **One shell per chat.** The page is opened with `?session=<chat id>`. The
  shell is `bash -l` as root with `TERM=xterm-256color`.
- **Working directory.** `?cwd=` names a path inside the container. The
  extension maps the chat's host path
  `/var/lib/remote/projects/<slug>/workspace[/sub]` to `/workspace[/sub]`, and
  the drawer's header shows that container path. The service starts the shell
  there only if it resolves, symlinks included, to a directory inside
  `/workspace`; otherwise in `/workspace`. `cwd` is ignored for a shell that is
  already running.
- **Reconnect.** The shell belongs to the service, not to the socket. When a
  connection drops while the drawer is open, the page retries after 1, 2, 4, 8
  and then every 10 seconds. A closed drawer does not retry; it reconnects when
  shown again. On every attach the service replays the last 256 KiB of output.
- **Keepalive.** The service pings every 25 seconds and drops a connection
  that has been silent for 80.
- **Ending.** A shell with no viewer is ended after 10 minutes. When a shell
  exits, the page shows a notice and starts a new one on Enter instead of
  reconnecting. Stopping, restarting, or upgrading the application ends every
  shell in the project. At most 64 shells run per project.
- **Find.** Ctrl/Cmd+F searches the terminal's buffer.
- **Drawer.** The pane, its header, the close button, the resize handle and the
  remembered width belong to Remote; see
  [`remote.ui.registerDrawer`](../../docs/dev/installable-applications/06-extension-api.md#remoteuiregisterdraweroptions).

## Security

- A shell is root in the project container, as the built-in terminal was.
  Anyone who can use the project can use it.
- The service does not authenticate. Remote's gateway is what restricts the
  application host to project members, and the port is never offered for
  public preview sharing. Inside the container network the port is reachable
  without credentials, like other application web services.
- The service refuses a WebSocket whose `Origin` is not its own host, and its
  pages may only be framed by the Remote origin that owns the application host.
- The extension accepts status messages only from the frame it created, on the
  project's application origin.

## Verify an installation

1. Install Terminal in a project and wait for **running**.
2. Open a chat in that project and select **Container terminal**. The header
   should read `Connected - /workspace`.
3. Run `MARKER=1`, reload the page, reopen the drawer, and run `echo $MARKER`.
   It prints `1`: the same shell.
4. In the container, `systemctl status remote-terminal` shows the service and
   `curl -fsS http://127.0.0.1:8843/health` prints `ok`.

## Develop

```bash
go test ./applications/terminal/...
node --test applications/terminal/ui/scripts/main.test.mjs \
  applications/terminal/backend/container/cmd/remote-terminal/web/reconnect.test.mjs
```

The Go tests start real shells in pseudo-terminals, so they need Linux.

`web/vendor/` holds unmodified ESM builds of `@xterm/xterm` 6.0.0,
`@xterm/addon-fit` 0.11.0 and `@xterm/addon-search` 0.16.0 under their MIT
license (`web/vendor/LICENSE`). To update them, copy `lib/*.mjs` and
`css/xterm.css` from the npm packages and bump `version` in `application.json`
so installed copies rebuild.
