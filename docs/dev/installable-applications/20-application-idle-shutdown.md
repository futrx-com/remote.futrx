# Optional application idle shutdown

This capability is independent of the project web gateway, uninstall scripts,
JSON settings, file-opening extensions, and container recovery. It does not
change the built-in editor or install any editor application.

Apps without `service.socketProxy` keep their existing service behavior.
A socket-activated application can use an allocated host port through
`port.internal`, which must match the socket listener port, or run only inside
the container without allocating a host port. For a project web app,
`web.port` must match `listenPort`. The existing app subdomain gateway forwards
to this listener; socket activation does not change hostnames or authentication.
This capability is opt-in and does not enable idle shutdown for Code Server
or any other application unless its manifest declares `service.socketProxy`.

```json
"service": {
  "name": "workspace-editor",
  "command": ["/usr/local/bin/workspace-editor", "--listen", "127.0.0.1:8401"],
  "socketProxy": {
    "listenPort": 8400,
    "targetPort": 8401,
    "idleSeconds": 600,
    "readyPath": "/healthz"
  }
}
```

The application install script supplies its executable and dependencies.
Remote supplies the units; it does not require an application uninstall script.

## Starting only when a connection arrives

Remote writes three units in `/etc/systemd/system/`:

| Unit for the example | Responsibility |
|---|---|
| `workspace-editor.socket` | Listens on `0.0.0.0:8400` and activates the proxy |
| `workspace-editor-proxy.service` | Requires and starts the editor service, then forwards connections to `127.0.0.1:8401` through `systemd-socket-proxyd` |
| `workspace-editor.service` | Runs the declared command with `StopWhenUnneeded=yes` |

Install disables/stops any eagerly enabled editor service and enables the
socket. Start enables the socket again. Neither operation intentionally starts
the editor process directly. A configured `healthcheck.command` that connects
to the socket can nevertheless activate it during an install or start.

The proxy exits after `idleSeconds` with no connections; its service
dependency can then stop through `StopWhenUnneeded`. An open WebSocket or other
long-lived connection keeps the application active. This is connection
inactivity, not a timer since the last keyboard interaction. The database
instance remains `running` while its process is idle: `running` means the
application is enabled, not that a PID is continuously present.

`readyPath` adds an `ExecStartPost` HTTP check against the loopback target before
the proxy starts forwarding. It tries `curl -fsS` up to 50 times with a
0.2-second sleep between failures. There is no per-curl timeout in this generated
command, so 50 attempts do not imply a strict ten-second deadline. This differs
from the separate `healthcheck.command` install/start probe, which requires
`port.internal` and has its own retry policy.

The container must supply `/usr/lib/systemd/systemd-socket-proxyd`; the readiness
check also needs `/usr/bin/bash` and `/usr/bin/curl`. Read the field validation
rules below.

Stop removes an allocated host proxy, if any, then requests socket disable,
proxy stop, and process stop. Uninstall also removes the generated socket,
proxy, service, environment directory, and idle declaration during uninstall.
The installer currently ignores individual stop/disable and file-removal
command errors; an issued command is not proof of successful OS cleanup.


## Validation

Listener and target ports must be distinct integers between 1024 and 65535.
`idleSeconds` must be positive. A declared `port.internal` must match
`listenPort`; a declared `web.port` must also match it. Optional `readyPath` must match `^/[A-Za-z0-9/_-]*$`.

## Verification

Installer tests inspect the generated units and Install/Start/Stop/Uninstall
commands. Registry tests reject invalid ports, timeout and readiness paths.
They do not prove a real systemd idle cycle. On a disposable container, verify
that the listener starts the process, open connections prevent idle exit,
closing every connection stops it after the timeout, reconnecting restarts it,
and Stop disables the listener. The app remains installed throughout.

### Responsibility boundaries

- [installer_socket.go](../../../backend/internal/integration/containers/applications/installer_socket.go) — Owns socket/proxy unit rendering and the shared best-effort shutdown sequence used by Stop and Uninstall.
