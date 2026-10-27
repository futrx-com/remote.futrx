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
| Service | `backend/container/cmd/remote-terminal/` | Runs inside the project container as `remote-terminal.service`, listening on `0.0.0.0:8843`. Manages the shells and serves the terminal page. |
| Page | `backend/container/cmd/remote-terminal/web/` | xterm.js, embedded in the service binary. Connects over WebSocket, reconnects after a dropped connection, and searches terminal output. |
| Extension | `ui/scripts/` | Registers a drawer with `remote.ui.registerDrawer` and loads the page in an iframe. |

The manifest declares `"web": { "port": 8843, "subdomain": "terminal" }`, so
the page lives at `https://terminal--<project-slug>.<host>/`. Remote's gateway
checks the session and project membership on every request, including the
WebSocket upgrade, before proxying to the service. The page and its socket
share the same origin. The drawer loads the page in an iframe so it can
connect to its WebSocket from that origin.

Remote builds the service from source inside the container. It uses only the
Go standard library, with custom WebSocket and PTY code. Installation needs
no host backend, install script, or user input.

## Behaviour

- **One shell per chat.** The page is opened with `?session=<chat id>`. The
  shell is `bash -l` as root with `TERM=xterm-256color`.
- **Working directory.** `?cwd=` names a path inside the container. The
  extension maps the chat's host path
  `/var/lib/remote/projects/<slug>/workspace[/sub]` to `/workspace[/sub]`, and
  the drawer's header shows that container path. The service starts the shell
  there if the path resolves to a directory inside `/workspace`, including
  through symlinks. Otherwise, it starts in `/workspace`. `cwd` is ignored
  for a shell that is already running.
- **Reconnect.** The service keeps the shell running when its WebSocket
  disconnects. If the connection drops while the drawer is open, the page
  retries after 1, 2, 4, 8
  and then every 10 seconds. A closed drawer does not retry; it reconnects when
  shown again. On every attach the service replays the last 256 KiB of output.
- **Keepalive.** The service pings every 25 seconds and drops a connection
  that has been silent for 80 seconds.
- **Ending.** The service ends a shell after 10 minutes without a viewer.
  When a shell exits, the page shows a notice and starts a new one on Enter
  instead of
  reconnecting. Stopping, restarting, or upgrading the application ends every
  shell in the project. At most 64 shells run per project.
- **Find.** Ctrl/Cmd+F searches the terminal's buffer.
- **Drawer.** The pane, its header, the close button, the resize handle and the
  remembered width belong to Remote; see
  [`remote.ui.registerDrawer`](../../docs/dev/installable-applications/06-extension-api.md#remoteuiregisterdraweroptions).

## Security

- The shell runs as root in the project container, as the built-in terminal did.
  Anyone who can use the project can use it.
- Remote's gateway restricts the application host to project members and
  excludes the port from public preview sharing. The service itself has no
  authentication, so its port is reachable without credentials inside the
  container network, like other application web services.
- The service refuses a WebSocket whose `Origin` is not its own host, and its
  pages may only be framed by the Remote origin that owns the application host.
- The extension accepts status messages only from the frame it created, on the
  project's application origin.

## Verify an installation

1. Install Terminal in a project and wait for **running**.
2. Open a chat in that project and select **Container terminal**. The header
   should read `Connected - /workspace`.
3. Run `MARKER=1`, reload the page, reopen the drawer, and run `echo $MARKER`.
   It should print `1`, confirming that you returned to the same shell.
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
